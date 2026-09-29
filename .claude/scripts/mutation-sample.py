#!/usr/bin/env python3
"""mutation-sample — mutate a sample of changed production Go lines and report survivors.

Usage: .claude/scripts/mutation-sample.py [--base REF] [--max N] [--timeout SECS]
                                          [--profile FILE]
  --base REF     lines added since REF (default: merge-base of HEAD and origin/main), plus
                 every line of untracked, non-ignored non-test .go files.
  --max N        mutants to try (default 20). Guards come first: lines with `if`, a
                 comparison, `&&`/`||`, or `return ... err`; then round-robin across files.
  --timeout S    per-mutant `go test` timeout in seconds (default 120).
  --profile FILE coverage profile from the Verify run; lines no test executes are skipped
                 (uncovered-diff.py already reports them — a mutant there survives trivially).

Each mutant flips ONE operator on ONE line (== / !=, < / >=, <= / >, && / ||, true / false,
`return ..., err` -> `..., nil`). It runs against an isolated copy of the working tree under
$TMPDIR (tracked + untracked, non-ignored files), never the worktree itself, so parallel
agents and reviewers are unaffected. For each mutant the copy builds the mutated package
(build failure = non-viable, not a survivor), then runs the tests of every package whose test
build depends on it. Any failure kills the mutant; all passing = SURVIVED.

Output (stdout, paste into the test-reviewer prompt): a summary line, then one row per
survivor `path:line (func) — before → after`. A survivor on a guard means no test pins that
guard: a MAJOR candidate the reviewer confirms or dismisses with a reason.

Exit status: 0 (survivors are findings, not tool failures); 2 on tool failure, including a
copy whose unmutated tests already fail. Standard library only; Python 3.9+.
"""

from __future__ import annotations

import argparse
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
from collections import defaultdict
from dataclasses import dataclass
from pathlib import Path

HUNK_RE = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@")
FUNC_RE = re.compile(r"^func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)")
# (pattern, replacement) — first applicable one wins per line. Patterns avoid <-, <<, >>, :=.
OPERATORS = [
    (re.compile(r"!="), "=="),
    (re.compile(r"(?<![=!<>:])==(?!=)"), "!="),
    (re.compile(r"(?<![<-])<=(?!=)"), ">"),
    (re.compile(r"(?<![>])>=(?!=)"), "<"),
    (re.compile(r"(?<![<\-])<(?![-=<])"), ">="),
    (re.compile(r"(?<![>\-=])>(?![>=])"), "<="),
    (re.compile(r"&&"), "||"),
    (re.compile(r"\|\|"), "&&"),
    (re.compile(r"\btrue\b"), "false"),
    (re.compile(r"\bfalse\b"), "true"),
]
RETURN_ERR_RE = re.compile(r"^(\s*return\b.*?)\berr\b(\s*)$")
GUARD_RE = re.compile(r"\bif\b|==|!=|<=|>=|&&|\|\||\breturn\b.*\berr\b")


@dataclass
class Mutant:
    path: str
    line: int
    func: str
    before: str
    after: str
    guard: bool


def git(*args: str) -> str:
    return subprocess.run(["git", *args], check=True, capture_output=True, text=True).stdout


def added_lines(base: str) -> dict[str, list[int]]:
    out: dict[str, list[int]] = defaultdict(list)
    current = None
    for line in git("diff", "-U0", base, "--", "*.go").splitlines():
        if line.startswith("+++ "):
            name = line[4:].removeprefix("b/")
            current = None if name == "/dev/null" or name.endswith("_test.go") else name
        elif current and (m := HUNK_RE.match(line)):
            start, count = int(m.group(1)), int(m.group(2) or "1")
            out[current].extend(range(start, start + count))
    for name in git("ls-files", "--others", "--exclude-standard", "--", "*.go").splitlines():
        if not name.endswith("_test.go"):
            out[name].extend(range(1, len(Path(name).read_text().splitlines()) + 1))
    return out


