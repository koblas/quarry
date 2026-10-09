# Brief: feature metrics

Orchestrator only — no build-stage agent reads this or ledger it describes (`pipeline-reviewer` retro does).

## Feature metrics

`docs/specifications/<feature-slug>/METRICS.md` is feature's cost ledger. **No build-stage agent reads it** (retro does) — exists so person can compare cadences and gate cost across features, and judge pipeline changes against `docs/pipeline-efficiency-adoption.md` baseline. Orchestrator appends; nobody rewrites. `intent-and-goal` creates it from template below.

```markdown
# Metrics: <feature-slug>

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 | code-first | A, B1, V (+1 PARTIAL) | 0/1/2/0 | yes |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / timed out / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, test | 1/2/4/1 | 2 / 0 / 20, 140s | BLOCKED |

## Tokens
<output of `.claude/scripts/feature-metrics.py --strict <feature-slug>`, pasted once at SHIP>

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
```

- **Scenarios** row appended at step 5b, after `V` (checkpoint runs before it). *Checkpoint fix pass* cell: `no`, `folded into V` (pin-only findings), or `yes` (a production-changing fix pass ran). Light-lane scenario: cadence cell reads `code-first (light)`, so light and planned units compare.
- **Final gate** row appended per `/run-reviewers` round at step 7/9; findings counts from that round's `REVIEW-NN.md`.
- **Tokens** pasted once at SHIP. Attribution comes from run tags (CLAUDE.md → *Rules* → Run tags): triage, product-vision (all passes) → `scope`; architect incl. sizing pass → `plan`; developer runs `A`/`B*`/`L`/`V` → `build`; checkpoint → `checkpoint`, its fix → `checkpoint-fix`; gate reviewers → `review`, fix passes incl. SHIP WITH CHANGES → `gate-fix`; retro → `retro`. Unit `-` for feature-wide runs; `--strict` exits 1 if any run untagged — fix the tag habit, then paste anyway with attribution line intact. Orchestrator row is upper bound for session windows that ran feature's agents. Agent column comes from each transcript's `.meta.json`; when that file is missing, a tagged run's kind names (tagged runs only) the agent where only one agent has it (`plan`, `build`, fix and checkpoint kinds, `retro`). An `unknown` agent row means "no meta file, and a scope or review run", not "untagged". "Weighted" = input-equivalent tokens; compare features on it, not on raw cache reads.
- **Caught late** — row per gate BLOCKER/MAJOR (stage `gate R<n>`) and per final product-vision change (stage `final pass`), with scenario whose checkpoint let it through. Appended in the same edit as the round's *Final gate* row (or the final pass's record) — never later; an empty section after a blocked round leaves the next retro nothing to rank by.
- **Escaped defects** — row added whenever bug found in feature's code within 30 days of merge (issue, later feature's triage, user report). Append-only; earlier rows never edited. Empty section after 30 days is data, not omission.
