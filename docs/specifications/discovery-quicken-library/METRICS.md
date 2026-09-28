# Metrics: discovery-quicken-library

## Scenarios
| Scenario | Cadence | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- |
| SCENARIO-01 (+02..05 folded) | code-first | 0/0/0/0 | no |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Verdict |
| --- | --- | --- | --- |
| 1 | arch, correctness, test, refactor | 0/1/9/3 | FAIL |
| 2 | correctness | 0/0/1/0 | PASS WITH FOLLOW-UPS |
| PV final | product-vision | — | SHIP |

## Tokens
Subagent tokens for `discovery-quicken-library` across 4 project dir(s). Main-thread (orchestrator) tokens not included.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output |
| --- | --- | --- | --- | --- | --- | --- |
| developer | 2 | claude-sonnet-5 | 0k | 337k | 30,590k | 13k |
| test-reviewer | 1 | claude-sonnet-5 | 0k | 101k | 2,900k | 2k |
| correctness-reviewer | 2 | claude-opus-5-5 | 0k | 151k | 2,604k | 2k |
| architect | 1 | claude-sonnet-5 | 0k | 101k | 1,805k | 2k |
| product-vision | 1 | claude-opus-5-5 | 0k | 46k | 231k | 1k |
| **total** | 7 | | 1k | 735k | 38,129k | 20k |

| Scenario | Architect runs | Developer runs | Cache read | Output |
| --- | --- | --- | --- | --- |
| SCENARIO-01 | 1 | 1 | 21,429k | 12k |
| fix pass | 0 | 1 | 10,966k | 3k |
