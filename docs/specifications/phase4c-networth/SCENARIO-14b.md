---
id: SCENARIO-14b
status: open
---

# SCENARIO-14b: Net worth warns about USD balances it cannot convert

Cadence: code-first (no mandatory test-first item: no write-safety guard, no atomic adapter, no bug fix)
Acceptance test: `cmd/quarry/run_networth_rates_test.go` `Test_run_networth_warns_when_a_usd_balance_has_no_exchange_rate_and_totals_it_apart`
Narrow loop: `go test ./internal/report/... ./internal/cli/ ./internal/store/duckstore/ -run 'NetWorth|net_worth|networth|NeedsRate' && go test ./cmd/quarry/ -run 'networth'`
Mutation checks: `NeedsRate` zero-balance exclusion → `Test_networth_warns_only_for_a_row_with_a_balance_no_rate_converts`; native-mode skip in `rateWarnings` → `Test_net_worth_native_listing_has_no_rate_warning`; snapshot-only extra total in `(NetWorth).total` → `Test_networth_history_totals_leave_out_a_row_no_rate_converts`
Runs: A (1) | B1 (2-3) | B2 (4) | B3 (5-6) | V (7-8)
Size: OWNS A RUN — 3 batches, `internal/report` (+ `document`) over `store` / `duckstore` / `cli` renderers; no new package, no wiring change

## Implementation Plan

Surface already surveyed (no new port): `Store.NetWorth` is the one read; implementers are `duckstore.(*Store).NetWorth` (`networth.go:29`) and `report` `fakeStore` (`fakes_test.go:122`); a new field on `store.NetWorth` touches neither signature. `Server.NetWorth` copies read fields at `report/networth.go:72-88`. `cli/networth.go:70` already calls `document.NetWorthWarnings`; no RunE change (parsing stays before `openReport`).

### Acceptance (red)
- [x] Step 1: new `cmd/quarry/run_networth_rates_test.go` `Test_run_networth_warns_when_a_usd_balance_has_no_exchange_rate_and_totals_it_apart` — `seedNetWorthStore` (`run_networth_test.go:26`, first rate Mar 10), `networth --as-of 2026-03-05` at `holdingsClock()`: stdout rows show `no rate` in In, CAD In total, then a `Total  USD` row (Currency/Balance cells filled, In blank); stderr is the one before-first-rate line (`USD balances on 2026-03-05, before 2026-03-10, ...`); exit 0. No stubs needed (command-slice only). Must fail at the stdout assertion.

### Build
- [x] Step 2 (B1): `store/store.go:~650-665` `store.NetWorth` + `duckstore/networth.go:29-75` — add `FirstRate time.Time` read in the same `openRead` via `firstRate(ctx, db)` (`charges.go:75`, `Holdings` pattern `holdings.go:57-62`), after the unvalued query, `openFault` on error; zero when no rates; empty-`Dates` early return stays. Tests in `net_worth_read_test.go`: first rate read / zero without rates; fault: `read_faults_test.go:44` NetWorth row must reach the new call (inject failure on that query, same error shape as the other `rowReads`).
- [x] Step 3 (B1): `report/networth.go:36-93,155-173` — `NetWorth.FirstRate` copied from the read (same open: extend `Test_networth_reads_the_store_once...` `networth_test.go:41`); new `(NetWorth).NeedsRate(row)` = converted listing, `Converted(row)==nil`, `Balance != 0` (mirrors `Holdings.NeedsRate`); `total` snapshot (`Window == nil`, converted) appends, after the reporting-currency total (absent when no row converts), one `NetWorthTotal` per other currency summing `Balance` of rows needing a rate, via `nativeNetWorthTotals` order. History `Totals` unchanged (reporting total only). Tests: rewrite `networth_test.go:104-121` (now yields the USD total), add `Test_networth_warns_only_for_a_row_with_a_balance_no_rate_converts` (zero-balance no-rate row ignored; native listing never needs a rate; CAD-in-USD symmetric), `Test_networth_history_totals_leave_out_a_row_no_rate_converts`; new `TypeNeedsRate(date, type)` helper (type has a row needing a rate and none converts) for the history cell.
- [x] Step 4 (B2): `document/holdings_left_out.go:15-17` `NetWorthWarnings` appends `rateWarnings(n)` after `unvaluedWarnings` (rate line last); new `document/networth_rate_warnings.go`: none in native mode or when no row needs a rate; `FirstRate` zero → no-rates line; snapshot → `<OTHER> balances on <as-of>, before <FirstRate>, the first exchange rate in the store, are not converted to <CUR> and are left out of the <CUR> total; pass --currency native to list them`; history → `<OTHER> balances on N month ends before <FirstRate>, ...` with N = listed dates having a row needing a rate (`humanize.Count(N, "month end", "month ends")`); `<OTHER>` = `money.NativeOf(n.Currency)`. Tests in `document/networth_test.go`/new file: three CAD variants verbatim, USD-reporting snapshot (`CAD balances on ... not converted to USD ... USD total`), native none, N=1 and N>1, a date with no-rate rows counted once however many types, order after a no-price line, `[]` never nil; `NewNetWorth` JSON: `converted_balance` null for the no-rate row and `totals` carries `{currency:"USD", value}` after the reporting total (`networth_test.go:137`).
- [ ] Step 5 (B3): `cli/render_networth.go:20-95` — snapshot In cell `no rate` where `n.NeedsRate(row)` (blank stays for no-row/zero rows; reuse the cell string of `holdingNoRateCell` `render_holdings.go:20`, rename to a shared name if cheap); `netWorthTotalRow` chooses by `total.Currency == n.Currency.String()` (holdings pattern `render_holdings.go:50-54,62-67`): reporting total in the last column, other-currency total under Currency/Balance with In blank. Rewrite `render_networth_internal_test.go:63-76` (blank -> `no rate`, `Total` CAD/USD rows) plus a case with only no-rate rows (no In total row, `Total USD` only).
- [ ] Step 6 (B3): `cli/render_networth_history.go:34-60` `convertedHistoryRow` — cell `no rate` when `n.TypeConverted==nil && n.TypeNeedsRate`, blank when the type has no row; a type with a converting row and a no-rate row shows the converting sum; total cell is `Totals[0]` only when it is the reporting currency's (history Totals hold no other). Rewrite `render_networth_internal_test.go:171-181`; add mixed-cell case. cmd tests in the acceptance file, each its own cell: history before first rate (`--since 2026-01 --until 2026-03` on the history fixture with first rate moved after Jan 31: `no rate` cells, N month ends in stderr), no-rates store, `--currency USD` (CAD balances, symmetric line), `--currency native` (no warning, no `no rate`, unchanged), `--json` (`converted_balance` null, `totals` order, `warnings[]` holds the line), exit 0 each. `--account` n/a: networth has no such flag.

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `NeedsRate`, `TypeNeedsRate`, `rateWarnings`, `FirstRate` (budgets per `clean-architecture`); update `NetWorth` doc comments that say rows with no rate "left out" without the new rows.

