---
id: SCENARIO-10
status: done
---

# SCENARIO-10: Cashflow converts each period

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_cashflow_fx_test.go` `Test_run_cashflow_converts_each_period_to_cad_by_default`
Acceptance test (SCENARIO-09, folded): `cmd/quarry/run_cashflow_invariant_test.go` `Test_run_cashflow_spent_equals_spend_total_in_every_reporting_currency`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/report/ ./internal/cli/ ./cmd/quarry/ -run '(?i)cash_?flow|currency'`
Mutation checks: NULL arm of the cashflow source (a split with no converted cell keeps its own currency, by month, by year and in Totals) → `Test_cash_flow_read_keeps_an_unrated_split_native_beside_the_converted_ones`; round-then-sum → `Test_cash_flow_read_rounds_each_split_to_the_cent_before_adding_them`; the `money.Native` placeholder → `Test_run_cashflow_converts_each_period_to_cad_by_default` (read `.claude/briefs/proof.md` → *Mutation verification*)
Runs: A (1-2) | B1 (3-5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (report) + duckstore CashFlow + cli; folds 09

Existence (go doc, grep): `CashFlowParams`/`CashFlowRequest`/`report.CashFlow` have no Currency. `cashFlowQuery` (`duckstore/cashflow.go:25-41`) reads `v_cash_flow` literally, and Totals already sort `ORDER BY grp, period_key, currency` (alphabetical, CAD first), so no sort change. No new port or adapter: a field on `CashFlowParams` reaches `duckstore.Store.CashFlow` and `fakeReportStore` (`fakes_test.go:47`) unchanged.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_cashflow_fx_test.go` (new) — 10: store from `replaceStoreWithRates` (`run_sql_fx_test.go:32`, rate before every split, `rateOnJan2` in `run_spend_fx_test.go:30`), `cashFlowRows` (`run_helpers_test.go:222`); CAD + USD income and spending in two months. Text caption ends `, amounts in CAD`, one CAD row per month, one CAD Total, income/spent/net are converted sums (two 0.10 USD splits at 1.25 make 0.26 rounded each, 0.25 summed first); `--json` `currency` is `"CAD"` and every period/total `currency` equals it. Also `run_cashflow_invariant_test.go:28-85` (09): new `Test_run_cashflow_spent_equals_spend_total_in_every_reporting_currency` over {CAD, USD, native} on a rated store, `--account` and all-accounts, a window reaching before the first rate (NULL arm: both commands show the same CAD and USD Totals), and the half-cent rounding pair; compare `totalsColumn` per currency against a LITERAL `wantSpent` per case (as `:28` does), so a regression shared by both commands cannot pass. New function rather than rows added to `:28`'s table: that table's store has no rates and must stay the unrated control.
- [x] Step 2: no stubs (tests call `runWith` and existing helpers); confirm both fail at the caption / `currency` / Total assertion, not at setup.

### Build
- [x] Step 3 (duckstore): `internal/store/store.go:550` `CashFlowParams.Currency money.Currency`; `duckstore/cashflow.go:12-41` `cashFlowSource(currency)` beside `spendingSource` (`spending.go:19-33`): native is literally `v_cash_flow` (SQL text unchanged), CAD/USD project `amount_cad`/`amount_usd` as `amount` and the target as `currency`, except a split with a NULL converted cell keeps native `amount` and `currency` via `CASE`/`COALESCE`. The relation projects `account_id, date, currency, flow, amount` (`flow` passes through from `v_cash_flow`, `cashFlowQuery:36` reads it). Splice it at `:36` in place of `v_cash_flow`, income/expense/GROUPING SETS untouched; savings rate stays per currency row on converted cents. Do not extract a helper shared with `spendingSource` (its SQL stays untouched). No new fallible call, so existing fault tests (`cashflow_test.go:360-412`) cover it. New `cashflow_fx_test.go` via `newStoreWithRates` (`views_fx_test.go:42`); names are `Test_cash_flow_read_<…>` because `views_fx_test.go:57-157` and `views_test.go` already own `Test_cash_flow_*` view tests (same package: a clash does not compile). One pin per arm; the two mutation-check tests above are among them:
  - CAD, USD, native on a rated store: native equals the pre-change rows; all-CAD store in CAD equals native.
  - NULL arm in CAD and USD modes (split before the first rate; CAD split in USD mode), each in by month, by year, and Totals (CAD total ahead of the unconverted USD total).
  - Per-split rounding for income AND spent, with a sum-then-round control and a net that differs from converting the native net.
  - Savings rate from converted sums (a row where it differs from native).
  - Weekend (Friday rate), future-dated (latest prior rate), closed account, `AccountIDs` of one USD account in CAD, another currency stays native.
- [x] Step 4 (report + cli): `internal/report/cashflow.go:11-15,28-39,50-67` `CashFlowRequest.Currency`, `CashFlow.Currency`, passed into `CashFlowParams` and echoed; extend `report/cashflow_test.go:184` (request currency reaches the store params and the result). `internal/cli/cashflow.go:76,86` thread `reportCurrency` from `currency.resolve` instead of `_`. `render_cashflow.go:33` `windowCaption(..., c.Currency)` replaces `money.Native`. `json_cashflow.go:6-14,55-63` `Currency` after `By`, from `c.Currency.String()`. Tests:
  - `json_cashflow_internal_test.go:39,85`: add `"currency": "native"` (zero value) and a top-level key-ORDER pin `since,until,by,currency,account_filter,periods,totals,warnings`, identical in CAD and native.
  - `render_cashflow_internal_test.go:29,44,54`: stay native (no suffix); add a CAD/USD caption row.
  - New `cli/cashflow_currency_test.go` mirroring `spend_currency_test.go`: `gotCashFlow.Currency` per resolver arm (config, flag, flag beats config, native).
- [x] Step 5 (Long, edge matrix): `cashflow.go:46-60` Long becomes `Show income, spending and what was left over for each month or year.`, blank line, `reportCurrencyLong` (`currency.go:18`) unchanged, blank line, the rules paragraph with `…equals quarry spend's total for the same period, accounts and currency.`, then the savings paragraph. Pin verbatim in `report_help_test.go:75-98`. New table in `run_cashflow_fx_test.go` over {closed USD account, one USD `--account`, empty window, future-dated with `--until` past the last rate, weekend, before-first-rate} × {CAD, USD, native} × {text caption/Total, `--json` `currency` + totals}, plus `--by year`:
  - native × weekend and native × future-dated are n/a: native never reads `usd_cad`.
  - Before-first-rate × text pins the whole table: the USD Total makes `fillSeries` add a zero USD row to every period beside its CAD row, deliberately (see Traps).
  - Every silent row asserts stderr empty; before-first-rate shows USD rows under `amounts in CAD` with NO warning (interim, 12/13 add it).
  - Empty window: all three currencies in text AND `--json`: caption suffix and `currency` set, no totals, stderr is exactly the existing empty-window note.
  - `Test_run_cashflow_json_reads_back_with_every_amount_in_the_reporting_currency` (`encoding/json`): each period/total `currency` == `.currency`; the periods of each currency sum to its Total.
  - Not re-pinned: cross-currency transfer excluded (view).

### Sweep
- [x] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`. Doc comments on `CashFlowParams.Currency`, `CashFlowRequest.Currency`, `CashFlow.Currency`, `cashFlowSource`. Bump the caption pins that gain `, amounts in CAD`: `cmd/quarry/run_cashflow_test.go:45,83`, `run_cashflow_refusals_test.go:57`, `internal/cli/cashflow_test.go:65,81,234`. Bump the JSON pin `cmd/quarry/run_cashflow_json_test.go:35` (add `"currency": "CAD"` after `by`). Replace the pinned cashflow Long in `report_help_test.go:75`.

### Verify
- [x] Step 7: full verification + `spec-check.py phase2f-fx`; tick SCENARIO-10 with its acceptance test, tick SCENARIO-09 as `delivered by SCENARIO-10` with `Test_run_cashflow_spent_equals_spend_total_in_every_reporting_currency` last on the line; rewrite STATE.md (drop the cashflow trap and the cashflow "Left unbuilt" item).

## Handoff

**Binding decisions:**
- **`cashFlowSource` mirrors `spendingSource`: a split with no converted cell stays on a row of its own currency, chosen in the same statement.** A mixed row or Total is impossible. 12/13 add counts and warnings only and must not regroup.
- **Native reads `v_cash_flow` byte-identically** (the 11 outline's premise).
- **Savings rate is computed per currency row on converted cents** (as today), never converted.
- **Cashflow JSON `currency` sits right after `by`; the caption comes from `windowCaption(…, c.Currency)`.** The Long reuses `reportCurrencyLong` unchanged.
- **Net is converted income minus converted spent, each split rounded first.** This is what keeps spend Total = cashflow Spent.

**Left unbuilt:**
- The before-first-rate and no-rates warnings and their counts (cashflow shares spend's wording): 12/13.
- `RecurringRequest`/`AnomaliesRequest`/accounts currency, and the `money.Native` placeholders at `render_recurring.go:81` and `render_anomalies.go:48`: 17-19.

**Traps:**
- The existing invariant test (`:28`) is green only because `replaceStore` seeds no `fx_rates`. Seeding rates there turns it into a CAD-mode test that expects unconverted USD totals. New cases use `replaceStoreWithRates`.
- On an unrated store CAD and native are identical, so a "converted" pin proves nothing there.
- `render_cashflow_internal_test.go` / `render_escape_internal_test.go:118` build `report.CashFlow` with zero Currency: native, no suffix. Do not bump them.
- A USD Total from a pre-rate split makes `report` `fillSeries` (over `currencyList(Totals)`) add a zero-valued USD row to EVERY period in CAD mode. Accepted for now and pinned; 12/13 and the final product-vision should rule on it.
- `-run` is case-sensitive; keep `(?i)`.

## Phase report

Run V done. Covered full suite rc=0; `uncovered-diff.py` 0 uncovered added lines since 31c4648; `-race` on duckstore, report, cli, cmd/quarry rc=0; `golangci-lint run ./...` 0 issues; `spec-check.py phase2f-fx` OK. SCENARIO-10 and SCENARIO-09 (delivered by 10) ticked in specification.md; STATE.md rewritten; steps 6-7 ticked; status done.

Counts (`test-stats.py --base 31c4648 --changed`): cmd/quarry 455 (+6), internal/cli 378 (+3), internal/report 233 (+1), internal/store/duckstore 432 (+15); TOTAL 1498 (+25).

Open for the final product-vision pass (recorded in STATE.md Open debts): zero USD row from `fillSeries` beside CAD in each period when one USD split predates the first rate.
