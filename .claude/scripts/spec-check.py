#!/usr/bin/env python3
"""spec-check — mechanical checks on a feature's specification.md.

For the named feature (docs/specifications/<slug>/specification.md):

  1. One behaviour per scenario: every Gherkin `Scenario:` / `Scenario Outline:` block has
     exactly one `When` step. (`And`/`But` after a `When` are part of that one action.)
  2. Traceability: every ticked `## BDD Acceptance Progress` line names its acceptance test as
     two backticked spans after the title — the test file (repo-relative) and the test name:
        - [x] SCENARIO-04: Title — `internal/scaffold/finish_test.go` `Test_does_the_thing`
  3. The named test exists: `func <Name>(` in that file (a `Test_a/sub` name checks the
     top-level func and the subtest string).

Only specs carrying the `<!-- spec-check: v1 -->` marker are enforced; an unmarked
(pre-convention) spec is reported as not opted in and passes, unless --force.

Usage (from anywhere in the repo):  .claude/scripts/spec-check.py [--force] <feature-slug> [...]
Exit status is non-zero if any check fails.
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

SCENARIO_RE = re.compile(r"^\s*Scenario(?: Outline)?:\s*(SCENARIO-\d+[a-z]?)\b")
STEP_RE = re.compile(r"^\s*(Given|When|Then|And|But)\b")
PROGRESS_HEADING = "## BDD Acceptance Progress"
PROGRESS_RE = re.compile(r"^- \[([ xX])\]\s*\**(SCENARIO-\d+[a-z]?)\**")
OPT_IN_MARKER = "<!-- spec-check: v1 -->"
TEST_REF_RE = re.compile(r"`([^`]+_test\.go)`\s+`([^`]+)`\s*$")


def repo_root() -> Path:
    out = subprocess.run(
        ["git", "rev-parse", "--show-toplevel"], check=True, capture_output=True, text=True
    )
    return Path(out.stdout.strip())


def scenario_when_counts(text: str) -> dict[str, int]:
    counts: dict[str, int] = {}
    current: str | None = None
    for line in text.splitlines():
        m = SCENARIO_RE.match(line)
        if m:
            current = m.group(1)
            counts.setdefault(current, 0)
            continue
        if current is None:
            continue
        if line.strip().startswith("```") or line.startswith("#"):
            current = None
            continue
        step = STEP_RE.match(line)
        if step and step.group(1) == "When":
            counts[current] += 1
    return counts


def progress_lines(text: str) -> list[tuple[str, bool, str]]:
    lines = text.splitlines()
    try:
        start = next(i for i, l in enumerate(lines) if l.strip() == PROGRESS_HEADING)
    except StopIteration:
        return []
    out: list[tuple[str, bool, str]] = []
    for line in lines[start + 1 :]:
        if line.startswith("## "):
            break
        m = PROGRESS_RE.match(line)
        if m:
            out.append((m.group(2), m.group(1).lower() == "x", line))
    return out


def go_test_exists(src: str, name: str) -> bool:
    top, _, sub = name.partition("/")
    if not re.search(rf"^func {re.escape(top)}\(", src, re.MULTILINE):
        return False
    return not sub or f'"{sub}"' in src


def check(root: Path, slug: str, force: bool) -> list[str]:
    spec = root / "docs" / "specifications" / slug / "specification.md"
    if not spec.is_file():
        return [f"{slug}: no specification at {spec.relative_to(root)}"]
    text = spec.read_text()
    if not force and OPT_IN_MARKER not in text:
        print(f"{slug}: not opted in ({OPT_IN_MARKER} absent) — skipped; --force to check anyway")
        return []
    problems: list[str] = []

    counts = scenario_when_counts(text)
    if not counts:
        problems.append(f"{slug}: no `Scenario: SCENARIO-NN` blocks found")
    for sid, n in counts.items():
        if n != 1:
            problems.append(f"{slug} {sid}: {n} `When` steps — one behaviour per scenario; split it")

    progress = progress_lines(text)
    if not progress:
        problems.append(f"{slug}: no `{PROGRESS_HEADING}` entries found")
    for sid, ticked, line in progress:
        if sid not in counts:
            problems.append(f"{slug} {sid}: in progress list but has no Gherkin scenario")
        if not ticked:
            continue
        ref = TEST_REF_RE.search(line)
        if not ref:
            problems.append(f"{slug} {sid}: ticked but names no acceptance test (`<file>` `<test>`)")
            continue
        path, name = ref.group(1), ref.group(2)
        test_file = root / path
        if not test_file.is_file():
            problems.append(f"{slug} {sid}: acceptance test file not found: {path}")
            continue
        src = test_file.read_text()
        if not go_test_exists(src, name):
            problems.append(f"{slug} {sid}: `{name}` not found in {path}")
    return problems


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("slugs", nargs="+", metavar="feature-slug")
    ap.add_argument("--force", action="store_true", help="check a spec without the opt-in marker")
    args = ap.parse_args()
    root = repo_root()
    problems = [p for slug in args.slugs for p in check(root, slug, args.force)]
    for p in problems:
        print(p)
    if problems:
        print(f"spec-check: {len(problems)} problem(s)", file=sys.stderr)
        return 1
    print(f"spec-check: {', '.join(args.slugs)} OK")
    return 0


if __name__ == "__main__":
    sys.exit(main())
