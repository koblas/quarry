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

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| gate R1 | MCP `text ""`/`since ""`/`until ""` rows unpinned | internal/mcp/search_test.go:63 | SCENARIO-10 |
| gate R1 | vacuous `assert.Zero` on zero sentinel | internal/report/search_test.go:118 | SCENARIO-04 |
| gate R1 | unreachable arms on exported constructible type | internal/report/amount.go:49, internal/mcp/search.go:93 | SCENARIO-03 / SCENARIO-10 |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
