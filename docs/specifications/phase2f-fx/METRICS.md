# Metrics: phase2f-fx

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-07 | test-first (Replace rates hook) | A, B1, B2, V | 0/2/5/2 | yes (fx_rates PK/CHECK unpinned; local-date Need.Last unpinned; test-only + comments) |
| SCENARIO-01 | test-first (Replace import_runs rates UPDATE) | A, B1, B2, V | 0/2/3/1 | yes (later-span failure unreached; Valet scheme/header unpinned; test-only + comments); 1 copy ruling (rates_checked_from, store.rates placement, nothing-fetched lines) |
| SCENARIO-04 (+02) | test-first (carry + floor in Replace) | A, B1, B2, V | 0/2/4/0 | yes (rates-before-prune order unpinned; rates surviving unreadable import_runs unpinned; test-only + comments); 1 copy ruling (carry reasons, combined line) |
| SCENARIO-03 (+05) | test-first (failed fetch still swaps) | A, B1, B2, V | 0/1/4/0 | yes (per-request timeout scope unpinned; test-only + docs); 1 copy ruling (warning order, partial Rates line, stop rule) |
| SCENARIO-06 | code-first (light) | L, V | 0/0/6/0 | no (MINORs → STATE.md) |
| SCENARIO-16 (+15) | code-first | A, B1, B2, V | 0/3/2/0 | yes (usage-before-config order unpinned at 3 sites; resolve CAD default unpinned; accounts warnings order unpinned; test-only + docs); 1 behaviour/copy ruling (table-shape refusal, status warns, sync/findings/snapshots refuse, P2d-10 amended) |
| SCENARIO-08 (+14) | code-first | A, B1, V | 0/2/3/0 | yes (month NULL arm unpinned; empty-window USD/native text cells; test-only + comments) |
| SCENARIO-10 (+09) | code-first | A, B1, V | 0/0/5/0 | no (MINORs → STATE.md) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |

## Tokens
<output of `.claude/scripts/feature-metrics.py --strict phase2f-fx`, pasted once at SHIP>

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
