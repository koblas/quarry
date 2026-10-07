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
| 4 | test (re-gate c7713737..4478fa91, after final-pass fix) | 0/1/1/2 | n/a | PASS WITH FOLLOW-UPS (MAJOR pin-only, deferred under fix-pass cap) |

## Tokens
Tokens for `mcp-install` across 9 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 50 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 26 | claude-sonnet-5-5 | 1k | 3,155k | 45,619k | 21k | 8,613k | 66% |
| architect | 7 | claude-opus-5-5, claude-sonnet-5-5 | 0k | 778k | 8,400k | 9k | 1,857k | 14% |
| test-reviewer | 10 | claude-sonnet-5-5 | 0k | 807k | 6,964k | 4k | 1,724k | 13% |
| product-vision | 2 | claude-opus-5-5 | 0k | 266k | 1,135k | 1k | 450k | 3% |
| correctness-reviewer | 2 | claude-opus-5-5 | 0k | 137k | 1,073k | 0k | 280k | 2% |
| triage | 1 | claude-sonnet-5-5 | 0k | 53k | 236k | 1k | 95k | 1% |
| refactor-advisor | 1 | claude-sonnet-5-5 | 0k | 41k | 48k | 0k | 56k | 0% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 28k | 99k | 0k | 45k | 0% |
| **subagent total** | 50 | | 2k | 5,265k | 63,575k | 36k | 13,120k | 100% |
| orchestrator (upper bound) | - | claude-fable-5-1 | 2k | 305k | 28,510k | 93k | 3,925k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 3 | 545k | 4% |
| plan | 7 | 1,857k | 14% |
| build | 21 | 6,839k | 52% |
| checkpoint | 6 | 1,034k | 8% |
| checkpoint-fix | 1 | 158k | 1% |
| review | 8 | 1,072k | 8% |
| gate-fix | 4 | 1,615k | 12% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 1 | 0 | 0 | 0 | 4 | 3,459k | 26% |
| SCENARIO-01 | 1 | 4 | 1 | 1 | 0 | 1,952k | 15% |
| SCENARIO-04 | 1 | 4 | 1 | 0 | 0 | 2,071k | 16% |
| SCENARIO-06 | 1 | 3 | 1 | 0 | 0 | 1,224k | 9% |
| SCENARIO-10 | 1 | 3 | 1 | 0 | 0 | 1,753k | 13% |
| SCENARIO-12 | 1 | 4 | 1 | 0 | 0 | 1,566k | 12% |
| SCENARIO-17 | 1 | 3 | 1 | 0 | 0 | 1,096k | 8% |

Developer runs: 26; weighted per run median 308k, p90 457k, max 1,011k.

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| gate R1 | lists decoded from combined stdout+stderr buffer; stderr noise breaks idempotency check | internal/claudeplugin/state.go:396 | SCENARIO-01 (seam ruled at scoping; SCENARIO-04 built adapter) |
| gate R1 | toolrun.Run output unpinned on exit/signal/cancel returns | internal/platform/toolrun/toolrun_test.go:106 | SCENARIO-04 |
| gate R2 | signal/cancel pins cannot tell combined from stdout | internal/platform/toolrun/toolrun_test.go:241,268 | gate-fix 1 |
| final pass | Kept line says "a single project" while hints name several/any scope | internal/cli/render_claude.go:36 | SCENARIO-12 (copy ruled at scoping, flagged at 12's checkpoint) |
| final pass | R6 hint advises `--scope <scope>` for scopes `claude plugin uninstall` rejects | internal/cli/render_claude.go:124-130 | SCENARIO-12 |
| gate R4 | other-scope hint rows pin only named scopes; allow-list mutant survives; %q unpinned | internal/cli/claude_uninstall_test.go:173-199 | gate-fix 3 (deferred under cap) |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
