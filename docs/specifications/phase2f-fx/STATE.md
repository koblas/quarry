# phase2f-fx — current state

Scenarios complete: SCENARIO-07. Last updated by SCENARIO-07.

## Binding decisions
- Rates port, frozen (01/03/04 fill fields, never reshape): `duckstore.RatesSource{Refresh(ctx, store.RatesRequest) (store.RatesRefresh, error)}`; `store.RatesRequest{Need, Have DateSpan}`, `DateSpan{First, Last time.Time}` (zero = empty); `store.RatesRefresh{Rates []Rate; Added int; FetchError string; Partial bool}`; `store.Rate{Date time.Time; USDCAD money.Rate; Series string}`. Types live in driver-free `internal/store` so `*fx.Server` satisfies the port with no adapter; fx never imports duckstore (SCENARIO-07)
- Error contract: Refresh returns non-nil error ONLY when ctx ended; network/HTTP/timeout/parse failures come back as `FetchError` with nil error and the store is still swapped. Refresh never returns a rate <= 0 or a date inside `Have`; fx_rates PK + CHECK fail the build loudly if it does (SCENARIO-07)
- `internal/platform/money` (leaf): `Rate` = CAD per 1 USD in millionths (`DECIMAL(10,6)` in fx_rates); `Currency` = CAD/USD/Native; `Convert(cents, from, to, rate) (int64, bool)` is the one Go conversion, half away from zero, parity with the SQL views, pinned by `Test_money_convert_matches_every_spending_rows_converted_columns`. `!ok` for Native source, unknown target, cross-currency with rate <= 0. `ParseCurrency` not built (16) (SCENARIO-07)
- Wiring: `duckstore.WithRates(src)`; without it fx_rates is empty. Replace order: build (no store_info) -> Refresh (Need.First = earliest `rows.Transactions` date, Need.Last = today local, Have zero) -> append fx_rates -> store_info -> CheckpointClose -> ctx gate -> rename. store_info stays last. Refresh error returns before any further build work (SCENARIO-07)
- Schema v5 (`FormatVersion` 5): `fx_rates(date PK, usd_cad DECIMAL(10,6) CHECK > 0, series)`; import_runs +`rates_first DATE, rates_last DATE, rates_fetch_error VARCHAR` (28 cells). v4->v5 carry fills NULL (SCENARIO-07)
- View columns (SQL fragment in `duckstore/convert_sql.go` `convertedTo`): `v_account_balances` +`balance_cad`, `balance_usd` (sum then convert at latest rate dated <= `current_date`); `v_cash_flow` +`amount_cad`, `amount_usd`, `usd_cad` (ASOF LEFT JOIN on `t.date >= r.date`, so weekends use the prior rate; `usd_cad` set on every row with a rate, CAD rows too); `v_spending` +`spent_cad`, `spent_usd` (= -amount_*), `usd_cad`. Same-currency identity needs no rate; cross-currency with no rate is NULL; currency other than CAD/USD is NULL; investment NULL balance stays NULL. CAD = DECIMAL(38,2) cast rounds half away; USD = HUGEINT integer division (SCENARIO-07)
- `quarry sql` Long carries the ruled converted-columns paragraph, pinned verbatim at `internal/cli/sql_test.go` (SCENARIO-07)

## Left unbuilt
- `duckstore.WithRates` wiring in `cmd/quarry/run.go` and the fake-fetcher test default - 01
- `store.Replaced` rates fields; writing `rates_first`/`rates_last` - 01; `rates_fetch_error` - 03
- fx_rates carry across rebuild and a real `Have` from readHistory - 04
- `money.ParseCurrency` (flag/config parse) - 16
- Every read command's converted output, `--currency`, status coverage - 06, 08-19

## Traps
- The new import_runs row is appended inside `build`, before the fetch, so its rates columns are NULL in 07: 01/03 must UPDATE them after Refresh (SCENARIO-07)
- DECIMAL x anything keeps width 18 and overflows (`amount*100` too): widen to DECIMAL(38,.) before multiplying; DECIMAL/DECIMAL is DOUBLE (SCENARIO-07)
- A carried import_runs row must have 28 cells or the v5->v5 Appender fails (SCENARIO-07)
- `current_date` cannot be moved in tests: use past rates plus a 2099 rate to prove the balance cutoff (SCENARIO-07)
- Test seeding: `replaceStoreWithRates` / `fakeRates` in `cmd/quarry/run_sql_fx_test.go` seed rates via `duckstore.WithRates` without a sync (SCENARIO-07)

## Open debts
- DuckDB `current_date` uses the local TimeZone, which is how Need.Last is computed - unverified; close in 01 or 12 with a zone-pinned test, else unowned - dies unless re-opened (SCENARIO-07)
- No checkpoint review has run on SCENARIO-07 yet (orchestrator step 5a)
