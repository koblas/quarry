# Metrics: real-money-float-noise

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 (+02, 03, 04, 05) | test-first | A, B1, B2, V | 0/1/3/0 | yes |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, test, refactor | 0/0/7/1 | 1 / 9, 110s | PASS WITH FOLLOW-UPS |

## Tokens
Tokens for `real-money-float-noise` across 6 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 15 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 5 | claude-sonnet-5-5 | 0k | 370k | 4,704k | 8k | 975k | 52% |
| architect | 2 | claude-sonnet-5-5 | 0k | 122k | 729k | 1k | 229k | 12% |
| test-reviewer | 2 | claude-sonnet-5-5 | 0k | 101k | 574k | 1k | 189k | 10% |
| product-vision | 2 | claude-opus-5-5 | 0k | 90k | 247k | 2k | 149k | 8% |
| triage | 1 | claude-sonnet-5-5 | 0k | 68k | 565k | 1k | 146k | 8% |
| correctness-reviewer | 1 | claude-opus-5-5 | 0k | 59k | 291k | 1k | 107k | 6% |
| refactor-advisor | 1 | claude-sonnet-5-5 | 0k | 30k | 49k | 0k | 42k | 2% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 28k | 49k | 0k | 40k | 2% |
| **subagent total** | 15 | | 0k | 868k | 7,208k | 14k | 1,878k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 0k | 80k | 4,306k | 24k | 710k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 3 | 295k | 16% |
| plan | 2 | 229k | 12% |
| build | 4 | 751k | 40% |
| checkpoint | 1 | 109k | 6% |
| checkpoint-fix | 1 | 224k | 12% |
| review | 4 | 269k | 14% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 1 | 0 | 0 | 0 | 0 | 639k | 34% |
| SCENARIO-01 | 1 | 4 | 1 | 1 | 0 | 1,239k | 66% |

Developer runs: 5; weighted per run median 181k, p90 224k, max 224k.

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
