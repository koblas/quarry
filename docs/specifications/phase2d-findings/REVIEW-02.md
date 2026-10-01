# Review round 2 — phase2d-findings (re-gate after fix pass 1)

Range `6f2a5b6..HEAD`. Full suite rc=0, 0 uncovered added lines, 4 declared unreachable (all judged valid). Mutation sample: 8/8, 7 killed, 1 survived (equivalent: `status.go:33` `COALESCE(first_found_at = built_at, …)` — both columns NOT NULL), 439s.

Reviewers: correctness (PASS — REVIEW-01 MAJORs 1, 2 and MINORs closed), test (PASS WITH FOLLOW-UPS — items 3-8 and folded MINORs closed).

## MINOR (→ STATE.md Open debts)
- test — `internal/store/duckstore/findings_test.go:306`: unknown-type test pins only a prefix of the carried row; assert the exact row (code stamps fixed_at = built_at).
- test — `internal/store/duckstore/status_test.go:986-994`: loop re-derives state; assert `[]store.Finding` directly and add a New-in-latest-build row.
- test — `internal/cli/render_findings_internal_test.go:217,223`: use `ignoreHint`, not the literal.
- test — `internal/cli/export_test.go:1`: header naming `UseZone`.
- test — footer table row names in `render_findings_status_internal_test.go`.
- NIT — `finding_test.go:24` `Known` test name; dead first COALESCE in `status.go:33`.

## Verdict: PASS WITH FOLLOW-UPS
