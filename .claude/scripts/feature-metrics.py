#!/usr/bin/env python3
"""feature-metrics — token usage of the pipeline runs that worked on one feature.

Scans every Claude Code project directory for this repo (the main checkout and every
worktree: ~/.claude/projects/<encoded-repo-path>*) and reads each subagent transcript under
<session>/subagents/.

Attribution. A run is attributed from the tag the orchestrator puts on the FIRST line of its
prompt (CLAUDE.md → *Rules* → Run tags):

    run: <kind> feature: <slug> unit: <unit>

<kind> is one of KINDS below; <unit> is a scenario ID, or `-` for feature-wide runs. An
untagged run falls back to the old heuristic — attributed to the feature when its prompt
names docs/specifications/<slug>/ as the first spec folder it mentions, kind guessed from
the agent type and prompt text — and is counted as "heuristic" in the attribution line so a
reader knows how much of the table is guessed.

Orchestrator. For each session that spawned at least one of the feature's runs, the main
session's own usage is counted between the start of the first and the end of the last such
run. Other work the orchestrator did inside that window is included, so the row is an upper
bound for that window and says so.

Weighting. Raw tokens are misleading: a cache read bills at a fraction of fresh input.
"Weighted" is input-equivalent tokens (IE):
    input + 1.25 x cache-write-5m + 2 x cache-write-1h + R x cache-read + O x output
with R (--cache-read-weight, default 0.1) and O (--output-weight, default 5) relative to the
model's own base input price, per the Anthropic prompt-caching page. Check both defaults
against current pricing for the models in use; the report prints the weights it used.
IE is comparable across features on the same model mix, not across price changes.

Usage is de-duplicated by requestId (a streamed response is logged once per content block)
and the model is taken from the transcript, not from agent frontmatter.

Usage:  .claude/scripts/feature-metrics.py <feature-slug> [--projects-dir DIR]
                [--cache-read-weight R] [--output-weight O] [--strict]
Prints markdown tables to paste into docs/specifications/<slug>/METRICS.md → *Tokens*.
Exit status: 0; 1 when no run is attributed; with --strict, also 1 when any run was
attributed by heuristic (R1: every run tagged).
"""

from __future__ import annotations

import argparse
import json
import re
import statistics
import subprocess
import sys
from collections import defaultdict
from dataclasses import dataclass, field
from pathlib import Path

FIELDS = ("input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens", "output_tokens")
KINDS = ("scope", "plan", "build", "checkpoint", "checkpoint-fix", "review", "gate-fix", "retro",
         "other")
TAG_RE = re.compile(
    r"^\s*run:\s*(?P<kind>[a-z-]+)\s+feature:\s*(?P<slug>[a-z0-9-]+)\s+unit:\s*(?P<unit>\S+)"
)
SPEC_FOLDER_RE = re.compile(r"docs/specifications/([a-z0-9-]+)/")
SCENARIO_RE = re.compile(r"SCENARIO-\d+[a-z]?")
FIX_PASS_RE = re.compile(r"Review Findings|[Ff]ix mode|REVIEW-\d+\.md")
REVIEWERS = ("test-reviewer", "correctness-reviewer", "arch-reviewer", "refactor-advisor",
             "api-reviewer", "pipeline-reviewer")
# Agent a run kind implies, for transcripts whose .meta.json is missing; scope and review
# kinds have several agents, so they stay unknown.
AGENT_BY_KIND = {"plan": "architect", "build": "developer", "checkpoint-fix": "developer",
                 "gate-fix": "developer", "checkpoint": "test-reviewer", "retro": "pipeline-reviewer"}


@dataclass
class Weights:
    cache_read: float
    output: float


@dataclass
class Run:
    agent_type: str
    kind: str = "other"
    unit: str = "-"
    tagged: bool = False
    session: Path | None = None
    start: str = ""
    end: str = ""
    models: set[str] = field(default_factory=set)
    usage: dict[str, int] = field(default_factory=lambda: dict.fromkeys(FIELDS, 0))
    write_1h: int = 0

    def weighted(self, w: Weights) -> float:
        write_5m = self.usage["cache_creation_input_tokens"] - self.write_1h
        return (self.usage["input_tokens"] + 1.25 * write_5m + 2 * self.write_1h
                + w.cache_read * self.usage["cache_read_input_tokens"]
                + w.output * self.usage["output_tokens"])

    def add(self, other: Run) -> None:
        self.models |= other.models
        self.write_1h += other.write_1h
        for f in FIELDS:
            self.usage[f] += other.usage[f]


def encoded_project_prefix() -> str:
    top = subprocess.run(
        ["git", "rev-parse", "--path-format=absolute", "--git-common-dir"],
        check=True, capture_output=True, text=True,
    ).stdout.strip()
    repo = Path(top).parent
    return "".join(c if c.isalnum() else "-" for c in str(repo))


