---
id: SCENARIO-12
status: open
---

# SCENARIO-12: Net worth month by month

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_networth_history_test.go` `Test_run_networth_lists_each_month_end_with_a_column_per_type_ending_with_today`
Narrow loop: `go test ./internal/report/... ./internal/cli/ ./internal/store/duckstore/ -run 'NetWorth|networth|net_worth|Window|window|month' && go test ./cmd/quarry/ -run 'networth'`
Mutation checks: until clamp in `report.ParseMonthEndWindow` → acceptance test; empty-`Dates` guard in `duckstore.(*Store).NetWorth` → `Test_net_worth_reads_no_rows_for_no_dates`
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches, 1 feature package (`report`; `report/document`, `duckstore`, `cli`, `cmd/quarry` are its surfaces)

Contract: `quarry networth --since <d> [--until <d>]` / `--until <d>` → history on stdout, exit 0; config warnings on stderr as S10. Not-a-date (either bound), since after until, `--until` alone before Jan 1 → existing `WindowError` line on stderr, exit 2. Interim (until S15): `--since` after today → empty history (caption + header, exit 0), no new string — `WindowSinceAfterToday` ("pass --until…", which networth clamps away) and `WindowChargeSinceAfterToday` ("lists charges") both mis-describe networth, so neither may be reached.

**Unruled shape — orchestrator rules before B2 (defaults the developer builds if confirmed):**
1. History cell with no row for that type (converted) or type × currency (native) on that date: blank. (Alt: `0.00`.)
2. Every listed month end gets a text line, rows or not (N-9 "lists each month end"); converted Total blank when nothing converts. Native: one line per currency with a row that date, CAD first; a date with no rows gets no native line. All-before-data shape stays S16's.
3. Caption on an empty list (interim future `--since`): `<s>`/`<u>` from the resolved window's since and clamped until.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_networth_history_test.go` (new) `Test_run_networth_lists_each_month_end_with_a_column_per_type_ending_with_today` + `seedNetWorthHistoryStore` — `holdingsClock()` (2026-03-12, past DuckDB's today per trap), `--since 2026-01 --until 2027`; CAD chequing, CAD credit_card, USD chequing with balances on every listed date and the USD rate before 2026-01-31, so no interim blank cell; rows 2026-01-31, 2026-02-28, 2026-03-12, full stdout asserted
- [x] Step 2: `internal/cli/networth.go:11-63` — register `--since`/`--until` (help verbatim from Surface & Copy; not via `reportFlags.bind`, which adds `--account`); stub `report.ParseMonthEndWindow` in `internal/report/window.go` after `:103`; `Window *store.Window` on `NetWorthRequest` (`networth.go:15-19`) and `NetWorth` (`:34-39`), unused — test fails at its stdout assertion

### Build
- [ ] Step 3: `internal/report/window.go:91-148` `ParseMonthEndWindow` — reuse `parseDateBound`, `WindowNotADate`/`WindowSinceAfterUntil`/`WindowUntilBeforeDefault`; must NOT pass through `parseWindow`'s future-since arm (`:128-129`); since defaults Jan 1 (`DefaultWindow`), until defaults today, until clamped to `Today(now)` after the since-after-until check (user's values). Tests in `window_test.go`: not-a-date per bound; since after until; until alone Dec 31 last year refused / Jan 1 accepted; until today (kept) / today+1 (clamped) / year 2027 (clamped); since after today alone and with a later until → window with since > until, no error
- [ ] Step 4: `internal/report/networth.go:53-85` `(*Server).NetWorth` history + unexported `monthEnds` + pivot helpers on `NetWorth` (types non-zero on any date, alphabetical; per-type converted sum; per-type-currency native balance), and `internal/store/duckstore/networth.go:24-37` guard: empty `Dates` returns empty `store.NetWorth` after `openRead` (no-store / format refusals still fire). One `NetWorth` call with every month end; rows grouped into dates by civil day (`civilDay`-style Y-M-D key, never `time.Time ==` — Location pointers differ); every requested date gets an entry, rows or not; totals per date via `total`. Tests: `monthEnds` grid in `networth_internal_test.go` (new) — since on a month end (included), day after one, until on a month end (no duplicate), until mid-month (appended), since == until, inverted (empty), both in this month (one row = until), Jan 31→Feb 28, leap Feb 29→Mar 31, Dec→Jan; `networth_test.go:41-51` neighbour: history reads once with all month ends, fake returns rows for two dates plus one empty date → three entries, rows in the right ones; pivot helpers (zero type dropped, mixed-currency converted sum, nil converted left out); `net_worth_read_test.go:86-106` neighbour `Test_net_worth_reads_no_rows_for_no_dates`. Trim `networth.go:13` const doc to 1 line (debt)
- [ ] Step 5: `internal/cli/networth.go:40-58` history mode when `--since` or `--until` Changed → `ParseMonthEndWindow` → `UsageError` (exit 2); `internal/report/document/networth.go:42-65` `NewNetWorth` — `as_of` null, `since`/`until` set (resolved since, clamped until) when `Window` non-nil; doc stops saying "a snapshot". Tests: `document/networth_test.go:87` neighbour (history keys, empty date keeps `balances: []`, native history `converted_balance` null); `run_networth_history_test.go` cmd rows: each refusal line verbatim + exit 2; `--until 2026-02` alone succeeds (since Jan 1) next to the `--until 2025-12` refusal; `--since 2026-03-05` → one row today; `--json` history pinning `since` "2026-01-01" and first `dates[].date` "2026-01-31" and `until` today in one test; interim `--since 2027-01` → caption + header, exit 0, JSON `dates: []`; `run_networth_surfaces_test.go:14-57` asserts `--since`/`--until` help lines
- [ ] Step 6: `internal/cli/render_networth.go:11-48` history branch: caption `Net worth at each month end <first> to <last>, amounts in <CUR>` (native drops `, amounts in X`; first/last = listed month ends), `Month end` + type columns + `Total` (converted), `Month end  Currency  <types…>  Total` (native); per shape rulings 1-3. Doc comment trimmed to budget (debt `:11`). Tests in `render_networth_internal_test.go`: converted pivot, native pivot, zero-everywhere type has no column, date with no rows still listed, no-rate cell blank and out of Total (interim), empty list caption; rename `:63` test to `..._blank` (debt). Cmd: native history row in `run_networth_history_test.go`

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on new/changed exported symbols; trim `seedNetWorthStore` doc (`cmd/quarry/run_networth_test.go:25-28`, debt)

