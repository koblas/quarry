# Metrics: version-command

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 (+04, 05 folded) | code-first | A, B1, V | 0/1/4/1 | folded into V |
| SCENARIO-02 | code-first (light) | L, V | 0/0/1/1 | folded into V |
| SCENARIO-03 | code-first (light) | L, V | 0/0/3/0 | folded into V |
| SCENARIO-06 | code-first (light) | L, V | 0/0/1/0 | folded into V |
| SCENARIO-07 | code-first (light) | L, V | 0/0/3/0 | folded into V |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / timed out / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, refactor, test | 0/0/5/3 | 1 / 0 / 15, 469s | PASS WITH FOLLOW-UPS |

## Tokens
Tokens for `version-command` across 10 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 27 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 11 | claude-sonnet-5-5 | 0k | 1,007k | 12,595k | 116k | 3,098k | 58% |
| test-reviewer | 6 | claude-sonnet-5-5 | 0k | 366k | 2,053k | 48k | 905k | 17% |
| product-vision | 4 | claude-opus-5-5 | 0k | 155k | 929k | 25k | 411k | 8% |
| architect | 2 | claude-sonnet-5-5 | 0k | 142k | 717k | 24k | 368k | 7% |
| correctness-reviewer | 1 | claude-opus-5-5 | 0k | 83k | 1,167k | 19k | 316k | 6% |
| triage | 1 | claude-sonnet-5-5 | 0k | 57k | 304k | 7k | 139k | 3% |
| refactor-advisor | 1 | claude-sonnet-5-5 | 0k | 35k | 91k | 3k | 66k | 1% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 35k | 58k | 2k | 58k | 1% |
| **subagent total** | 27 | | 1k | 1,880k | 17,915k | 244k | 5,361k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 0k | 157k | 45,083k | 58k | 5,113k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 5 | 550k | 10% |
| plan | 2 | 368k | 7% |
| build | 11 | 3,098k | 58% |
| checkpoint | 5 | 749k | 14% |
| review | 4 | 597k | 11% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 1 | 0 | 0 | 0 | 0 | 1,292k | 24% |
| SCENARIO-01 | 1 | 3 | 1 | 0 | 0 | 1,550k | 29% |
| SCENARIO-02 | 0 | 2 | 1 | 0 | 0 | 536k | 10% |
| SCENARIO-03 | 0 | 2 | 1 | 0 | 0 | 669k | 12% |
| SCENARIO-06 | 0 | 2 | 1 | 0 | 0 | 657k | 12% |
| SCENARIO-07 | 0 | 2 | 1 | 0 | 0 | 658k | 12% |

Developer runs: 11; weighted per run median 290k, p90 376k, max 485k.

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
