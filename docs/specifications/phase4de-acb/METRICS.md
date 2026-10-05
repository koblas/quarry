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
