# Metrics: phase3b-analysis-tools

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 | code-first | A, B1, B2, V | 0/0/2/2 | no (MINORs → STATE.md) |
| SCENARIO-02 (+06, +07, +12) | code-first | A, B1, V | 0/0/2/2 | no (MINORs → STATE.md) |
| SCENARIO-03 | code-first | A, B1, V | 0/0/1/2 | no (MINOR → STATE.md); + standalone fix 2482e84 (QueryRows/QueryTable lost mid-iteration cancel; pre-existing flake) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |

## Tokens

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
