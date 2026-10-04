# Metrics: phase4a-investments

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 | code-first | A, B1, V | 0/1/2/0 | yes (quote Z_ENT filter unpinned; MINOR folds: big.Rat exponent row replaces unreachable claim, three doc budgets) |
| SCENARIO-02 | code-first | A, B1, B2, B3, V | 0/2/4/0 | yes (account/position Valid guards and positions ZACCOUNT arm unpinned; MINOR folds: negative-shares row, comment budgets, plan status) |
| SCENARIO-03 | code-first (light) | L, V | 0/1/0/1 | yes (ratio cross-product cells unpinned; folds: negative ratio side refuses, two-nameless-securities pin) |
| SCENARIO-04 (+05, +08) | test-first | A, B1, B2, V | 0/1/3/0 | yes (missing-Lot-entity ranking unpinned; folds: lot class order recorded, decimalOf silent drop → error, comment budgets) |
| SCENARIO-06 (+07) | code-first | A, B1, B2, V | 0/1/2/1 | yes (cross-tier mismatch sort priority unpinned; folds: both-DIFFER render subtest, comment budgets) |

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
