---
id: SCENARIO-02
status: open
---

# SCENARIO-02: v_holdings values each holding on each day held

Cadence: code-first (read-only view and reader; no write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_holdings_view_test.go` `Test_run_sql_values_each_holding_on_a_date_from_v_holdings`
Narrow loop: `go test ./internal/store/duckstore/ -run 'holding|query|reads' && go test ./cmd/quarry/ -run 'v_holdings|schema_reference|references'`
Mutation checks: `least(coalesce(to_date, current_date), current_date)` cap in `holdingsViewDDL` → `Test_holdings_view_stops_each_span_at_today`; ASOF price direction (`date >= p.date`) → `Test_holdings_view_takes_the_latest_price_on_or_before_the_date`
Runs: A (1) | B1 (2-4) | V (5-6)
Size: OWNS A RUN — 3 batches, 1 feature package (store + duckstore)

Spec: `specification.md` SCENARIO-02, H-2 (comment text R2 verbatim), H-3, H-5, S.6 rows on price/rate/split/negative/currency. No `FormatVersion` bump: `build` (`duckstore.go:454`) creates every view on each Replace, and no v7 store has shipped (STATE binding).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_holdings_view_test.go` (new) `Test_run_sql_values_each_holding_on_a_date_from_v_holdings` — build the store with `replaceStoreWithRates` (`run_sql_fx_test.go:32`) and `usdRate` (`run_anomalies_fx_edges_test.go:47`). Fixture: one CAD and one USD holding (accounts, securities, investment transactions); prices on days before, on and after the queried date; two rates that differ on consecutive days; a shares×price product that is not whole cents. Then `run(... "sql", "SELECT … FROM v_holdings WHERE date = '…'")`, following `run_sql_fx_test.go`. Assert price, price_date, value, value_cad, value_usd and usd_cad for both rows. No stubs: red = non-zero exit at the exit-code assertion, because `v_holdings` does not exist yet.

### Build
- [x] Step 2: `internal/store/duckstore/schema.go:245-254` (new `holdingsViewDDL` after `spendingViewDDL`) + `duckstore.go:454` (append it to the build Exec) — the view's day expansion and `COMMENT ON VIEW v_holdings` (H-2 R2 text verbatim).
  - **Shape:** `generate_series(from_date, least(coalesce(to_date, current_date), current_date), INTERVAL 1 DAY)` cast to DATE. ASOF LEFT JOIN `prices` on `security_id` and `date >= p.date`. ASOF LEFT JOIN `fx_rates` on `date >= r.date`. LEFT JOIN `securities`, so a holding is never dropped. No window, DISTINCT or ORDER BY above the expansion, so a `date` filter pushes down.
  - **Tests** in `holdings_view_test.go` (new), one row per arm:
    - closed span: `from_date` and `to_date` both included;
    - open span: last row is local today, no row for tomorrow (`Test_holdings_view_stops_each_span_at_today`);
    - a span closed after today (future sell): stops at today;
    - a future `from_date` span: no rows;
    - sold to zero and rebought: no rows in the gap;
    - two adjacent spans: one row per day, no duplicate date;
    - a negative-shares span: expanded;
    - `typeof(date)` = DATE;
    - a holding whose security row is missing: listed with NULL security, ticker and currency.
  - Also columns-in-order (sibling `views_test.go:352`) and the comment pin (sibling `views_test.go:329`).
  - **Forced by regen**, all in this batch:
    - add `v_holdings` to `query_test.go:29-35` `storeRelations`;
    - regenerate `plugin/skills/quarry/references/schema.md` with `go test ./cmd/quarry -run Test_skill_schema_reference_matches_the_committed_file -update`;
    - `run_skill_schema_reference_test.go:173` `require.Len(t, comments, 2)` → 3;
    - drop `v_holdings` from `:182`;
    - drop `holdings` from `phase4ViewPattern` at `run_skill_references_test.go:21`;
    - re-point the `:101` case at `v_balances_daily`, so each remaining alternative of the pattern keeps a row. This deviates on purpose from S.7's "remove its case".
- [x] Step 3: `schema.go` `holdingsViewDDL` value, `price` and FX columns + `convert_sql.go:31-46` `convertedTo`.
  - **Columns:** `value` = `CAST(shares*price AS DECIMAL(38,2))`. `value_cad` and `value_usd` are DECIMAL(38,2) via `convertedTo` over `securities.currency` and `r.usd_cad`. `usd_cad` = the rate in force on `date`, on every row whatever the currency, NULL before the first rate (as `v_cash_flow`).
  - **`convertedTo`:** its two branches hard-code a DECIMAL(18,2) result. Give it a result-type parameter, or make it a thin 18,2 wrapper over a sibling that takes the width, so one function still owns the half-away-from-zero rule. The four existing call sites (`schema.go:196-197,228-229`) keep DECIMAL(18,2), so their FX tests in `views_fx_test.go` stay green unchanged.
  - **Tests** in `holdings_value_test.go` (new), one row each, differing from a control in one variable:
    - **Price lookup:**
      - a price dated after the date is not used (`Test_holdings_view_takes_the_latest_price_on_or_before_the_date`);
      - another security's price is not used;
      - an old price: `price_date` shows its day;
      - no price ever: price, price_date, value, value_cad and value_usd all NULL;
      - zero price: value 0.00, used as recorded;
      - the 1899-12-29 placeholder price: used as recorded.
    - **Value and rounding:**
      - half a cent rounds away from zero, positive and negative shares;
      - 0.004999 rounds down;
      - negative shares give a negative value;
      - split mid-history: the value changes on the split day at that day's price;
      - the largest DECIMAL(18,6) shares × the largest price, positive and negative shares, a CAD row and a USD row: the value and both converted cells read back without error (`Test_holdings_view_values_the_largest_holding_without_overflow`).
    - **FX:**
      - CAD row: value_cad = value, value_usd converted;
      - USD row: the mirror;
      - rate gap (Saturday): Friday's rate (`fridayAndMonday`, `views_fx_test.go:51`);
      - after the last rate: the last rate;
      - before the first rate, and with no rates at all: a CAD row keeps value_cad = value with value_usd NULL; a USD row keeps value_usd = value with value_cad NULL; usd_cad is NULL;
      - NULL currency in a CAD account: all converted cells NULL, no fallback to the account currency;
      - EUR: both converted cells NULL (out-of-domain row);
      - security currency ≠ account currency: the security's currency wins.
- [x] Step 4: `internal/store/store.go` (types after `Price`, `:152-158`) `store.HoldingsParams{AsOf}`, `store.Holdings`, `store.Holding` + `internal/store/duckstore/holdings.go` (new) `(*Store).Holdings(ctx, store.HoldingsParams)`.
  - **What it does:** reads `v_holdings WHERE date = $1`, joined to `accounts` for name, source_id and closed, and to `securities` for source_id. Sorted per S.2: account name, account source_id, security name, security source_id. Shape and error handling follow `accounts.go:21-58` (`openRead`, `QueryRows`, `openFault`, `nullInt64Ptr`).
  - **Tests** in `holdings_test.go` (new):
    - every column read, NULL arms included (no price, NULL currency, NULL ticker, closed account);
    - each sort tier its own row, the final tiebreak included, plus a mixed-case name row that pins plain, non-folded order;
    - a date with nothing held: empty and no error;
    - a date after today: empty.
  - **Value types:** `Value`, `ValueCAD` and `ValueUSD` are `*big.Int` cents, nil when NULL. Read them as `CAST(… * 100 AS HUGEINT)`, because a value can pass int64 cents. `Price` and `Shares` stay int64 millionths, as `store.Price` does. Test: the largest-holding row from Step 3 reads back through `Holdings` with exact cents.
  - **Fault rows:** add `Holdings` to `rowReads()` and its doc comment (`read_faults_test.go:20-48`). This covers the open, query, scan and close faults.

### Sweep
- [ ] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`. Doc comments on `holdingsViewDDL`, `Holdings` and the `store.Holding*` types. Add `v_holdings` and `Holdings` to the view list in `duckstore/doc.go:6-14`. Bump any other exact relation or view list the full suite reports.

### Verify
- [ ] Step 6: full verification + `spec-check.py phase4b-holdings` → tick SCENARIO-02 with its acceptance test; rewrite STATE.md

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- **Reader signature:** `(*duckstore.Store).Holdings(ctx, store.HoldingsParams) (store.Holdings, error)`, reading only `v_holdings`. S03's `report.Store.Holdings` copies the signature. S08 (first-rate facts), S09 (transaction span) and S10 (`AccountIDs`) extend the params and result; they never add a second call (*One read per command*).
- **Same-currency conversion needs no rate:** `value_cad`/`value_usd` follow `convertedTo`, so a CAD row has `value_cad = value` even before the first rate, and a USD row has `value_usd = value`. H-2's "NULL when there is no rate" applies only when a conversion actually needs a rate. S08's `no rate` cell and warning must test that, not `value_cad IS NULL`; NULL also covers no price, NULL currency and EUR.
- **`usd_cad` on every row:** each row carries the rate in force on `date`, whatever its currency; NULL before the first rate.
- **No format bump:** the view is created at every build. A dev store built at v7 before this scenario lacks `v_holdings` until re-synced.
- **Sort:** S.2 tiers on plain names, not case-folded, as the 4a mismatch rows sort (orchestrator ruling).
- **No overflow on the value columns:** `value`, `value_cad` and `value_usd` are DECIMAL(38,2) (orchestrator ruling), so no overflow failure exists. |shares| and |price| are each < 1e12, so their product is < 1e24, and multiplying by the rate stays well inside 38 digits. `convertedTo` takes the result type and still owns the rounding rule. In Go the values are `*big.Int` cents. S03's renderers, `document.Holdings` and the H-3 totals must sum and format `*big.Int`, never int64.

**Left unbuilt:**
- `report.Store.Holdings`, `Server.Holdings`, the command and the document — SCENARIO-03.
- The `v_holdings` / `holding_shares` sentence in `report/sql_conventions.go`, and the SKILL.md copy — SCENARIO-15.

**Traps:**
- **Sort differs from `quarry accounts`.** `accountsQuery` (`accounts.go:16`) sorts `lower(name), name`, so the two commands can order mixed-case account names differently. Run V records this in STATE.md `## Open debts`.
- **`convertedTo`'s 18,2 result is the default for existing callers.** Widening it in place would change four views' column types and the regenerated `schema.md`.
- **Today is DuckDB's `current_date`.** Use `localToday()` (`accounts_test.go:19`) for "through today" arms. Fixed dates must not be after the real date.
- **`phase4ViewPattern` is shared.** The schema.md regen, the `Len` 2→3 change and both pin retirements fail in isolation, so land them together.

## Phase report

Run B1 (steps 2-4) done. Steps 1-4 ticked; steps 5-6 (Sweep, Verify) open for run V.

- `internal/store/duckstore/schema.go` `holdingsViewDDL()` (+ consts `holdingsViewComment`, `lastHeldDay`), appended in `duckstore.go` `build`. `convert_sql.go`: `convertedTo` is now a 18-digit wrapper over `convertedToWide(target, amount, currency, rate, digits)`; the view uses 38.
- `internal/store/store.go`: `HoldingsParams{AsOf}`, `Holdings{Holdings []Holding}`, `Holding` (cents `*big.Int`, `USDCAD money.Rate` zero when no rate). `duckstore/holdings.go`: `(*Store).Holdings`, LEFT JOINs accounts and securities so a holding is never dropped; order `a.name, a.source_id, v.account_id, v.security, s.source_id, v.security_id`.
- Tests: `holdings_view_test.go`, `holdings_value_test.go`, `holdings_test.go` (new); `read_faults_test.go` Holdings row; `query_test.go` `storeRelations`; `run_skill_schema_reference_test.go` Len 3 and `v_holdings` dropped; `run_skill_references_test.go` pattern without `holdings`, crafted case re-pointed at `v_balances_daily`; `schema.md` regenerated.
- Acceptance test is green. Narrow loops green; `golangci-lint run ./...` 0 issues.
- Two real overflows found by the largest-holding tests (red first, fixed): the view's `shares*price` multiplied DECIMAL(18,6) pair in 64 bits (now DECIMAL(19,6) casts), and `Holdings` reading `shares*1000000` (now via DECIMAL(38,6)).
- Mutations (restored, diffed): cap -> `coalesce(to_date, current_date)` reddens `Test_holdings_view_stops_each_span_at_today/a_span_closed_after_today_ends_today` (got extra day); price `d.date >= p.date` -> `<=` reddens 5 rows of `Test_holdings_view_takes_the_latest_price_on_or_before_the_date`.
- Not done, left for V: `duckstore/doc.go:6-14` view list (add `v_holdings`, `Holdings`), `typeof(date)` pin skipped on purpose (`Test_holdings_view_lists_its_columns_in_order` pins `date DATE`), doc-comment pass, full verification, spec tick, STATE.md, SCENARIO-02 `status: done`. STATE.md Open debts: sort differs from `quarry accounts` (lower(name) there).
