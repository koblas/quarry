# Metrics: phase3a-mcp-core

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 | code-first | A, B1, B2, V | 0/0/7/0 | no (MINORs → STATE.md) |
| SCENARIO-02 | code-first | A, B1, V | 0/1/4/1 | yes (mcp WithVersion wiring unpinned; test names/rows + doc budgets) |
| SCENARIO-15 (+16) | code-first | A, B1, V | 0/0/4/2 | no (MINORs → STATE.md; handoff contradiction fixed by orchestrator) |
| SCENARIO-03 (+04, +05) | code-first | A, B1 (+B2 steps), V | 0/0/4/1 | no (MINORs → STATE.md); 1 copy ruling (limit-1 wording, no-statement SQL) |
| SCENARIO-07 (+08) | code-first | A, B1, V | 0/0/3/2 | no (MINORs → STATE.md); 1 orchestrator ordering ruling (account sort = quarry accounts order, overflow grouping, warning order) |
| SCENARIO-09 (+10, +14) | code-first | A, B1, V | 0/0/1/1 | no (MINOR → STATE.md) |
| SCENARIO-11 (+12) | code-first | A, B1, V | 0/0/2/0 | no (MINORs → STATE.md); 1 copy ruling (warning order, status word for all, advice tail by limit/type) |
| SCENARIO-06 (+13, +17) | code-first | A, B1, V | 0/1/4/3 | yes (handler Canceled arm unpinned; InterruptedBy own-package test, refusal chain row, bounded wait, doc trim); 1 orchestrator copy ruling (timeout line renders configured seconds) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, test, refactor | 0/1/13/10 | 0 / 20, 901s | BLOCKED |
| 2 | correctness, test, refactor | 0/0/8/7 | 0 / 4, 214s | PASS WITH FOLLOW-UPS |
| final product-vision | — | 0/0/2/1 | — | SHIP |
| 3 (post-ship: cancel hang fix pass 2) | correctness, arch, test | 0/2/5/3 | 0 / 5, 102s | BLOCKED |
| 4 (after fix pass 3) | correctness, test | 0/0/4/0 | 0 / 5, 279s | PASS WITH FOLLOW-UPS |

## Tokens
Tokens for `phase3a-mcp-core` across 6 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 57 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 28 | claude-sonnet-5-5 | 1k | 2,775k | 51,859k | 43k | 8,869k | 56% |
| architect | 9 | claude-opus-5-5, claude-sonnet-5-5 | 0k | 1,130k | 16,681k | 11k | 3,138k | 20% |
| test-reviewer | 10 | claude-sonnet-5-5 | 0k | 893k | 6,524k | 13k | 1,837k | 12% |
| correctness-reviewer | 2 | claude-opus-5-5 | 0k | 269k | 7,370k | 4k | 1,094k | 7% |
| product-vision | 5 | claude-opus-5-5 | 0k | 300k | 2,203k | 6k | 628k | 4% |
| refactor-advisor | 2 | claude-sonnet-5-5 | 0k | 103k | 488k | 1k | 184k | 1% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 39k | 166k | 0k | 66k | 0% |
| **subagent total** | 57 | | 2k | 5,510k | 85,290k | 79k | 15,815k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 0k | 607k | 30,944k | 86k | 4,738k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 5 | 628k | 4% |
| plan | 9 | 3,138k | 20% |
| build | 25 | 7,510k | 47% |
| checkpoint | 8 | 1,120k | 7% |
| checkpoint-fix | 2 | 348k | 2% |
| review | 7 | 2,061k | 13% |
| gate-fix | 1 | 1,011k | 6% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 1 | 0 | 0 | 0 | 1 | 4,016k | 25% |
| SCENARIO-01 | 1 | 4 | 1 | 0 | 0 | 1,585k | 10% |
| SCENARIO-02 | 1 | 3 | 1 | 1 | 0 | 1,467k | 9% |
| SCENARIO-03 | 1 | 3 | 1 | 0 | 0 | 1,483k | 9% |
| SCENARIO-06 | 1 | 3 | 1 | 1 | 0 | 2,270k | 14% |
| SCENARIO-07 | 1 | 3 | 1 | 0 | 0 | 1,365k | 9% |
| SCENARIO-09 | 1 | 3 | 1 | 0 | 0 | 1,139k | 7% |
| SCENARIO-11 | 1 | 3 | 1 | 0 | 0 | 1,323k | 8% |
| SCENARIO-15 | 1 | 3 | 1 | 0 | 0 | 1,165k | 7% |

Developer runs: 28; weighted per run median 274k, p90 513k, max 1,011k.

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| gate R1 | stderr log line copies DuckDB reason (row values/SQL) — Rule 4 | internal/mcp/result.go:58 | SCENARIO-03 |
| post-gate (orchestrator loop) | next call hangs in cgo after early cancel (DuckDB shared instance cache) | internal/platform/duckdb/duckdb.go:80 | SCENARIO-06 |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
