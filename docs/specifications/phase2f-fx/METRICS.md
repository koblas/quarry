# Metrics: phase2f-fx

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-07 | test-first (Replace rates hook) | A, B1, B2, V | 0/2/5/2 | yes (fx_rates PK/CHECK unpinned; local-date Need.Last unpinned; test-only + comments) |
| SCENARIO-01 | test-first (Replace import_runs rates UPDATE) | A, B1, B2, V | 0/2/3/1 | yes (later-span failure unreached; Valet scheme/header unpinned; test-only + comments); 1 copy ruling (rates_checked_from, store.rates placement, nothing-fetched lines) |
| SCENARIO-04 (+02) | test-first (carry + floor in Replace) | A, B1, B2, V | 0/2/4/0 | yes (rates-before-prune order unpinned; rates surviving unreadable import_runs unpinned; test-only + comments); 1 copy ruling (carry reasons, combined line) |
| SCENARIO-03 (+05) | test-first (failed fetch still swaps) | A, B1, B2, V | 0/1/4/0 | yes (per-request timeout scope unpinned; test-only + docs); 1 copy ruling (warning order, partial Rates line, stop rule) |
| SCENARIO-06 | code-first (light) | L, V | 0/0/6/0 | no (MINORs → STATE.md) |
| SCENARIO-16 (+15) | code-first | A, B1, B2, V | 0/3/2/0 | yes (usage-before-config order unpinned at 3 sites; resolve CAD default unpinned; accounts warnings order unpinned; test-only + docs); 1 behaviour/copy ruling (table-shape refusal, status warns, sync/findings/snapshots refuse, P2d-10 amended) |
| SCENARIO-08 (+14) | code-first | A, B1, V | 0/2/3/0 | yes (month NULL arm unpinned; empty-window USD/native text cells; test-only + comments) |
| SCENARIO-10 (+09) | code-first | A, B1, V | 0/0/5/0 | no (MINORs → STATE.md) |
| SCENARIO-12 (+13) | code-first | A, B1, B2, V | 0/4/7/1 | yes (zero-fill not in plan; inclusive window bounds; USD-mode unrated + empty-window json cells; cashflow --account positive cell; test-only + plan + comments); 1 copy ruling (warning order, FX scope, zero fill) |
| SCENARIO-17 | code-first | A, B1, B2, V | 0/2/5/0 | yes (recurring left-out→FX order unpinned; --json edge cells missing; test-only + comments); 1 copy ruling (JSON key order, cell/prefix rule, sort tier, Long wrap) |
| SCENARIO-18 | code-first | A, B1, B2, V | 0/0/4/2 | no (MINORs → STATE.md); 1 copy ruling (Long wrap, JSON keys, prefix) |
| SCENARIO-19 (+11) | code-first | A, B1, V | 0/1/4/1 | yes (blank cell before Status unpinned; test-only + plan + comments); 1 copy ruling (accounts warnings, key order, alignment, Long) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness ×2, test ×2, refactor | 1/5/~30/~10 | 0 / 20, 866s | BLOCKED |
| 2 | arch, correctness-A, test-A | 0/2/3/2 | 0 / 8, 445s | BLOCKED |
| 3 | correctness-A, test-A | 0/0/1/2 | 0 / 2, 297s | PASS WITH FOLLOW-UPS |
| 4 (after final product-vision SHIP WITH CHANGES) | correctness-B, test-B | 0/0/3/1 | 0 / 1, 68s | PASS WITH FOLLOW-UPS |

