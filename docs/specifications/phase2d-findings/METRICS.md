# Metrics: phase2d-findings

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 (+02, 03) | test-first | A, B1, B2, V | 0/0/3/1 | no (comment MINORs → STATE.md) |
| SCENARIO-04 (+05) | code-first | A, B1, B2, V | 0/0/2/2 | no (comment MINORs → STATE.md) |
| SCENARIO-06 (+07) | code-first | A, B1, B2, V | 0/2/3/0 | yes (carried-flag wiring and carried type unpinned; test-only fix) |
| SCENARIO-08 | code-first | A, B1, B2, V | 0/2/3/0 | yes (findings read-fault reason and silent format-3 rows unpinned; test-only fix); 1 copy ruling (both carry faults) |
| SCENARIO-09 (+10) | code-first (light) | L, V | 0/1/0/2 | yes (reverse-order day bound unpinned; test-only fix) |
| SCENARIO-11 (+12, 13) | code-first | A, B1, B2, V | 0/1/5/2 | yes (false unreachable on unknown-type rows; fallback + MINOR folds); 1 copy ruling (uncategorized Long row) |
| SCENARIO-14 | code-first (light) | L, V | 0/3/1/0 | yes (counts per field, null payee, stdout write fault unpinned; test-only + helper extraction) |
| SCENARIO-15 | code-first | A, B1, V | 0/1/4/1 | yes (hand-written TOML item splitter unpinned; tree now decides non-string, splitter spells only) |
| SCENARIO-16 (+17) | code-first | A, B1, B2, V | 0/1/2/0 | yes (W1 quoting/order unpinned at cli boundary; test-only); sizing SPLIT overruled to one 4-batch run |
| SCENARIO-18 | code-first | A, B1, B2, V | 0/0/1/0 | no (comment MINOR → STATE.md); 1 copy ruling (5 status/type outcomes) |
| SCENARIO-19 (+20) | code-first | A, B1, V | 0/0/1/1 | no (plan drift MINOR fixed in plan Handoff); 1 copy ruling (status Long ¶2, warnings[]) |
| SCENARIO-21 | code-first | A, B1, V | 0/0/1/1 | no (comment MINOR → STATE.md) |
| SCENARIO-22 (+23) | code-first (light) | L, V | 0/4/2/0 | yes (listing fault, id columns, --csv --json precedence, ignored row unpinned; test-only + rename) |
| SCENARIO-24 | code-first | A, B1, B2, V | 0/1/3/0 | yes (deposit-first pair unpinned; test-only + comment trims) |
| SCENARIO-25 | code-first | A, B1, B2, V | 0/1/5/2 | yes (count join not type-gated; SQL gate + test + comment trims) |
| SCENARIO-26 | code-first | A, B1, B2, V | 0/2/3/1 | yes (key Unicode rows, name tiebreak unpinned; test-only + comment trims) |
| SCENARIO-27 | code-first | A, B1, B2, V | 0/2/2/1 | yes (key Unicode rows, path sort tier unpinned; test-only + unreachable marker); 1 id ruling (income prefix) |
| SCENARIO-28 | code-first | A, B1, B2, B3, V | 0/1/4/1 | yes (plan/spec drift for two extra reference sources; docs + deterministic sort test); 2 reference-scope rulings |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness ×2, test ×2, refactor | 0/8/~20/~6 | 0 / 20, 636s | BLOCKED |
| 2 | correctness, test | 0/0/5/1 | 1 / 8, 439s (equivalent) | PASS WITH FOLLOW-UPS |
| 3 (after final product-vision SHIP WITH CHANGES) | correctness, test | 0/0/8/1 | 0 / 1, 75s | PASS WITH FOLLOW-UPS |

## Tokens

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
