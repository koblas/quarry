# Metrics: phase4c-networth

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01a (+02) | code-first | A, B1, B2, V | 0/0/4/1 | no (MINOR → STATE.md) |
| SCENARIO-01b (+05) | code-first | A, B1, B2, V | 0/0/3/1 | no (MINOR → STATE.md) |
| SCENARIO-03 (+04) | code-first (light) | L (PARTIAL: 4 copy lines; ruled SKILL description → S10), L, V | 0/1/4/0 | yes (--account/--json cells for investment rows unpinned; folds: retirement account, misc by category, spend buy control) |
| SCENARIO-06 | code-first | A, B1, B2, V | 0/1/1/0 | yes (cash/balance before first transaction unpinned — coalesce arm; fold: comment trim) |
| SCENARIO-07 (+08) | code-first | A, B1, B2, V | 0/1/3/0 | no (MAJOR was an unrecorded timing result B1 had measured — orchestrator recorded it in STATE; MINOR → STATE.md) |
| SCENARIO-09 (+18) | code-first | A, B1, B2, V | 0/1/2/0 | yes (no investment account with holdings in v_net_worth — sum(cash) mutant survived; folds: both-flags case, plan file damaged by V restored) |
| SCENARIO-10 (+13) | code-first | A, B1 (did 3-7), V | 0/0/5/1 | no (MINOR → STATE.md) |
| SCENARIO-12 | code-first | A, B1, B2, V (+ product-vision copy ruling on history cells) | 0/2/2/0 | yes (same-day since/until control; native zero-balance cell; fold: monthEnd doc) |
| SCENARIO-15 (+11) | code-first | A, B1, V | 0/1/1/0 | yes (as-of × native and refusal × --json cells; fold: seed doc) |
| SCENARIO-14a | code-first | A, B1, B2, V | 0/2/3/1 | yes (investment-type guard on unvalued read; closed account × networth; folds: zero price, two doc trims) |
| SCENARIO-14b | code-first | A, B1, B2, B3, V (+ product-vision copy ruling on USD form / history totals) | 0/1/4/1 | yes (history × --json; folds: USD-reporting history row, 3 doc trims, STATE dedupe) |
| SCENARIO-16 | code-first | A, B1, B2, V (+ product-vision copy ruling: FirstBalance, new copy) | 0/0/4/1 | no (MINOR → STATE.md) |
| SCENARIO-17 | code-first | A, B1, B2, V | 0/1/3/0 | yes (warnings-from-full-report-before-cut unpinned; folds: 2 doc trims) |
| SCENARIO-19 | reference (orchestrator) | — | — | — (no rule gap; first manual compare used a stale snapshot, re-run on the latest matched) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, test, refactor | 1/3/11/6 | 0 / 20 (of 113), 1135s | BLOCKED (kind-6 copy ruled mid-round by product-vision) |
| 2 | correctness, test | 0/0/1/1 | 0 / 7, 661s | PASS WITH FOLLOW-UPS (EUR-account MAJOR ruled MINOR: importer refuses non-CAD/USD accounts) |
| final pass | product-vision | 0/5/1/0 | - | SHIP WITH CHANGES (MCP rate advice wording, 3 conventions/COMMENT sentences, findings.md note) |
| 3 | correctness, test | 0/1/1/1 | 0 mutable lines | BLOCKED (findings.md bullet unpinned) |
| 4 | test | 0/0/0/0 | - | PASS (fix-pass cap reached) |
| final confirmation | product-vision | 0/0/0/0 | - | SHIP |

## Tokens

Tokens for `phase4c-networth` across 7 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 105 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 62 | claude-sonnet-5-5 | 3k | 6,655k | 171,881k | 54k | 25,779k | 71% |
| architect | 13 | claude-opus-5-5, claude-sonnet-5-5 | 1k | 1,625k | 23,435k | 6k | 4,404k | 12% |
| test-reviewer | 17 | claude-sonnet-5-5 | 0k | 1,386k | 16,609k | 7k | 3,429k | 10% |
| correctness-reviewer | 3 | claude-opus-5-5 | 0k | 283k | 8,203k | 3k | 1,192k | 3% |
| product-vision | 7 | claude-opus-5-5 | 0k | 387k | 3,224k | 2k | 814k | 2% |
| triage | 1 | claude-sonnet-5-5 | 0k | 103k | 1,601k | 1k | 294k | 1% |
| refactor-advisor | 1 | claude-sonnet-5-5 | 0k | 60k | 218k | 0k | 97k | 0% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 38k | 141k | 0k | 63k | 0% |
| **subagent total** | 105 | | 4k | 10,538k | 225,312k | 73k | 36,072k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 0k | 1,088k | 85,246k | 179k | 11,594k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 6 | 887k | 2% |
| plan | 13 | 4,404k | 12% |
| build | 50 | 19,022k | 53% |
| checkpoint | 13 | 1,758k | 5% |
| checkpoint-fix | 8 | 1,719k | 5% |
| review | 11 | 3,243k | 9% |
| gate-fix | 4 | 5,038k | 14% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 1 | 0 | 0 | 0 | 4 | 9,158k | 25% |
| SCENARIO-01a | 1 | 4 | 1 | 0 | 0 | 2,143k | 6% |
| SCENARIO-01b | 1 | 4 | 1 | 0 | 0 | 1,604k | 4% |
| SCENARIO-03 | 0 | 3 | 1 | 1 | 0 | 1,109k | 3% |
| SCENARIO-06 | 1 | 4 | 1 | 1 | 0 | 1,988k | 6% |
| SCENARIO-07 | 1 | 4 | 1 | 0 | 0 | 3,141k | 9% |
| SCENARIO-09 | 1 | 4 | 1 | 1 | 0 | 1,690k | 5% |
| SCENARIO-10 | 1 | 3 | 1 | 0 | 0 | 2,643k | 7% |
| SCENARIO-12 | 1 | 4 | 1 | 1 | 0 | 2,146k | 6% |
| SCENARIO-14a | 1 | 4 | 1 | 1 | 0 | 2,882k | 8% |
| SCENARIO-14b | 1 | 5 | 1 | 1 | 0 | 2,404k | 7% |
| SCENARIO-15 | 1 | 3 | 1 | 1 | 0 | 1,567k | 4% |
| SCENARIO-16 | 1 | 4 | 1 | 0 | 0 | 1,603k | 4% |
| SCENARIO-17 | 1 | 4 | 1 | 1 | 0 | 1,996k | 6% |

Developer runs: 62; weighted per run median 282k, p90 693k, max 3,073k.

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| gate R1 | priced CAD/USD holding lacking a rate into its account's currency left out silently | internal/report/document/holdings_left_out.go:30-45 | SCENARIO-14a (false premise that 14b's rate lines covered it) |
| gate R1 | `Store.Accounts` BIGINT cast overflows past 64 bits of cents | internal/store/duckstore/accounts.go:14-15 | SCENARIO-07 |
| gate R1 | negative investment cash unpinned | internal/store/duckstore/balances_daily_view_test.go | SCENARIO-06 |
| gate R1 | anomalies / recurring / spend --by payee readers of investment rows unpinned; N-2 recurring claim false | cmd/quarry | SCENARIO-03 |
| final pass | MCP rate warning named a CLI flag | internal/report/document/networth_rate_warnings.go:21-29 | SCENARIO-17 |
| gate R3 | findings.md bullet unpinned | plugin/skills/quarry/references/findings.md:17 | fix pass 2 |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
