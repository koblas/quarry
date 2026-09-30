# Metrics: phase2d-findings

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 (+02, 03) | test-first | A, B1, B2, V | 0/0/3/1 | no (comment MINORs → STATE.md) |
| SCENARIO-04 (+05) | code-first | A, B1, B2, V | 0/0/2/2 | no (comment MINORs → STATE.md) |
| SCENARIO-06 (+07) | code-first | A, B1, B2, V | 0/2/3/0 | yes (carried-flag wiring and carried type unpinned; test-only fix) |
| SCENARIO-08 | code-first | A, B1, B2, V | 0/2/3/0 | yes (findings read-fault reason and silent format-3 rows unpinned; test-only fix); 1 copy ruling (both carry faults) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |

## Tokens

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
