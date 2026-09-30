# Metrics: phase2b-spending

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 (+02, 03, 04) | code-first | A, B1, B2, V | 0/0/1/0 | no (MINOR → STATE.md) |
| SCENARIO-26 (+25) | code-first (LIGHT) | L, V | 0/0/0/1 | no |
| SCENARIO-06 (+08) | code-first | A, B1, B2, V | 0/2/4/1 | yes |
| SCENARIO-09 (+05, 07) | code-first | A, B1, B2, V | 0/2/4/0 | yes |
| SCENARIO-16 | code-first (LIGHT) | L, V | 0/0/0/0 | no |
| SCENARIO-10 | code-first (LIGHT) | L, V | 0/1/3/0 | yes |
| SCENARIO-11 | code-first | A, B1, B2, V | 0/1/2/0 | yes |
| SCENARIO-13 | code-first | A (PARTIAL: Bash outage), B1, B2, V | 0/1/3/1 | yes |
| SCENARIO-12 (+18) | code-first | A, B1, B2, V | 0/0/2/2 | no (→ STATE.md) |
| SCENARIO-14 (+15, 19) | code-first | A, B1, B2, V | 0/1/3/1 | yes (real bug: `--account ""` matched an empty name) |
| SCENARIO-17 | code-first (LIGHT) | L, V | 0/0/4/0 | no (MINORs → STATE.md); 26 mutants, 24 killed, 2 equivalent |
| SCENARIO-20 (+21, 23, 24) | code-first | A, B1, B2, V | 0/0/4/0 | no (MINORs → STATE.md); ~30 mutants, 3 equivalent survivors |
| SCENARIO-22 | code-first (LIGHT) | L, V | 0/0/1/1 | no (spec line removed by orchestrator) |
| SCENARIO-27 (+28, 29; post-Gate) | code-first | A, B1, B2, V | 0/0/5/1 | no (comment MINORs → STATE.md) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, test, refactor | 0/4/10/10 | 0 / 20, 202s | BLOCKED |
| 2 | test | 0/0/0/1 | — (no production logic change) | PASS WITH FOLLOW-UPS |
| 3 (post-Gate SCENARIO-27) | arch, correctness, test, refactor | 0/0/6/5 | 0 / 4, 81s | PASS WITH FOLLOW-UPS |
| 4 (after final product-vision SHIP WITH CHANGES) | test | 0/0/0/0 | — (strings only) | PASS |

## Tokens
Tokens for `phase2b-spending` across 6 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 97 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 55 | claude-opus-5-5, claude-sonnet-5-5 | 2k | 4,681k | 74,183k | 69k | 13,618k | 62% |
| test-reviewer | 18 | claude-sonnet-5-5 | 0k | 1,355k | 16,669k | 15k | 3,437k | 16% |
| architect | 10 | claude-opus-5-5, claude-sonnet-5-5 | 0k | 1,144k | 13,738k | 15k | 2,878k | 13% |
| product-vision | 7 | claude-opus-5-5 | 0k | 370k | 2,934k | 9k | 802k | 4% |
| correctness-reviewer | 2 | claude-opus-5-5 | 0k | 194k | 2,210k | 2k | 475k | 2% |
| triage | 1 | claude-sonnet-5-5 | 0k | 160k | 2,499k | 4k | 470k | 2% |
| arch-reviewer | 2 | claude-sonnet-5-5 | 0k | 77k | 379k | 1k | 137k | 1% |
| refactor-advisor | 2 | claude-sonnet-5-5 | 0k | 82k | 250k | 0k | 130k | 1% |
| **subagent total** | 97 | | 3k | 8,063k | 112,861k | 116k | 21,946k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 1k | 2,063k | 176,063k | 184k | 22,653k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 9 | 1,577k | 7% |
| plan | 9 | 2,572k | 12% |
| build | 46 | 11,281k | 51% |
| checkpoint | 14 | 1,913k | 9% |
| checkpoint-fix | 6 | 848k | 4% |
| review | 10 | 2,265k | 10% |
| gate-fix | 3 | 1,488k | 7% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 0 | 0 | 0 | 0 | 3 | 5,209k | 24% |
| SCENARIO-01 | 1 | 4 | 1 | 0 | 0 | 1,613k | 7% |
| SCENARIO-06 | 1 | 4 | 1 | 1 | 0 | 1,398k | 6% |
| SCENARIO-09 | 1 | 4 | 1 | 1 | 0 | 1,536k | 7% |
| SCENARIO-10 | 0 | 2 | 1 | 1 | 0 | 725k | 3% |
| SCENARIO-11 | 1 | 4 | 1 | 1 | 0 | 1,342k | 6% |
| SCENARIO-12 | 1 | 4 | 1 | 0 | 0 | 1,473k | 7% |
| SCENARIO-13 | 1 | 4 | 1 | 1 | 0 | 1,231k | 6% |
| SCENARIO-14 | 1 | 4 | 1 | 1 | 0 | 1,461k | 7% |
| SCENARIO-16 | 0 | 2 | 1 | 0 | 0 | 373k | 2% |
| SCENARIO-17 | 0 | 2 | 1 | 0 | 0 | 813k | 4% |
| SCENARIO-20 | 1 | 4 | 1 | 0 | 0 | 2,445k | 11% |
| SCENARIO-22 | 0 | 2 | 1 | 0 | 0 | 488k | 2% |
| SCENARIO-26 | 0 | 2 | 1 | 0 | 0 | 384k | 2% |
| SCENARIO-27 | 1 | 4 | 1 | 0 | 0 | 1,456k | 7% |

Developer runs: 55; weighted per run median 213k, p90 419k, max 1,162k.

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
