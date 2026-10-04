---
id: SCENARIO-01
status: open
---

# SCENARIO-01: Sync imports securities and their prices

Cadence: code-first — nothing on the mandatory test-first set (the format bump guards reads; no write-safety guard, no atomic adapter touched)
Acceptance test: `cmd/quarry/run_investments_test.go` `Test_run_sync_imports_securities_and_their_prices`
Narrow loop: `go test ./internal/quicken/v9/v9fixture/ ./internal/importer/ ./internal/store/duckstore/ ./cmd/quarry/ -run 'Builder|Securit|Price|Entit|Fault|Replace|Carr|Status|Investments|ImportRuns'`
Mutation checks: none
Runs: A (1-2) | B1 (3-5) | V (6-7)
Size: OWNS A RUN — 3 Build batches (+ fixture in A), 1 feature package (importer; `store`/`duckstore` are its Store-port adapter, as in the sizing table)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `internal/quicken/v9/v9fixture/builder.go:16-21,164-200,209-237,378-500` `SecurityRow`, `SecurityQuoteRow`, `(*Builder).Security`, `(*Builder).SecurityQuote`, `EntSecurity`(66) / `EntSecurityQuote`(68) — rows default their `Z_ENT` (pattern `:231-233`); `Seed` inserts ZSECURITY (name, ticker, currency, deletion) and ZSECURITYQUOTE (security, quote date, closing price as text so SQLite affinity picks integer/real/text, deletion); the Z_PRIMARYKEY loop (`:468-477`), `maxPKForEntity` (`:485-498`) and `WithEntity`/`WithoutEntity` accept "Security"/"SecurityQuote"; extend `builder_test.go:18` read-back
- [x] Step 2: `cmd/quarry/run_investments_test.go` (new) `Test_run_sync_imports_securities_and_their_prices` — `run(sync --quicken)` then `stringMap` over `securities` and `prices` (pattern `run_import_test.go:23-39`): ticker present / empty → NULL / NULL; currency CAD / USD / NULL; zero price kept; NULL price and NULL date absent; `12.3456785` stored `12.345678`. Expected red: store query fails, `securities` does not exist

### Build
- [ ] Step 3: securities — `internal/store/store.go:133-155` `Security` type + `Rows.Securities`; `internal/importer/entities.go:11-52` optional `Security`/`SecurityQuote` resolved from one optional-entity slice, placeholders generated (`:27` is hard-coded to 4); `internal/importer/securities.go` (new) `mapSecurities` (`Z_ENT = ?`, non-deleted, `sec-<Z_PK>`, ticker NULL when NULL or "", currency as recorded) called from `importer.go:84-121`; `duckstore/schema.go:12-132` `securities` table; `duckstore.go:431-465` load + `securityRows`. Tests (Server-method, `fakeStore`): `securities_test.go` — each ticker/currency arm, deleted excluded, `WithoutEntity("Security")` → no securities and no prices, no error; fault rows for the ZSECURITY query in `import_faults_test.go:52` and `:139`; duplicate-id case in `duckstore_test.go:324-338`; DuckStore read-back of NULL ticker/currency
- [ ] Step 4: prices — `store.go` `Price` (security id, source id, day, price in millionths) + `Rows.Prices`; `internal/importer/price.go` (new) `parsePrice(typ, text)` — exact on decimal text via `big.Rat`, half-even to 6, bound |rounded| < 10^12 checked after rounding, `Inf`/text/blob as in `money.go:32-69`; `mapPrices` in `securities.go`: `ZQUOTEDATE` cast to REAL → `coreDataToDate` (`transactions.go:14-24`), only quotes of an imported security, NULL/deleted filtered **before** dedupe, highest `Z_PK` wins a (security, UTC day); refusals `reasonPriceNotANumber` / `reasonPriceTooLarge` in `reasons.go` (S.5 copy verbatim; offender dated, `account` = security name, classes `classNotANumber`/`classTooLarge`); `schema.go` `prices` table `PRIMARY KEY (security_id, date)`, `price DECIMAL(18,6) NOT NULL`; `duckstore.go:51-52` price width/scale + `priceRows` via `duckdb.Decimal`. Tests: `price_internal_test.go` table (combinatorial — justifies a pure-func test): tie→even down, odd-digit tie up, negative tie, 7+ decimal non-tie, exponent form, integer, `Inf`, text, bound 10^12−10^-6 in / 10^12 out; `prices_test.go` via `Import`: highest-`Z_PK` duplicate carrying the *lower* price wins, same UTC day different times is a duplicate, higher-`Z_PK` NULL-price and higher-`Z_PK` deleted quote do not erase a valid one, quote of a deleted security absent, NULL date absent, zero kept, `WithoutEntity("SecurityQuote")` → securities only, both refusal reasons verbatim; ZSECURITYQUOTE rows in `import_faults_test.go:52,:139`; prices duplicate case `duckstore_test.go:324-338`; out-of-range price at Replace (twin of `:363-373`)
- [ ] Step 5: counts + import_runs + format — `store.go:212-221` `Counts.Securities/Prices` set at `importer.go:122-125`; `schema.go:79-108` nullable `securities_rows`, `prices_rows` BIGINT appended after `rates_fetch_error`; `duckstore.go:580-597` `importRunRows` appends both after the three rates `nil`s (rates are set by name, `rates.go:21`); `history.go:41-44` `optionalRunColumns` + `carriedRun` `:310-343` (NullInt64 targets/values); `status.go:16-29,59-68` `COALESCE(...,0)` into `Run.Counts`; `duckstore.go:25` `FormatVersion = 6`. Tests: `minimalRows` (`duckstore_test.go:25`) run gets non-zero security/price counts so `history_test.go:77-94` carries values; `history_test.go:96-110` adds both `IS NULL`; `status_test.go:18` read-back and `:63` NULL→0; importer test that `ImportRuns[0].Counts` carries both; cmd-level `Test_run_sync_records_security_and_price_counts_in_import_runs` in `run_investments_test.go`. Output arms: n/a — S01 renders nothing; Rows clause, `--json` `store.rows` keys are S04's

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; re-pin `cmd/quarry/run_shared_documents_test.go:132` `format_version` 6; `duckstore/query_test.go:30-35` `storeRelations` adds `prices`, `securities`; Builder doc comments naming "five entity kinds" (`builder.go:184-187,209-211,375-377`); doc comments on new types/funcs; regenerate `plugin/skills/quarry/references/schema.md` with `go test ./cmd/quarry/ -run SchemaReference -update`

