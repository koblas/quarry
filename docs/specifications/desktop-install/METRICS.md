# Metrics: desktop-install

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 | test-first | A, B1, B2, V | 0/2/4/2 | folded into V |
| SCENARIO-02 | test-first | A, B1, B2, V | 0/0/4/1 | folded into V |
| SCENARIO-04 | test-first | A, B1, B2, V | 0/1/2/1 | folded into V |
| SCENARIO-06 | test-first | A, B1, B2, V | 0/2/3/0 | folded into V |
| SCENARIO-08 | code-first | A, B1, B2, V | 0/3/2/1 | folded into V |
| SCENARIO-13 | test-first | A, B1, B2, V (+1 checkpoint fix) | 0/2/2/0 | yes |
| SCENARIO-09 | code-first | A, B1, B2, V | 0/0/3/2 | folded into V |
| SCENARIO-11 | code-first | A, B1, B2, V | 0/1/3/1 | folded into V |
| SCENARIO-16 | code-first | A, B1, B2, V | 0/0/0/2 | folded into V |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, test, refactor | 0/1/17/7 | 0 / 20 (2 killed, 18 timed out), 2589s; reviewer export 1 / 29 survived | BLOCKED |
| 2 | correctness, test, refactor (re-gate d24a598f..c4e46c58) | 0/0/0/4 | 0 / 13 (13 timed out), 1829s | PASS WITH FOLLOW-UPS |
| 3 | test (re-gate 73d3cd90..34438e81, after final-pass README fix) | 0/0/0/0 | n/a (no production Go) | PASS |

## Tokens
Tokens for `desktop-install` across 9 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 69 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 39 | claude-sonnet-5-5 | 1k | 4,794k | 55,331k | 35k | 11,700k | 65% |
| test-reviewer | 12 | claude-sonnet-5-5 | 0k | 1,089k | 11,760k | 10k | 2,589k | 14% |
| architect | 10 | claude-opus-5-5, claude-sonnet-5-5 | 0k | 1,164k | 10,655k | 9k | 2,567k | 14% |
| product-vision | 2 | claude-opus-5-5 | 0k | 363k | 852k | 0k | 541k | 3% |
| correctness-reviewer | 2 | claude-opus-5-5 | 0k | 143k | 1,376k | 0k | 318k | 2% |
| refactor-advisor | 2 | claude-sonnet-5-5 | 0k | 96k | 210k | 6k | 170k | 1% |
| triage | 1 | claude-sonnet-5-5 | 0k | 61k | 375k | 1k | 117k | 1% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 38k | 118k | 0k | 59k | 0% |
| **subagent total** | 69 | | 2k | 7,747k | 80,678k | 62k | 18,061k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 0k | 336k | 34,474k | 100k | 4,622k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 3 | 658k | 4% |
| plan | 10 | 2,567k | 14% |
| build | 36 | 10,478k | 58% |
| checkpoint | 9 | 1,610k | 9% |
| checkpoint-fix | 1 | 344k | 2% |
| review | 8 | 1,525k | 8% |
| gate-fix | 2 | 878k | 5% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 1 | 0 | 0 | 0 | 2 | 3,254k | 18% |
| SCENARIO-01 | 1 | 4 | 1 | 0 | 0 | 1,711k | 9% |
| SCENARIO-02 | 1 | 4 | 1 | 0 | 0 | 1,529k | 8% |
| SCENARIO-04 | 1 | 4 | 1 | 0 | 0 | 1,145k | 6% |
| SCENARIO-06 | 1 | 4 | 1 | 0 | 0 | 1,611k | 9% |
| SCENARIO-08 | 1 | 4 | 1 | 0 | 0 | 1,498k | 8% |
| SCENARIO-09 | 1 | 4 | 1 | 0 | 0 | 1,774k | 10% |
| SCENARIO-11 | 1 | 4 | 1 | 0 | 0 | 2,034k | 11% |
| SCENARIO-13 | 1 | 4 | 1 | 1 | 0 | 2,510k | 14% |
| SCENARIO-16 | 1 | 4 | 1 | 0 | 0 | 994k | 6% |

Developer runs: 39; weighted per run median 307k, p90 387k, max 737k.

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| final pass | README Desktop paragraph omits uninstall backup + quit-first (contradicts uninstall Long) | README.md (Desktop paragraph) | SCENARIO-16 |
| gate R1 | D9 temp check's `exe` half unpinned (mutant drops `exe` from the slice, survives) | internal/claudedesktop/claudedesktop.go:263 | SCENARIO-08 |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
