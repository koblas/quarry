# Metrics: desktop-install

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 | test-first | A, B1, B2, V | 0/2/4/2 | folded into V |
| SCENARIO-02 | test-first | A, B1, B2, V | 0/0/4/1 | folded into V |
| SCENARIO-04 | test-first | A, B1, B2, V | 0/1/2/1 | folded into V |
| SCENARIO-06 | test-first | A, B1, B2, V | 0/2/3/0 | folded into V |
| SCENARIO-08 | code-first | A, B1, B2, V | 0/3/2/1 | folded into V |
| SCENARIO-13 | test-first | A, B1, B2, V (+1 checkpoint fix) | 0/2/2/0 | yes |
| SCENARIO-09 | code-first | A, B1, B2, V | 0/0/3/2 | folded into V |
| SCENARIO-11 | code-first | A, B1, B2, V | 0/1/3/1 | folded into V |
| SCENARIO-16 | code-first | A, B1, B2, V | 0/0/0/2 | folded into V |

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
