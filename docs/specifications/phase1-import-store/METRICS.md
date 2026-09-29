# Metrics: phase1-import-store

## Scenarios
| Scenario | Cadence | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- |
| PREP-c | test-first (2 guards built code-first, mutation-verified after) | 1/13/8/0 | yes |
| SCENARIO-01a (+04, 05) | test-first | 0/1/7/0 | yes (+ P1-5d, reason 11 rulings) |
| SCENARIO-01d | code-first | 0/0/2/0 | no (MINORs → STATE.md) |
| SCENARIO-01b (+06) | test-first (guard batch built with tests; 2 of 4 named mutations overstated, fixed) | 3/1/21/0 | yes |
| SCENARIO-09 (+11) | code-first | 0/1/15/0 | yes |
| SCENARIO-01c (+07, 12, 21) | code-first | 0/1/4/0 | yes |
| SCENARIO-08 (+19) | code-first | 0/0/6/0 | no (MINORs → STATE.md) |
| SCENARIO-02 (+10, 18) | code-first | 0/1/4/0 | yes |
| SCENARIO-14 (+13) | test-first | 0/0/3/1 | no (MINORs → STATE.md) |
| SCENARIO-20 | test-first | 0/2/2/1 | yes |
| SCENARIO-03 (+16) | code-first | 3/1/0/0 | yes |
| SCENARIO-15 (+17) | code-first (acceptance test written after code; red reconstructed) | 0/3/7/0 | yes |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Verdict |
| --- | --- | --- | --- |
| 1 | arch, correctness, test, refactor + spec-check | 1/6/14/5 | FAIL |
| 2 | correctness, test, arch, refactor (re-gate) | 0/1/10/3 | FAIL |
| 3 | correctness, test, refactor (re-gate) | 0/1/0/2 | FAIL |
| 4 | test (re-gate) | 0/0/1/0 | PASS WITH FOLLOW-UPS |

## Tokens
Subagent tokens for `phase1-import-store` across 6 project dir(s). Main-thread (orchestrator) tokens not included.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output |
| --- | --- | --- | --- | --- | --- | --- |
| developer | 14 | claude-opus-5-5, claude-sonnet-5 | 4k | 8,308k | 575,364k | 296k |
| test-reviewer | 15 | claude-sonnet-5 | 2k | 2,082k | 73,219k | 50k |
| architect | 12 | claude-opus-5-5, claude-sonnet-5 | 1k | 1,636k | 32,441k | 30k |
| product-vision | 1 | claude-opus-5-5 | 0k | 1,064k | 1,939k | 10k |
| correctness-reviewer | 2 | claude-opus-5-5 | 0k | 176k | 2,402k | 2k |
| arch-reviewer | 1 | claude-sonnet-5 | 0k | 80k | 1,388k | 2k |
| refactor-advisor | 2 | claude-sonnet-5 | 0k | 150k | 1,155k | 5k |
| **total** | 47 | | 7k | 13,497k | 687,908k | 394k |

| Scenario | Architect runs | Developer runs | Cache read | Output |
| --- | --- | --- | --- | --- |
| SCENARIO-01a | 2 | 2 | 220,872k | 67k |
| SCENARIO-01b | 1 | 1 | 85,875k | 64k |
| SCENARIO-01c | 1 | 1 | 14,637k | 11k |
| SCENARIO-01d | 1 | 1 | 46,229k | 24k |
| SCENARIO-02 | 1 | 1 | 25,875k | 45k |
| SCENARIO-03 | 1 | 1 | 10,136k | 4k |
| SCENARIO-08 | 1 | 1 | 13,456k | 7k |
| SCENARIO-09 | 1 | 1 | 46,106k | 46k |
| SCENARIO-14 | 1 | 1 | 9,374k | 3k |
| SCENARIO-15 | 1 | 1 | 71,584k | 35k |
| SCENARIO-20 | 1 | 1 | 25,974k | 12k |
| fix pass | 0 | 1 | 34,085k | 4k |
| unattributed | 0 | 1 | 3,601k | 3k |
