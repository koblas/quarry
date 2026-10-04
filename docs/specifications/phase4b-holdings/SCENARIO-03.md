---
id: SCENARIO-03
status: open
---

# SCENARIO-03: quarry holdings lists today's holdings in the reporting currency

Cadence: code-first — nothing on the mandatory set (read-only command, no write guard, no adapter)
Acceptance test: `cmd/quarry/run_holdings_test.go` `Test_run_holdings_lists_todays_holdings_in_the_reporting_currency`
Narrow loop: `go test ./internal/report/... ./internal/cli/ ./internal/platform/humanize/ ./cmd/quarry/ -run 'Holdings|holdings|Today|Thousands|BigMoney|Usage|Refuse|Help|Currency|Config|Interrupt'`
Mutation checks: currency pick in `report.Holdings.Converted` (CAD→`ValueCAD`, USD→`ValueUSD`) → `Test_holdings_converts_each_value_to_the_asked_currency`; nil skip in the Total sum → `Test_holdings_total_leaves_out_values_with_no_conversion`
Runs: A (1-2) | B1 (3-5) | B2 (6-7) | V (8-9)
Size: OWNS A RUN — 5 batches, 1 feature package (`report` + `report/document`; cli/cmd wiring not counted)

Spec: `specification.md` SCENARIO-03; copy from S.1, S.2, S.3 (order only), S.5; H-3. Contract: `quarry holdings [--currency CAD|USD|native] [--json]` → table on stdout, exit 0; warnings on stderr as `quarry: warning: …`; no store / old format / interrupt → existing refusals, exit 1; positional arg, bad or valueless `--currency`, unknown flag → existing usage lines, exit 2. `--as-of` (S05) and `--account` (S10) are NOT registered here.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_holdings_test.go` (new) `Test_run_holdings_lists_todays_holdings_in_the_reporting_currency` — `replaceStoreWithRates` (`run_sql_fx_test.go:32`) store with a CAD brokerage, a USD account and a closed account each holding a priced security (one ticker equal to its name, one differing). Run through `runWith` + `spendEnvAt(…, 2026-03-12 12:00 UTC)` (a past date: DuckDB `current_date` trap). Assert the whole stdout (caption `Holdings on 2026-03-12 in all accounts, amounts in CAD; cash not included`, header, rows, `Total` + In CAD sum), empty stderr, exit 0
- [x] Step 2: signature-only stubs — `internal/report/store.go:11-32` `Store.Holdings(ctx, store.HoldingsParams) (store.Holdings, error)` (same signature as `duckstore`); `Holdings` on `fakeStore` (`internal/report/fakes_test.go:11-95`, it does not embed the port, so the build breaks without it) and on `fakeReportStore` (`internal/cli/fakes_test.go:18-77`; it embeds the port, so the `currencyCommands` loop panics on nil without it), each with a `holdings` field and a `gotHoldings *store.HoldingsParams` recorder; `(*report.Server).Holdings` stub; `newHoldingsCommand` stub with the ruled Short and registration at `internal/cli/root.go:28-39` with `env.Now`. The root-help pin `cmd/quarry/run_status_test.go:123-137` goes red here and stays red until Step 7; B1 does not fix it

### Build
- [x] Step 3: `internal/report/holdings.go` (new) `HoldingsRequest{AsOf, Currency}`, `Holdings{Rows []store.Holding, Totals []HoldingsTotal, AsOf, Currency}`, `HoldingsTotal{Currency string, Value *big.Int}`, `(Holdings).Converted(store.Holding) *big.Int`, `(*Server).Holdings` (one store read; error → `readRefusal(ctx, "holdings", err)`); `internal/report/window.go:76-84` extract exported `Today(now)` (now's calendar day in now's own zone, as UTC midnight) and have `DefaultWindow` use it. Tests `internal/report/holdings_test.go`: AsOf passed through (recording fake); `Test_holdings_converts_each_value_to_the_asked_currency` (CAD, USD, native → nil, one row each); `Test_holdings_total_leaves_out_values_with_no_conversion`; total sums `*big.Int` past `MaxInt64`; sole zero-value contributor still gets a total `0`; all rows nil-converted → no total; negative value counts; empty result → no total; fault rows: `*store.OpenError` → refusal copy, cancelled ctx → `holdings interrupted`, other error unchanged. `Today`: 23:30 at UTC−5 stays that day
- [x] Step 4: `internal/cli/holdings.go` (new) `newHoldingsCommand(newReport, loadConfig, now, jsonOut)` — Use `holdings`, Short/Long/Example verbatim from S.1 (Long wrapped as ruled), `Args: currency.args`, `--currency` with new `holdingsCurrencyHelp` at `internal/cli/currency.go:11-15` (S.1 text); RunE in the cashflow order (`internal/cli/cashflow.go:65-95`): resolve currency → `openReport` → `srv.Holdings(report.Today(now()))` → `document.HoldingsWarnings` → `emitReport` with `withConfigWarnings`. Tests `internal/cli/holdings_test.go`: help Short/Long/Example verbatim plus `--currency code` line by regexp (not the whole Flags section, so S05/S10 can add flags); AsOf from the clock's day (recording fake); `--currency USD` shows `In USD` from `ValueUSD`; store error → runtime error; failed stdout write (`failingWriter`) → `cannot write the result to stdout`; a config warning comes first on stderr and in `warnings[]`
- [x] Step 5: `internal/cli/render_holdings.go` (new) `renderHoldings`, `holdingsCaption` (shares `accountsCaption`, `render_table.go:63-73`), `holdingCells`, the single In-cell func `holdingInCell`, `formatPrice` (from `formatShares`, `render.go:316-332`, padded to ≥2 decimals), `formatBigMoney`; `internal/platform/humanize/humanize.go:5-13` extract `ThousandsDigits(string)` that `Thousands` calls; `internal/report/document/holdings.go` (new) `BigMoney(*big.Int) string` (ungrouped, the one owner of the cents split; `formatBigMoney` groups its whole part). Aligns L L R R L L R R; In column only when converted (accounts precedent `render_accounts.go:24-37`). Tests `render_holdings_internal_test.go` + `humanize_test.go` + `document/holdings_test.go`, one row each: ` (closed)` suffix; ticker nil / equal to name / differing; `\n` in account and security names escaped; currency NULL → `none`; price `31.420000`→`31.42`, `5`→`5.00`, `12.345678` kept, `0`→`0.00`, `1234.5`→`1,234.50`; nil price → `no price`, blank Priced on, blank Value, blank In; zero price → `0.00` and in the total; negative shares → negative value, in the total; CAD + USD rows with only the In sum on the Total row; old price and the `1899-12-29` placeholder shown as recorded; caption with USD; money `-0.05`, `-1,234.56`, a value past int64, `0.00`. Not-in-reports and linked-tracking accounts are listed with no left-out warning: holdings does not reuse the spend/cashflow/recurring composers (`document/warnings.go:51-146`)
- [ ] Step 6: `internal/report/document/holdings.go` `Holdings`, `Holding`, `HoldingsTotal`, `NewHoldings(h, warnings)` (S.5 keys in order; `holdings`/`totals`/`account_filter` `[]` from a nil slice; `shares`/`price` via `Shares`; nullable price, price_date, currency, value, converted_value, security, ticker; top-level `currency` = `h.Currency.String()`), `HoldingsWarnings(h) []string` (never nil, empty today); `internal/cli/json_holdings.go` (new) `renderHoldingsJSON`. Tests: read back with `encoding/json` (counts and values equal input, key order, null vs `""`); `converted_value` null in native; closed → `account_closed: true`; nil `Holdings` → `"holdings": []`
- [ ] Step 7: all-commands tables, one row each: root help `run_status_test.go:123-137`; `report_help_test.go:172-199` (+ `holdingsCurrencyHelp` const); `currency_test.go:23`; `run_read_usage_test.go:38-50` (`quarry: holdings takes no arguments`) and `:79`; `run_usage_test.go:212` and `:229-245`; `run_read_refusals_test.go:47-60`, `:90-98`, `:210-222`; `run_config_test.go:218-227` (config warnings first, R3 slot 1). n/a: `currency_test.go:104-110,259-266` (no other flag until S05); `run_investments_test.go:205-215` (spend/cashflow leakage only); older-format store (shared `readRefusal`, pinned by spend); drift and completion (derived from help, completion disabled)

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on the new exported symbols; add holdings to `internal/cli/root.go:7-10`, `internal/report/doc.go` and `internal/report/document/doc.go`

### Verify
- [ ] Step 9: full verification + `spec-check.py phase4b-holdings` → tick SCENARIO-03 with its acceptance test; rewrite STATE.md

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- One port call: `report.Store.Holdings` returns the same as `(*duckstore.Store).Holdings`; S08/S09/S10 extend `store.HoldingsParams`/`store.Holdings` and never add a second read (one read per command)
- `report.Holdings.Totals` is the only owner of H-3: a total exists for a currency when at least one row contributes a non-nil value, not when the sum is non-zero. S04 adds native per-currency totals (CAD, USD, then alphabetical) and S08 adds the unconverted per-currency totals after the converted one, both in this slice; renderers and the document only format it
- `report.Today(now)` is the as-of default; S05's `--as-of` parser defaults through it and passes the same `HoldingsRequest.AsOf`
- Warnings, R3 order: config warnings come first by structure (`currency.resolve` prints them; `withConfigWarnings` puts them first in JSON). `document.HoldingsWarnings` concatenates the rest in slots (2) non-investment `--account`, (3) empty, (4) no price, (5) no rates / before the first rate, (6) no currency, (7) other currency. Each scenario adds its composer in its slot; there are no stub composers
- Price cell groups thousands like `formatShares` (`1,234.50`): S.2 does not rule on grouping, so this pin is the decision
- `document.BigMoney` owns the `*big.Int` cents split; cli `formatBigMoney` groups its output. Neither goes through int64

**Left unbuilt** — named so nobody assumes it exists:
- `--currency native` arm: no In column ships here, but native per-currency `Total` rows and the caption without ", amounts in" belong to S04
- `--as-of` flag and its refusals (S05/S11); the `no price` warning and its JSON row pin (S06/S12); `not converted` (S07); `no rate` and unconverted totals (S08); empty-result warnings (S09); `--account`, its caption and a non-empty `account_filter` (S10); MCP `holdings` (S13)

**Traps** — things that look right and are not:
- `big.Int` `DivMod`/`Div`/`Mod` are Euclidean: -5 cents splits as -1 and 95. Take `Abs` first or use `QuoRem`
- `holdingInCell` must branch on row facts (security currency, whether a rate was needed), never on `Converted == nil`: nil also means no price, NULL currency, EUR and no rate. S07/S08 add their arms there
- `convertedCell` and `noRateCell` are already taken in `render_accounts.go:19,69`: use `holding*` names
- Sort is the ruled plain `a.name`; `quarry accounts` sorts `lower(name)`. Left for the final product-vision pass (STATE Open debts), not changed here
- A nil `Security` (dangling security row) renders as "" in text and `null` in JSON, the same as a missing account reads as ""

## Phase report

Run B1 (steps 3-5) done, green on the narrow loop. Acceptance `Test_run_holdings_lists_todays_holdings_in_the_reporting_currency` is green (fixture: `cmd/quarry/run_holdings_test.go`).
Red now, expected: `cmd/quarry` `Test_run_help_prints_quarrys_description` (root-help pin; `holdings` is now listed), Step 7 re-pins it. Nothing else fails in `./cmd/quarry/ ./internal/cli/ ./internal/report/...`.

Files:
- `internal/report/holdings.go`: `Holdings.Converted` (CAD/USD pick, nil otherwise), `(*Server).Holdings` (one read, `readRefusal(ctx, "holdings", err)`), `Holdings.total()` (H-3: a total exists when one row contributes a non-nil value; none in native). `internal/report/window.go:76-91` `Today` extracted, `DefaultWindow` uses it. Tests `holdings_test.go`, `window_test.go` `Test_today_...`.
- `internal/cli/holdings.go`: Long/Example verbatim, `Args: currency.args`, `--currency` with `holdingsCurrencyHelp` (`currency.go:14`), RunE in the cashflow order. `render_holdings.go`: `renderHoldings`, `holdingsCaption`, `holdingCells`, `holdingInCell`, `formatPrice`, `formatBigMoney`. Tests `holdings_test.go` (black-box), `render_holdings_internal_test.go`.
- `internal/platform/humanize/humanize.go` `ThousandsDigits` (+ test). `internal/report/document/holdings.go`: `BigMoney` and `HoldingsWarnings` (real, returns `[]string{}`); tests `document/holdings_test.go`.

Mutations (run, restored, diffed): `Converted` CAD and USD swapped -> `Test_holdings_converts_each_value_to_the_asked_currency/CAD_reads_the_CAD_value` and `/USD_reads_the_USD_value` red. Nil skip removed in `total()` -> `Test_holdings_total_leaves_out_values_with_no_conversion` red (nil dereference panic in `sum.Add`). Extra: `contributed` forced true -> `Test_holdings_has_no_total_when_no_row_converts` three subtests red.

Next run (B2, steps 6-7) must know:
- `internal/cli/json_holdings.go` `renderHoldingsJSON` is a signature stub returning `nil, nil`: Step 6 fills it. `document.Holding`, `HoldingsTotal`, `Holdings`, `NewHoldings` are not built; add them to `document/holdings.go` (`BigMoney`, `HoldingsWarnings` already there).
- Step 4's "config warning in `warnings[]`" is not asserted yet (needs JSON); `Test_holdings_prints_a_config_warning_on_stderr_and_still_lists_the_holdings` covers stderr only. Add `holdings` to `currencyCommands` (`currency_test.go:23`) in Step 7: its loops cover stderr-first and `warnings[]`.
- The `holdingsCurrencyHelp` test const is in `internal/cli/holdings_test.go`, not `report_help_test.go`; Step 7's row at `report_help_test.go:172-199` can use it. Short is already asserted through root help in `holdings_test.go`.
- `renderHoldings` already drops the In column and the ", amounts in" caption suffix for `money.Native` (one test); native `Total` rows are not built (`Totals` is empty in native).
- `holdingsCaption` calls `accountsCaption(nil)`: no `--account` flag exists.
- Lint on the new files has not been run (Sweep).