### Verify
- [ ] Step 7: full verification + `spec-check.py phase4a-investments` → tick SCENARIO-01 with its acceptance test; write `STATE.md`

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Optional entities live in one slice in `entities.go` with generated placeholders; missing → that data is empty, never a refusal — S02 appends `Position`, S04 appends `Lot`.
- Quote selection: deleted, NULL-date and NULL-price quotes and quotes of a non-imported (deleted/absent) security are dropped **before** the highest-`Z_PK` dedupe; every remaining quote is parsed, so an unreadable superseded duplicate still refuses — 4b's `prices` readers rely on one row per (security_id, date), enforced by the PK.
- `store.Price.Price` is int64 millionths loaded as DECIMAL(18,6); "too large" = |rounded| ≥ 10^12 — S02 shares (also DECIMAL(18,6)) should reuse the same scale constant.
- `securities_rows`/`prices_rows` are nullable and sit after `rates_fetch_error`; import_runs columns are filled positionally by `importRunRows` and carried via `optionalRunColumns` — S02/S04 append theirs the same way.
- `FormatVersion` is 6 from this scenario; later schema changes in 4a do not bump it again.

**Left unbuilt** — named so nobody assumes it exists:
- `reasonSecurityNoName` and the NULL/empty-name refusal — SCENARIO-03 (decides whether `""` is "no name").
- `document.Rows`/`NewRows` keys, the Rows investment clause — SCENARIO-04.
- Dropping `investment_transactions_not_imported` from `requiredRunColumns`, `surveyTransactions` — SCENARIO-09.

**Traps** — things that look right and are not:
- ZNAME is scanned into a plain `string`: a NULL fails the import loudly via `rows.Scan` until S03. Switching to `sql.NullString` before S03 makes a nameless security import silently.
- `ZQUOTEDATE` must be `CAST(... AS REAL)` (`transactions.go:37-40`), or the driver returns a Unix-epoch `time.Time`.
- A fixture row without its default `Z_ENT` is silently filtered out by `Z_ENT = ?` — tests pass empty.
- Half-even on `float64` passes most rows and fails ties; round on the decimal text.

## Phase report

Run A (steps 1-2) done.
- `internal/quicken/v9/v9fixture/builder.go`: `EntSecurity`(66)/`EntSecurityQuote`(68), `SecurityRow{Entity,Name,Ticker,Currency,Deleted}`, `SecurityQuoteRow{Entity,Security,QuoteDate,ClosingPrice,Deleted}`, `(*Builder).Security`/`SecurityQuote`, Seed inserts (ZSECURITY, ZSECURITYQUOTE), Z_PRIMARYKEY loop + `maxPKForEntity` + `WithEntity` cover "Security"/"SecurityQuote". Doc comments already say "seven entity kinds" (step 6 item for builder docs is done).
- `internal/quicken/v9/v9fixture/builder_test.go`: new `Test_builder_seeds_securities_and_their_quotes_with_entity_rows` (green).
- `cmd/quarry/run_investments_test.go` (new): `Test_run_sync_imports_securities_and_their_prices` — RED. Fixture needs one account or sync refuses "has no accounts". Failure at `run_investments_test.go:55` (via `stringMap`, `run_import_test.go:37`): `Catalog Error: Table with name securities does not exist!`
- Expected rows: `sec-<PK>` -> `name|ticker|currency` (NULL shown as `NULL`); prices keyed `"<sec id> <date>"` -> `12.500000` / `0.000000` / `12.345678` (NULL price and NULL date absent).
- Not yet run: rest of the suite. Z_PRIMARYKEY now seeds two more rows (66, 68) in every fixture bundle — B1/V watch importer/cmd tests that count entity rows.
