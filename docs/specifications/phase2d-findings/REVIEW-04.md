# Review round 4 — phase2d-findings (after product-vision re-check: no-snapshots absolute path)

Range `b9e66b1..HEAD` (65e4b15). Suite rc=0, 0 uncovered, lint 0. Developer mutations: both red.

Reviewers: correctness (PASS), test (PASS WITH FOLLOW-UPS).

## MINOR / NIT (→ STATE.md Open debts)
- test — `internal/config/problem_test.go:13-25`: no wrapped-refusal case for `Problem`/`ProblemAbsolute` (no production wrapper today).
- NIT — `internal/snapshot/list.go:52` `NoSnapshotsAbsolute` doc line length.

## Verdict: PASS WITH FOLLOW-UPS
