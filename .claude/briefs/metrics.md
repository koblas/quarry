# Brief: feature metrics

Orchestrator only — no agent reads this or ledger it describes.

## Feature metrics

`docs/specifications/<feature-slug>/METRICS.md` is feature's cost ledger. **No agent reads it** — exists so person can compare cadences and gate cost across features, and judge pipeline changes against `docs/pipeline-efficiency-adoption.md` baseline. Orchestrator appends; nobody rewrites. `intent-and-goal` creates it from template below.

```markdown
# Metrics: <feature-slug>

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 | code-first | A, B1, V (+1 PARTIAL) | 0/1/2/0 | yes |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, test | 1/2/4/1 | 2 / 20, 140s | BLOCKED |

## Tokens
<output of `.claude/scripts/feature-metrics.py --strict <feature-slug>`, pasted once at SHIP>

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
```

- **Scenarios** row appended at step 5b, after checkpoint (and its fix pass, if any).
- **Final gate** row appended per `/run-reviewers` round at step 7/9; findings counts from that round's `REVIEW-NN.md`.
- **Tokens** pasted once at SHIP. Attribution comes from run tags (CLAUDE.md → *Run tags*); `--strict` exits 1 if any run untagged — fix the tag habit, then paste anyway with attribution line intact. Orchestrator row is upper bound for session windows that ran feature's agents. "Weighted" = input-equivalent tokens; compare features on it, not on raw cache reads.
- **Escaped defects** — row added whenever bug found in feature's code within 30 days of merge (issue, later feature's triage, user report). Append-only; earlier rows never edited. Empty section after 30 days is data, not omission.
