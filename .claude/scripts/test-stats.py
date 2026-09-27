#!/usr/bin/env python3
"""The one agreed way to count tests in this repo.

Usage: .claude/scripts/test-stats.py [--base REF] [--changed] [--run] [PKGDIR ...]
  PKGDIR     defaults to every package under internal/ that has tests.
  --base REF also count the same packages at REF (read from git, no checkout) and print the
             delta — the "count and delta" every report owes. REF is usually the commit the
             scenario or fix pass started from.
  --changed  only packages whose test files differ from the base (default base: the
             merge-base of HEAD and origin/main).
  --run      also run the packages' tests once (`go test -json`, packages in parallel) and
             count PASS/FAIL/SKIP leaves, subtests included.

Columns (static, from the working tree — untracked test files included):
  tests    top-level `func Test` functions
  tempdir  `t.TempDir()` call sites
  disk     top-level tests whose own body touches real disk: t.TempDir(, t.Chdir(, or an
           os.WriteFile/MkdirAll/Mkdir/Symlink/Chmod/Remove call. A test that only calls a
           disk-backed helper is not counted — read "disk" as a floor, and quote it the same
           way before and after a change so the delta means something.

Quote these numbers, from this script, in every report. Hand-rolled counts drifted by up to
nine tests between agents on the same commit.

Exit status: 0, 1 when --run saw a failing test, 2 on a tool failure.
Standard library only; runs on the macOS system python3 (3.9+).
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from collections import defaultdict
from pathlib import Path

DISK = re.compile(r"t\.TempDir\(|t\.Chdir\(|os\.(WriteFile|MkdirAll|Mkdir|Symlink|Chmod|Remove)\(")
STATIC = ("tests", "tempdir", "disk")
RUN = ("pass", "fail", "skip")


class ToolError(Exception):
    pass


def git(*args: str, stdin: str | None = None) -> str:
    out = subprocess.run(["git", *args], text=True, capture_output=True, input=stdin)
    if out.returncode != 0:
        raise ToolError(f"git {' '.join(args)} failed: {out.stderr.strip()}")
    return out.stdout


def worktree_sources() -> dict[str, list[str]]:
    """Package dir -> test file contents, from the working tree, files in name order."""
    by_pkg: dict[str, list[str]] = defaultdict(list)
    for f in sorted(Path("internal").rglob("*_test.go")):
        by_pkg[str(f.parent)].append(f.read_text())
    return by_pkg


def ref_sources(ref: str) -> dict[str, list[str]]:
    """Package dir -> test file contents at ref, read in one `git cat-file --batch` call."""
    paths = sorted(p for p in git("ls-tree", "-r", "--name-only", ref, "--", "internal").splitlines()
                   if p.endswith("_test.go"))
    if not paths:
        return {}
    raw = subprocess.run(["git", "cat-file", "--batch"], input="".join(f"{ref}:{p}\n" for p in paths).encode(),
                         capture_output=True)
    if raw.returncode != 0:
        raise ToolError(f"git cat-file failed: {raw.stderr.decode().strip()}")
    by_pkg: dict[str, list[str]] = defaultdict(list)
    data, pos = raw.stdout, 0
    for p in paths:
        header_end = data.index(b"\n", pos)
        size = int(data[pos:header_end].split()[2])
        body = data[header_end + 1: header_end + 1 + size]
        pos = header_end + 1 + size + 1  # trailing newline after each object
        by_pkg[os.path.dirname(p)].append(body.decode())
    return by_pkg


def static_counts(files: list[str]) -> dict[str, int]:
    # A package's test files are read as one stream, in name order, so a function's body
    # is everything from its `func` line to the next `func` line anywhere after it.
    lines = [line for text in files for line in text.splitlines()]
    disk = 0
    in_test = hit = False
    for line in lines:
        if line.startswith("func "):
            disk += in_test and hit
            in_test, hit = line.startswith("func Test"), False
        elif in_test and DISK.search(line):
            hit = True
    disk += in_test and hit
    return {
        "tests": sum(1 for line in lines if line.startswith("func Test")),
        "tempdir": sum(1 for line in lines if "t.TempDir()" in line),
        "disk": disk,
    }


def run_counts(pkgs: list[str], module: str) -> dict[str, dict[str, int]]:
    """Leaf pass/fail/skip per package from one `go test -json` over all of them."""
    counts: dict[str, dict[str, int]] = {p: dict.fromkeys(RUN, 0) for p in pkgs}
    out = subprocess.run(["go", "test", "-count=1", "-json", *(f"./{p}" for p in pkgs)],
                         text=True, capture_output=True)
    prefix = module + "/"
    for line in out.stdout.splitlines():
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            continue
        if not ev.get("Test") or ev.get("Action") not in RUN:
            continue
        pkg = ev.get("Package", "")
        pkg = pkg[len(prefix):] if pkg.startswith(prefix) else pkg
        if pkg in counts:
            counts[pkg][ev["Action"]] += 1
    if out.returncode != 0 and not any(c["fail"] for c in counts.values()):
        raise ToolError("go test failed before any test ran:\n" + out.stderr.strip()[-2000:])
    return counts


def cell(value: int, before: int | None) -> str:
    return str(value) if before is None else f"{value} ({value - before:+d})"


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--base", help="also count at this git ref and print the delta")
    ap.add_argument("--changed", action="store_true", help="only packages whose tests differ from the base")
    ap.add_argument("--run", action="store_true", help="also run the tests and count leaves")
    ap.add_argument("pkgs", nargs="*")
    args = ap.parse_args(argv)

    try:
        os.chdir(git("rev-parse", "--show-toplevel").strip())
        now = worktree_sources()
        base_ref = args.base
        if args.changed and not base_ref:
            base_ref = git("merge-base", "HEAD", "origin/main").strip()
        then = ref_sources(base_ref) if base_ref else {}

        pkgs = args.pkgs or sorted(set(now) | set(then))
        if args.changed:
            pkgs = [p for p in pkgs if now.get(p) != then.get(p)]

        runs = {}
        if args.run and pkgs:
            module = subprocess.run(["go", "list", "-m"], text=True, capture_output=True, check=True).stdout.strip()
            runs = run_counts([p for p in pkgs if p in now], module)
    except (ToolError, OSError, subprocess.CalledProcessError) as e:
        print(f"test-stats: {e}", file=sys.stderr)
        return 2

    width = 15 if base_ref else 8
    cols = STATIC + (RUN if args.run else ())
    header = f"{'package':<34}" + "".join(f" {c:>{width}}" for c in STATIC) + "".join(f" {c:>6}" for c in RUN if args.run)
    print(header)

    total_now = dict.fromkeys(cols, 0)
    total_then = dict.fromkeys(STATIC, 0)
    for pkg in pkgs:
        if pkg not in now and pkg not in then:
            continue
        cur = static_counts(now.get(pkg, []))
        prev = static_counts(then.get(pkg, [])) if base_ref else None
        row = f"{pkg:<34}" + "".join(f" {cell(cur[c], prev[c] if prev else None):>{width}}" for c in STATIC)
        for c in STATIC:
            total_now[c] += cur[c]
            if prev:
                total_then[c] += prev[c]
        if args.run:
            r = runs.get(pkg, dict.fromkeys(RUN, 0))
            row += "".join(f" {r[c]:>6}" for c in RUN)
            for c in RUN:
                total_now[c] += r[c]
        print(row)

    total = f"{'TOTAL':<34}" + "".join(
        f" {cell(total_now[c], total_then[c] if base_ref else None):>{width}}" for c in STATIC)
    if args.run:
        total += "".join(f" {total_now[c]:>6}" for c in RUN)
    print(total)
    if base_ref:
        print(f"test-stats: base {base_ref[:12]}", file=sys.stderr)
    return 1 if args.run and total_now["fail"] else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
