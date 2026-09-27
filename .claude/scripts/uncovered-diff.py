#!/usr/bin/env python3
"""List production Go lines added since BASE that no test executes.

Usage: .claude/scripts/uncovered-diff.py [--profile FILE] [--lines] [BASE] [PKG ...]
  BASE     defaults to the merge-base of HEAD and origin/main (the branch's own additions).
  PKG      the packages whose tests run; defaults to ./...
  --profile FILE
           read coverage from FILE instead of running the tests. Produce it with the one
           verification run a Verify phase already makes:
             go test -count=1 -coverpkg=./... -coverprofile=FILE ./...
  --lines  one row per uncovered line instead of one row per consecutive run.

Added lines are those `git diff -U0 BASE` adds to non-test .go files (committed and
uncommitted). A line is uncovered when every coverage block containing it has count 0;
non-statement lines (comments, declarations, lone closing braces) are skipped.

Output (stdout): one row per run of consecutive uncovered lines,
  path:first[-last] (func): first line of source
then, if any, a "declared unreachable" section: runs whose lines, or the line just above
them, carry a `// unreachable: <reason>` comment. Those are listed for the reviewer to
judge but do not fail the gate. A summary goes to stderr.

Exit status: 0 when every uncovered line is declared unreachable (or there are none), 1
when any is not, 2 on a tool failure (a failing test prints its output).
Standard library only; runs on the macOS system python3 (3.9+).
"""

from __future__ import annotations

import argparse
import os
import re
import subprocess
import sys
import tempfile
from collections import defaultdict

HUNK = re.compile(r"^@@ -\S+ \+(\d+)(?:,(\d+))? @@")
# module/path/file.go:startLine.startCol,endLine.endCol numStmts count
BLOCK = re.compile(r"^(.+):(\d+)\.\d+,(\d+)\.\d+ \d+ (\d+)$")
FUNC = re.compile(r"^func (?:\([^)]*\) )?(\w+)")
UNREACHABLE = re.compile(r"//\s*unreachable:\s*(.+)$")
FAIL_CONTEXT_LINES = 40


class ToolError(Exception):
    pass


def git(*args: str) -> str:
    out = subprocess.run(["git", *args], text=True, capture_output=True)
    if out.returncode != 0:
        raise ToolError(f"git {' '.join(args)} failed: {out.stderr.strip()}")
    return out.stdout


def default_base() -> str:
    try:
        return git("merge-base", "HEAD", "origin/main").strip()
    except ToolError:
        return "origin/main"


def added_lines(base: str) -> dict[str, set[int]]:
    """Map each non-test .go file to the line numbers `git diff -U0 base` added to it."""
    added: dict[str, set[int]] = defaultdict(set)
    path = None
    for line in git("diff", "-U0", base, "--", "*.go", ":(exclude)*_test.go").splitlines():
        if line.startswith("+++ "):
            target = line[4:]
            path = target[2:] if target.startswith("b/") else None  # "/dev/null" on delete
            continue
        m = HUNK.match(line)
        if m and path:
            start = int(m.group(1))
            count = 1 if m.group(2) is None else int(m.group(2))
            added[path].update(range(start, start + count))
    return added


def run_tests(profile: str, pkgs: list[str]) -> None:
    out = subprocess.run(
        ["go", "test", "-count=1", "-covermode=set", "-coverpkg=./...", f"-coverprofile={profile}", *pkgs],
        text=True, capture_output=True,
    )
    if out.returncode != 0:
        tail = (out.stdout + out.stderr).splitlines()[-FAIL_CONTEXT_LINES:]
        raise ToolError("go test failed:\n" + "\n".join(tail))


