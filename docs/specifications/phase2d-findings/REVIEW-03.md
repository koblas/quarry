# Review round 3 — phase2d-findings (after final product-vision SHIP WITH CHANGES)

Range `4ae5857..HEAD`. Full suite rc=0, 0 uncovered added lines, lint 0. Mutation sample: 1/1 killed, 75s.

Reviewers: correctness (PASS WITH FOLLOW-UPS), test (PASS WITH FOLLOW-UPS).

## MINOR (→ STATE.md Open debts)
- correctness — `internal/config/refusal.go:13,47`: `Problem` reads `err.Error()`, `ProblemAbsolute` reads the inner `refusalError`; make both read the struct.
- correctness — `internal/cli/findings.go:94`: W1's `~` form re-abbreviated in cli; expose `Config.Shown`.
- correctness — `internal/snapshot/list.go:91`: `snapshots --json` mixes an absolute config path with the `~` no-snapshots warning (2c ruled copy) — product-vision ruling.
- test — `cmd/quarry/run_config_test.go:52-55` doc comment misplaced by `configPath`; file at 400 lines.
- test — widths-after-escape unpinned for `balanceMismatchRows`, `splitMismatchRows`, `itemRows` (two-row escaped + clean case).
- test — logic in test bodies building expected absolute strings (`run_findings_ignore_test.go:96-117`, `run_status_findings_test.go:153`).
- test — `render_escape_internal_test.go` nine-behaviour test → table; misnamed hidden-status test; split `escapeCell` test.
- test — `config/problem_test.go:62-76` double load; helper placement.

## Verdict: PASS WITH FOLLOW-UPS
