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
| 5 | correctness | 1/0/0/0 | 0 / 3, 72s | BLOCKED |
| 6 | orchestrator verification (one-line prescribed fix) | 0/0/0/0 | — | PASS WITH FOLLOW-UPS |
| 7 (after final product-vision SHIP WITH CHANGES) | test; product-vision narrow | 0/2/0/1 | 0 / 4, 47s | PASS WITH FOLLOW-UPS |

## Tokens
Tokens for `phase2c-snapshots-config` across 6 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 85 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 46 | claude-opus-5-5, claude-sonnet-5-5 | 2k | 5,804k | 126,042k | 77k | 20,245k | 60% |
| test-reviewer | 16 | claude-sonnet-5-5 | 0k | 1,669k | 19,922k | 27k | 4,216k | 13% |
| architect | 12 | claude-opus-5-5, claude-sonnet-5-5 | 0k | 1,469k | 20,400k | 24k | 3,996k | 12% |
| correctness-reviewer | 6 | claude-opus-5-5 | 0k | 1,020k | 11,175k | 7k | 2,427k | 7% |
| product-vision | 2 | claude-opus-5-5 | 0k | 1,404k | 4,270k | 10k | 2,233k | 7% |
| triage | 1 | claude-sonnet-5-5 | 0k | 118k | 1,377k | 1k | 292k | 1% |
| refactor-advisor | 1 | claude-sonnet-5-5 | 0k | 86k | 339k | 1k | 146k | 0% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 49k | 303k | 1k | 95k | 0% |
| **subagent total** | 85 | | 3k | 11,620k | 183,829k | 148k | 33,649k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 0k | 2,452k | 101,697k | 159k | 15,871k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 3 | 2,525k | 8% |
| plan | 12 | 3,996k | 12% |
| build | 37 | 14,025k | 42% |
| checkpoint | 11 | 2,054k | 6% |
| checkpoint-fix | 3 | 1,073k | 3% |
| review | 13 | 4,830k | 14% |
| gate-fix | 6 | 5,147k | 15% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 1 | 0 | 0 | 0 | 6 | 12,839k | 38% |
| PRE-01 | 1 | 3 | 1 | 0 | 0 | 1,177k | 3% |
| SCENARIO-01 | 1 | 3 | 1 | 1 | 0 | 2,486k | 7% |
| SCENARIO-05 | 1 | 3 | 1 | 0 | 0 | 1,271k | 4% |
| SCENARIO-13 | 1 | 4 | 1 | 0 | 0 | 2,690k | 8% |
| SCENARIO-15 | 0 | 2 | 1 | 0 | 0 | 547k | 2% |
| SCENARIO-16 | 1 | 4 | 1 | 0 | 0 | 2,785k | 8% |
| SCENARIO-18 | 0 | 2 | 1 | 0 | 0 | 690k | 2% |
| SCENARIO-21 | 4 | 6 | 1 | 1 | 0 | 3,650k | 11% |
| SCENARIO-24 | 1 | 4 | 1 | 0 | 0 | 1,922k | 6% |
| SCENARIO-28 | 0 | 2 | 1 | 1 | 0 | 881k | 3% |
| SCENARIO-29 | 1 | 4 | 1 | 0 | 0 | 2,711k | 8% |

Developer runs: 46; weighted per run median 380k, p90 720k, max 2,319k.

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