### Verify
- [ ] Step 8: full verification block + `.claude/scripts/spec-check.py phase4c-networth` → tick SCENARIO-14b with its acceptance test, rewrite STATE.md (drop the "interim" lines for no-rate cell and rate warning; the history `no rate` open debt closes)

## Handoff

**Orchestrator: copy ruling 2026-10-04 (product-vision) — recorded in `specification.md` Surface & Copy.** Decisions 1, 3, 4, 5 confirmed (USD-reporting warning by symmetry, verbatim lines in spec; mixed history cell = converting sum; zero no-rate rows silent; N=1 via `humanize.Count`). **Decision on history `totals` REPLACED:** history `Totals` carry the same as snapshot — reporting-currency total of converting rows (absent if none), then one total per other currency for no-rate rows; `convertedHistoryRow` selects the total whose currency is the reporting currency, never `Totals[0]`; a date whose rows all need a rate shows Total `no rate`.


**Binding decisions** — a later scenario must not contradict these without saying so:
- First-rate fact is `store.NetWorth.FirstRate` -> `report.NetWorth.FirstRate`, read in `Store.NetWorth`'s one open — S16/S17 reuse it (STATE: one read per command).
- A row "needs a rate" iff converted listing, `Converted(row)==nil` and `Balance != 0` (`NetWorth.NeedsRate`); zero-balance no-rate rows are invisible everywhere (text already drops them), so they raise no warning and no cell.
- `Totals` in a snapshot = reporting-currency total (only converting rows; absent if none) then one total per other currency of the no-rate rows; history `Totals` stay reporting-only, so JSON `totals` mirrors the text rows in both modes (literal reading of "follows the text Total-row order"). If product-vision wants history JSON to carry the unconverted total, it is a `total` change, not a renderer one.
- Warning 6 is composed in `document.NetWorthWarnings`, last of all kinds; the USD-reporting form (`CAD balances ... not converted to USD ... USD total`) is derived by symmetry via `money.NativeOf`, as holdings does; the spec text only names the CAD form.
- History cell: `no rate` iff the type has a row needing a rate and none converts; a mixed cell shows the converting sum silently (warning counts the month end).

**Left unbuilt**:
- Empty-result and empty-history warning line (goes before `unvaluedWarnings` in `NetWorthWarnings`) and the no-Total empty table — SCENARIO-16.
- MCP `net_worth` — SCENARIO-17; it must read `Totals` and `FirstRate` from `report.NetWorth`, not recompute.

