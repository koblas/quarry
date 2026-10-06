## Review Report — round 01 (2026-10-06)

### Target
Changed files 60083e0..49141837 (merge-base with origin/main → HEAD).

### Triggered reviewers
- arch-reviewer, correctness-reviewer, refactor-advisor: production Go under cmd/ and internal/
- test-reviewer: `*_test.go`

### Skipped reviewers
- api-reviewer: no HTTP files
- pipeline-reviewer: no `.claude/**` changes

### Gate inputs
- verify.sh 60083e0: build, suite, race, lint all rc=0; uncovered-diff 0 open, 1 declared unreachable (internal/mcp/summary.go:107, judged valid by correctness and test reviewers).
- spec-check --run phase4f-summary: OK.
- mutation-sample: 20 of 78 sampled — 19 killed, 0 survived, 1 timed out (summary_recurring.go:13 `&&`→`||`; test-reviewer killed it in 0.4 s, harness timeout).

### BLOCKER
none

### MAJOR
1. correctness-reviewer — internal/report/networth_change.go:71-79 (`onlyReportingTotal`): a day with rows whose balances are all 0 and have no rate has empty `Totals`, so the converted Change total is `no rate` (JSON null) while every type cell converts and no warning fires (NeedsRate is false for a zero balance). Constructible: CAD reporting, a USD-only zero-balance start day before the first rate.
   **Ruling (orchestrator, U5 intent):** nothing on such a day needs a rate, so its total counts 0, like an empty day. Fix: guard on `len(date.Totals) == 0` instead of `len(date.Rows) == 0`. Test half (test-reviewer #1): a `Test_change_total_…` in internal/report/summary_change_test.go with the start day an unrated zero-balance USD row and the end day a converted row; assert the total equals the end total and the type cells convert; red against today's guard.

### MINOR
- arch-reviewer + refactor-advisor — internal/report/month.go:60 `MonthError.Error()` carries CLI vocabulary (`--month`, "summary"); mcp rewords via `monthWording`. → Open debt.
- arch-reviewer + refactor-advisor — internal/cli/summary.go:128 / internal/mcp/summary.go:56 config-to-choice decision and findings tally (cli/summary.go:97, mcp/summary.go:41) written in both surfaces. → Open debt.
- refactor-advisor — internal/report/summary.go:79 + internal/cli/render_summary.go:62: `Summary.Change` stored and also recomputed by the text renderer. → Open debt.
- refactor-advisor — internal/cli/summary.go:70-110 RunE ~40 lines, six phases (compose method). → Open debt.
- refactor-advisor — internal/mcp/summary.go (unreachable comment on `monthRefusal`) cites a foreign file path. → FOLD: `// unreachable: report.ParseMonth returns only MonthError.`
- test-reviewer — internal/report/summary_recurring_arms_test.go:103-147 one table spans three families with optional `runCharges`. → Open debt.
- test-reviewer — cmd/quarry/run_summary_refusals_test.go:112 closures in case struct; two different Givens. → FOLD: two plain tests.

### NIT
- arch-reviewer — internal/report/summary.go:60 `summaryCommand` duplicates the cli command word unenforced. → Open debt.
- refactor-advisor — internal/report/summary.go long `recurringFrom(...)` line; `keepAll` name for the always-true filter. → FOLD.
- refactor-advisor — recurring.go `recurringFrom` leaves `Accounts` to the caller. → Open debt.
- refactor-advisor — `SnapshotCovers` as zero value. → Open debt.
- refactor-advisor — internal/report/pricechange.go:50-58 quarter-of-steps sentence on three symbols. → FOLD (keep on `steadyRun`).
- refactor-advisor — internal/mcp/tools.go `monthlySummaryDescription` lacks a doc comment. → FOLD.
- refactor-advisor — internal/cli/render_summary.go:62-66 name `firstEmpty`/`lastEmpty`. → FOLD.
- refactor-advisor — store.go / report/store.go / duckstore/summary.go same sentence on three symbols. → FOLD (state on `store.Summary`).
- refactor-advisor — document/summary_warnings.go `SummaryWarnings` doc run-on. → FOLD (first sentence contract only).
- test-reviewer — internal/store/duckstore/summary_read_test.go:78-86 name leaks `summaryQueries`. → FOLD (rename to say every read runs over one open).
- test-reviewer — internal/store/duckstore/summary_read_test.go:47 year-one test pins no-fault only. → FOLD (rename to "does not fail on …").

### Strengths
- `runBothSurfaces` parity pins CLI and MCP documents byte-identical.
- Fake `Summary` filters by `Through` with a read-count spy; one-open guarantee pinned by the close tests.
- Boundary tables at exact day edges (45/46, 400/401, month first/last day, a nanosecond before local midnight); DST and leap-February coverage.
- Dependency rule clean; business rules in `internal/report`, delivery layers thin.

### Verdict: BLOCKED (1 MAJOR)
