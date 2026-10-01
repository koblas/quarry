---
id: SCENARIO-07
status: done
---

# SCENARIO-07: Store views carry exact converted amounts

Cadence: test-first — the rates hook inside Replace's temp-then-rename (`duckstore.go:280-332`)
Acceptance test: `cmd/quarry/run_sql_fx_test.go` `Test_run_sql_views_carry_each_amount_converted_at_its_dates_rate`
Narrow loop: `go test ./internal/platform/money/ ./internal/store/duckstore/ && go test ./cmd/quarry/ ./internal/cli/ -run '_sql_'`
Mutation checks: post-Refresh error return in `(*Store).Replace` → `Test_replace_does_no_build_work_after_a_rate_fetch_the_context_interrupted`; abort-on-`FetchError` inverted into Replace → `Test_replace_swaps_in_the_store_when_the_rate_fetch_fails`; fx_rates append dropped from Replace → `Test_replace_stores_each_fetched_rate_with_its_series`
Runs: A (1-2) | B1 (3-5) | B2 (6) | V (7-8)
Size: OWNS A RUN — 4 batches, 1 feature package (duckstore) + new leaf `internal/platform/money`

Acceptance test lives in cmd/quarry, not duckstore: the When is "query with quarry sql", `replaceStore` (`run_helpers_test.go:74-80`) already seeds view tests without Quicken (`run_sql_views_test.go:76`), and a sibling `replaceStoreWithRates` seeds rates through `duckstore.WithRates` + a fake fetcher, so no sync (01) is needed. One `quarry sql` query UNION ALLs the three views plus `typeof` of each converted column. Run A's red is the binder error "amount_cad not found" (exit 1 ≠ 0).

