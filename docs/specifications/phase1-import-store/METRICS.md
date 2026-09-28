# Metrics: phase1-import-store

## Scenarios
| Scenario | Cadence | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- |
| PREP-c | test-first (2 guards built code-first, mutation-verified after) | 1/13/8/0 | yes |
| SCENARIO-01a (+04, 05) | test-first | 0/1/7/0 | yes (+ P1-5d, reason 11 rulings) |
| SCENARIO-01d | code-first | 0/0/2/0 | no (MINORs → STATE.md) |
| SCENARIO-01b (+06) | test-first (guard batch built with tests; 2 of 4 named mutations overstated, fixed) | 3/1/21/0 | yes |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Verdict |
| --- | --- | --- | --- |

## Tokens
