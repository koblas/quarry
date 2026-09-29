# REVIEW-03 — phase1-import-store final gate, round 3 (re-gate of fix pass 2)

Range: `c64bb15..17e2512`. Coverage vs `0ecb753`: 0 uncovered, 11 declared unreachable (unchanged set). `spec-check`: OK.

## Reviewers
- correctness-reviewer: PASS — round-2 MAJOR (partial race), exponent split, `.Valid` checks closed; 7 mutations reddened as claimed; regex anchors probed.
- test-reviewer: BLOCKED — all 5 round-2 rows closed; 5 mutation spot-checks genuine; `execOn` Z_PK-0 fixture judged sound; one new MAJOR.
- refactor-advisor: PASS — all 6 round-2 rows closed; 2 NITs.
- arch-reviewer: not re-run (only ADR-001 text changed; no import or package change).

## MAJOR
- test-reviewer — `internal/store/duckstore/duckstore.go:117-121`: fix pass 2 removed the pass-1 `ErrExists` special case (cleanup now always removes the run's own partial + `.wal` on a create failure, per Ruling 1), but no test discriminates it — reinstating `if !errors.Is(err, duckdb.ErrExists) { removePartial(partialPath) }` passes every test. Fix: extend `Test_replace_reports_a_create_collision_as_an_untagged_build_failure` (or a sibling) to assert the run's own partial and `.wal` are gone after an `ErrExists` failure; prove with that reinstatement mutation.

## NIT
- refactor-advisor — `internal/importer/offenders.go:58-61` `firstError` doc reflow.
- refactor-advisor — `internal/store/duckstore/duckstore.go:25-26` const-block doc 2 lines (budget 1); optional.

## Verdict: FAIL (1 MAJOR) — fix pass 3 of 3.