def coverage(profile: str, module: str) -> tuple[set[tuple[str, int]], set[tuple[str, int]]]:
    """Return (lines inside any block, lines inside a block with count > 0)."""
    seen: set[tuple[str, int]] = set()
    covered: set[tuple[str, int]] = set()
    prefix = module + "/"
    with open(profile) as f:
        for raw in f:
            m = BLOCK.match(raw.strip())
            if not m:
                continue  # the "mode:" header
            path = m.group(1)
            if path.startswith(prefix):
                path = path[len(prefix):]
            start, end, count = int(m.group(2)), int(m.group(3)), int(m.group(4))
            for n in range(start, end + 1):
                seen.add((path, n))
                if count > 0:
                    covered.add((path, n))
    return seen, covered


def enclosing_func(source: list[str], n: int) -> str:
    for i in range(n - 1, -1, -1):
        m = FUNC.match(source[i])
        if m:
            return m.group(1)
    return "?"


def runs_of(lines: list[int]) -> list[list[int]]:
    runs: list[list[int]] = []
    for n in lines:
        if runs and n == runs[-1][-1] + 1:
            runs[-1].append(n)
        else:
            runs.append([n])
    return runs


def declared_reason(source: list[str], run: list[int]) -> str | None:
    for n in [run[0] - 1, *run]:
        if 1 <= n <= len(source):
            m = UNREACHABLE.search(source[n - 1])
            if m:
                return m.group(1).strip()
    return None


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(add_help=True, description=__doc__.splitlines()[0])
    ap.add_argument("--profile", help="reuse an existing -coverprofile instead of running the tests")
    ap.add_argument("--lines", action="store_true", help="one row per line instead of per run")
    ap.add_argument("base", nargs="?", help="diff base (default: merge-base of HEAD and origin/main)")
    ap.add_argument("pkgs", nargs="*", default=["./..."])
    args = ap.parse_args(argv)

    try:
        os.chdir(git("rev-parse", "--show-toplevel").strip())
        base = args.base or default_base()
        added = added_lines(base)
        if not added:
            print(f"uncovered-diff: no production Go lines added since {base[:12]}", file=sys.stderr)
            return 0
        module = subprocess.run(["go", "list", "-m"], text=True, capture_output=True, check=True).stdout.strip()

        if args.profile:
            seen, covered = coverage(args.profile, module)
        else:
            fd, profile = tempfile.mkstemp(prefix="uncovered.", dir=os.environ.get("TMPDIR"))
            os.close(fd)
            try:
                run_tests(profile, args.pkgs)
                seen, covered = coverage(profile, module)
            finally:
                os.remove(profile)
    except (ToolError, OSError, subprocess.CalledProcessError) as e:
        print(f"uncovered-diff: {e}", file=sys.stderr)
        return 2

    open_rows: list[str] = []
    declared_rows: list[str] = []
    open_lines = 0
    for path in sorted(added):
        try:
            with open(path) as f:
                source = f.read().splitlines()
        except OSError:
            continue  # added then deleted in the working tree
        uncovered = [
            n for n in sorted(added[path])
            if (path, n) in seen and (path, n) not in covered
            and n <= len(source) and source[n - 1].strip() != "}"
        ]
        groups = [[n] for n in uncovered] if args.lines else runs_of(uncovered)
        for run in groups:
            first, last = run[0], run[-1]
            loc = f"{path}:{first}" if first == last else f"{path}:{first}-{last}"
            text = UNREACHABLE.sub("", source[first - 1]).strip()
            row = f"{loc} ({enclosing_func(source, first)}): {text}"
            reason = declared_reason(source, run)
            if reason is None:
                open_rows.append(row)
                open_lines += len(run)
            else:
                declared_rows.append(f"{row}  [unreachable: {reason}]")

    for row in open_rows:
        print(row)
    if declared_rows:
        print("# declared unreachable (reviewer judges; does not fail the gate)")
        for row in declared_rows:
            print(row)

    print(
        f"uncovered-diff: {open_lines} uncovered added line(s) in {len(open_rows)} run(s) since {base[:12]}"
        + (f"; {len(declared_rows)} declared unreachable" if declared_rows else ""),
        file=sys.stderr,
    )
    return 1 if open_rows else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
