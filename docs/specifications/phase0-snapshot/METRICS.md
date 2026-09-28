# Metrics: phase0-snapshot

## Scenarios
| Scenario | Cadence | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- |
| SCENARIO-01a (+03 folded) | test-first | 1/4/3/0 | yes |
| SCENARIO-01b (+02, +14 folded) | test-first | 0/3/0/0 | yes |
| SCENARIO-05 | code-first | 1/2/0/0 | yes |
| SCENARIO-04 | code-first | 0/2/0/0 | yes |
| SCENARIO-06 (+07 folded) | test-first | 0/2/0/0 | yes |
| SCENARIO-08 | test-first | 0/1/0/0 | yes |
| SCENARIO-10 (+09 folded) | code-first | 0/0/0/0 | no |
| SCENARIO-11 (+12 folded) | test-first | 0/1/1/0 | yes |
| SCENARIO-13 | test-first | 0/1/0/0 | yes |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Verdict |
| --- | --- | --- | --- |
| 1 | arch, correctness, test, refactor | 0/4/27/9 | FAIL |
| 2 | arch, correctness, test, refactor | 0/1/9/3 | FAIL |
| 3 | correctness, test | 0/1/3/1 | FAIL |
| 4 | correctness | 0/0/0/2 | PASS WITH FOLLOW-UPS |
| 5 (after PV SHIP WITH CHANGES) | correctness, test, arch | 1/2/12/1 | FAIL |
| 6 | correctness, test | 0/0/0/0 | PASS |
| PV recheck | product-vision | — | SHIP |

## Tokens
Subagent tokens for `phase0-snapshot` across 2 project dir(s). Main-thread (orchestrator) tokens not included.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output |
| --- | --- | --- | --- | --- | --- | --- |
| developer | 14 | claude-sonnet-5 | 5k | 4,649k | 564,170k | 281k |
| test-reviewer | 13 | claude-sonnet-5 | 1k | 1,472k | 35,635k | 59k |
| architect | 9 | claude-opus-5-5, claude-sonnet-5 | 1k | 1,970k | 31,644k | 103k |
| correctness-reviewer | 5 | claude-opus-5-5 | 0k | 430k | 8,121k | 12k |
| refactor-advisor | 1 | claude-sonnet-5 | 0k | 107k | 2,121k | 7k |
| arch-reviewer | 1 | claude-sonnet-5 | 0k | 117k | 2,076k | 4k |
| product-vision | 2 | claude-opus-5-5 | 0k | 119k | 1,472k | 3k |
| **total** | 45 | | 7k | 8,864k | 645,240k | 470k |

| Scenario | Architect runs | Developer runs | Cache read | Output |
| --- | --- | --- | --- | --- |
| SCENARIO-01a | 1 | 1 | 90,893k | 31k |
| SCENARIO-01b | 1 | 1 | 30,660k | 38k |
| SCENARIO-04 | 1 | 1 | 11,648k | 17k |
| SCENARIO-05 | 1 | 1 | 16,910k | 11k |
| SCENARIO-06 | 1 | 1 | 54,304k | 36k |
| SCENARIO-08 | 1 | 1 | 37,775k | 37k |
| SCENARIO-10 | 1 | 1 | 27,813k | 29k |
| SCENARIO-11 | 1 | 1 | 40,924k | 29k |
| SCENARIO-13 | 1 | 1 | 31,376k | 33k |
| fix pass | 0 | 5 | 253,511k | 121k |
