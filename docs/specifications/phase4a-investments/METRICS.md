# Metrics: phase4a-investments

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 | code-first | A, B1, V | 0/1/2/0 | yes (quote Z_ENT filter unpinned; MINOR folds: big.Rat exponent row replaces unreachable claim, three doc budgets) |
| SCENARIO-02 | code-first | A, B1, B2, B3, V | 0/2/4/0 | yes (account/position Valid guards and positions ZACCOUNT arm unpinned; MINOR folds: negative-shares row, comment budgets, plan status) |
| SCENARIO-03 | code-first (light) | L, V | 0/1/0/1 | yes (ratio cross-product cells unpinned; folds: negative ratio side refuses, two-nameless-securities pin) |
| SCENARIO-04 (+05, +08) | test-first | A, B1, B2, V | 0/1/3/0 | yes (missing-Lot-entity ranking unpinned; folds: lot class order recorded, decimalOf silent drop → error, comment budgets) |
| SCENARIO-06 (+07) | code-first | A, B1, B2, V | 0/1/2/1 | yes (cross-tier mismatch sort priority unpinned; folds: both-DIFFER render subtest, comment budgets) |
| SCENARIO-09 (+10) | code-first | A, B1, B2, V | 0/0/1/1 | no (MINOR/NIT → STATE.md) |
| SCENARIO-11 | code-first | A, B1, B2, V | 0/0/0/1 | no |
| SCENARIO-13 | code-first (light) | L, V | 0/0/3/1 | no (MINOR/NIT → STATE.md); appended after reference check run 1 |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, test, refactor | 0/3/14/9 | 0 / 20, 1431s | BLOCKED |
| 2 | test | 0/0/0/0 | 0 / 4, 274s | PASS |
| final pass | product-vision | 0/0/3/0 | - | SHIP |

## Tokens

Tokens for `phase4a-investments` across 7 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 57 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 34 | claude-sonnet-5-5 | 2k | 3,918k | 106,790k | 36k | 15,757k | 67% |
| architect | 7 | claude-opus-5-5, claude-sonnet-5-5 | 0k | 985k | 15,764k | 4k | 2,829k | 12% |
| test-reviewer | 10 | claude-sonnet-5-5 | 0k | 988k | 14,452k | 9k | 2,726k | 12% |
| product-vision | 2 | claude-opus-5-5 | 0k | 911k | 1,898k | 1k | 1,332k | 6% |
| correctness-reviewer | 1 | claude-opus-5-5 | 0k | 143k | 2,295k | 4k | 430k | 2% |
| triage | 1 | claude-sonnet-5-5 | 0k | 120k | 1,070k | 2k | 267k | 1% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 69k | 257k | 0k | 112k | 0% |
| refactor-advisor | 1 | claude-sonnet-5-5 | 0k | 60k | 193k | 0k | 94k | 0% |
| **subagent total** | 57 | | 3k | 7,193k | 142,719k | 56k | 23,548k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 0k | 376k | 44,076k | 116k | 5,741k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 3 | 1,598k | 7% |
| plan | 7 | 2,829k | 12% |
| build | 28 | 13,522k | 57% |
| checkpoint | 8 | 1,385k | 6% |
| checkpoint-fix | 5 | 1,571k | 7% |
| review | 5 | 1,978k | 8% |
| gate-fix | 1 | 664k | 3% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 1 | 0 | 0 | 0 | 1 | 4,576k | 19% |
| SCENARIO-01 | 1 | 3 | 1 | 1 | 0 | 2,707k | 11% |
| SCENARIO-02 | 1 | 5 | 1 | 1 | 0 | 3,641k | 15% |
| SCENARIO-03 | 0 | 2 | 1 | 1 | 0 | 1,326k | 6% |
| SCENARIO-04 | 1 | 4 | 1 | 1 | 0 | 5,229k | 22% |
| SCENARIO-06 | 1 | 4 | 1 | 1 | 0 | 2,353k | 10% |
| SCENARIO-09 | 1 | 4 | 1 | 0 | 0 | 1,812k | 8% |
| SCENARIO-11 | 1 | 4 | 1 | 0 | 0 | 1,176k | 5% |
| SCENARIO-13 | 0 | 2 | 1 | 0 | 0 | 728k | 3% |

Developer runs: 34; weighted per run median 351k, p90 937k, max 1,611k.

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| gate R1 | I4-5 "all dates count" unpinned (future-dated walk filter survives) | internal/store/duckstore/shares.go:30-36 | SCENARIO-04 |
| gate R1 | I4-1 security currency "no refusal for other values" unpinned | internal/importer/securities.go:57-59 | SCENARIO-01 |
| gate R1 | whitespace-only security name unpinned | internal/importer/securities.go:50 | SCENARIO-03 |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
