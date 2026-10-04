# Metrics: phase4b-holdings

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 (+14) | test-first | A, B1, B2, V | 0/1/4/0 | yes (leaving a negative count unpinned; folds: negative DECIMAL edge, mid-history net-zero day, one-millionth boundary, overflow decision recorded) |
| SCENARIO-02 | code-first | A, B1, V | 0/2/2/0 | yes (not-in-reports/linked-tracking listing unpinned; no-price row converted cells unpinned; folds: positional asserts, stale doc) |
| SCENARIO-03 | code-first | A, B1, B2, V | 0/1/5/1 | yes (zero price/value in --json unpinned; folds: total-rule test copy, read counter, home-unset row, test structure, currency escape) |
| SCENARIO-04 | code-first (light) | L, V | 0/0/2/0 | no (MINOR → STATE.md) |
| SCENARIO-05 (+11) | code-first | A, B1, B2, V | 0/0/3/2 | no (MINOR → STATE.md) |
| SCENARIO-06 (+12) | code-first | A, B1, V | 0/0/3/1 | no (MINOR → STATE.md; S07 folds the doc fix) |
| SCENARIO-07 | code-first | A, B1, V | 0/0/1/2 | no (MINOR → STATE.md) |

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
