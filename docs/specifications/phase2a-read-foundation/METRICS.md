# Metrics: phase2a-read-foundation

## Scenarios
| Scenario | Cadence | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- |
| SCENARIO-01 | code-first | 0/0/1/2 | no (MINOR/NITs → STATE.md) |
| SCENARIO-02 | code-first | 0/1/5/2 | yes |
| SCENARIO-03 | code-first | 0/0/2/1 | no (MINORs → STATE.md) |
| SCENARIO-04 (+05) | code-first | 0/0/0/2 | no (NIT → STATE.md) |
| SCENARIO-07 (+06) | code-first | 0/1/2/0 | yes |
| SCENARIO-08 | code-first | 0/0/3/1 | no (MINORs → STATE.md) |
| SCENARIO-12 (+13, 14, 20) | test-first | 0/0/3/0 | no (MINORs → STATE.md) |
| SCENARIO-15 (+16, 17) | code-first | 0/1/3/1 | yes |
| SCENARIO-09 (+10) | code-first | 0/0/3/0 | no (MINORs → STATE.md) |
| SCENARIO-18 (+11, 19, 21) | code-first | 0/0/2/1 | yes (+ leading-dash copy ruling) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Verdict |
| --- | --- | --- | --- |
| 1 | arch, correctness, test, refactor | 0/5/18/5 | FAIL |
| 2 | correctness, test, refactor | 0/0/1/3 | PASS WITH FOLLOW-UPS |

## Tokens
Subagent tokens for `phase2a-read-foundation` across 6 project dir(s). Main-thread (orchestrator) tokens not included.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output |
| --- | --- | --- | --- | --- | --- | --- |
| developer | 16 | claude-opus-5-5, claude-sonnet-5-5 | 1k | 2,547k | 90,161k | 22k |
| architect | 11 | claude-opus-5-5, claude-sonnet-5-5 | 0k | 1,234k | 15,969k | 17k |
| test-reviewer | 12 | claude-sonnet-5-5 | 0k | 1,046k | 10,877k | 15k |
| correctness-reviewer | 2 | claude-opus-5-5 | 0k | 228k | 4,499k | 4k |
| product-vision | 11 | claude-opus-5-5 | 0k | 441k | 2,053k | 9k |
| refactor-advisor | 2 | claude-sonnet-5-5 | 0k | 93k | 339k | 1k |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 40k | 185k | 1k |
| **total** | 55 | | 2k | 5,630k | 124,083k | 68k |

| Scenario | Architect runs | Developer runs | Cache read | Output |
| --- | --- | --- | --- | --- |
| SCENARIO-01 | 2 | 1 | 8,724k | 4k |
| SCENARIO-02 | 1 | 1 | 8,939k | 2k |
| SCENARIO-03 | 1 | 1 | 3,248k | 3k |
| SCENARIO-04 | 1 | 1 | 7,702k | 2k |
| SCENARIO-07 | 1 | 1 | 3,850k | 3k |
| SCENARIO-08 | 1 | 1 | 14,136k | 2k |
| SCENARIO-09 | 1 | 1 | 3,489k | 2k |
| SCENARIO-12 | 1 | 1 | 10,608k | 2k |
| SCENARIO-15 | 1 | 1 | 13,020k | 3k |
| SCENARIO-18 | 1 | 1 | 10,163k | 5k |
| fix pass | 0 | 6 | 22,251k | 10k |
