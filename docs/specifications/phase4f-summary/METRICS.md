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

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |

## Tokens

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
