# Metrics: phase3c-search

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 (+05) | code-first | A, B1, B2, V | 0/1/4/1 | yes (single-flag --since/--until arms unpinned; split interleave on duplicate source_id — real defect, txn_id tiebreak; doc trims) |
| SCENARIO-02 (+06, +07, +12a) | code-first | A, B1, B2, V | 0/0/2/1 | no (MINORs → STATE.md) |

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
