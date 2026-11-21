# Metrics: version-command

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 (+04, 05 folded) | code-first | A, B1, V | 0/1/4/1 | folded into V |
| SCENARIO-02 | code-first (light) | L, V | 0/0/1/1 | folded into V |
| SCENARIO-03 | code-first (light) | L, V | 0/0/3/0 | folded into V |
| SCENARIO-06 | code-first (light) | L, V | 0/0/1/0 | folded into V |
| SCENARIO-07 | code-first (light) | L, V | 0/0/3/0 | folded into V |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / timed out / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, refactor, test | 0/0/5/3 | 1 / 0 / 15, 469s | PASS WITH FOLLOW-UPS |

## Tokens

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
