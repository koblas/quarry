# Metrics: parentless-split

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 (+02, 03, 04, 05) | code-first | A, B1, B2, V | 0/0/1/1 | no (MINOR/NIT → STATE.md) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, test, refactor | 0/0/6/1 | 0 / 2, 31s | PASS WITH FOLLOW-UPS |

## Tokens

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
