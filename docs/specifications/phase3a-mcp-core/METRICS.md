# Metrics: phase3a-mcp-core

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 | code-first | A, B1, B2, V | 0/0/7/0 | no (MINORs → STATE.md) |
| SCENARIO-02 | code-first | A, B1, V | 0/1/4/1 | yes (mcp WithVersion wiring unpinned; test names/rows + doc budgets) |
| SCENARIO-15 (+16) | code-first | A, B1, V | 0/0/4/2 | no (MINORs → STATE.md; handoff contradiction fixed by orchestrator) |
| SCENARIO-03 (+04, +05) | code-first | A, B1 (+B2 steps), V | 0/0/4/1 | no (MINORs → STATE.md); 1 copy ruling (limit-1 wording, no-statement SQL) |

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