def records(lines: list[str]) -> list[dict]:
    out = []
    for line in lines:
        try:
            out.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    return out


def first_prompt(recs: list[dict]) -> str:
    for o in recs:
        if o.get("type") != "user":
            continue
        content = o.get("message", {}).get("content")
        if isinstance(content, str):
            return content
        if isinstance(content, list):
            return " ".join(b.get("text", "") for b in content if isinstance(b, dict))
    return ""


def heuristic_kind(agent_type: str, prompt: str) -> str:
    if agent_type == "developer":
        return "gate-fix" if FIX_PASS_RE.search(prompt) else "build"
    if agent_type == "architect":
        return "plan"
    if agent_type == "test-reviewer" and prompt.lstrip().lower().startswith("checkpoint"):
        return "checkpoint"
    if agent_type == "pipeline-reviewer" and "retro" in prompt[:200]:
        return "retro"
    if agent_type in REVIEWERS:
        return "review"
    if agent_type in ("triage", "product-vision"):
        return "scope"
    return "other"


def accumulate(run: Run, recs: list[dict], since: str = "", until: str = "") -> None:
    by_request: dict[str, tuple[str, dict]] = {}
    for o in recs:
        msg = o.get("message") or {}
        usage = msg.get("usage")
        if o.get("type") != "assistant" or not usage or msg.get("model") == "<synthetic>":
            continue
        ts = o.get("timestamp", "")
        if since and not (since <= ts <= until):
            continue
        by_request[o.get("requestId") or o.get("uuid", "")] = (msg.get("model", "?"), usage)
    for model, usage in by_request.values():
        run.models.add(model)
        for f in FIELDS:
            run.usage[f] += int(usage.get(f) or 0)
        run.write_1h += int((usage.get("cache_creation") or {}).get("ephemeral_1h_input_tokens") or 0)


def read_run(transcript: Path, slug: str) -> Run | None:
    recs = records(transcript.read_text(errors="replace").splitlines())
    prompt = first_prompt(recs)
    meta = transcript.with_suffix(".meta.json")
    agent_type = "unknown"
    if meta.is_file():
        agent_type = json.loads(meta.read_text()).get("agentType", "unknown")
    run = Run(agent_type, session=transcript.parent.parent)
    tag = TAG_RE.match(prompt)
    if tag:
        if tag.group("slug") != slug:
            return None
        kind = tag.group("kind")
        run.kind = kind if kind in KINDS else "other"
        run.unit = tag.group("unit")
        run.tagged = True
        if run.agent_type == "unknown":
            run.agent_type = AGENT_BY_KIND.get(run.kind, "unknown")
    else:
        folder = SPEC_FOLDER_RE.search(prompt)
        if not folder or folder.group(1) != slug:
            return None
        run.kind = heuristic_kind(agent_type, prompt)
        scenario = SCENARIO_RE.search(prompt)
        if scenario:
            run.unit = scenario.group(0)
    stamps = [o["timestamp"] for o in recs if o.get("timestamp")]
    if stamps:
        run.start, run.end = min(stamps), max(stamps)
    accumulate(run, recs)
    return run


def orchestrator(runs: list[Run]) -> Run:
    windows: dict[Path, tuple[str, str]] = {}
    for r in runs:
        if r.session is None or not r.start:
            continue
        lo, hi = windows.get(r.session, (r.start, r.end))
        windows[r.session] = (min(lo, r.start), max(hi, r.end))
    total = Run("orchestrator", kind="orchestrator")
    for session, (lo, hi) in windows.items():
        main = session.with_suffix(".jsonl")
        if main.is_file():
            accumulate(total, records(main.read_text(errors="replace").splitlines()), lo, hi)
    return total


def k(n: float) -> str:
    return f"{n / 1000:,.0f}k"


def pct(part: float, whole: float) -> str:
    return f"{100 * part / whole:.0f}%" if whole else "-"


def print_agents(runs: list[Run], orch: Run, w: Weights) -> float:
    totals: dict[str, Run] = defaultdict(lambda: Run(""))
    counts: dict[str, int] = defaultdict(int)
    for r in runs:
        totals[r.agent_type].agent_type = r.agent_type
        totals[r.agent_type].add(r)
        counts[r.agent_type] += 1
    grand = Run("total")
    for t in totals.values():
        grand.add(t)
    sub_weighted = grand.weighted(w)
    print("| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |")
    print("| --- | --- | --- | --- | --- | --- | --- | --- | --- |")
    for name, t in sorted(totals.items(), key=lambda kv: -kv[1].weighted(w)):
        u = t.usage
        print(f"| {name} | {counts[name]} | {', '.join(sorted(t.models))} | {k(u['input_tokens'])} "
              f"| {k(u['cache_creation_input_tokens'])} | {k(u['cache_read_input_tokens'])} "
              f"| {k(u['output_tokens'])} | {k(t.weighted(w))} | {pct(t.weighted(w), sub_weighted)} |")
    u = grand.usage
    print(f"| **subagent total** | {len(runs)} | | {k(u['input_tokens'])} "
          f"| {k(u['cache_creation_input_tokens'])} | {k(u['cache_read_input_tokens'])} "
          f"| {k(u['output_tokens'])} | {k(sub_weighted)} | 100% |")
    o = orch.usage
    print(f"| orchestrator (upper bound) | - | {', '.join(sorted(orch.models))} | {k(o['input_tokens'])} "
          f"| {k(o['cache_creation_input_tokens'])} | {k(o['cache_read_input_tokens'])} "
          f"| {k(o['output_tokens'])} | {k(orch.weighted(w))} | - |")
    return sub_weighted


