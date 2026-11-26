#!/usr/bin/env python3
# Vendored copy of plugins/pipeline/scripts/not-built.py from koblas/throughline at
# 40cb78205ff33235bb2a2456dad94a2dffe4f46b; update this copy when that script changes.
"""not-built — every requirement the product spec marks unbuilt has an open issue.

A requirement in docs/product/*.md that is written but not built carries its issue inline:

    - Members can transfer ownership of an account. (not built: #123)

Fails when a mark names no issue, or names one that is not open (closed, or no such issue).
Closing the issue and removing the mark belong in the same pull request, so a mark left behind
after its issue closed fails here too. This keeps unbuilt features visible in one issue query.

Usage (from anywhere in the repo):
  scripts/not-built.py                  # open issues from `gh issue list` (needs gh auth)
  scripts/not-built.py --open 12,40     # open issue numbers given, no network
  scripts/not-built.py --offline        # format only: every mark names an issue
Exit status: 0 ok, 1 a mark is wrong, 2 the open issues could not be read.
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

MARK_RE = re.compile(r"\(not built(?::\s*([^)]*))?\)", re.I)
ISSUE_RE = re.compile(r"#(\d+)\b")


def repo_root() -> Path:
    out = subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True)
    return Path(out.stdout.strip()) if out.returncode == 0 else Path.cwd()


def marks(root: Path) -> list[tuple[str, int, list[int]]]:
    """(repo-relative file, line number, issue numbers) for every not-built mark under docs/product/."""
    found = []
    for path in sorted((root / "docs" / "product").rglob("*.md")):
        rel = path.relative_to(root).as_posix()
        in_fence = False
        for n, line in enumerate(path.read_text().splitlines(), 1):
            if line.lstrip().startswith("```"):
                in_fence = not in_fence
                continue
            if in_fence:
                continue
            for m in MARK_RE.finditer(line):
                found.append((rel, n, [int(i) for i in ISSUE_RE.findall(m.group(1) or "")]))
    return found


def problems(found: list[tuple[str, int, list[int]]], open_issues: set[int] | None) -> list[str]:
    """What is wrong with each mark; `open_issues` None checks format only."""
    out = []
    for rel, n, issues in found:
        if not issues:
            out.append(f"{rel}:{n}: `(not built)` names no issue — open one and write `(not built: #N)`")
            continue
        if open_issues is None:
            continue
        for i in issues:
            if i not in open_issues:
                out.append(f"{rel}:{n}: #{i} is not an open issue — built? remove the mark; "
                           "else reopen the issue or name the open one")
    return out


def gh_open_issues(root: Path) -> set[int] | None:
    out = subprocess.run(["gh", "issue", "list", "--state", "open", "--limit", "5000", "--json", "number"],
                         cwd=root, capture_output=True, text=True)
    if out.returncode != 0:
        print(f"not-built: could not list open issues: {out.stderr.strip()[:300]}", file=sys.stderr)
        return None
    return {row["number"] for row in json.loads(out.stdout or "[]")}


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    src = ap.add_mutually_exclusive_group()
    src.add_argument("--open", help="comma-separated open issue numbers instead of asking gh")
    src.add_argument("--offline", action="store_true", help="check only that every mark names an issue")
    args = ap.parse_args()
    root = repo_root()
    found = marks(root)
    open_issues: set[int] | None = None
    if args.open is not None:
        open_issues = {int(x) for x in args.open.replace("#", "").split(",") if x.strip()}
    elif not args.offline and any(issues for _, _, issues in found):
        open_issues = gh_open_issues(root)
        if open_issues is None:
            return 2
    wrong = problems(found, open_issues)
    for w in wrong:
        print(w)
    if wrong:
        print(f"not-built: {len(wrong)} problem(s)", file=sys.stderr)
        return 1
    checked = "format only" if open_issues is None else "against open issues"
    print(f"not-built: {len(found)} mark(s) ok ({checked})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
