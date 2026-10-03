# Metrics: phase3b-analysis-tools

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 | code-first | A, B1, B2, V | 0/0/2/2 | no (MINORs → STATE.md) |
| SCENARIO-02 (+06, +07, +12) | code-first | A, B1, V | 0/0/2/2 | no (MINORs → STATE.md) |
| SCENARIO-03 | code-first | A, B1, V | 0/0/1/2 | no (MINOR → STATE.md); + standalone fix 2482e84 (QueryRows/QueryTable lost mid-iteration cancel; pre-existing flake) |
| SCENARIO-04 (+05) | code-first | A, B1, V | 0/0/0/2 | no (NITs only, accepted) |
| SCENARIO-09 (+08, +14) | code-first | A, B1, B2, V | 0/1/2/1 | yes (by-default unprovable: silent parse fallback → error-on-miss; warnings non-empty pin; cap order pin) |
| SCENARIO-10 (+10b, +11, +11b) | code-first | A, B1, B2, V | 0/1/0/1 | yes (cap-line-last ordering unpinned in both cap tests; test-only) |
| SCENARIO-13 | code-first | A, B1, V | 0/0/1/1 | no (accepted) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, test, refactor | 0/1/18/12 | 1 / 20, 943s | BLOCKED |
| 2 | correctness, test, refactor | 0/0/0/4 | 0 / 6, 266s | PASS WITH FOLLOW-UPS |
| final product-vision | — | 0/0/2/0 | — | SHIP |

## Tokens
Tokens for `phase3b-analysis-tools` across 6 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 52 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 28 | claude-sonnet-5-5 | 1k | 2,601k | 43,855k | 25k | 7,761k | 59% |
| architect | 8 | claude-opus-5-5, claude-sonnet-5-5 | 0k | 944k | 10,612k | 3k | 2,256k | 17% |
| test-reviewer | 9 | claude-sonnet-5-5 | 0k | 820k | 8,039k | 5k | 1,853k | 14% |
| correctness-reviewer | 2 | claude-opus-5-5 | 0k | 197k | 2,886k | 1k | 543k | 4% |
| product-vision | 2 | claude-opus-5-5 | 0k | 187k | 2,062k | 1k | 447k | 3% |
| refactor-advisor | 2 | claude-sonnet-5-5 | 0k | 93k | 249k | 0k | 142k | 1% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 56k | 162k | 0k | 87k | 1% |
| **subagent total** | 52 | | 2k | 4,898k | 67,864k | 35k | 13,088k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 0k | 236k | 54,892k | 66k | 6,291k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 2 | 447k | 3% |
| plan | 8 | 2,256k | 17% |
| build | 24 | 6,478k | 49% |
| checkpoint | 7 | 1,034k | 8% |
| checkpoint-fix | 2 | 397k | 3% |
| review | 7 | 1,590k | 12% |
| gate-fix | 2 | 886k | 7% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 1 | 0 | 0 | 0 | 2 | 3,182k | 24% |
| SCENARIO-01 | 1 | 4 | 1 | 0 | 0 | 1,819k | 14% |
| SCENARIO-02 | 1 | 3 | 1 | 0 | 0 | 1,590k | 12% |
| SCENARIO-03 | 1 | 3 | 1 | 0 | 0 | 880k | 7% |
| SCENARIO-04 | 1 | 3 | 1 | 0 | 0 | 1,441k | 11% |
| SCENARIO-09 | 1 | 4 | 1 | 1 | 0 | 1,658k | 13% |
| SCENARIO-10 | 1 | 4 | 1 | 1 | 0 | 1,701k | 13% |
| SCENARIO-13 | 1 | 3 | 1 | 0 | 0 | 815k | 6% |

Developer runs: 28; weighted per run median 227k, p90 482k, max 673k.

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| gate R1 | windowRefusal passthrough unpinned (surviving mutant → silent nil success) | internal/mcp/window.go:16 | SCENARIO-03 |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