def covered(profile: str | None, module: str) -> dict[str, set[int]] | None:
    if not profile:
        return None
    hit: dict[str, set[int]] = defaultdict(set)
    for row in Path(profile).read_text().splitlines()[1:]:
        m = re.match(r"(.+):(\d+)\.\d+,(\d+)\.\d+ \d+ (\d+)$", row)
        if m and int(m.group(4)) > 0:
            path = m.group(1).removeprefix(module + "/")
            hit[path].update(range(int(m.group(2)), int(m.group(3)) + 1))
    return hit


def strip_strings(code: str) -> str:
    """Blank out string/rune literal contents so operators inside them are not mutated."""
    return re.sub(r'"(?:\\.|[^"\\])*"|`[^`]*`|\'(?:\\.|[^\'\\])*\'',
                  lambda m: m.group(0)[0] + " " * (len(m.group(0)) - 2) + m.group(0)[-1], code)


def mutate(src: str) -> str | None:
    code, _, comment = src.partition("//")
    if not code.strip() or code.lstrip().startswith(("import", "package")):
        return None
    masked = strip_strings(code)
    for pat, repl in OPERATORS:
        m = pat.search(masked)
        if m:
            return code[: m.start()] + repl + code[m.end():] + (("//" + comment) if comment else "")
    m = RETURN_ERR_RE.match(code.rstrip())
    if m:
        return m.group(1) + "nil" + m.group(2)
    return None


def candidates(base: str, profile: str | None, module: str) -> tuple[list[Mutant], int]:
    hit = covered(profile, module)
    out, skipped = [], 0
    for path, lines in added_lines(base).items():
        src = Path(path).read_text().splitlines()
        for n in sorted(set(lines)):
            if n > len(src):
                continue
            if hit is not None and n not in hit.get(path, set()):
                skipped += 1
                continue
            after = mutate(src[n - 1])
            if after is None or after == src[n - 1]:
                continue
            func = next((FUNC_RE.match(src[i]).group(1) for i in range(n - 1, -1, -1)
                         if FUNC_RE.match(src[i])), "-")
            out.append(Mutant(path, n, func, src[n - 1].strip(), after.strip(),
                              bool(GUARD_RE.search(strip_strings(src[n - 1].partition("//")[0])))))
    return out, skipped


def sample(muts: list[Mutant], limit: int) -> list[Mutant]:
    by_file: dict[str, list[Mutant]] = defaultdict(list)
    for m in sorted(muts, key=lambda m: (not m.guard, m.path, m.line)):
        by_file[m.path].append(m)
    picked: list[Mutant] = []
    for guard_pass in (True, False):
        queues = [[m for m in ms if m.guard == guard_pass] for ms in by_file.values()]
        while len(picked) < limit and any(queues):
            for q in queues:
                if q and len(picked) < limit:
                    picked.append(q.pop(0))
    return picked


def isolated_copy() -> Path:
    dest = Path(tempfile.mkdtemp(prefix="mutation-sample.", dir=os.environ.get("TMPDIR")))
    for name in git("ls-files", "-co", "--exclude-standard", "-z").split("\0"):
        if name and Path(name).is_file():
            target = dest / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(name, target)
    return dest


def dependents(copy: Path, pkgs: set[str]) -> dict[str, list[str]]:
    """Map each mutated package dir to the packages whose test build imports it."""
    listing = subprocess.run(
        ["go", "list", "-test", "-f", "{{.ImportPath}}|{{.Dir}}|{{join .Deps \" \"}}", "./..."],
        cwd=copy, check=True, capture_output=True, text=True).stdout
    by_dir, deps_of = {}, {}
    for row in listing.splitlines():
        imp, d, deps = row.split("|", 2)
        by_dir.setdefault(d, imp.split(" ")[0])
        deps_of[imp] = set(deps.split())
    out = {}
    for pkg in pkgs:
        imp = by_dir.get(str((copy / pkg).resolve()))
        if imp is None:
            out[pkg] = ["./" + pkg]
            continue
        hits = {i.split(" ")[0].removesuffix("_test") for i, deps in deps_of.items()
                if imp in deps or i.split(" ")[0] == imp}
        out[pkg] = sorted(h for h in hits if not h.endswith(".test"))
    return out


