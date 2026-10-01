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
| 2 | correctness, test | 0/0/0/1 | developer mutations 4/4 red | PASS |
| 3 (after final product-vision SHIP WITH CHANGES) | correctness, test | 0/0/1/0 | developer mutations 6/6 red | PASS WITH FOLLOW-UPS |

## Tokens
Tokens for `phase2e-recurring-anomalies` across 6 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 62 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 34 | claude-sonnet-5-5 | 1k | 3,234k | 56,589k | 63k | 10,020k | 62% |
| test-reviewer | 12 | claude-sonnet-5-5 | 0k | 1,098k | 15,523k | 15k | 2,998k | 18% |
| architect | 6 | claude-opus-5-5, claude-sonnet-5-5 | 0k | 678k | 9,819k | 12k | 1,892k | 12% |
| correctness-reviewer | 4 | claude-opus-5-5 | 0k | 236k | 2,531k | 3k | 564k | 3% |
| product-vision | 3 | claude-opus-5-5 | 0k | 207k | 1,320k | 4k | 411k | 3% |
| triage | 1 | claude-sonnet-5-5 | 0k | 83k | 715k | 1k | 179k | 1% |
| refactor-advisor | 1 | claude-sonnet-5-5 | 0k | 49k | 90k | 1k | 73k | 0% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 41k | 196k | 0k | 71k | 0% |
| **subagent total** | 62 | | 2k | 5,626k | 86,782k | 99k | 16,209k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 0k | 268k | 81,569k | 75k | 9,065k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 4 | 590k | 4% |
| plan | 6 | 1,892k | 12% |
| build | 28 | 8,261k | 51% |
| checkpoint | 8 | 1,476k | 9% |
| checkpoint-fix | 4 | 536k | 3% |
| review | 10 | 2,230k | 14% |
| gate-fix | 2 | 1,223k | 8% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 1 | 0 | 0 | 0 | 2 | 4,209k | 26% |
| SCENARIO-01 | 1 | 6 | 1 | 1 | 0 | 3,032k | 19% |
| SCENARIO-05 | 0 | 2 | 1 | 0 | 0 | 627k | 4% |
| SCENARIO-07 | 1 | 4 | 1 | 1 | 0 | 1,528k | 9% |
| SCENARIO-11 | 1 | 4 | 1 | 1 | 0 | 1,940k | 12% |
| SCENARIO-14 | 1 | 4 | 1 | 0 | 0 | 2,083k | 13% |
| SCENARIO-17 | 0 | 2 | 1 | 0 | 0 | 709k | 4% |
| SCENARIO-19 | 0 | 2 | 1 | 0 | 0 | 608k | 4% |
| SCENARIO-20 | 1 | 4 | 1 | 1 | 0 | 1,472k | 9% |

Developer runs: 34; weighted per run median 288k, p90 504k, max 811k.

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| gate R1 | H1 refusal rows missing for recurring/anomalies | `cmd/quarry/run_spend_refusals_test.go:67` | SCENARIO-11, SCENARIO-20 |
| final pass | S3 refusal fix line (pass --until) is a dead end for recurring/anomalies, which drop future-dated charges | `internal/report/window.go:67-69` | SCENARIO-13, SCENARIO-22 |
| gate R1 | non-ASCII column width unpinned after `renderTable` rewrite | `internal/cli/render_table.go:31` | SCENARIO-01 |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
