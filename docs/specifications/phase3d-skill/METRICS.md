# Metrics: phase3d-skill

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 (+07) | code-first | A, B1, V | 0/2/3/2 | yes (plan steps 6-8 unticked; §S.9 notices/PRD strings unpinned; MINOR folds: ticks control, rates.last type, §10 link count) |
| SCENARIO-02 | code-first (light) | L, V | 0/0/4/0 | no (MINORs → STATE.md) |
| SCENARIO-04 (+05) | code-first | A, B1, B2, V | 0/2/1/1 | yes (shipped income test clock-dependent from 2027; shipped category/currency/income params unpinned; opening-line scanner controls) |
| SCENARIO-06 | code-first (light) | L, V | 0/0/2/2 | no (MINORs → STATE.md); acceptance green on arrival, per-row mutation proof |
| SCENARIO-03 | code-first | A, B1, B2, V | 0/1/4/1 | yes (duplicate/uncategorized type pins were bare substrings; MINOR folds: reference-file link base controls, no-MCP-tool pin, two comment budgets) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, test, refactor | 0/3/23/13 | skipped (no production Go) | BLOCKED |
| 2 | correctness, test | 0/0/1/2 | skipped (no production Go) | PASS WITH FOLLOW-UPS |
| final pass | product-vision | 0/3/0/1 | - | SHIP WITH CHANGES |
| 3 | correctness, test | 0/2/0/2 (1 MAJOR rejected) | skipped (no production Go) | BLOCKED |
| 4 | correctness, test | 0/0/1/0 | skipped (no production Go) | PASS WITH FOLLOW-UPS |

## Tokens

Tokens for `phase3d-skill` across 6 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 44 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 21 | claude-sonnet-5-5 | 1k | 1,813k | 28,118k | 10k | 5,131k | 55% |
| test-reviewer | 9 | claude-sonnet-5-5 | 0k | 644k | 5,159k | 6k | 1,351k | 14% |
| architect | 4 | claude-opus-5-5, claude-sonnet-5-5 | 0k | 442k | 5,671k | 2k | 1,130k | 12% |
| correctness-reviewer | 4 | claude-opus-5-5 | 0k | 330k | 6,676k | 1k | 1,085k | 12% |
| product-vision | 3 | claude-opus-5-5 | 0k | 205k | 2,083k | 1k | 470k | 5% |
| triage | 1 | claude-sonnet-5-5 | 0k | 48k | 146k | 0k | 74k | 1% |
| refactor-advisor | 1 | claude-sonnet-5-5 | 0k | 40k | 214k | 0k | 72k | 1% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 33k | 80k | 0k | 51k | 1% |
| **subagent total** | 44 | | 1k | 3,554k | 48,148k | 21k | 9,364k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 0k | 236k | 20,519k | 79k | 2,920k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 4 | 544k | 6% |
| plan | 4 | 1,130k | 12% |
| build | 15 | 3,602k | 38% |
| checkpoint | 5 | 609k | 7% |
| checkpoint-fix | 3 | 483k | 5% |
| review | 10 | 1,950k | 21% |
| gate-fix | 3 | 1,046k | 11% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 1 | 0 | 0 | 0 | 3 | 3,802k | 41% |
| SCENARIO-01 | 1 | 3 | 1 | 1 | 0 | 957k | 10% |
| SCENARIO-02 | 0 | 2 | 1 | 0 | 0 | 480k | 5% |
| SCENARIO-03 | 1 | 4 | 1 | 1 | 0 | 1,795k | 19% |
| SCENARIO-04 | 1 | 4 | 1 | 1 | 0 | 1,770k | 19% |
| SCENARIO-06 | 0 | 2 | 1 | 0 | 0 | 559k | 6% |

Developer runs: 21; weighted per run median 208k, p90 423k, max 587k.

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| final pass | SKILL.md §1 renders `null` for dates.last / rates.last | plugin/skills/quarry/SKILL.md:15,17 | SCENARIO-01 |
| final pass | README claims quarry sends nothing anywhere | README.md:23 | SCENARIO-01 |
| final pass | recipe currency not tied to reporting currency | plugin/skills/quarry/references/spending.md:45 | SCENARIO-03 |
| gate R3 | README privacy line still false (rate fetch start_date) | README.md:23 | SCENARIO-01 |
| gate R1 | recipe currency ELSE arm mislabels non-CAD values as converted USD | plugin/skills/quarry/references/sql/spending-trend.sql:16 | SCENARIO-04 |
| gate R1 | trend guidance lacks partial-period caveat | plugin/skills/quarry/references/spending.md:48 | SCENARIO-03 |
| gate R1 | P4 recipe pins driven by hand-kept maps, not disk | cmd/quarry/run_skill_recipes_test.go:487 | SCENARIO-04 |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