**Traps**:
- `convertedHistoryRow` reads `Totals[0]`; it is right only because history `Totals` carry no other currency. Adding an unconverted total to history makes an all-no-rate date print the USD sum in the Total column.
- `report/networth_test.go:104` and `render_networth_internal_test.go:63,171` pin the interim blank; they must change, not be deleted.
- Copy for a ruling at the final product-vision pass: history N=1 reads "on 1 month end before" (singular noun, plural verb); the mixed-cell rule above.
- Hand-built `report.NetWorth` values in renderer tests carry `Totals` directly; set `FirstRate`/rows consistently or `NeedsRate` and `Totals` disagree.

## Phase report

Run A (done). `cmd/quarry/run_networth_rates_test.go`: const `netWorthBeforeFirstRateLine`, helper `netWorthNoRateLine` (netWorthLine with trailing spaces trimmed, for a blank In cell), the acceptance test. Red at stderr (no warning line) and stdout (blank In cells, no `Total USD  1,720.00`).

Run B1 (done, steps 2-3, tick them). Reuse `netWorthBeforeFirstRateLine` / `netWorthNoRateLine` in step-6 cmd tests; seed `seedNetWorthStore` (first rate Mar 10, USD 920.00 brokerage + 800.00 chequing on Mar 5).
- `store.NetWorth.FirstRate` (`store/store.go`), read last in `duckstore/networth.go` `NetWorth` via `firstRate` (third query; `Test_net_worth_for_no_dates_runs_no_query` now counts 3). Tests `net_worth_read_test.go`: first rate / none / query fault / scan fault (`passQueries: 2`).
- `report.NetWorth.FirstRate`, `NeedsRate(row)`, `TypeNeedsRate(date, type)` (`report/networth.go`). `total` now, per the orchestrator's ruling, in snapshot AND history: reporting-currency total of converting rows (absent when none), then `nativeNetWorthTotals` of the rows needing a rate (their own `row.Currency`, not `NativeOf(reporting)`). History `Totals` are no longer reporting-only, so the replaced mutation check `Test_networth_history_totals_leave_out_a_row_no_rate_converts` is `Test_networth_history_totals_carry_a_total_for_the_rows_no_rate_converts`.
- `report/networth_test.go`: `typedRow` now carries `Date: netWorthDay`; old no-total test rewritten (`..._has_no_reporting_currency_total_when_no_row_converts`); new NeedsRate / TypeNeedsRate / FirstRate-copy tests. Did not extend the reads-once test: it pins params, the copy is its own test.
- Mutations (restored, diff clean): drop `&& row.Balance.Sign() != 0` -> reds `..._warns_only_for_a_row_with_a_balance_no_rate_converts/a_row_with_a_zero_balance`, `_type_needs_a_rate.../its_only_row_has_a_zero_balance`, `..._sum_of_the_converted_balances.../a_row_with_no_conversion_and_a_zero_balance_adds_no_total`; drop the no-rate append in `total` -> reds the total, no-reporting-total and history-totals tests.
- Green now: report, document, cli, duckstore narrow loop. Red: only the acceptance test (needs B2 warning, B3 cell/Total row).
- B3 MUST: `convertedHistoryRow` reads `Totals[0]`; history `Totals` can now start with a non-reporting currency (all-no-rate date), so select by currency == reporting currency and print `no rate` when absent (step 6). No cmd test pins that today, so it is unguarded until B3.

Run B2 (done, step 4, ticked). `document/networth_rate_warnings.go`: `rateWarnings(n)` (none when `datesNeedingRate` is 0; no-rates line when `FirstRate` zero; else snapshot `on <as-of>, before …` / history `on N month ends before …` via `humanize.Count`; `<OTHER>` = `money.NativeOf(n.Currency)`), appended last in `NetWorthWarnings` (`holdings_left_out.go:15-18`, doc updated). Tests `networth_rate_warnings_test.go`: CAD snapshot / USD symmetric / no-rates (3 variants) / history N=1 and N>1 / month end counted once for several types / all-convert month end uncounted / nothing-needs-a-rate (`[]` not nil, zero-balance row) / native / order after no-price line / `NewNetWorth` JSON null converted balance + `totals` order.
- Plan said "native-mode skip in `rateWarnings`": there is no explicit skip there; native is already excluded by `NetWorth.NeedsRate` (`report/networth.go`, `n.Currency != money.Native`). Mutation (drop that clause, restored, diff clean) reds `Test_net_worth_native_listing_has_no_rate_warning` (both subtests).
- Green: report, document, cli, duckstore narrow loop, lint 0 issues on `./internal/report/...`. Red: only the acceptance test, now stdout only (blank In cells, `Total` row without `USD` currency cell); stderr line already matches. B3 owns `no rate` cell, `Total USD` row, history cell and the `Totals[0]` fix.
- B3 still MUST: `convertedHistoryRow` select the total whose currency is the reporting currency, `no rate` when absent. Reuse `rateDay`/`usdRow`-style helpers only within `document_test`; cmd tests reuse `netWorthBeforeFirstRateLine` / `netWorthNoRateLine`.

