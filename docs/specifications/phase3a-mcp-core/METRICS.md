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
