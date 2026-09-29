---
id: SCENARIO-12
status: open
---

# SCENARIO-12: spend groups by month and fills empty months

Cadence: code-first (no bug fix, write-safety or atomicity item touched)
Acceptance test: `cmd/quarry/run_spend_by_test.go` `Test_run_spend_by_month_fills_empty_months_and_marks_a_cut_short_month_partial`
Acceptance test (SCENARIO-18, folded): `cmd/quarry/run_spend_refusals_test.go` `Test_run_spend_rejects_a_period_it_cannot_use`
Narrow loop: `go test ./internal/store/duckstore/ -run 'spending' && go test ./internal/report/ -run 'spend' && go test ./internal/cli/ -run 'spend|Spend' && go test ./cmd/quarry/ -run 'Test_run_spend'`
Mutation checks: first bound `Since.After(first)` -> `!Since.Before(first)` -> `Test_spend_by_month_marks_the_first_month_partial_only_when_the_window_starts_after_its_first_day`; last bound `Until.Before(last)` -> `!Until.After(last)` -> `Test_spend_by_month_marks_the_last_month_partial_only_when_the_window_ends_before_its_last_day`; month's last day as first + 28 days (or first of next month) -> `Test_spend_by_month_ends_february_on_its_last_day_in_leap_and_common_years`; series loop ending on `Month()` alone -> `Test_spend_by_month_runs_the_series_across_a_year_end`; zero fill skipped -> `Test_spend_by_month_fills_a_month_with_no_spending_as_zero`; fill currencies taken from rows, not Totals -> `Test_spend_by_month_fills_every_month_for_each_currency_in_the_totals`
Runs: A (1-2) | B1 (3-5) | B2 (6-7) | V (8-9)
Size: OWNS A RUN — 5 Build batches, 1 feature package (report; duckstore, cli and cmd tests do not count); absorbs SCENARIO-18

