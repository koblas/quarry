# Metrics: phase4f-summary

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01a (+02, 19, 03, 20, 04) | code-first | A, B1, B2, B3, fix, V (+ product-vision U-rulings pre-spec; orchestrator: weekly-ended-is-new, 0001-01 bound, empty end-day Change) | 0/4/2/1 | yes (empty end-day Change total — production) + folded into V (3 pin-only MAJORs: vacuous S03, recurring currency, window/New) |
| SCENARIO-01b (+05, 08, 09, 13, 15) | code-first | A, B1, B2, fix, V (+ orchestrator: interim --json refusal, nil Change `no rate`, W2 placement, currency-before-month, summary kept out of currency loops until S12) | 0/6/4/0 | yes (W2 printed before store open — production) + folded into V (5 pin-only MAJORs: openReport fault, home-unset row, W1, W2 shape, currency cells) |
| SCENARIO-06 (+07) | code-first | A, B1, B2, V (+ orchestrator: 4 plan defaults) | 0/0/2/0 | folded into V (2 MINORs: end-to-end not-judged footer, doc trim) |
| SCENARIO-10 (+11) | code-first | A, B1, V (+ orchestrator: 5 plan defaults) | 0/1/2/1 | folded into V (1 pin-only MAJOR: covered snapshot + no transactions/first month → empty stderr; zone pin, doc wraps) |
| SCENARIO-12 | code-first | A, B1, V (+ orchestrator: 6 defaults; currency-test deletion) | 0/2/2/2 | folded into V (2 pin-only MAJORs: change-entry key order, --currency USD × --json currency; doc trim; spec example fixed≥newly_fixed) |
| SCENARIO-14 | code-first | A, B1, V (+ orchestrator: 3 defaults) | 0/1/0/1 | folded into V (1 pin-only MAJOR: JSON currency USD after retarget) |
| SCENARIO-16 | code-first | A, B1, B2, V | 0/4/1/1 | folded into V (3 pin-only MAJORs: W1/W2 order, classification → findings, local-day clock; 1 scope-record: parseCurrency extraction; not-ended log row) |
| SCENARIO-17 | code-first | A, B1, B2, V (+ orchestrator: 4 copy defaults, checkpoint sibling-sentence ruling) | 0/2/3/1 | folded into V (1 pin-only MAJOR: launchd recipe byte pin; 1 docs MAJOR: sections 2-3 pointer, pin red first; 3 MINOR test tightenings) |
| SCENARIO-21 (added by the SCENARIO-18 reference check, U17) | test-first | A+B1, V (+ triage diagnosis on the scratch copy) | 0/0/0/2 | none (NIT doc reword folded into V) |
| SCENARIO-22 (added by the SCENARIO-18 reference check, U18) | test-first | A+B1, V (+ triage follow-up on the scratch copy) | 0/0/0/0 | none (test rename folded into V) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 01 | arch, correctness, test, refactor | 0/1/7/11 | 0 / 20 (1 timeout, killed by hand), 2053 s | BLOCKED |
| 02 | correctness, test (re-gate) | 0/0/0/0 | not re-sampled | PASS |
| 03 | product-vision (final) | 0/1/1/0 | - | SHIP WITH CHANGES → PRD list fixed; launchd TCC line deferred by user (recipe optional) |

## Tokens

Tokens for `phase4f-summary` across 7 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 140 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| unknown | 70 | claude-opus-5-5, claude-sonnet-5-5 | 3k | 8,579k | 169,649k | 72k | 28,052k | 50% |
| developer | 38 | claude-sonnet-5-5 | 2k | 4,852k | 116,416k | 49k | 17,955k | 32% |
| test-reviewer | 12 | claude-sonnet-5-5 | 1k | 1,348k | 22,985k | 8k | 4,024k | 7% |
| architect | 11 | claude-opus-5-5, claude-sonnet-5-5 | 0k | 1,502k | 19,431k | 10k | 3,872k | 7% |
| product-vision | 3 | claude-opus-5-5 | 0k | 298k | 4,508k | 1k | 831k | 1% |
| triage | 2 | claude-sonnet-5-5 | 0k | 277k | 3,176k | 2k | 672k | 1% |
| correctness-reviewer | 2 | claude-opus-5-5 | 0k | 185k | 2,692k | 1k | 504k | 1% |
| refactor-advisor | 1 | claude-sonnet-5-5 | 0k | 66k | 249k | 1k | 111k | 0% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 51k | 192k | 0k | 83k | 0% |
| **subagent total** | 140 | | 6k | 17,159k | 339,298k | 144k | 56,104k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 1k | 963k | 136,032k | 272k | 16,890k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 8 | 2,479k | 4% |
| plan | 22 | 7,744k | 14% |
| build | 68 | 34,456k | 61% |
| checkpoint | 20 | 5,332k | 10% |
| checkpoint-fix | 4 | 681k | 1% |
| review | 14 | 4,639k | 8% |
| gate-fix | 4 | 774k | 1% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 2 | 0 | 0 | 0 | 4 | 7,618k | 14% |
| SCENARIO-01a | 2 | 10 | 2 | 2 | 0 | 8,029k | 14% |
| SCENARIO-01b | 2 | 8 | 2 | 2 | 0 | 10,326k | 18% |
| SCENARIO-06 | 2 | 8 | 2 | 0 | 0 | 3,658k | 7% |
| SCENARIO-10 | 2 | 6 | 2 | 0 | 0 | 4,326k | 8% |
| SCENARIO-12 | 2 | 6 | 2 | 0 | 0 | 5,228k | 9% |
| SCENARIO-14 | 2 | 6 | 2 | 0 | 0 | 3,567k | 6% |
| SCENARIO-16 | 2 | 8 | 2 | 0 | 0 | 6,186k | 11% |
| SCENARIO-17 | 2 | 8 | 2 | 0 | 0 | 2,953k | 5% |
| SCENARIO-18 | 0 | 0 | 0 | 0 | 0 | 1,002k | 2% |
| SCENARIO-21 | 2 | 4 | 2 | 0 | 0 | 1,749k | 3% |
| SCENARIO-22 | 2 | 4 | 2 | 0 | 0 | 1,463k | 3% |

Developer runs: 38; weighted per run median 394k, p90 838k, max 1,241k.

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| Final gate 01 (correctness) | Converted Change total `no rate` for a day of zero-balance unrated rows while type cells convert | internal/report/networth_change.go:71-79 | SCENARIO-01a (U5 wording keyed on Totals shape) |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
