# Metrics: phase2f-fx

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-07 | test-first (Replace rates hook) | A, B1, B2, V | 0/2/5/2 | yes (fx_rates PK/CHECK unpinned; local-date Need.Last unpinned; test-only + comments) |
| SCENARIO-01 | test-first (Replace import_runs rates UPDATE) | A, B1, B2, V | 0/2/3/1 | yes (later-span failure unreached; Valet scheme/header unpinned; test-only + comments); 1 copy ruling (rates_checked_from, store.rates placement, nothing-fetched lines) |

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
