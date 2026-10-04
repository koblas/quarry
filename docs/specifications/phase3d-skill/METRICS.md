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

## Tokens

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| gate R1 | recipe currency ELSE arm mislabels non-CAD values as converted USD | plugin/skills/quarry/references/sql/spending-trend.sql:16 | SCENARIO-04 |
| gate R1 | trend guidance lacks partial-period caveat | plugin/skills/quarry/references/spending.md:48 | SCENARIO-03 |
| gate R1 | P4 recipe pins driven by hand-kept maps, not disk | cmd/quarry/run_skill_recipes_test.go:487 | SCENARIO-04 |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
