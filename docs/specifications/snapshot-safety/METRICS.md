# Metrics: snapshot-safety

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 (+02, 05, 08) | test-first | A, B1, V (+ orchestrator: kill -9 pin ruling) | 0/0/7/0 | folded into V (3 pins: GC-held child, folder-missing pass-through, hard-link alias; Mode(99); 3 comment trims) |
| SCENARIO-03 (+04, 07, 09) | test-first | A, B1, B2, B3, V | 0/1/1/0 | folded into V (1 pin-only MAJOR: --json cells of 4 edge rows; 1 comment trim) |

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
