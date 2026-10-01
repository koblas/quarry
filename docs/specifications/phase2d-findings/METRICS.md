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
| 4 (after product-vision re-check: no-snapshots path) | correctness, test | 0/0/1/1 | developer mutations 2/2 red | PASS WITH FOLLOW-UPS |

## Tokens
Tokens for `phase2d-findings` across 6 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 135 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 81 | claude-sonnet-5-5 | 3k | 7,631k | 136,620k | 116k | 23,785k | 62% |
| test-reviewer | 23 | claude-sonnet-5-5 | 1k | 2,111k | 27,799k | 27k | 5,553k | 15% |
| architect | 16 | claude-opus-5-5, claude-sonnet-5-5 | 1k | 1,907k | 29,180k | 24k | 5,421k | 14% |
| correctness-reviewer | 5 | claude-opus-5-5 | 0k | 463k | 7,949k | 6k | 1,406k | 4% |
| product-vision | 7 | claude-opus-5-5 | 0k | 675k | 3,734k | 30k | 1,368k | 4% |
| triage | 1 | claude-sonnet-5-5 | 0k | 276k | 2,007k | 3k | 561k | 1% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 47k | 274k | 0k | 88k | 0% |
| refactor-advisor | 1 | claude-sonnet-5-5 | 0k | 52k | 159k | 1k | 84k | 0% |
| **subagent total** | 135 | | 5k | 13,163k | 207,721k | 207k | 38,265k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 0k | 1,093k | 83,755k | 164k | 11,380k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 8 | 1,929k | 5% |
| plan | 16 | 5,421k | 14% |
| build | 64 | 18,194k | 48% |
| checkpoint | 18 | 2,760k | 7% |
| checkpoint-fix | 13 | 2,598k | 7% |
| review | 12 | 4,370k | 11% |
| gate-fix | 4 | 2,992k | 8% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 1 | 0 | 0 | 0 | 4 | 9,325k | 24% |
| SCENARIO-01 | 1 | 4 | 1 | 0 | 0 | 2,450k | 6% |
| SCENARIO-04 | 1 | 4 | 1 | 0 | 0 | 1,178k | 3% |
| SCENARIO-06 | 1 | 4 | 1 | 1 | 0 | 1,456k | 4% |
| SCENARIO-08 | 1 | 4 | 1 | 1 | 0 | 1,544k | 4% |
| SCENARIO-09 | 0 | 2 | 1 | 1 | 0 | 703k | 2% |
| SCENARIO-11 | 1 | 4 | 1 | 1 | 0 | 2,594k | 7% |
| SCENARIO-14 | 0 | 2 | 1 | 1 | 0 | 884k | 2% |
| SCENARIO-15 | 1 | 3 | 1 | 1 | 0 | 1,062k | 3% |
| SCENARIO-16 | 1 | 4 | 1 | 1 | 0 | 1,781k | 5% |
| SCENARIO-18 | 1 | 4 | 1 | 0 | 0 | 1,572k | 4% |
| SCENARIO-19 | 1 | 3 | 1 | 0 | 0 | 1,447k | 4% |
| SCENARIO-21 | 1 | 3 | 1 | 0 | 0 | 919k | 2% |
| SCENARIO-22 | 0 | 2 | 1 | 1 | 0 | 1,058k | 3% |
| SCENARIO-24 | 1 | 4 | 1 | 1 | 0 | 1,615k | 4% |
| SCENARIO-25 | 1 | 4 | 1 | 1 | 0 | 1,976k | 5% |
| SCENARIO-26 | 1 | 4 | 1 | 1 | 0 | 1,627k | 4% |
| SCENARIO-27 | 1 | 4 | 1 | 1 | 0 | 1,966k | 5% |
| SCENARIO-28 | 1 | 5 | 1 | 1 | 0 | 3,108k | 8% |

Developer runs: 81; weighted per run median 243k, p90 506k, max 1,064k.

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
