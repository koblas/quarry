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

## Tokens

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
