# Metrics: phase2f-fx

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-07 | test-first (Replace rates hook) | A, B1, B2, V | 0/2/5/2 | yes (fx_rates PK/CHECK unpinned; local-date Need.Last unpinned; test-only + comments) |

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