def print_kinds(runs: list[Run], w: Weights, sub_weighted: float) -> None:
    by_kind: dict[str, list[Run]] = defaultdict(list)
    for r in runs:
        by_kind[r.kind].append(r)
    print("\n| Run kind | Runs | Weighted (IE) | Share |")
    print("| --- | --- | --- | --- |")
    for kind in KINDS:
        if kind in by_kind:
            total = sum(r.weighted(w) for r in by_kind[kind])
            print(f"| {kind} | {len(by_kind[kind])} | {k(total)} | {pct(total, sub_weighted)} |")


def print_units(runs: list[Run], w: Weights, sub_weighted: float) -> None:
    per_unit: dict[str, dict[str, float]] = defaultdict(lambda: defaultdict(float))
    for r in runs:
        row = per_unit[r.unit]
        row[r.kind] += 1
        row["weighted"] += r.weighted(w)
    print("\n| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |")
    print("| --- | --- | --- | --- | --- | --- | --- | --- |")
    for unit, row in sorted(per_unit.items()):
        print(f"| {unit} | {row['plan']:.0f} | {row['build']:.0f} | {row['checkpoint']:.0f} "
              f"| {row['checkpoint-fix']:.0f} | {row['gate-fix']:.0f} | {k(row['weighted'])} "
              f"| {pct(row['weighted'], sub_weighted)} |")


def print_developer_runs(runs: list[Run], w: Weights) -> None:
    sizes = sorted(r.weighted(w) for r in runs if r.agent_type == "developer")
    if not sizes:
        return
    p90 = sizes[min(len(sizes) - 1, int(round(0.9 * (len(sizes) - 1))))]
    print(f"\nDeveloper runs: {len(sizes)}; weighted per run median {k(statistics.median(sizes))}, "
          f"p90 {k(p90)}, max {k(sizes[-1])}.")


def unique_transcripts(project_dirs: list[Path]) -> list[Path]:
    """One transcript per agent id: the same run can be stored under several project dirs
    (main checkout and worktree); the copy with a .meta.json wins."""
    best: dict[str, Path] = {}
    for d in project_dirs:
        for t in d.glob("*/subagents/agent-*.jsonl"):
            seen = best.get(t.name)
            if seen is None or (not seen.with_suffix(".meta.json").is_file()
                                and t.with_suffix(".meta.json").is_file()):
                best[t.name] = t
    return list(best.values())


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("slug", metavar="feature-slug")
    ap.add_argument("--projects-dir", default=str(Path.home() / ".claude" / "projects"))
    ap.add_argument("--cache-read-weight", type=float, default=0.1)
    ap.add_argument("--output-weight", type=float, default=5.0)
    ap.add_argument("--strict", action="store_true", help="exit 1 if any run is untagged")
    args = ap.parse_args()
    w = Weights(args.cache_read_weight, args.output_weight)

    prefix = encoded_project_prefix()
    project_dirs = [d for d in Path(args.projects_dir).glob(prefix + "*") if d.is_dir()]
    runs = [r for t in unique_transcripts(project_dirs) if (r := read_run(t, args.slug)) is not None]
    if not runs:
        print(f"feature-metrics: no subagent runs mention `{args.slug}`", file=sys.stderr)
        return 1

    heuristic = sum(1 for r in runs if not r.tagged)
    print(f"Tokens for `{args.slug}` across {len(project_dirs)} project dir(s). "
          f"Weighted = input-equivalent tokens (IE): cache read x{w.cache_read:g}, "
          f"cache write x1.25 (5m) / x2 (1h), output x{w.output:g}. "
          f"Attribution: {len(runs) - heuristic} tagged, {heuristic} heuristic. "
          "Orchestrator row counts the main session between the feature's first and last "
          "run in each session, so it may include other work.\n")
    sub_weighted = print_agents(runs, orchestrator(runs), w)
    print_kinds(runs, w, sub_weighted)
    print_units(runs, w, sub_weighted)
    print_developer_runs(runs, w)
    if args.strict and heuristic:
        print(f"feature-metrics: {heuristic} run(s) untagged (--strict)", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
