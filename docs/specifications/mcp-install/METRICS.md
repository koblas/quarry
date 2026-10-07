# Metrics: mcp-install

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 | code-first | A, B1, B2, V | 0/4/3/0 | yes |
| SCENARIO-04 | code-first | A, B1, B2, V | 0/3/5/0 | folded into V |
| SCENARIO-06 | code-first | A, B1, V | 0/1/2/0 | folded into V |
| SCENARIO-10 | test-first (deletion guard) | A, B1, V | 0/1/4/0 | folded into V |
| SCENARIO-12 | test-first (deletion guard) | A, B1, B2, V | 0/1/2/0 | folded into V |
| SCENARIO-17 | code-first | A, B1, V | 0/0/1/4 | folded into V |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, test, refactor | 0/2/10/5 | 0 / 20 (7 killed, 13 timed out), 2249s | BLOCKED |
| 2 | correctness, test (re-gate d8a9a171..964f5ddd) | 0/1/3/2 | n/a (re-gate; developer mutations in REVIEW-02) | BLOCKED |
| 3 | test (re-gate 37fa483c..9dabaaf1) | 0/0/0/0 | 4 / 4 killed by reviewer on export | PASS |

## Tokens
<output of `.claude/scripts/feature-metrics.py --strict mcp-install`, pasted once at SHIP>

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| gate R1 | lists decoded from combined stdout+stderr buffer; stderr noise breaks idempotency check | internal/claudeplugin/state.go:396 | SCENARIO-01 (seam ruled at scoping; SCENARIO-04 built adapter) |
| gate R1 | toolrun.Run output unpinned on exit/signal/cancel returns | internal/platform/toolrun/toolrun_test.go:106 | SCENARIO-04 |
| gate R2 | signal/cancel pins cannot tell combined from stdout | internal/platform/toolrun/toolrun_test.go:241,268 | gate-fix 1 |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
