# Metrics: core-data-entity-renumbering

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-02 (+01, 04, 05, 06 folded) | code-first | A, B1, B2, V | 0/1/5/2 | folded into V |
| SCENARIO-03 (+07 folded) | code-first | A, B1, V | 0/1/2/0 | folded into V |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / timed out / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, refactor, test | 1/0/7/5 | 2 / 0 / 20, 851s | BLOCKED |
| 2 | correctness, test, refactor | 0/0/2/4 (1 MAJOR downgraded by ruling) | 1 / 0 / 7, 563s | PASS WITH FOLLOW-UPS |

## Tokens
Tokens for `core-data-entity-renumbering` across 10 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 25 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 8 | claude-sonnet-5-5 | 0k | 1,024k | 22,959k | 172k | 4,436k | 54% |
| architect | 3 | claude-opus-5-5, claude-sonnet-5-5 | 0k | 343k | 3,399k | 77k | 1,153k | 14% |
| correctness-reviewer | 2 | claude-opus-5-5 | 0k | 185k | 3,959k | 55k | 903k | 11% |
| test-reviewer | 4 | claude-sonnet-5-5 | 0k | 321k | 2,167k | 51k | 875k | 11% |
| product-vision | 4 | claude-opus-5-5 | 0k | 198k | 939k | 23k | 457k | 6% |
| triage | 1 | claude-sonnet-5-5 | 0k | 81k | 483k | 10k | 198k | 2% |
| refactor-advisor | 2 | claude-sonnet-5-5 | 0k | 83k | 112k | 6k | 144k | 2% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 43k | 91k | 3k | 76k | 1% |
| **subagent total** | 25 | | 1k | 2,278k | 34,108k | 397k | 8,243k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 0k | 266k | 20,420k | 61k | 2,881k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 6 | 969k | 12% |
| plan | 2 | 840k | 10% |
| build | 7 | 3,421k | 42% |
| checkpoint | 2 | 528k | 6% |
| review | 7 | 1,470k | 18% |
| gate-fix | 1 | 1,015k | 12% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 0 | 0 | 0 | 0 | 1 | 3,454k | 42% |
| SCENARIO-02 | 1 | 4 | 1 | 0 | 0 | 3,331k | 40% |
| SCENARIO-03 | 1 | 3 | 1 | 0 | 0 | 1,458k | 18% |

Developer runs: 8; weighted per run median 531k, p90 724k, max 1,015k.

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| gate R1 | importer keeps last Z_ENT for duplicated Z_NAME → silent empty import (pre-existing, rewritten fn) | internal/importer/entities.go:50 | SCENARIO-03 |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
