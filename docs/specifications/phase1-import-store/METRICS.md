# Metrics: phase1-import-store

## Scenarios
| Scenario | Cadence | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- |
| PREP-c | test-first (2 guards built code-first, mutation-verified after) | 1/13/8/0 | yes |
| SCENARIO-01a (+04, 05) | test-first | 0/1/7/0 | yes (+ P1-5d, reason 11 rulings) |
| SCENARIO-01d | code-first | 0/0/2/0 | no (MINORs → STATE.md) |
| SCENARIO-01b (+06) | test-first (guard batch built with tests; 2 of 4 named mutations overstated, fixed) | 3/1/21/0 | yes |
| SCENARIO-09 (+11) | code-first | 0/1/15/0 | yes |
| SCENARIO-01c (+07, 12, 21) | code-first | 0/1/4/0 | yes |
| SCENARIO-08 (+19) | code-first | 0/0/6/0 | no (MINORs → STATE.md) |
| SCENARIO-02 (+10, 18) | code-first | 0/1/4/0 | yes |
| SCENARIO-14 (+13) | test-first | 0/0/3/1 | no (MINORs → STATE.md) |
| SCENARIO-20 | test-first | 0/2/2/1 | yes |
| SCENARIO-03 (+16) | code-first | 3/1/0/0 | yes |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Verdict |
| --- | --- | --- | --- |

## Tokens