def run(cmd: list[str], cwd: Path, timeout: int) -> str:
    env = {**os.environ, "GOFLAGS": "-mod=readonly", "GOPROXY": "off"}
    try:
        p = subprocess.run(cmd, cwd=cwd, env=env, capture_output=True, text=True, timeout=timeout)
    except subprocess.TimeoutExpired:
        return "timeout"
    return "ok" if p.returncode == 0 else "fail"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--base")
    ap.add_argument("--max", type=int, default=20)
    ap.add_argument("--timeout", type=int, default=120)
    ap.add_argument("--profile")
    args = ap.parse_args()
    started = time.time()
    try:
        os.chdir(git("rev-parse", "--show-toplevel").strip())
        base = args.base or git("merge-base", "HEAD", "origin/main").strip()
        module = next((l.split()[1] for l in Path("go.mod").read_text().splitlines()
                       if l.startswith("module ")), "")
        muts, skipped = candidates(base, args.profile, module)
    except (subprocess.CalledProcessError, OSError) as e:
        print(f"mutation-sample: {e}", file=sys.stderr)
        return 2
    picked = sample(muts, args.max)
    if not picked:
        print(f"mutation-sample: 0 mutable changed lines since {base[:12]} "
              f"({skipped} uncovered skipped)")
        return 0

    copy = isolated_copy()
    try:
        pkgs = {str(Path(m.path).parent) for m in picked}
        targets = dependents(copy, pkgs)
        for pkg in sorted(pkgs):
            if run(["go", "test", "-count=1", *targets[pkg]], copy, args.timeout * 3) != "ok":
                print(f"mutation-sample: unmutated tests fail for {pkg}; fix them first",
                      file=sys.stderr)
                return 2
        results = defaultdict(list)
        for m in picked:
            f = copy / m.path
            original = f.read_text()
            lines = original.splitlines(keepends=True)
            ending = "\n" if lines[m.line - 1].endswith("\n") else ""
            indent = lines[m.line - 1][: len(lines[m.line - 1]) - len(lines[m.line - 1].lstrip())]
            lines[m.line - 1] = indent + m.after + ending
            f.write_text("".join(lines))
            try:
                pkg = str(Path(m.path).parent)
                if run(["go", "build", "./" + pkg], copy, args.timeout) != "ok":
                    results["non-viable"].append(m)
                    continue
                outcome = run(["go", "test", "-count=1", "-failfast", *targets[pkg]], copy,
                              args.timeout)
                results[{"ok": "survived", "fail": "killed", "timeout": "timed out"}[outcome]].append(m)
            finally:
                f.write_text(original)
    except (subprocess.CalledProcessError, OSError) as e:
        print(f"mutation-sample: {e}", file=sys.stderr)
        return 2
    finally:
        shutil.rmtree(copy, ignore_errors=True)

    print(f"mutation-sample: {len(picked)} sampled of {len(muts)} candidates since {base[:12]} — "
          f"{len(results['killed'])} killed, {len(results['survived'])} survived, "
          f"{len(results['non-viable'])} non-viable, {len(results['timed out'])} timed out; "
          f"{skipped} uncovered lines skipped; {time.time() - started:.0f}s")
    for m in results["survived"]:
        tag = "guard" if m.guard else "line"
        print(f"SURVIVED [{tag}] {m.path}:{m.line} ({m.func}) — `{m.before}` → `{m.after}`")
    for m in results["timed out"]:
        print(f"TIMED OUT {m.path}:{m.line} ({m.func}) — `{m.before}` → `{m.after}`")
    return 0


if __name__ == "__main__":
    sys.exit(main())