## Tokens
Tokens for `phase2f-fx` across 6 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 104 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 55 | claude-sonnet-5-5 | 3k | 6,276k | 140,889k | 102k | 22,447k | 61% |
| test-reviewer | 17 | claude-sonnet-5-5 | 1k | 2,189k | 30,893k | 45k | 6,053k | 16% |
| architect | 12 | claude-opus-5-5, claude-sonnet-5-5 | 1k | 1,557k | 25,905k | 34k | 4,709k | 13% |
| correctness-reviewer | 5 | claude-opus-5-5 | 0k | 548k | 11,662k | 9k | 1,899k | 5% |
| product-vision | 11 | claude-opus-5-5 | 0k | 603k | 4,079k | 13k | 1,226k | 3% |
| triage | 1 | claude-sonnet-5-5 | 0k | 149k | 1,998k | 2k | 397k | 1% |
| arch-reviewer | 2 | claude-sonnet-5-5 | 0k | 83k | 336k | 1k | 143k | 0% |
| refactor-advisor | 1 | claude-sonnet-5-5 | 0k | 46k | 182k | 1k | 82k | 0% |
| **subagent total** | 104 | | 4k | 11,451k | 215,945k | 209k | 36,956k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 0k | 1,160k | 74,744k | 160k | 10,594k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 13 | 1,910k | 5% |
| plan | 11 | 4,422k | 12% |
| build | 43 | 16,736k | 45% |
| checkpoint | 12 | 3,496k | 9% |
| checkpoint-fix | 9 | 2,919k | 8% |
| review | 13 | 4,680k | 13% |
| gate-fix | 3 | 2,793k | 8% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 0 | 0 | 0 | 0 | 3 | 8,784k | 24% |
| SCENARIO-01 | 1 | 4 | 1 | 1 | 0 | 2,750k | 7% |
| SCENARIO-03 | 1 | 4 | 1 | 1 | 0 | 2,981k | 8% |
| SCENARIO-04 | 1 | 4 | 1 | 1 | 0 | 2,971k | 8% |
| SCENARIO-06 | 0 | 2 | 1 | 0 | 0 | 960k | 3% |
| SCENARIO-07 | 1 | 4 | 1 | 1 | 0 | 2,253k | 6% |
| SCENARIO-08 | 1 | 3 | 1 | 1 | 0 | 1,738k | 5% |
| SCENARIO-10 | 1 | 3 | 1 | 0 | 0 | 1,522k | 4% |
| SCENARIO-12 | 1 | 4 | 1 | 1 | 0 | 2,816k | 8% |
| SCENARIO-16 | 1 | 4 | 1 | 1 | 0 | 2,261k | 6% |
| SCENARIO-17 | 1 | 4 | 1 | 1 | 0 | 2,803k | 8% |
| SCENARIO-18 | 1 | 4 | 1 | 0 | 0 | 2,443k | 7% |
| SCENARIO-19 | 1 | 3 | 1 | 1 | 0 | 2,673k | 7% |

Developer runs: 55; weighted per run median 311k, p90 762k, max 1,701k.

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| gate R1 | BLOCKER interior fx_rates gap after last stored rate (silent stale conversion) | internal/fx/plan.go:36-38 | SCENARIO-04 (Have/planner; spans from 01) |
| gate R1 | MAJOR checked floor resurrected after rates fault | internal/store/duckstore/history.go:193,226 | SCENARIO-04 |
| gate R1 | MAJOR newRatesSource mutable package global | cmd/quarry/run.go:36 | SCENARIO-01 |
| gate R1 | MAJOR Valet per-observation decode reason unpinned (mutant survived) | internal/fx/valet.go:98 | SCENARIO-03 |
| gate R1 | MAJOR inverted Need (all transactions future) sent to Valet | internal/store/duckstore/rates.go needSpan | SCENARIO-07 |
| gate R2 | MAJOR head-span gap (mirror of R1 BLOCKER) | internal/fx/plan.go:35 | SCENARIO-01 |
| gate R2 | MAJOR shipped fx wiring unpinned after global removed | cmd/quarry/run.go:66 | gate fix pass 1 |
| final pass | MAJOR empty window zero-fills rows beside the empty note; --json periods/rows no longer [] | internal/report/period.go fillSeries | SCENARIO-12 (zero-fill ruling) |
| final pass | MAJOR partial-fetch warning omits "later dates convert at the <last> rate" | internal/snapshot/import.go:106 | SCENARIO-03 (ruled copy) |
| final pass | MAJOR status Long Rates paragraph unwrapped (113 cols) | internal/cli/status.go:25 | SCENARIO-06 (ruled copy) |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
