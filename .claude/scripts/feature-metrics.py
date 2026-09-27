#!/usr/bin/env python3
"""feature-metrics — token usage of the pipeline agents that worked on one feature.

Scans every Claude Code project directory for this repo (the main checkout and every
worktree: ~/.claude/projects/<encoded-repo-path>*), reads each subagent transcript under
<session>/subagents/, and attributes a subagent run to the feature when its first prompt
names docs/specifications/<slug>/ as the FIRST spec folder it mentions (a later feature's
prompt citing this one as a dependency names its own folder first). Architect and developer
runs are also broken down by the first SCENARIO-NN in their prompt, so tokens can be joined
to the per-scenario cadence in METRICS.md; a prompt naming fix mode, Review Findings or a
REVIEW-NN.md file is counted as a fix pass instead.
Usage is de-duplicated by requestId (a streamed response is
logged once per content block) and the model is taken from the transcript, not from agent
frontmatter.

Tokens only — no dollar figures. The orchestrating (main-thread) session's own tokens are
not in subagents/ and are NOT counted; the report says so.

Usage:  .claude/scripts/feature-metrics.py <feature-slug> [--projects-dir DIR]
Prints a markdown table to paste into docs/specifications/<slug>/METRICS.md.
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from collections import defaultdict
from dataclasses import dataclass, field
from pathlib import Path

FIELDS = ("input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens", "output_tokens")
SPEC_FOLDER_RE = re.compile(r"docs/specifications/([a-z0-9-]+)/")
SCENARIO_RE = re.compile(r"SCENARIO-\d+[a-z]?")
FIX_PASS_RE = re.compile(r"Review Findings|[Ff]ix mode|REVIEW-\d+\.md")
PER_SCENARIO_AGENTS = ("architect", "developer")


@dataclass
class Run:
    agent_type: str
    scenario: str = "unattributed"
    models: set[str] = field(default_factory=set)
    usage: dict[str, int] = field(default_factory=lambda: dict.fromkeys(FIELDS, 0))


def encoded_project_prefix() -> str:
    top = subprocess.run(
        ["git", "rev-parse", "--path-format=absolute", "--git-common-dir"],
        check=True, capture_output=True, text=True,
    ).stdout.strip()
    repo = Path(top).parent
    return "".join(c if c.isalnum() else "-" for c in str(repo))


def first_prompt(lines: list[str]) -> str:
    for line in lines:
        try:
            o = json.loads(line)
        except json.JSONDecodeError:
            continue
        if o.get("type") != "user":
            continue
        content = o.get("message", {}).get("content")
        if isinstance(content, str):
            return content
        if isinstance(content, list):
            return " ".join(b.get("text", "") for b in content if isinstance(b, dict))
    return ""


def read_run(transcript: Path, slug: str) -> Run | None:
    lines = transcript.read_text(errors="replace").splitlines()
    prompt = first_prompt(lines)
    folder = SPEC_FOLDER_RE.search(prompt)
    if not folder or folder.group(1) != slug:
        return None
    meta = transcript.with_suffix(".meta.json")
    agent_type = "unknown"
    if meta.is_file():
        agent_type = json.loads(meta.read_text()).get("agentType", "unknown")
    run = Run(agent_type)
    scenario = SCENARIO_RE.search(prompt)
    if FIX_PASS_RE.search(prompt):
        run.scenario = "fix pass"
    elif scenario:
        run.scenario = scenario.group(0)
    by_request: dict[str, tuple[str, dict]] = {}
    for line in lines:
        try:
            o = json.loads(line)
        except json.JSONDecodeError:
            continue
        msg = o.get("message") or {}
        usage = msg.get("usage")
        if o.get("type") != "assistant" or not usage or msg.get("model") == "<synthetic>":
            continue
        by_request[o.get("requestId") or o.get("uuid", "")] = (msg.get("model", "?"), usage)
    for model, usage in by_request.values():
        run.models.add(model)
        for f in FIELDS:
            run.usage[f] += int(usage.get(f) or 0)
    return run


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("slug", metavar="feature-slug")
    ap.add_argument("--projects-dir", default=str(Path.home() / ".claude" / "projects"))
    args = ap.parse_args()

    prefix = encoded_project_prefix()
    project_dirs = [d for d in Path(args.projects_dir).glob(prefix + "*") if d.is_dir()]
    runs = [
        r
        for d in project_dirs
        for t in d.glob("*/subagents/agent-*.jsonl")
        if (r := read_run(t, args.slug)) is not None
    ]
    if not runs:
        print(f"feature-metrics: no subagent runs mention `{args.slug}`", file=sys.stderr)
        return 1

    totals: dict[str, Run] = defaultdict(lambda: Run(""))
    counts: dict[str, int] = defaultdict(int)
    for r in runs:
        t = totals[r.agent_type]
        t.agent_type = r.agent_type
        t.models |= r.models
        counts[r.agent_type] += 1
        for f in FIELDS:
            t.usage[f] += r.usage[f]

    def k(n: int) -> str:
        return f"{n / 1000:,.0f}k"

    print(f"Subagent tokens for `{args.slug}` across {len(project_dirs)} project dir(s). "
          "Main-thread (orchestrator) tokens not included.\n")
    print("| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output |")
    print("| --- | --- | --- | --- | --- | --- | --- |")
    grand = dict.fromkeys(FIELDS, 0)
    for name, t in sorted(totals.items(), key=lambda kv: -sum(kv[1].usage.values())):
        u = t.usage
        for f in FIELDS:
            grand[f] += u[f]
        print(f"| {name} | {counts[name]} | {', '.join(sorted(t.models))} | {k(u['input_tokens'])} "
              f"| {k(u['cache_creation_input_tokens'])} | {k(u['cache_read_input_tokens'])} "
              f"| {k(u['output_tokens'])} |")
    print(f"| **total** | {len(runs)} | | {k(grand['input_tokens'])} "
          f"| {k(grand['cache_creation_input_tokens'])} | {k(grand['cache_read_input_tokens'])} "
          f"| {k(grand['output_tokens'])} |")

    per_scenario: dict[str, dict[str, int]] = defaultdict(lambda: defaultdict(int))
    for r in runs:
        if r.agent_type in PER_SCENARIO_AGENTS:
            row = per_scenario[r.scenario]
            row[f"{r.agent_type} runs"] += 1
            row["cache read"] += r.usage["cache_read_input_tokens"]
            row["output"] += r.usage["output_tokens"]
    print("\n| Scenario | Architect runs | Developer runs | Cache read | Output |")
    print("| --- | --- | --- | --- | --- |")
    for sid, row in sorted(per_scenario.items()):
        print(f"| {sid} | {row['architect runs']} | {row['developer runs']} "
              f"| {k(row['cache read'])} | {k(row['output'])} |")
    return 0


if __name__ == "__main__":
    sys.exit(main())
