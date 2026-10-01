---
id: SCENARIO-17
status: open
---

# SCENARIO-17: Recurring detects in native currency and shows converted amounts

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_recurring_fx_test.go` `Test_run_recurring_detects_in_native_currency_and_converts_at_the_latest_charges_rate`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/report/ ./internal/cli/ ./cmd/quarry/ -run '(?i)charges|recurring|help|first_rate|without_rates|warns'`
Mutation checks: uncovered-series guard in `report.seriesOf` → `Test_recurring_shows_a_series_whose_first_charge_has_no_rate_entirely_native`; native price-change judgement in `priceChangesOf` → `Test_recurring_finds_no_price_change_when_only_the_rate_moves`
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches, 1 feature package (`report`; duckstore is its Store adapter, as in 08/10/12) + cli

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_recurring_fx_test.go` (new) `Test_run_recurring_detects_in_native_currency_and_converts_at_the_latest_charges_rate` — on `replaceStoreWithRates`: a USD 10.00 monthly subscription whose rates move >5% between charges, with the first charge's rate ≠ the latest's, plus a rate dated after the latest charge, so converting at the store's newest rate gives a different number; one payee charging monthly in CAD and in USD. Text subtest: caption `, amounts in CAD`, `CAD (USD)` cell, empty price-change cell, Amount/Per year at the latest charge's rate, two rows for the payee, one CAD Total. `--json` subtest: `native_currency`, `native_amount`, `native_first_amount`, `price_changes[].currency`.
- [x] Step 2: `internal/store/store.go:617-645` `Charge` + `Charges`, `internal/report/recurring.go:118-175` `RecurringRequest`/`Series`/`Recurring` — signature-only fields (named in Handoff). Red must be at the assertion (caption or cell), not a compile error.

### Build
- [ ] Step 3: `internal/store/duckstore/charges.go:11-30` `chargesQuery`, `:35-58` `(*Store).Charges`, `:61-75` `scanCharge` — sum `spent_cad`/`spent_usd` per transaction from `v_spending`, plus `usd_cad` as exact millionths (`CAST(min(usd_cad)*1000000 AS BIGINT)`, never float), plus a third query on the same connection for `min(fx_rates.date)` → `Charges.FirstRate`. Tests in `charges_test.go`, each arm its own row:
  - USD charge between rates: both cells, and `USDCAD` equal to the view's.
  - Split USD charge whose half-cent splits round differently from their sum: per-split sum (R2).
  - CAD charge with no rates stored: `AmountCAD == Amount`, `AmountUSD` nil. This pins the identity the report relies on.
  - Before the first rate: cross cells nil, `USDCAD` 0. `FirstRate`: zero without rates, else the min date. Weekend charge takes the prior rate.
  - Fault tests: FirstRate query fault and scan fault (`passQueries: 2`). Existing range-fault tests at `:295-313` must keep hitting `transactionRange`.
  - Fakes (`internal/cli/fakes_test.go:54`, `internal/report/fakes_test.go:62`) return their fixture verbatim: no change needed.
- [ ] Step 4: `internal/report/recurring.go:186-212` `(*Server).Recurring`, `:216-242` `seriesOf`, `:264-272` `compareSeries`, `:283-296` `yearlyTotals` — detection stays native: group key, `latestRun`, `priceChangesOf` and `ChangeTenths` read `Amount` only. Display converts:
  - `Amount` at the latest charge's cell, `FirstAmount` at the first's, `PerYear` = converted Amount × charges a year; `Native*` always filled.
  - Pick the cell with an unexported `chargeIn(c, target)`: Native → no conversion; `c.Currency == target` → `Amount` (identity); else `AmountCAD`/`AmountUSD`.
  - A series whose first OR latest charge has no cell stays entirely native and counts in `Recurring.Unconverted.Transactions`, counting listed series only. `FirstRate` comes from the same `Charges` call.
  - New final sort tier: native currency (group key currency).

  Tests in `recurring_fx_test.go` (new), against the fake:
  - Arms: converted; first uncovered; latest uncovered (fake-only); same currency with nil cells (converted, not counted); native target; USD target picks `AmountUSD`.
  - `first_amount` uses the first charge's cell. PerYear is the converted Amount × count.
  - `Test_recurring_finds_no_price_change_when_only_the_rate_moves`, plus the reverse: native moves >5% while converted stays flat, so a change is found.
  - Counting: a filtered-out uncovered series (window, `--account`) adds nothing; an ended uncovered series counts.
  - Totals: converted series go to the target's Total, unconverted ones to a native Total, CAD first.
  - Sort: CAD and `CAD (USD)` interleave by converted PerYear; native-currency tie pinned in both detection orders. One read: `chargesReads == 1`.
- [ ] Step 5: `internal/cli/recurring.go:56,66` thread `currency.resolve`'s value into `RecurringRequest.Currency`. `:82-90` `recurringWarnings` adds `unconvertedWarnings(r.Currency, r.Unconverted)` in 12's order: config, left-out, FX, empty-window.
  - `internal/cli/fx_warning.go:16-38`: give the counted subject a parameter, so spend and cashflow keep `N transactions dated before`, recurring gets `N series with a charge dated before`, and 18 can pass `charges` with no rewrite.
  - Pin it in new `fx_warning_internal_test.go`: transactions arm unchanged; series arm at 1 (`is`) and 2 (`are`); no-rates arm; zero arm.
  - `render_recurring.go:63-82`: the Currency cell is `CAD (USD)` when native ≠ row. `:41-48` `priceChangesCell` reads `NativeFirstAmount`/`NativeAmount`, prefixed with the native code only when native ≠ row. Caption `windowCaption(..., r.Currency)` replaces the `money.Native` placeholder at `:81`.
  - Same batch, or B2 ends red: repoint the CAD-mode `Recurring charges` caption pins to `, amounts in CAD` in `internal/cli/{recurring,recurring_account,report_clock}_test.go`, `cmd/quarry/run_recurring{,_empty,_price,_state,_account}_test.go`, `run_charges_edges_test.go`; fill `Native*` in the fixtures at `render_recurring_internal_test.go:170,186,220`.
  - Internal render tests for: a same-currency row unchanged; `USD (CAD)` in USD mode; a new series (`, new`); an ended converted series (Per year blank, no Total).
- [ ] Step 6: `internal/cli/json_recurring.go:5-13` top-level `currency` right after `until`. `:15-33` adds `native_currency`, `native_amount`, `native_first_amount` right after `per_year`. `:41-47` `price_changes[].currency` right after `date` (= native currency). `:66-112` fills them.
  - Tests: key-order pin with `topLevelKeys` (`json_spend_internal_test.go:177`) for the top level, a series and a price change. Native mode has `currency == native_currency` and the same key set.
  - Add the `native_*` and price-change `currency` keys to the map-equality fixture at `json_recurring_internal_test.go:122`.
  - Read-back test: decode with `encoding/json`; assert the series count, converted and native values, `per_year` null for an ended series, and `price_changes` `[]` (not null) for a series with none.
  - `recurring.go:27-44` Long: insert the ruled paragraph after paragraph 1, and change `:42-43` "from one charge to the next." to "…, in the series' own currency.". Pin it verbatim with a new `Test_recurring_help_says_series_are_found_in_their_own_currency` in `report_help_test.go`.

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on the new fields.

### Verify
- [ ] Step 8: full verification + `spec-check.py phase2f-fx` → tick SCENARIO-17 with its acceptance test; rewrite STATE.md.

Edge rows × currency × format — cmd tests in `run_recurring_fx_test.go` on `replaceStoreWithRates` unless stated; text cells in Step 5, `--json` cells in Step 6, each row in both:
- USD series in CAD; same payee CAD+USD (Step 1). First charge before the first rate (USD row + USD Total + "series" line; json row currency ≠ `.currency`, `warnings[]`). Unrated USD series (no-rates line). Unrated all-CAD in CAD (silent). USD mode (`USD (CAD)`, `CAD ...` prefix).
- `--currency native` on a RATED USD store: text byte-equal to today; json `currency:"native"`, `native_*` == row values. Closed account converts. One USD `--account`, default CAD: caption names it + `amounts in CAD`, CAD Total. Empty window on an unrated USD store: empty note only, no FX line; json `currency` set, `totals []`. Ended series; new series.
- Config USD (caption `amounts in USD`): one cli test with a USD `LoadConfig`. Weekend / after-last-rate: n/a, view-owned (07, Step 3's weekend row).

## Handoff

**Binding decisions:**
- `store.Charge` gains `AmountCAD, AmountUSD *int64` and `USDCAD money.Rate`; `store.Charges` gains `FirstRate time.Time`.
  - The cells are currency-blind and `ChargeParams` is unchanged, so 18 reuses the port without reopening it.
  - Each cell is the per-split sum of v_spending's converted column; nil means no rate.
  - `USDCAD` 0 means no rate on or before the date. `money.Convert` reports false at rate <= 0 cross-currency.
- Report treats a charge in the target currency as its own conversion. Step 3's CAD/no-rates row is the store-side pin that keeps that shortcut honest.
- `report.Series` gains `NativeCurrency string, NativeAmount, NativeFirstAmount int64`, always filled. `Recurring` gains `Currency money.Currency` and `Unconverted store.Unconverted`; on `Recurring`, `Transactions` counts listed series.
- Per year = converted Amount × charges a year, never the native Per year converted. This is the spec's rule, so Per year cells sum to Total.
- A series with its first or latest charge uncovered is entirely native. With ASOF rates, latest-uncovered implies first-uncovered, so only a fake can build that arm.
- 11 is folded into 19 on the premise that 17 pins recurring native. This plan's rated `--currency native` row is that pin.

**Left unbuilt:** anomalies' `Usual` (`money.Convert(median, native, target, c.USDCAD)`) and its `charges` warning subject belong to 18.

**Traps:**
- `run_recurring_json_test.go:136` (unrated USD series, CAD default) now draws the no-rates line but asserts no warnings: leave it; never seed rates there.
- `spyReadDB.passQueries` order in Charges is rows, then range, then FirstRate.
- `sum()` skips NULLs. It is all-or-none only because a transaction's splits share date and currency.
- A `Series` fixture with zero `Native*` renders `CAD ()` and `0.00` in the price-change cell.
- `AmountCAD` is a sum of rounded splits, so for a split charge it can differ by a cent from `Convert(Amount, USDCAD)`.

**Unruled copy (orchestrator: one copy ruling before run A — the sort tier lands in B1):**
- Position of the `native_*` keys (after `per_year`) and of `price_changes[].currency` (after `date`).
- The `USD (CAD)` cell and the `CAD ...` prefix in USD mode. The spec shows only the CAD-mode form.
- The native-currency final sort tier (CAD row before `CAD (USD)` on a tie), derived from the edge row's order.
- Line wrapping of the inserted Long paragraph. The spec gives it unwrapped.

## Phase report

Run A done (acceptance red + signature-only stubs); acceptance test red at its assertions, on purpose, until B2.
- Acceptance fixture `recurringFXStore` (`run_recurring_fx_test.go`): USD Netflix 10.00 steady; Gym in CAD 20.00 and in USD 12.00 -> 15.00 (June); rates 1.30 (01-02), 1.40 (06-01), 1.50 (09-20, after the last charge). Wanted text rows: `Gym CAD (USD) 21.00 252.00 ... 1: USD 12.00 -> USD 15.00 (+25.0%)`, `Gym CAD 20.00 240.00`, `Netflix.com CAD (USD) 14.00 168.00` (no price change), `Total CAD 660.00`. Red now: caption lacks `, amounts in CAD`, rows are native, two Totals. JSON red: no `currency`/`native_*`/price-change `currency`.
- Stubs (zero values, no behaviour): `store.Charge.AmountCAD, AmountUSD *int64`, `USDCAD money.Rate`; `store.Charges.FirstRate time.Time`; `report.RecurringRequest.Currency`; `report.Series.NativeCurrency/NativeAmount/NativeFirstAmount`; `report.Recurring.Currency` and `.Unconverted`. Nothing reads them yet; cli does not thread `currency.resolve` yet.
- Deviation: the shared JSON decode structs in `cmd/quarry/run_recurring_json_test.go` gained `Currency` (doc, after `Until`), `NativeCurrency/NativeAmount/NativeFirstAmount` (series, after `PerYear`) and price-change `Currency` (after `Date`). Existing JSON tests leave them zero and pass today; B2 (Step 6) must fill them in every expected document there (`run_recurring_json_test.go`, `run_recurring_price_test.go:~30`, other cmd recurring JSON pins) once the renderer emits them.
- Narrow loop (all green except the acceptance test): `go test ./internal/store/... ./internal/report/ ./internal/cli/` ok; `go test ./cmd/quarry/ -run '(?i)recurring'` fails only the two acceptance subtests; `golangci-lint run ./...` 0 issues.
- Next (B1, Steps 3-4): the acceptance fixture's rates produce wanted values only when `Charges` fills `AmountCAD/AmountUSD/USDCAD/FirstRate` from `v_spending`; the unconverted counting and the native sort tier are pinned by Step 4's own tests, not by the acceptance test.
