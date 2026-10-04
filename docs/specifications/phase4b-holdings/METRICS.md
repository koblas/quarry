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
| SCENARIO-08 | code-first | A, B1, B2, V | 0/0/4/0 | no (MINOR → STATE.md; nil-deref path verified unreachable) |
| SCENARIO-10 | code-first | A, B1, V | 0/0/3/0 | no (MINOR → STATE.md) |
| SCENARIO-09 | code-first | A, B1, V | 0/0/4/1 | no (MINOR → STATE.md) |
| SCENARIO-13 | code-first (light) | L, V | 0/0/2/2 | no (MINOR → STATE.md); cap ruled mid-scenario, built in V |
| SCENARIO-15 | code-first | A, B1, B2, B3, V | 0/0/2/2 | no (MINOR → STATE.md) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, test, refactor | 0/3/10/8 | 0 / 20 (1 timed out), 1212s | BLOCKED |
| 2 | correctness, test | 0/0/0/0 | 0 / 6, 729s (first run aborted: isolated-copy duckstore failure, not reproduced) | PASS |
| final pass | product-vision | 0/2/0/0 | - | SHIP WITH CHANGES |
| 3 | correctness | 0/2/0/0 | 0 mutable lines | BLOCKED (both ruled by product-vision, fixed in pass 3) |
| 4 | correctness, test | 0/0/5/2 | - | PASS WITH FOLLOW-UPS (fix-pass cap reached) |
| final confirmation | product-vision | 0/0/0/0 | - | SHIP |

## Tokens

Tokens for `phase4b-holdings` across 7 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 82 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 46 | claude-sonnet-5-5 | 2k | 5,101k | 126,313k | 88k | 19,451k | 67% |
| test-reviewer | 15 | claude-sonnet-5-5 | 0k | 1,384k | 19,505k | 23k | 3,796k | 13% |
| architect | 11 | claude-opus-5-5, claude-sonnet-5-5 | 0k | 1,299k | 17,532k | 6k | 3,408k | 12% |
| correctness-reviewer | 5 | claude-opus-5-5 | 0k | 334k | 6,760k | 5k | 1,121k | 4% |
| product-vision | 2 | claude-opus-5-5 | 0k | 671k | 2,080k | 1k | 1,050k | 4% |
| triage | 1 | claude-sonnet-5-5 | 0k | 78k | 426k | 1k | 143k | 0% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 68k | 513k | 0k | 137k | 0% |
| refactor-advisor | 1 | claude-sonnet-5-5 | 0k | 52k | 96k | 0k | 75k | 0% |
| **subagent total** | 82 | | 3k | 8,986k | 173,226k | 125k | 29,183k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 0k | 415k | 106,729k | 118k | 12,091k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 3 | 1,194k | 4% |
| plan | 11 | 3,408k | 12% |
| build | 40 | 15,649k | 54% |
| checkpoint | 12 | 1,871k | 6% |
| checkpoint-fix | 3 | 795k | 3% |
| review | 10 | 3,258k | 11% |
| gate-fix | 3 | 3,008k | 10% |

Developer runs: 46; weighted per run median 336k, p90 848k, max 1,358k.

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| gate R1 | MCP holdings missing from deadline test | internal/mcp/timeout_test.go:104-121 | SCENARIO-13 |
| gate R1 | S.1 "no --all flag" unpinned | cmd/quarry/run_usage_test.go | SCENARIO-03 |
| gate R1 | zero / placeholder price not read through real reader | internal/store/duckstore/holdings_test.go | SCENARIO-02 |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
