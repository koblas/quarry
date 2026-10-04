# Review 04 — phase3d-skill (re-gate after fix pass 3)

## Target
Range `8ba9902..9a32205`. The fix pass changed copy and tests only.

## Triggered reviewers
correctness-reviewer and test-reviewer, the two that blocked round 3.

## BLOCKER
None.

## MAJOR
None. Both REVIEW-03 MAJORs are closed:
- The README privacy paragraph is true against the code, per the correctness reviewer. The rate fetch at `internal/fx/valet.go:63` is the only network request. It carries nothing but the series and dates. The read-only DuckDB opens disable extension fetches.
- The ended-threshold clause is pinned, and the currency rows are pinned as full rows.

## MINOR
- correctness-reviewer, `internal/platform/duckdb/duckdb.go:50`: `Create` (the sync write path) does not set `autoinstall_known_extensions=false`. A future build statement that needs an unbundled extension would then make a second network request, which would make the README claim false. Today's build needs no such extension.

## Verdict: PASS WITH FOLLOW-UPS
