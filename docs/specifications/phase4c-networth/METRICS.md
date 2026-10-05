# Metrics: phase4c-networth

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01a (+02) | code-first | A, B1, B2, V | 0/0/4/1 | no (MINOR → STATE.md) |
| SCENARIO-01b (+05) | code-first | A, B1, B2, V | 0/0/3/1 | no (MINOR → STATE.md) |
| SCENARIO-03 (+04) | code-first (light) | L (PARTIAL: 4 copy lines; ruled SKILL description → S10), L, V | 0/1/4/0 | yes (--account/--json cells for investment rows unpinned; folds: retirement account, misc by category, spend buy control) |
| SCENARIO-06 | code-first | A, B1, B2, V | 0/1/1/0 | yes (cash/balance before first transaction unpinned — coalesce arm; fold: comment trim) |
| SCENARIO-07 (+08) | code-first | A, B1, B2, V | 0/1/3/0 | no (MAJOR was an unrecorded timing result B1 had measured — orchestrator recorded it in STATE; MINOR → STATE.md) |
| SCENARIO-09 (+18) | code-first | A, B1, B2, V | 0/1/2/0 | yes (no investment account with holdings in v_net_worth — sum(cash) mutant survived; folds: both-flags case, plan file damaged by V restored) |
| SCENARIO-10 (+13) | code-first | A, B1 (did 3-7), V | 0/0/5/1 | no (MINOR → STATE.md) |
| SCENARIO-12 | code-first | A, B1, B2, V (+ product-vision copy ruling on history cells) | 0/2/2/0 | yes (same-day since/until control; native zero-balance cell; fold: monthEnd doc) |
| SCENARIO-15 (+11) | code-first | A, B1, V | 0/1/1/0 | yes (as-of × native and refusal × --json cells; fold: seed doc) |
| SCENARIO-14a | code-first | A, B1, B2, V | 0/2/3/1 | yes (investment-type guard on unvalued read; closed account × networth; folds: zero price, two doc trims) |
| SCENARIO-14b | code-first | A, B1, B2, B3, V (+ product-vision copy ruling on USD form / history totals) | 0/1/4/1 | yes (history × --json; folds: USD-reporting history row, 3 doc trims, STATE dedupe) |
| SCENARIO-16 | code-first | A, B1, B2, V (+ product-vision copy ruling: FirstBalance, new copy) | 0/0/4/1 | no (MINOR → STATE.md) |
| SCENARIO-17 | code-first | A, B1, B2, V | 0/1/3/0 | yes (warnings-from-full-report-before-cut unpinned; folds: 2 doc trims) |

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
