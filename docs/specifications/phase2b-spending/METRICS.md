# Metrics: phase2b-spending

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 (+02, 03, 04) | code-first | A, B1, B2, V | 0/0/1/0 | no (MINOR → STATE.md) |
| SCENARIO-26 (+25) | code-first (LIGHT) | L, V | 0/0/0/1 | no |
| SCENARIO-06 (+08) | code-first | A, B1, B2, V | 0/2/4/1 | yes |
| SCENARIO-09 (+05, 07) | code-first | A, B1, B2, V | 0/2/4/0 | yes |
| SCENARIO-16 | code-first (LIGHT) | L, V | 0/0/0/0 | no |
| SCENARIO-10 | code-first (LIGHT) | L, V | 0/1/3/0 | yes |
| SCENARIO-11 | code-first | A, B1, B2, V | 0/1/2/0 | yes |
| SCENARIO-13 | code-first | A (PARTIAL: Bash outage), B1, B2, V | 0/1/3/1 | yes |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |

## Tokens

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
