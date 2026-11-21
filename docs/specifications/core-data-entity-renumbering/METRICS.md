# Metrics: core-data-entity-renumbering

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-02 (+01, 04, 05, 06 folded) | code-first | A, B1, B2, V | 0/1/5/2 | folded into V |
| SCENARIO-03 (+07 folded) | code-first | A, B1, V | 0/1/2/0 | folded into V |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / timed out / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 1 | arch, correctness, refactor, test | 1/0/7/5 | 2 / 0 / 20, 851s | BLOCKED |

## Tokens

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| gate R1 | importer keeps last Z_ENT for duplicated Z_NAME → silent empty import (pre-existing, rewritten fn) | internal/importer/entities.go:50 | SCENARIO-03 |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
