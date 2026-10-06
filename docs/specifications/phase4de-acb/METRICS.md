# Metrics: phase4de-acb

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 | code-first | A, B1, B2, V | 0/1/2/0 | yes (exact case-sensitive match unpinned; folds: doc wrap, --json warnings half) |
| SCENARIO-02 | code-first | A, B1, B2, V | 0/0/4/1 | no (MINOR → STATE.md) |
| SCENARIO-04 | code-first | A, B1, B2, V | 0/1/2/1 | yes (findings.md --csv mirror stale — ruled; folds: test comment, --json cmd pin) |
| SCENARIO-05 (+03) | code-first | A, B1, B2, V (+ product-vision copy ruling: unreadable-config warning) | 0/1/3/0 | yes (warning order unpinned at findings/accounts/MCP; folds: ReadTimeStates unit test, dead field copies dropped, doc trim) |
| SCENARIO-06 | code-first | A, B1, B2, V (+ P1/P2 probes; product-vision rulings: same-day order, cost refusal; user: S10 rewrite) | 0/0/2/2 | no (MINOR → STATE.md) |
| SCENARIO-07 | code-first | A, B1, B2, V | 0/1/0/0 | yes (refusal check-order precedence cells unpinned) |
| SCENARIO-08a (+09) | code-first | A, B1, B2, B3, V | 1/4/4/0 | yes (BLOCKER found by orchestrator real-file sign probe: sells stored negative, walk + all fixtures assumed positive; MAJORs: cross-security sale order, reinvest tier, USD proceeds grouping, empty-pool guard; folds: orphan note, markers, doc trims) |
| SCENARIO-08b | code-first | A, B1, B2, V (+ product-vision ruling: years JSON key) | 0/2/1/1 | yes (break-even realized sale; fractional shares text cell; folds: fixture doc, STATE debt) |
| SCENARIO-10 | code-first (light, Gherkin rewritten after P2a) | L, V | 0/3/1/0 | yes (zero-unit moves — orchestrator ruling, no event/no warning; tier cells; config×warning-5 order; fold: test doc) |
| SCENARIO-11 | code-first | A, B1, B2, V (+ product-vision ruling: ROC-above-ACB year totals + RD-before-ROC; orchestrator rulings: 5 plan defaults) | 0/1/3/0 | yes (ROC-excess event JSON unpinned; folds: 3 comment trims) |
| SCENARIO-12 | code-first | A, B1, V (+ orchestrator rulings: warning 3 years, per-row "other than shares sold", 6 edges) | 0/2/5/0 | yes (non-walked action counted as acquisition via iota-0 map miss — also in heldAt; remove_shares in heldAt unpinned; folds: vacuous test, redundant sort, 2 doc trims) |
| SCENARIO-13a | code-first (re-sized LIGHT → OWNS A RUN by warning-4 ruling) | A, B1, B2, V (+ product-vision ruling: warning 4 variants + unknown-cost span; orchestrator: reinvest units > 0, slot-4 order) | 0/0/2/1 | no (MINOR → STATE.md) |
| SCENARIO-13b | code-first | A, B1, B2, V (+ product-vision rulings: finding copy gaps; findings Long opening + MCP status description) | 0/1/3/1 | yes (findings Long + MCP status said read-time types get marked fixed; folds: CountFindings empty-classification row, ignored-count cmd row, split assert, plan test names) |
| SCENARIO-14 | code-first | A, B1, B2, V (+ product-vision ruling: warning 6 variants 6a/6b/6c, null cad/gain on unvalued event, exclusion permanent) | 0/1/4/2 | yes (sale arm of needsConversion unpinned; folds: ROC-excess arm isolated, no-rate superficial test, doc trims, full --json warnings pin) |
| SCENARIO-15 (+18) | code-first | A, B1, B2, V (+ product-vision ruling: --year shape, warning 2 three forms, Total row; plan lost to orchestrator script truncation and re-written by the same architect via SendMessage) | 0/1/4/0 | yes (injected clock not pinned for --year bound; folds: 3 cmd JSON cells, exact stderr in currency_test, 3 doc trims) |
| SCENARIO-16 | code-first | A, B1, B2, V (+ product-vision ruling: --security P1-P7/D1-D6, JSON unknown_cost, refusal line rewritten) | 0/5/2/0 | yes (nil-ticker guard, sort tiers, Today bound, false unreachable claim, registered-only x --year cell — all unpinned; folds: 2 doc trims) |
| SCENARIO-17 (+20) | code-first | A, B1, B2 (docs), V (+ product-vision ruling RULING-S17.md: R-6 wording, CAD-only --currency line, classify-first + ACB adjustments docs) | 0/2/3/0 | yes (currency/year-before-config precedence unpinned; ruled findings.md bodies + TOML examples unpinned; folds: R-6 flag/no-config rows, countUnclassified unexported, surface fixture classified) |
| SCENARIO-19 | code-first | A, B1, B2, B3, V (+ product-vision ruling RULING-S19.md: tool-worded refusals/warnings, integer year, finding Sentence, dataQualityDescription; plan re-scoped by same architect) | 0/3/1/0 | yes (adjustment-warning absolute path, ROC-only x security cell, ReinvestedDistribution mapping — all unpinned; fold: doc trim) |
| SCENARIO-22 | code-first (appended from SCENARIO-21 reference check) | A, B1, B2, V (+ product-vision ruling RULING-S22.md: short pool, units-only cover, warning 10; orchestrator: <x> = short after, cover-only no-cost keeps UnknownCost) | 0/1/2/1 | yes (covering-buy row pinned only in --json; folds: slot-10 uncut pins, 3 doc trims, phase-report NIT) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 01 | arch, correctness, test, refactor (+ spec-check --run OK) | 0/2/9/≈18 | 0 / 20, 1517s | BLOCKED |
| 02 | correctness, test (re-gate after passes 1-2) | 0/2/2/1 | — | BLOCKED |
| 03 | correctness, test (re-gate after pass 3; cap reached) | 0/4/3/3 | — | BLOCKED by contract; 4 MAJOR test-pin survivors + rest → STATE Open debts per cap |
| final PV | product-vision step 10 | 0/2/0/1 | — | SHIP WITH CHANGES (%q names in acb warnings; SKILL §7) |
| 04 | correctness, test (narrow, after final-PV fix) | 0/0/3/2 | — | PASS WITH FOLLOW-UPS → SHIP |

