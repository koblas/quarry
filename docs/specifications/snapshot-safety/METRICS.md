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
| SCENARIO-13 | test-first | A, B1, B2, V (+ orchestrator: 4 rulings — D1 order, two lines, non-regular manifest, BR-C5 fail-open) | 0/0/3/0 | folded into V (two-line D1 cmd cells; upper-case 3-name winner row; auto-prune no-warning assert) |

## Final gate
| Round | Reviewers run | Findings (B/M/m/n) | Mutants (survived / sampled, secs) | Verdict |
| --- | --- | --- | --- | --- |
| 01 | arch, correctness, test, refactor | 0/2/~20/~8 | 1 / 20 (2 timeouts; 1 re-run killed, 1 survivor), 1496 s | BLOCKED |
| 02 | correctness, test (re-gate after 2 fix passes) | 0/0/2/1 | survivor re-killed by hand | PASS WITH FOLLOW-UPS |
| 03 | correctness, test (re-gate of fix pass 3 from final product-vision) | 0/1/1/1 | not run | BLOCKED → MAJOR deferred under 3-pass cap |
| final | product-vision (SHIP WITH CHANGES → L6/L7; then SHIP after confirmation) | 0/1/0/0 | - | SHIP |

## Tokens

Tokens for `snapshot-safety` across 8 project dir(s). Weighted = input-equivalent tokens (IE): cache read x0.1, cache write x1.25 (5m) / x2 (1h), output x5. Attribution: 63 tagged, 0 heuristic. Orchestrator row counts the main session between the feature's first and last run in each session, so it may include other work.

| Agent | Runs | Model(s) | Input | Cache write | Cache read | Output | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| developer | 32 | claude-sonnet-5-5 | 2k | 4,942k | 110,671k | 57k | 17,530k | 72% |
| architect | 8 | claude-opus-5-5, claude-sonnet-5-5 | 0k | 962k | 11,947k | 6k | 2,429k | 10% |
| test-reviewer | 11 | claude-sonnet-5-5 | 0k | 926k | 11,751k | 15k | 2,408k | 10% |
| correctness-reviewer | 3 | claude-opus-5-5 | 0k | 249k | 4,110k | 1k | 729k | 3% |
| product-vision | 5 | claude-opus-5-5 | 0k | 332k | 2,491k | 2k | 676k | 3% |
| triage | 1 | claude-sonnet-5-5 | 0k | 97k | 952k | 2k | 224k | 1% |
| arch-reviewer | 1 | claude-sonnet-5-5 | 0k | 48k | 168k | 1k | 82k | 0% |
| refactor-advisor | 1 | claude-sonnet-5-5 | 0k | 52k | 110k | 0k | 78k | 0% |
| pipeline-reviewer | 1 | claude-sonnet-5-5 | 0k | 40k | 74k | 0k | 58k | 0% |
| **subagent total** | 63 | | 3k | 7,648k | 142,273k | 85k | 24,214k | 100% |
| orchestrator (upper bound) | - | claude-opus-5-5 | 0k | 993k | 58,255k | 91k | 8,266k | - |

| Run kind | Runs | Weighted (IE) | Share |
| --- | --- | --- | --- |
| scope | 4 | 628k | 3% |
| plan | 8 | 2,429k | 10% |
| build | 28 | 13,857k | 57% |
| checkpoint | 8 | 1,266k | 5% |
| checkpoint-fix | 1 | 534k | 2% |
| review | 10 | 2,304k | 10% |
| gate-fix | 3 | 3,139k | 13% |
| retro | 1 | 58k | 0% |

| Unit | Plan | Build | Checkpoint | Checkpoint fix | Gate fix | Weighted (IE) | Share |
| --- | --- | --- | --- | --- | --- | --- | --- |
| - | 1 | 0 | 0 | 0 | 3 | 6,365k | 26% |
| SCENARIO-01 | 1 | 3 | 1 | 0 | 0 | 2,069k | 9% |
| SCENARIO-03 | 1 | 5 | 1 | 0 | 0 | 2,045k | 8% |
| SCENARIO-06 | 1 | 5 | 1 | 0 | 0 | 2,344k | 10% |
| SCENARIO-10 | 1 | 3 | 1 | 1 | 0 | 2,856k | 12% |
| SCENARIO-12a | 1 | 3 | 1 | 0 | 0 | 2,267k | 9% |
| SCENARIO-12b | 0 | 2 | 1 | 0 | 0 | 524k | 2% |
| SCENARIO-13 | 1 | 4 | 1 | 0 | 0 | 3,258k | 13% |
| SCENARIO-14 | 1 | 3 | 1 | 0 | 0 | 2,486k | 10% |

Developer runs: 32; weighted per run median 441k, p90 1,294k, max 1,612k.

## Caught late
| Stage | Finding | Where (file:line) | Scenario that shipped it |
| --- | --- | --- | --- |
| gate R1 (correctness) | `// unreachable:` false on darwin: relative --from from an unsearchable cwd prints unruled copy (now F7) | internal/snapshot/from.go:107-111 | SCENARIO-12a (moved the pre-existing branch, re-declared unreachable) |
| gate R1 (test, mutation survivor) | Backup stat-check fallback unpinned under a listing fault; mutant could overwrite a snapshot | internal/snapshot/destination.go:94 | SCENARIO-13 (folderUses made the old stat-check pins unfalsifiable) |
| gate R3 (correctness) | prune-mode lock refusal names the quarry folder for a fault above it (copy-only, unreachable) | internal/platform/lockfile/lockfile.go:169-174 | fix pass 3 (final-pass L6/L7) — deferred |
| final pass (product-vision) | lock refusals blamed quarry.lock when the quarry folder itself faulted | internal/snapshot/lock_refusal.go | SCENARIO-06 (ruling 2 routed folder faults to L4b) |

## Escaped defects
| Found | Defect | Where (file:line or issue) | Scenario that shipped it |
| --- | --- | --- | --- |
