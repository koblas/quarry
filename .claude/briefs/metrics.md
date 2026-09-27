# Brief: feature metrics

Orchestrator only — no agent reads this or ledger it describes.

## Feature metrics

`docs/specifications/<feature-slug>/METRICS.md` is feature's cost ledger. **No agent reads it** — exists so person can compare cadences and gate cost across features. Orchestrator appends; nobody rewrites. `intent-and-goal` creates it from template below.

```markdown
# Metrics: <feature-slug>

## Scenarios
| Scenario | Cadence | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- |
| SCENARIO-01 | code-first | 0/1/2/0 | yes |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Verdict |
| --- | --- | --- | --- |
| 1 | arch, correctness, test | 1/2/4/1 | FAIL |

## Tokens
<output of `.claude/scripts/feature-metrics.py <feature-slug>`, pasted once at SHIP>
```

- **Scenarios** row appended at step 5b, after checkpoint (and its fix pass, if any).
- **Final gate** row appended per `/run-reviewers` round at step 7/9; findings counts from that round's `REVIEW-NN.md`.
- **Tokens** pasted once at SHIP. Script counts subagent transcripts only — orchestrator's own tokens not included, and table says so.
