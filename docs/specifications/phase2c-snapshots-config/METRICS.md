# Metrics: phase2c-snapshots-config

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| PRE-01 (pre-step, neutral) | code-first | B1, B2, V | 0/0/3/1 | no (MINORs → STATE.md) |
| SCENARIO-01 (+04, 09, 10) | code-first | A, B1, V | 0/1/4/2 | yes (array-of-tables `got` empty; 2 mid-feature copy rulings) |
| SCENARIO-05 (+06, 07, 08, 11, 12) | code-first | A, B1, V | 0/0/0/0 | no |
| SCENARIO-13 (+14) | test-first | A, B1, B2, V | 0/0/4/4 | no (MINORs → STATE.md); 1 mid-feature copy ruling |
| SCENARIO-15 | code-first (light) | L, V | 0/0/1/0 | no (MINOR → STATE.md) |
| SCENARIO-16 (+03, 17, 19, 20, 34) | code-first | A, B1, B2, V | 0/0/3/1 | no (MINORs → STATE.md); 1 mid-feature copy ruling (6 outcomes) |
| SCENARIO-18 | code-first (light) | L, V | 0/0/2/0 | no (MINORs → STATE.md) |
| SCENARIO-21 (+23, 25, 26, 27) | test-first | A, B1, B2, B3, B4, V | 0/2/6/2 | yes (ticked test missing; false unreachable marker); 1 copy ruling (U1–U7), plan amended once; 3 architect API 529s |
| SCENARIO-24 (+02, 22) | test-first | A, B1, B2, V | 0/0/4/0 | no (MINORs → STATE.md); 1 copy ruling (prune --json store read) |
| SCENARIO-28 | code-first (light) | L, V | 0/3/1/0 | yes (three ruled `--json` rows unpinned; test-only fix) |
| SCENARIO-29 (+30, 31, 32, 33) | test-first | A, B1, B2, V | 0/0/4/1 | no (MINORs → STATE.md); 1 copy ruling (interrupt + composites) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness ×2, test ×2, refactor | 1/4/34/12 | 0 / 20, 858s | BLOCKED |
| 2 | correctness, test | 0/2/2/4 | 2 / 15, 319s (1 equivalent) | BLOCKED |
| 3 | correctness, test | 1/0/0/3 | 0 / 5, 96s | BLOCKED |
| 4 | correctness | 1/0/2/1 | 0 / 2, 40s | BLOCKED |

## Tokens

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
