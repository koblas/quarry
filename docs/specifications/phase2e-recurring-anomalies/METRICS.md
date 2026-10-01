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
| SCENARIO-20 (+21, 22) | code-first | A, B1, B2+B3, V | 0/1/0/0 | yes (no-payee `--account` tally unpinned; test-only) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness ×2, test ×2, refactor | 0/2/~25/~10 | 0 / 20, 589s | BLOCKED |

## Tokens

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| gate R1 | H1 refusal rows missing for recurring/anomalies | `cmd/quarry/run_spend_refusals_test.go:67` | SCENARIO-11, SCENARIO-20 |
| gate R1 | non-ASCII column width unpinned after `renderTable` rewrite | `internal/cli/render_table.go:31` | SCENARIO-01 |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