### Verify
- [ ] Step 8: full verification + `spec-check.py phase4c-networth` → tick SCENARIO-12 with its acceptance test; STATE.md rewrite (clear the four debts closed here)

## Handoff

**Orchestrator: copy ruling 2026-10-04 (product-vision) on "Unruled shape" — recorded in `specification.md` → *Text, history* → Copy ruling.** Defaults 1 and 3 confirmed; item 2 changed: in native mode a date with no row in any currency gets ONE line with the date only (Currency and other cells blank), not no line. Zero-sum cells show `0.00`; column presence decided by native balance.


**Binding decisions** — a later scenario must not contradict these without saying so:
- History = one `store.NetWorth` call with every month end (`NetWorthParams.Dates`); S14a/14b/16 add facts to that same read — one-read rule.
- `NetWorthRequest.Window` / `NetWorth.Window` (`*store.Window`, nil = snapshot) is the mode switch; S15's `--as-of` sets `AsOf` with nil `Window`; document derives `as_of` vs `since`/`until` nulls from it.
- JSON `since` = resolved since (first day of the period), `until` = clamped until; caption uses first and last listed month ends — different on purpose (ruled example).
- Every requested date has a `Dates` entry, rows or not; JSON keeps empty dates.
- Rows join dates by calendar day, not `time.Time` equality.

**Left unbuilt** — named so nobody assumes it exists:
- Future-`--since` refusal (new `WindowError` kind) and `--as-of` × `--since`/`--until` conflict — SCENARIO-15; replacement site is the since > clamped-until path in `report.ParseMonthEndWindow`.
- History warnings (unpriced on N month ends, rates on N month ends, empty history) — 14a/14b/16. `no rate` cell in history — 14b.
- MCP `net_worth` since/until — SCENARIO-17.

**Traps** — things that look right and are not:
- `AddDate(0, 1, 0)` from Jan 31 lands on Mar 3 — derive a month end as first of next month minus one day.
- `report.ParseWindow` refuses a lone future `--since` with spend's "pass --until" line; networth must not call it.
- `reportFlags.bind` registers `--account`; networth has none, and `Test_each_reports_window_flags_describe_what_it_does_with_them` requires one — pin networth's help in `run_networth_surfaces_test.go` instead.

## Phase report

Run A done (steps 1-2). Acceptance test red at its stdout assertion, for the expected reason: the stubbed command prints the snapshot (`Net worth on 2026-03-12, amounts in CAD` + Type/Currency table) instead of the history.

- `cmd/quarry/run_networth_history_test.go` (new): `Test_run_networth_lists_each_month_end_with_a_column_per_type_ending_with_today`, `seedNetWorthHistoryStore`, `netWorthHistoryLine` (`%-10s  %8s  %11s  %8s`, chequing/credit_card columns only; a later run adding other columns needs its own line helper). Expected totals checked against the snapshot: today's Total 2,260.00 matches the history's last row.
- `internal/cli/networth.go`: `--since`/`--until` registered with Surface & Copy help verbatim, locals `since`/`until` unread until step 5.
- `internal/report/window.go`: `ParseMonthEndWindow(since, until *string, now time.Time) (store.Window, error)` signature-only stub (zero value, unused params; step 3 fills it and clears any lint).
- `internal/report/networth.go`: `Window *store.Window` on `NetWorthRequest` and `NetWorth`, unused.
- Green now: all of `internal/cli`, `internal/report`; cmd networth/help/window tests except the acceptance test. Nothing to undo; B1 starts at step 3.
