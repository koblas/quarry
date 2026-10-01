# Metrics: phase2e-recurring-anomalies

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 (+02, 03, 04) | code-first | A, B1, B2, B3, B4, V | 0/1/5/1 | yes (currency-above-state sort tier unpinned; test-only + comments) |
| SCENARIO-05 (+06) | code-first (light) | L, V | 0/0/1/1 | no (MINOR → STATE.md) |
| SCENARIO-07 (+08, 09, 10) | code-first | A, B1, B2, V | 0/1/1/1 | yes (JSON `new:false` arm unpinned; test-only) |
| SCENARIO-11 (+12, 13) | code-first | A, B1, B2, V | 0/1/1/0 | yes (closed-account `--account` edge row unpinned; test-only + comment) |
| SCENARIO-14 (+15, 16) | code-first | A, B1, B2, V | 0/0/6/1 | no (MINORs → STATE.md); 1 copy ruling (not_judged scope, empty-window trigger, no-payee cell) |
| SCENARIO-17 (+18) | code-first (light) | L, V | 0/0/0/1 | no (NIT → STATE.md) |
| SCENARIO-19 | code-first (light) | L, V | 0/0/0/2 | no (NITs → STATE.md) |

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
