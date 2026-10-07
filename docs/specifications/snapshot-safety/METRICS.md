# Metrics: snapshot-safety

## Scenarios
| Scenario | Cadence | Developer runs | Checkpoint findings (B/M/m/n) | Checkpoint fix pass |
| --- | --- | --- | --- | --- |
| SCENARIO-01 (+02, 05, 08) | test-first | A, B1, V (+ orchestrator: kill -9 pin ruling) | 0/0/7/0 | folded into V (3 pins: GC-held child, folder-missing pass-through, hard-link alias; Mode(99); 3 comment trims) |
| SCENARIO-03 (+04, 07, 09) | test-first | A, B1, B2, B3, V | 0/1/1/0 | folded into V (1 pin-only MAJOR: --json cells of 4 edge rows; 1 comment trim) |
| SCENARIO-06 | test-first | A, B1, B2, B3, V (+ orchestrator: 3 rulings) | 0/0/2/2 | folded into V (prune fifo row; Acquire doc trim; O_NOFOLLOW equivalence note). B3 caught a production copy bug (L5 missing "so") a B2 test had copied from production |
| SCENARIO-14 (+15, 16) | test-first | A, B1, V (+ orchestrator: 2 rulings) | 0/0/2/0 | folded into V (Pruned.Snapshots pin on cannot-tell arm; doc trim) |
| SCENARIO-10 (+11) | test-first | A, B1, checkpoint-fix, V (+ orchestrator: 4 plan rulings, BR-C8 checkpoint ruling) | 0/3/2/1 | yes (BR-C8 shared manifest kept — production; reds by mutation, not test-first) + folded into V (2 pin-only MAJORs: unreadable manifest path, --json manifest on-disk name; doc trim; upper-case orphan row) |
| SCENARIO-12a | code-first | A, B1, V (sized LIGHT; planned by architect — >3 ruled lines; + orchestrator: change 10 ruling) | 0/2/2/1 | folded into V (2 pin-only MAJORs: --from latest F3 row, F5 cmd cells; 2 comment trims) |
| SCENARIO-12b | code-first (light) | L, V (+ orchestrator: light lane kept — one ruling, four sites) | 0/0/0/1 | folded into V (snapshotExt doc) |

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
