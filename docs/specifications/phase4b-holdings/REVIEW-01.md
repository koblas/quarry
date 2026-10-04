# Review Report — phase4b-holdings, round 1

### Target
Changed files `2ca9d70..103959b` (branch worktree-phase4b-holdings).

### Triggered reviewers
- arch-reviewer, correctness-reviewer, refactor-advisor: `cmd/**/*.go`, `internal/**/*.go`
- test-reviewer: `**/*_test.go`

### Skipped reviewers
- api-reviewer: no HTTP files
- pipeline-reviewer: no `.claude/**` changes

### Gate inputs
- `uncovered-diff: 0 uncovered added line(s) in 0 run(s) since 2ca9d70; 1 declared unreachable` (`internal/mcp/holdings.go:58`, judged sound by correctness and test reviewers)
- test-stats TOTAL 2730 (+235)
- `mutation-sample: 20 sampled of 98 candidates since 2ca9d70 — 19 killed, 0 survived, 0 non-viable, 1 timed out; 1212s` (timed-out `shares.go:91` mutant judged killed on paper by `shares_test.go:380-381,418-425`)
- `spec-check.py phase4b-holdings`: OK

### BLOCKER
none

### MAJOR
1. test-reviewer — `internal/mcp/timeout_test.go:104-121`: `holdings` missing from `Test_each_tool_answers_its_deadline_with_its_ruled_line`; `stallingStore` has no `Holdings`. Fix: stalling `Holdings` (like Spending :66-69) + row `{"holdings", {}, time.Second, "holdings stopped after 1 second; try again"}`.
2. test-reviewer — S.1 "There is no `--all` flag" unpinned. Fix: row `holdings --all` → `quarry: unknown flag: --all; Run 'quarry holdings --help' for usage.` in `cmd/quarry/run_usage_test.go` (~:243-253).
3. test-reviewer — zero price and 1899-12-29 placeholder never read through the real `(*Store).Holdings`/`scanHolding`. Fix: rows in `internal/store/duckstore/holdings_test.go` asserting `Price == 0`, `Value/ValueCAD/ValueUSD` non-nil 0, `PriceDate` 1899-12-29; optional cmd zero-price test (value 0.00, total unchanged, empty stderr).

### MINOR
- test-reviewer — `cmd/quarry/run_holdings_no_price_test.go:77,99` reads `account_filter` as `[]string` with `assert.Empty` (stale after S10). Fix: `json.RawMessage` + `JSONEq("[]")`.
- test-reviewer — S.5 "Text is raw" unpinned in the document (newline in account/security names round-trips raw).
- test-reviewer — `internal/store/duckstore/shares_internal_test.go:115-167` value cases could be black-box.
- test-reviewer — older-format store refusal pinned only for `spend` (`cmd/quarry/run_read_refusals_test.go:145`); add `holdings`.
- test-reviewer — MCP `accounts: []` only on an empty day (`cmd/quarry/run_mcp_holdings_test.go:42-44`); add a populated day.
- test-reviewer — SKILL §4 holdings row not exercised by `skillUseCases()` (optional; follows accounts precedent).
- correctness-reviewer — `internal/store/duckstore/shares.go:197` `holdingWalker` uses zero `time.Time` as "no row yet"; a 0001-01-01 first row (synthetic only) merges into the next day. Fix: `started bool`.
- arch-reviewer + refactor-advisor — as-of default/parse duplicated in `internal/cli/holdings.go:35-54` and `internal/mcp/holdings.go:19-28`. Fix: `report.ResolveAsOf(value *string, now)`.
- arch-reviewer — `store.Actions()` exported with test-only callers; document deliberate or move.
- refactor-advisor — `holdingSpans` QueryRows closure ~35 lines (Compose method).

### NIT
- arch + refactor — `report.Convertible` uses "CAD"/"USD" literals vs `money.CAD/USD`.
- refactor — receiver `l` on `Holdings`; two total-row builders; `append(configWarnings, …)` → `slices.Concat`; `nativeTotals` early return; `sort.Slice` → `slices.SortFunc`; `HoldingsWarnings` "Slots:" doc; `Actions` doc "pinned against this list".
- arch — `AsOfError.Error()` names `--as-of` (mirrors `WindowError`; no change).

### Strengths
- `runBothSurfaces` byte-identical CLI/MCP oracle.
- `holding_shares` span-rule table and the byte-identical-on-failure load tests.
- DuckDB `current_date` vs `report.Today` verified empirically across time zones (correctness).
- Dependency rule, port placement, thin delivery all clean (arch).

### Verdict: BLOCKED
3 MAJOR (test-reviewer).