## Tokens

Tokens for `phase4de-acb` across 7 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 158 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 95 | claude-sonnet-5-5 | 5k | 11,303k | 304,029k | 96k | 45,018k | 71% |
| architect | 20 | claude-opus-5-5, claude-sonnet-5-5 | 1k | 2,819k | 45,833k | 20k | 8,208k | 13% |
| test-reviewer | 23 | claude-sonnet-5-5 | 1k | 2,210k | 34,682k | 27k | 6,367k | 10% |
| product-vision | 14 | claude-opus-5-5 | 0k | 765k | 9,920k | 3k | 1,963k | 3% |
| correctness-reviewer | 4 | claude-opus-5-5 | 0k | 463k | 12,517k | 2k | 1,840k | 3% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 65k | 403k | 0k | 121k | 0% |
| refactor-advisor | 1 | claude-sonnet-5-5 | 0k | 68k | 139k | 0k | 99k | 0% |
| **subagent total** | 158 | | 7k | 17,693k | 407,523k | 148k | 63,616k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 1k | 1,518k | 140,266k | 225k | 18,187k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 13 | 1,071k | 2% |
| plan | 20 | 8,208k | 13% |
| build | 75 | 34,710k | 55% |
| checkpoint | 19 | 3,797k | 6% |
| checkpoint-fix | 16 | 6,038k | 9% |
| review | 11 | 5,522k | 9% |
| gate-fix | 4 | 4,270k | 7% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 1 | 0 | 0 | 0 | 4 | 9,999k | 16% |
| SCENARIO-01 | 1 | 4 | 1 | 1 | 0 | 2,191k | 3% |
| SCENARIO-02 | 1 | 4 | 1 | 0 | 0 | 1,493k | 2% |
| SCENARIO-04 | 1 | 4 | 1 | 1 | 0 | 2,873k | 5% |
| SCENARIO-05 | 2 | 4 | 1 | 1 | 0 | 3,465k | 5% |
| SCENARIO-06 | 1 | 4 | 1 | 0 | 0 | 1,923k | 3% |
| SCENARIO-07 | 1 | 4 | 1 | 1 | 0 | 1,862k | 3% |
| SCENARIO-08a | 1 | 5 | 1 | 1 | 0 | 3,863k | 6% |
| SCENARIO-08b | 1 | 4 | 1 | 1 | 0 | 3,355k | 5% |
| SCENARIO-10 | 0 | 2 | 1 | 1 | 0 | 1,816k | 3% |
| SCENARIO-11 | 1 | 4 | 1 | 1 | 0 | 2,547k | 4% |
| SCENARIO-12 | 1 | 3 | 1 | 1 | 0 | 2,200k | 3% |
| SCENARIO-13a | 1 | 4 | 1 | 0 | 0 | 1,939k | 3% |
| SCENARIO-13b | 1 | 4 | 1 | 1 | 0 | 3,450k | 5% |
| SCENARIO-14 | 1 | 4 | 1 | 1 | 0 | 3,111k | 5% |
| SCENARIO-15 | 1 | 4 | 1 | 1 | 0 | 3,229k | 5% |
| SCENARIO-16 | 1 | 4 | 1 | 1 | 0 | 3,543k | 6% |
| SCENARIO-17 | 1 | 4 | 1 | 1 | 0 | 3,538k | 6% |
| SCENARIO-19 | 1 | 5 | 1 | 1 | 0 | 4,675k | 7% |
| SCENARIO-22 | 1 | 4 | 1 | 1 | 0 | 2,543k | 4% |

Developer runs: 95; weighted per run median 331k, p90 915k, max 2,195k.

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
