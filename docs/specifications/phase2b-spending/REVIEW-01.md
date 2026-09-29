# Review Report — REVIEW-01 (phase2b-spending, final gate round 1)

### Target
Feature branch `25c8c46..HEAD` (merge-base with origin/main), all changed Go files.

Coverage gate: first run exited 1 (`internal/cli/cashflow.go:87`, `spend.go:70` — `return err` after `renderResult`); bounced to developer (commit 8556977 added `// unreachable:` markers). Re-run: `uncovered-diff: 0 uncovered added line(s) in 0 run(s) since 25c8c4646909; 2 declared unreachable`.
`spec-check.py --run phase2b-spending`: OK (26/26 acceptance tests pass).
Mutation sample: `20 sampled of 101 candidates — 20 killed, 0 survived, 0 non-viable, 0 timed out; 202s`.
test-stats (base 25c8c46): cmd/quarry 121 (+26), cli 119 (+60), importer 129 (+7), report 73 (+55), duckstore 178 (+90); TOTAL 620 (+238).

### Triggered reviewers
- arch-reviewer, correctness-reviewer, refactor-advisor: `cmd/**/*.go`, `internal/**/*.go`
- test-reviewer: `**/*_test.go`

### Skipped reviewers
- api-reviewer: no HTTP surface
- pipeline-reviewer: no `.claude/**` change

### BLOCKER
none

### MAJOR
1. test-reviewer — `internal/store/duckstore/views_test.go:106-153` (+ `cmd/quarry/run_spend_test.go:77`, `run_spend_window_test.go:15`) — no test passes a row dated after the real wall clock through `v_cash_flow`/`v_spending`/`Spending`/`CashFlow`. P2b-6 "no date cutoff in the view" and the edge row "`--until` past today includes future-dated rows" are unpinned: adding `AND t.date <= current_date` to `v_cash_flow` survives the whole suite. Fix: duckstore `Spending` and `CashFlow` tests with a split on 2099-06-01, window 2099-01-01..2099-12-31, must return it; cmd `spend --since 2099 --until 2099` (clock 2026-09-29) case.
2. test-reviewer — `internal/store/duckstore/spending.go:129` — the category grouping's `currency` sort term is unpinned (removing it survives); ties rest on DuckDB behaviour. Fix: several currencies inserted out of order (USD, GBP, EUR, CAD) under one category; assert row and Totals order; re-mutate to confirm red.
3. test-reviewer — `internal/cli/spend_window_test.go:154-173`, `internal/cli/cashflow_test.go:181-190` — verbatim Surface & Copy help text mostly unpinned: spend `--by` help + default, spend Long (incl. tag-count paragraph), spend Example; cashflow Example, rest of Long, `--since`/`--until` help. Fix: help tests asserting each (Regexp for padded flag lines).
4. test-reviewer + correctness-reviewer (MINOR there) — `internal/cli/cashflow.go:88` / `internal/cli/json.go:137` — the `// unreachable:` chain relies on `marshalDocument`'s reason, which covers only floats `jsonSQLCell` lets through; cashflow's `savings_rate_pct *float64` comes unfiltered from the port. Finite in production (`duckstore/cashflow.go:27`: BIGINT tenths / 10.0, or NULL) but no reason says so, and a fake returning NaN reaches the branch. Fix: extend the reason to name how finiteness is established (duckstore cashFlowQuery, the only source), or guard + test NaN/Inf.

### MINOR
- test-reviewer — `internal/cli/cashflow_test.go:151-158` `Test_cashflow_returns_a_failed_stdout_write` asserts empty stderr vacuously (no warnings in fixture); seed an E line (Transactions set, no Totals).
- test-reviewer — no cashflow tests for E2, E2a, S2/S2d/S3 (`internal/cli/cashflow_test.go`, `cmd/quarry/run_cashflow_refusals_test.go`); `duckstore/cashflow_test.go` has no zero-range case.
- test-reviewer — no H1 row and no U9 `Run 'quarry spend --help'`/`cashflow --help` rows (`run_accounts_test.go:65`, `run_usage_test.go:175`).
- test-reviewer — `internal/store/duckstore/cashflow_test.go:107` pins three rules in one test ("and"); duplicates `views_test.go:106` — split or drop.
- test-reviewer — duplicated fixtures/helpers (cmd `spendEnv` inlined ×7, `Account{"acct-cad"…}` ×15, `viewRows` vs `spendRows`, shared helpers in per-command files; cli `spendTagNow`/`spendWindowNow`; duckstore `day`/`civil`) — consolidate into per-package helper files.
- test-reviewer — test data hidden from test bodies (`internal/cli/spend_empty_test.go:20-28` storeSpan/namedSpan; `cmd/quarry/run_spend_test.go:31-50` spendRows names).
- test-reviewer — comment budgets: `cmd/quarry/run_spend_test.go:20-22` spendSplit (3 lines); `internal/cli/fakes_test.go:10-12` fakeReportStore (3 lines).
- test-reviewer — `cmd/quarry/run_cashflow_refusals_test.go:12-75` mixes refusal rows and an exit-0 empty-period row with optional fields.
- refactor-advisor — spend/cashflow parallel pipelines duplicate at every layer: `render_cashflow.go:21-61` vs `render_spend.go:20-64` (extract `renderTable`, rename `spendingTotalLabel`/`spendingPartialStatus`/`spendingAccountsCaption`); duckstore `accountFilter`/`readArgs`/`marks`/`transactionRangeQuery` living in spending.go + duplicated range fetch in `Spending`/`CashFlow` (move to `filter.go`, extract `transactionRange`); `report` `fillMonths` vs `fillPeriods` (one fill helper, one `periodCurrency` type); cli command shape (`--since/--until/--account` into one flag struct; shared `renderResult`+`emit` tail).

### NIT
- refactor-advisor — `spendAccountDocument` used by cashflow (rename `accountDocument`, shared mapper); `cashFlowCells` 7 positional args (shared `CashFlowFigures`); `appendEmptyWindowWarning` recounts NotInReports (`allLeftOut`); date layout constants duplicate `time.DateOnly`; `DefaultWindow` belongs in window.go; `period.First` possibly unused; bare `"spend"`/`"cashflow"` command strings; `ErrUnsupportedPeriod`/`ErrUnsupportedGrouping` reachability note.
- test-reviewer — `stderrOf` rebuilds the warning prefix; `Test_spend_reads_the_window_from_the_env_clock` names mechanism; some new `package main` cmd test files lack the white-box header.

### Could not check (correctness-reviewer)
- Whether Quicken's reports treat transfers into accounts not in reports / outside an `--account` selection the same way (P2b-6 excludes every transfer leg) — the Gate CSVs decide.
- UTC-day register date for users east of UTC (P2b-3).
- Note (not a finding): `Spend`/`CashFlow` open the store twice when `--account` is given (Accounts, then read); a sync between them is tolerated since ids are stable.

### Strengths
- Architecture clean: dependency rule, `report.Store` port with two new params-struct methods, thin cli, clock ruling honoured.
- Correctness verified end to end: v_cash_flow predicates, transfer exclusion via transfers table, register date, placeholder numbering with numbered params, inclusive civil-day window, UTC-aligned series, SQL-owned rate with no negative zero, warning order/suppression, Close on every path.
- `utcMinus5` injected-clock cmd tests pin the zone trap end to end; invariant test compares both commands to hand-derived constants; views_test one-hazard-per-case design.

### Verdict: BLOCKED
0 BLOCKER, 4 MAJOR, 10 MINOR (grouped), 3+ NIT.
