# Metrics: parentless-split

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 (+02, 03, 04, 05) | code-first | A, B1, B2, V | 0/0/1/1 | no (MINOR/NIT → STATE.md) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, test, refactor | 0/0/6/1 | 0 / 2, 31s | PASS WITH FOLLOW-UPS |

## Tokens
Tokens for `parentless-split` across 6 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 13 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 4 | claude-sonnet-5-5 | 0k | 309k | 2,790k | 6k | 694k | 43% |
| architect | 2 | claude-sonnet-5-5 | 0k | 156k | 1,242k | 3k | 335k | 21% |
| test-reviewer | 2 | claude-sonnet-5-5 | 0k | 101k | 420k | 0k | 168k | 11% |
| product-vision | 2 | claude-opus-5-5 | 0k | 97k | 351k | 2k | 166k | 10% |
| correctness-reviewer | 1 | claude-opus-5-5 | 0k | 66k | 495k | 1k | 137k | 9% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 32k | 113k | 1k | 54k | 3% |
| refactor-advisor | 1 | claude-sonnet-5-5 | 0k | 29k | 83k | 0k | 45k | 3% |
| **subagent total** | 13 | | 0k | 789k | 5,494k | 12k | 1,599k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 0k | 56k | 5,188k | 18k | 722k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 2 | 166k | 10% |
| plan | 2 | 335k | 21% |
| build | 4 | 694k | 43% |
| checkpoint | 1 | 100k | 6% |
| review | 4 | 305k | 19% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 1 | 0 | 0 | 0 | 0 | 630k | 39% |
| SCENARIO-01 | 1 | 4 | 1 | 0 | 0 | 968k | 61% |

Developer runs: 4; weighted per run median 174k, p90 203k, max 203k.

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
