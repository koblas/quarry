# Metrics: phase3c-search

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 (+05) | code-first | A, B1, B2, V | 0/1/4/1 | yes (single-flag --since/--until arms unpinned; split interleave on duplicate source_id — real defect, txn_id tiebreak; doc trims) |
| SCENARIO-02 (+06, +07, +12a) | code-first | A, B1, B2, V | 0/0/2/1 | no (MINORs → STATE.md) |
| SCENARIO-03 | code-first | A, B1, B2, V | 0/0/3/1 | no (MINORs → STATE.md) |
| SCENARIO-04 (+08) | code-first | A, B1, B2, V | 0/0/5/2 | no (MINORs → STATE.md); 1 copy ruling (invalid UTF-8 text/category); pre-existing DECIMAL overflow found + fixed for search |
| SCENARIO-09 (+11, +12b) | code-first | A, B1, V | 0/0/3/2 | no (MINORs → STATE.md) |
| SCENARIO-10 (+13) | code-first | A, B1, B2, V | 0/0/1/2 | no (MINOR → STATE.md) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, test, refactor | 0/3/21/7 | 0 / 20, 825s | BLOCKED |
| 2 | test, correctness | 0/0/1/1 | 0 / 0 (no mutable lines) | PASS WITH FOLLOW-UPS |

## Tokens

Tokens for `phase3c-search` across 6 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 48 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 25 | claude-sonnet-5-5 | 1k | 2,627k | 49,646k | 17k | 8,336k | 60% |
| architect | 7 | claude-opus-5-5, claude-sonnet-5-5 | 0k | 919k | 13,316k | 6k | 2,508k | 18% |
| test-reviewer | 8 | claude-sonnet-5-5 | 0k | 801k | 7,360k | 4k | 1,757k | 13% |
| product-vision | 3 | claude-opus-5-5 | 0k | 256k | 2,088k | 1k | 535k | 4% |
| correctness-reviewer | 2 | claude-opus-5-5 | 0k | 133k | 1,773k | 1k | 349k | 3% |
| triage | 1 | claude-sonnet-5-5 | 0k | 89k | 544k | 0k | 167k | 1% |
| refactor-advisor | 1 | claude-sonnet-5-5 | 0k | 57k | 174k | 0k | 89k | 1% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 56k | 176k | 0k | 89k | 1% |
| **subagent total** | 48 | | 2k | 4,937k | 75,078k | 30k | 13,830k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 0k | 1,829k | 44,688k | 69k | 8,471k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 4 | 702k | 5% |
| plan | 7 | 2,508k | 18% |
| build | 23 | 7,590k | 55% |
| checkpoint | 6 | 1,022k | 7% |
| checkpoint-fix | 1 | 213k | 2% |
| review | 6 | 1,262k | 9% |
| gate-fix | 1 | 533k | 4% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 1 | 0 | 0 | 0 | 1 | 2,654k | 19% |
| SCENARIO-01 | 1 | 4 | 1 | 1 | 0 | 2,715k | 20% |
| SCENARIO-02 | 1 | 4 | 1 | 0 | 0 | 1,860k | 13% |
| SCENARIO-03 | 1 | 4 | 1 | 0 | 0 | 1,238k | 9% |
| SCENARIO-04 | 1 | 4 | 1 | 0 | 0 | 2,268k | 16% |
| SCENARIO-09 | 1 | 3 | 1 | 0 | 0 | 1,733k | 13% |
| SCENARIO-10 | 1 | 4 | 1 | 0 | 0 | 1,364k | 10% |

Developer runs: 25; weighted per run median 257k, p90 602k, max 837k.

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| gate R1 | MCP `text ""`/`since ""`/`until ""` rows unpinned | internal/mcp/search_test.go:63 | SCENARIO-10 |
| gate R1 | vacuous `assert.Zero` on zero sentinel | internal/report/search_test.go:118 | SCENARIO-04 |
| gate R1 | unreachable arms on exported constructible type | internal/report/amount.go:49, internal/mcp/search.go:93 | SCENARIO-03 / SCENARIO-10 |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