Probe (done at plan time, DuckDB v1.5.5): DECIMAL/DECIMAL and DECIMAL/INTEGER → DOUBLE; `round(DECIMAL, 2)` is half-away (0.125→0.13, -0.125→-0.13); HUGEINT `//` truncates toward zero (-7//2 = -3); the formula below returned 0.13/-0.13/0.12/-0.12 for ±0.20, ±0.19 CAD @1.6, typed DECIMAL(18,2). Results: Traps.
- CAD = `CAST(round(CAST(amount AS DECIMAL(38,2)) * usd_cad, 2) AS DECIMAL(18,2))`
- USD: with c = cents HUGEINT, r = rate micro-units HUGEINT, `CAST(CAST(sign(c) * ((abs(c) * 2000000 + r) // (2 * r)) AS DECIMAL(38,0)) * 0.01 AS DECIMAL(18,2))`

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_sql_fx_test.go` (new) `Test_run_sql_views_carry_each_amount_converted_at_its_dates_rate` + `replaceStoreWithRates`, `fakeRates`. Fixture: CAD and USD accounts; rates on Fri 2026-01-02 and Mon 2026-01-05; a Saturday txn; a txn on a rate date; USD 0.10 @1.25 and CAD 0.20 @1.6 (half-cent), with negatives; a CAD and a USD txn before the first rate
- [x] Step 2: signature-only stubs — `internal/platform/money` (new: `doc.go`, `Rate`, `Currency`), `internal/store/store.go:236-251` beside `Replaced` (`Rate`, `RatesRequest`, `RatesRefresh`, `DateSpan`), `duckstore.go:79-118` `RatesSource` port + `WithRates` (no-op)

### Build
- [x] Step 3: `internal/platform/money/money.go` (new) `Convert` + `Currency` (CAD/USD/Native) + `money_test.go`. Pin in-package: identity per currency and to Native with rate 0; USD→CAD and CAD→USD at ±0.125 (→ ±0.13) with a just-below-half control (→ 0.12) each way; zero amount; cross-currency with rate 0 or negative → !ok; Native as source → !ok; largest DECIMAL(18,2) amount at a rate below 1 without overflow (out-of-range result: n/a, cannot be stored)
- [x] Step 4: schema v5 — `schema.go:79-105` import_runs + `rates_first DATE, rates_last DATE, rates_fetch_error VARCHAR`; `schema.go:119-123` new `fx_rates (date DATE PRIMARY KEY, usd_cad DECIMAL(10,6) NOT NULL CHECK (usd_cad > 0), series VARCHAR NOT NULL)`; `duckstore.go:25` `FormatVersion` 5; `history.go:36-39` `optionalRunColumns`, `history.go:220-250` `carriedRun` +3 nullable; `duckstore.go:562-578` `importRunRows` +3 cells (new run: NULL). Tests: `duckstore_test.go:181` expects literal "5"; `query_test.go:27-32` `storeRelations` + `fx_rates`; `history_test.go:96` v4 store carries NULL in the 3 columns; new v5 round trip carries non-NULL values (editStore UPDATE)
- [x] Step 5 (test-first): `duckstore.go:79-118` `Store.rates` + `WithRates`; `duckstore.go:297-310` after `build`: call `Refresh` (Need = [earliest `rows.Transactions` date, today], Have zero), return on its error, append `fx_rates`, then `store_info` (moved out of `build` `:406`, so store_info stays last; `duckstore_internal_test.go:26` follows). Tests in `rates_test.go` (new): `Test_replace_stores_each_fetched_rate_with_its_series`; no `WithRates` → empty fx_rates; request carries earliest txn date and today; `Test_replace_swaps_in_the_store_when_the_rate_fetch_fails` (FetchError + partial rates → inserted, swapped) and its zero-rate arm (swapped, fx_rates empty); `Test_replace_does_no_build_work_after_a_rate_fetch_the_context_interrupted` (fetcher calls its own cancel, waits on `ctx.Done()`, returns `ctx.Err()`; a `faultDB` (`duckstore_test.go:479`) counter sees no fx_rates append and no `CheckpointClose`; previous store byte-identical; `ErrorIs context.Canceled`). Faults: `appendFaultTable: "fx_rates"` → previous store kept, no partial; a rate outside DECIMAL(10,6) → `duckdb.Decimal` fault, same outcome
- [x] Step 6: views — `schema.go:128-144` `v_account_balances` + `balance_cad`, `balance_usd` (sum first, then convert at the latest rate on or before `current_date`); `schema.go:151-171` `v_cash_flow` + `amount_cad`, `amount_usd`, `usd_cad` (ASOF LEFT JOIN fx_rates); `schema.go:174-181` `v_spending` + `spent_cad`, `spent_usd`, `usd_cad`. New `views_fx_test.go`, one row per arm: weekend → Friday rate; on a rate date → that rate; ±0.125 each way; same-currency identity with no rates and before the first rate; cross-currency before the first rate → NULL; `usd_cad` set on CAD rows; investment account NULL balance stays NULL; a 2099 rate never applies to balances; spent_* = −amount_* per row. `Test_money_convert_matches_every_spending_rows_converted_columns` (every v_spending row, both targets). Column/type pins: extend `views_test.go:223-244`, `:349-370`; add one for `v_account_balances`. `internal/cli/sql.go:39-40` ruled paragraph (Surface & Copy → sql Long) inserted right after "…negative is money leaving the account.", rewrapped; pin verbatim at `sql_test.go:191-215`

### Sweep
- [x] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `money`, `Convert`, `RatesSource`, `WithRates`, `store.Rate*`; `doc.go` (duckstore) names fx_rates

### Verify
- [x] Step 8: full verification + `.claude/scripts/spec-check.py phase2f-fx` → tick SCENARIO-07 with its acceptance test

## Handoff

**Binding decisions:**
- Port, frozen. 01, 03 and 04 fill these fields; they must not reshape them. `*fx.Server` must satisfy the port with no adapter, so the types live in driver-free `internal/store`. fx must never import duckstore, which links DuckDB.
  - `duckstore.RatesSource{ Refresh(ctx, store.RatesRequest) (store.RatesRefresh, error) }`
  - `store.RatesRequest{Need, Have store.DateSpan}`, where `DateSpan{First, Last time.Time}`; the zero value means empty.
  - `store.RatesRefresh{Rates []store.Rate; Added int; FetchError string; Partial bool}`
  - `store.Rate{Date time.Time; USDCAD money.Rate; Series string}`
- Error contract: Refresh returns non-nil error ONLY when ctx ended. Network, HTTP, timeout and parse failures come back as `FetchError` (ruled `<reason>`) with a nil error, and the store is still swapped (03 relies on this). Refresh never returns a rate ≤ 0 or a date inside `Have`. The PK and CHECK on fx_rates fail the build loudly if it does.
- `money.Rate` = CAD per 1 USD in millionths, `DECIMAL(10,6)` in fx_rates. `money.Convert(cents, from, to, rate) (int64, bool)` is the one Go conversion (18's Usual), at parity with the views.
- Replace order: build (no store_info) → Refresh → append fx_rates → store_info → CheckpointClose → ctx gate `:312` → rename. store_info stays last.
- Need.First = earliest `rows.Transactions` date; Need.Last = today's local date. Have is zero until 04 carries.

**Left unbuilt:**
- `duckstore.WithRates` wiring in `cmd/quarry/run.go:62` and the fake-fetcher test default → 01
- `Replaced` rates fields; writing `rates_first`/`rates_last` → 01, `rates_fetch_error` → 03
- fx_rates carry and `Have` from readHistory → 04
- `money.ParseCurrency` (flag/config parse) → 16

**Traps:**
- DECIMAL × anything keeps width 18 and overflows (`amount*100` too). Widen to DECIMAL(38,·) before multiplying.
- The new import_runs row is appended inside `build`, before the fetch. So 01/03 must UPDATE its rates columns after Refresh.
- A carried row must have 28 cells, or the v5→v5 Appender fails.
- `current_date` cannot be moved in tests. Use past rates, plus a 2099 rate to prove the cutoff.
- Not verified: DuckDB `current_date` uses the local TimeZone, which is how Need.Last is computed.

## Phase report
Run B2 done (step 6). Acceptance `Test_run_sql_views_carry_each_amount_converted_at_its_dates_rate` is GREEN (passed on first run of the views). Next: V (steps 7-8).

Files:
- `internal/store/duckstore/convert_sql.go` (new): `convertedTo(target, amount, currency, rate)` SQL fragment (CAD: cast of DECIMAL(38,2) * rate; USD: HUGEINT half-away integer division). No explicit `round()`: the DECIMAL cast already rounds half away from zero and a `round` mutation could not redden any test.
- `schema.go`: `v_account_balances` (CTE `b` + `r` latest rate <= current_date) +`balance_cad`, `balance_usd`; `cashFlowViewDDL` is now a func (ASOF LEFT JOIN fx_rates) +`amount_cad`, `amount_usd`, `usd_cad` (set on every row that has a rate); `v_spending` +`spent_cad`, `spent_usd` (= -amount_*), `usd_cad`. A currency other than CAD/USD gets NULL in both.
- `duckstore.go`: call site `cashFlowViewDDL()`.
- `views_fx_test.go` (new, 20 tests incl. 6-row rounding table and the Convert-parity test); `views_test.go` column pins extended; `internal/cli/sql.go` + `sql_test.go`: ruled paragraph inserted after "leaving the account." and rewrapped.

Green: money, store/..., cli, cmd/quarry packages; `golangci-lint run ./...` 0 issues. Full suite not run.
Mutations (restored): `2000000 +`->`-` -> CAD-to-USD rounding rows + parity test; ASOF `>=`->`>` -> rate-date test + parity; drop `date <= current_date` in `r` -> 2099 test; `-amount_cad`->`amount_cad` -> spending negation + parity; `LEFT JOIN r ON false` -> 4 balance tests; USD branch -> EUR -> weekend/after-last/third-currency tests.
V to do: doc.go (duckstore) must name fx_rates and the converted view columns; doc comments on money/Convert/RatesSource/WithRates/store.Rate*; full verify; spec tick; STATE.md.
