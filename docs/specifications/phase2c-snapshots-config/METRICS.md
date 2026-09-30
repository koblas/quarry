# Metrics: phase2c-snapshots-config

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| PRE-01 (pre-step, neutral) | code-first | B1, B2, V | 0/0/3/1 | no (MINORs → STATE.md) |
| SCENARIO-01 (+04, 09, 10) | code-first | A, B1, V | 0/1/4/2 | yes (array-of-tables `got` empty; 2 mid-feature copy rulings) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |

## Tokens

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