**Copy gaps — scoped `product-vision` ruling needed before run A** (the acceptance test asserts full stdout): (a) does the trailing column's header print `Status` (spec says "Status column", accounts heads it `Status`) or nothing (cashflow's sample header has no word there)? (b) is the column present when no row is partial, and in the header-only table of an empty window (S17)? Settled by reading, not a gap: an empty Status cell leaves no trailing spaces (Surface & Copy: "no trailing spaces"); a JSON month row is `{"month","currency","spent","partial"}`, `month` a non-null string.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_spend_by_test.go` `Test_run_spend_by_month_fills_empty_months_and_marks_a_cut_short_month_partial` — `replaceStore`+`spendRows` fixture: CAD splits 2026-01-10 (before the window), 2026-01-20, 2026-03-10; `env.Now` 2026-09-29; `spend --by month --since 2026-01-15 --until 2026-03`; exact stdout (caption, `Month` header, 2026-01 partial, 2026-02 `0.00`, 2026-03, Total), empty stderr, exit 0
- [x] Step 2: `internal/store/store.go:321-333` `SpendByMonth` enum value; `internal/cli/spend_grouping.go:100-104` month entry (`name: "month"`, `header: "Month"`); drop `"month"` from the refused list at `internal/cli/spend_test.go:68`. Expected red: duckstore returns `ErrUnsupportedGrouping`, so the test fails at its exit-code assertion

### Build
- [ ] Step 3: `internal/store/duckstore/spending.go:56-65` `spendingQueries[store.SpendByMonth]` — month query over `v_spending.month`, key produced as `YYYY-MM` text in SQL, order `month, currency`, keeps the `HAVING sum <> 0` row omission; new `spending_month_test.go`: `Test_spending_by_month_groups_each_currency_by_calendar_month`, `Test_spending_by_month_sorts_across_a_year_end_by_month_then_currency`, `Test_spending_by_month_omits_a_month_that_nets_to_zero_and_keeps_it_in_the_total`, `Test_spending_by_month_counts_the_windows_first_and_last_day_only`. No new fault test: the month query runs the same `QueryRows` path `spending_test.go:155-186` already fault-tests
- [ ] Step 4: `internal/report/spending.go:18-25,37-44` `Spending` stops embedding `store.Spending`: own `Rows []SpendingRow` (new `SpendingRow` embeds `store.SpendingRow`, adds `Partial bool`), `Totals`, `MultiTagSplits`, `Window`, `By`; `Spend` copies the store result. Same step, every call site so the build stays green: `internal/cli/json_spend.go:57-88` (`spendRowDocumentFor` param), `internal/cli/render_spend.go:21`, tests `internal/report/spending_test.go:52`, `internal/cli/render_spend_internal_test.go:21-90`, `internal/cli/json_spend_internal_test.go:23-30,65-72,107`. Behaviour-neutral: existing tests green unchanged in intent
- [ ] Step 5: new `internal/report/period.go` — unexported period series: `(window, unit)` -> ordered periods, each with first day, last day, label (`2006-01`) and partial flag; unit enum holds month only (SCENARIO-20 adds year). `(*Server).Spend` for `SpendByMonth` fills series x Totals currencies (Totals order), store row where the label matches, else a 0 row; other groupings untouched. Tests through `(*Server).Spend` + `fakeStore` in `internal/report/spending_month_test.go`: the six `Mutation checks:` tests, plus `Test_spend_by_month_lists_no_rows_when_the_window_holds_no_spending` and `Test_spend_by_month_marks_a_one_month_window_cut_at_both_ends_partial`. Leap cases: until `2024-02-29` not partial, `2024-02-28` partial, `2025-02-28` not partial; bounds: since on day 1 not partial, day 2 partial; until on the last day not partial, the day before partial
- [ ] Step 6: `internal/cli/render_spend.go:15-47` `renderSpending` — month grouping adds the trailing Status column per the copy ruling (`partial` or empty; Total rows empty), no trailing spaces on any line; `internal/cli/json_spend.go:41-88` `spendMonthRowDocument` (`month`, `currency`, `spent`, `partial`) + its `spendRowDocumentFor` case; cases in `render_spend_internal_test.go` `Test_renderSpending` (partial and non-partial months, width from widest cell) and `Test_renderSpendingJSON_writes_month_rows_with_their_partial_flag`
- [ ] Step 7: new `cmd/quarry/run_spend_refusals_test.go` `Test_run_spend_rejects_a_period_it_cannot_use` — SCENARIO-18 outline rows (`--since 2024-13`, `--until yesterday`, `--since 2025 --until 2024`, `--since 2099`, `--by vendor`, `extra`); HOME = empty temp dir, no store (exit 2, not R1's 1, proves the checks run before `openReport`); `env.Now` set; exact `quarry: `-prefixed stderr per Surface & Copy S1/S2/S3/S4/U8, empty stdout. Expected **green on arrival**: all six refusals predate this scenario (SCENARIO-10, 13, `noArgs`); coverage-only fold

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `SpendByMonth` (key is `YYYY-MM`, never nil), `store.SpendingRow.Key` (month never nil), `report.SpendingRow`, `report.Spending`, the period series

### Verify
- [ ] Step 9: full verification + `spec-check.py phase2b-spending` -> tick SCENARIO-12 with its acceptance test and SCENARIO-18 as "delivered by SCENARIO-12" with its test; STATE.md: remove the cmd-level exit-2 open debt, move `SpendByMonth` out of Left unbuilt

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Fill lives in `report`, not SQL — the series depends only on the window, not on stored rows; the store still owns every aggregate (P2b-9), and the date arithmetic (partial bounds, Dec->Jan, leap Feb) is tested without DuckDB. SCENARIO-20 reuses the period series (adds a year unit) and writes its own fill over it.
- Fill currencies = `Totals` currencies, in Totals order — an empty window has no Totals, so it yields no rows (SCENARIO-17's header-only output), and a currency netting to 0 still gets every month.
- Month rows are never omitted: a net-zero month is dropped by the query's `HAVING` and restored by the fill as `0.00`, identical to an empty month (edge row "month/year row with no activity printed 0.00").
- `partial` = window `Since` after the period's first day, or `Until` before its last day; only the first and last period can be.
- `report.Spending` owns its rows (`[]report.SpendingRow` = `store.SpendingRow` + `Partial`); `Partial` is never on the store port, which cannot know it for filled rows. SCENARIO-14's `account_filter`/caption fields go on `report.Spending`.
- Row order for month: series order, then currency in Totals order (spec sort `month, currency`).

**Left unbuilt** — named so nobody assumes it exists:
- Year period unit and any cashflow fill — SCENARIO-20/21.
- E1/E2 warnings for an empty month window — SCENARIO-17.

**Traps** — things that look right and are not:
- A DuckDB DATE scanned into `sql.NullString` is RFC3339 text: the `YYYY-MM` label must come from SQL.
- Passing a `strftime(...)` expression as `spendingQuery`'s key puts it inside `GROUPING()`, `GROUPING SETS` and `ORDER BY GROUPING()`: use a subquery/CTE exposing a key column, or a dedicated query const like the tag one.
- SQL label and Go series label must match byte for byte, or every month prints `0.00` beside the lost real rows — only the DuckDB-backed acceptance test sees that.

## Phase report

Run A done (acceptance red), commit follows this report. Test uses `env.Now` 2026-09-29; expected stdout is literal strings, widths verified with printf (`%-7s  %-8s  %6s  %s`; empty Status leaves no trailing spaces).
- `cmd/quarry/run_spend_by_test.go:83-110` acceptance test `Test_run_spend_by_month_fills_empty_months_and_marks_a_cut_short_month_partial`.
- `internal/store/store.go` `SpendByMonth` (iota 3); `internal/cli/spend_grouping.go` month entry (`missing` empty: month key never nil); `internal/cli/spend_test.go:68` `"month"` dropped from the refused list.
- RED now: `require.Equal(0, exitCode)` fails, actual 1, `quarry: spending grouping is not supported: 3` (duckstore `ErrUnsupportedGrouping`). Everything else green: `go test ./internal/cli ./internal/report ./internal/store/...`.
- KNOWN LINT RED until step 6: `internal/cli/json_spend.go:80` exhaustive switch lacks `store.SpendByMonth` (step 6 adds `spendMonthRowDocument` + case). Do not suppress it.
- Copy ruling applied to steps 6 (not yet built): header `Status` always present (incl. header-only), empty cell on non-partial and Total rows, no trailing spaces; JSON month row `{"month","currency","spent","partial"}`.
