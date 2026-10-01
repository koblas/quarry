---
id: SCENARIO-04
status: open
---

# SCENARIO-04: Rates survive the rebuild, including sync --from an older snapshot (folds SCENARIO-02)

Cadence: test-first — the carry and the floor UPDATE land inside Replace's temp-then-rename (`duckstore.go:300,319`), same item as SCENARIO-01; "carried rates are never deleted" is a loss guard
Acceptance test: `cmd/quarry/run_sync_rates_carry_test.go` `Test_run_sync_from_an_older_snapshot_keeps_every_carried_rate`
Acceptance test (SCENARIO-02, folded): `cmd/quarry/run_sync_rates_carry_test.go` `Test_run_sync_asks_only_for_the_dates_after_the_last_stored_rate`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/importer/ ./internal/snapshot/ && go test ./cmd/quarry/ -run 'Sync|Rates|Carry'`
Mutation checks: carried-rates append dropped from `finishBuild` → `Test_replace_carries_the_rates_of_the_previous_store`; `Have` passed zero in `refreshRates` → `Test_replace_tells_the_source_what_the_carried_rates_already_cover`; floor = Need.First (not min with previous) → `Test_replace_keeps_the_earliest_checked_floor_across_runs`; rates-read fault swallowed (rates dropped, no `RatesFault`) → `Test_replace_names_a_repeated_rate_date_as_the_rates_fault`; previous floor kept when rates were lost → `Test_replace_forgets_the_checked_floor_when_the_carried_rates_cannot_be_read`
Runs: A (1-2) | B1 (3-4) | B2 (5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 logic package (duckstore); the fault pass-through in `store`/`importer`/`snapshot` copies the findings-fault precedent line for line, as `cli`/`cmd` wiring does

Surveyed (no new port): `RatesSource`/`RatesRequest{Need,Have}` frozen by 07/01; `Have` is the only field this scenario starts filling. Caller table: `history`/`readHistory` (LSP: only `Replace`), `finishBuild`/`refreshRates`/`askedFrom` (LSP: `rates.go` + `duckstore.go:319`), `carryWarnings` (`snapshot/import.go:152` only).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_sync_rates_carry_test.go` (new) `Test_run_sync_from_an_older_snapshot_keeps_every_carried_rate` — `runWith` with `fx.NewServer(WithHTTPClient(<recording wrapper over fakeValet, run_sync_rates_test.go:28-44>))`. Sync bundle A (txn 2017-01-03; valet serves 01-03, 01-04) and take its snapshot id (`syncThenWrite` pattern, `run_sync_prune_test.go:121`); sync bundle B (txns 2017-01-03 and 01-10; valet serves 01-05, 01-06); then `sync --from <A's id>` against an EMPTY valet. Asserts: `quarry sql` still lists all four rates (the source no longer serves them, so only a carry keeps them), the request log is one FXUSDCAD request starting 2017-01-07, Rates line `USD/CAD 2017-01-03 to 2017-01-06 (up to date)`. Red at the fx_rates assertion
- [x] Step 2: same file `Test_run_sync_asks_only_for_the_dates_after_the_last_stored_rate` (02) — sync, then sync again with valet newly serving 01-05, 01-06: one request starting 2017-01-05 (`start_date`), four rates, `(2 new)`. Red at the request assertion. No stubs needed: both compile today

### Build
- [x] Step 3 (batch 1, carry): `history.go:23-31` `history` gains `rates []store.Rate`, `ratesCarried bool`; `history.go:54-84` `readHistory` calls new `readRates` after findings (template: `readFindings` `history.go:175-215`; an absent fx_rates table = none, silently; usd_cad read as millionths, never float); `rates.go:23-46` `finishBuild` takes `carried history` (`duckstore.go:319`) and appends carried rows before the fetched ones, with or without a source. Tests `rates_test.go`/`history_test.go`: `Test_replace_carries_the_rates_of_the_previous_store` (dates, exact millionths, series), carried + fetched rows coexist, carried with no `WithRates`, `Test_replace_carries_no_rates_and_stays_silent_from_a_store_without_fx_rates` (`newStoreFile` + `phase1ImportRunsDDL`, `history_test.go:97`); faults on rates presence query, rows query, row scan (`spyReadDB` `passQueries`, `history_faults_test.go:346`) close the connection; ctx ended during the read keeps the previous store
- [x] Step 4 (batch 2, unreadable → warning + full fetch): `history.go:48-52` reasons `its fx_rates table repeats a date` / `its fx_rates table is incomplete` (**pending copy ruling**, see Handoff), `ratesFault(path, err)` beside `findingsFault` `history.go:100-109`; a repeated date, NULL/non-positive/out-of-range cell or missing column → `ratesFault`, nothing carried, the rest of the history still carried; `store.go:216-252` `RatesFault *OpenError` on `Result` and `Replaced`; `duckstore.go:349-352` sets it; `importer.go:141-142` copies it; `snapshot/import.go` (struct near :30-41, `warnings` :49-61, new `ratesCarryWarning(result, display)` beside `carryWarnings` :81, `:152`) renders `could not carry exchange rates from the previous store (<reason>); fetching them all again`, ordered history, findings, rates, prune. Whole-store-unreadable → NO rates warning (the combined history warning owns it). Tests: duckstore `Test_replace_names_a_repeated_rate_date_as_the_rates_fault` (+ incomplete arm, `RatesFault` nil for v4 and for a healthy carry, `HistoryFault`/`FindingsFault` unaffected, request `Have` zero); importer pass-through; `snapshot` warning text + order with a findings fault; cmd e2e, text and `--json` (stderr `quarry: warning: …` / `warnings[]`, exit 0, full-span request, rates refetched): `editStore` repeats a date (`DROP TABLE fx_rates CASCADE` — the views depend on it); v4 shape (table dropped) is silent in both formats
- [x] Step 5 (batch 3, Have + cumulative floor; 02 arms): `history.go:113-168` `readRuns` records the floor = latest-by-id non-NULL `rates_checked_from`; `rates.go:48-71` `refreshRates` sends `Have` = [min(floor, min(date)), max(date)] (zero with no carried rates; floor ignored when no rates were carried), `askedFrom` becomes the cumulative floor = min(previous floor, Need.First) when asked with no FetchError and Need non-empty, else the previous floor carried (NULL only if none ever). Tests: `Test_replace_tells_the_source_what_the_carried_rates_already_cover` (floor older than first rate; floor NULL; run 3 NULL after run 2 non-NULL → run 2's; no rates + floor → zero), `Test_replace_keeps_the_earliest_checked_floor_across_runs` (older-then-newer AND newer-then-older Need.First; FetchError carries previous; no transactions carries previous; first-ever NULL stays NULL), `Test_replace_forgets_the_checked_floor_when_the_carried_rates_cannot_be_read`. Cmd cells, each text + `--json` (`first,last,added:0,fetch_error:null`): tail asked and answered empty → `(up to date)`; Need covered by a carried 2099 rate (`replaceStoreWithRates`, `run_sql_fx_test.go:25`) → zero requests, `(up to date)`; no transactions but rates carried → `USD/CAD <first> to <last> (up to date)`; an older Need.First answered empty lowers the floor, and the next sync sends no head request

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `readRates`, `ratesFault`, `RatesFault`, `ratesCarryWarning`; repoint any pin the carry moved (import_runs rates floors, `Warnings()` order tests)

### Verify
- [ ] Step 7: full verification + `spec-check.py phase2f-fx` → tick SCENARIO-04 with its acceptance test, and SCENARIO-02 as `delivered by SCENARIO-04 — ` then its test (test reference stays last on the line)

## Handoff

**Binding decisions:**
- Carry is independent of the source and of the import_runs read: fx_rates is read after findings and fails on its own (`RatesFault`), exactly like `FindingsFault` — a lost run history does not lose rates (then Have.First falls back to min(date))
- `Have` = [min(floor, min(date)), max(date)]; zero when no rates are carried, even with a floor — an empty answer is re-asked in full next sync (one request). Floor = latest non-NULL `rates_checked_from` by id; the carried value on a FetchError or empty-Need run
- A floor is valid only for rates that were carried: unreadable fx_rates resets it (else a later Have would claim dates whose rates were lost)
- Whole-store-unreadable prints no rates warning; v4 (no fx_rates table) prints none (spec R7)
- Carried and fetched rows must never share a date: the Refresh contract (none inside Have) is the only guard, and Have ⊇ every carried date by construction

**Left unbuilt:** fetch warnings, `<reason>` copy, 30 s timeout, `rates_fetch_error` write, partial range — 03; status Rates line — 06; `Test_run_sync_*` pins for `(not refreshed…)`/`none (not fetched…)` — 03

**Traps:**
- The two rates reason phrases are unruled copy (spec rules only the warning sentence). Modeled on the findings precedent; orchestrator should have `product-vision` rule them (and the silence for an unreadable whole store) before B1 dispatches, or accept as written
- `DROP TABLE fx_rates` alone fails in a test: `v_account_balances`/`v_cash_flow`/`v_spending` depend on it (`CASCADE`)
- A repeated carried date would PK-fail the whole build, so it must surface as `RatesFault`, never as a build error
- Existing `askedFrom` tests assume no previous run; the FetchError-NULL arm stays valid only on a fresh store

## Phase report

Run B2 (step 5) done. Floor work only; B1 had already pulled the basic `Have` forward.

Changed: `history.go` (`history.ratesFloor` = newest non-NULL `rates_checked_from` by run id, set in the `readRuns` row loop, reset to zero when `readHistory` records a `RatesFault`); `rates.go` (`refreshRates` takes the whole `history`; `coveredBy(rates, floor)` = [earliest(floor, first rate), last rate], empty with no rates; `earliest` ignores a zero time; `askedFrom(need, refresh, previous)` = earliest(previous, Need.First), previous carried on a FetchError or an empty Need).
Tests: new `duckstore/rates_floor_test.go` (stores built from DDL with seeded floors: `newStoreWithFloors`; Have table, cumulative-floor table, no-source carry, rates-lost reset) and `cmd/quarry/run_sync_rates_floor_test.go` (tail answered empty, Need covered by a carried 2099 rate, no transactions with carried rates, lowered floor means no head request; each text and `--json`, added 0).
Green on arrival: the three cmd arms other than the lowered floor (the B1 `Have` already covers them); the rates-lost reset test passed before the code too, and reddens only under the mutation.
Mutations all red: floor = Need.First; floor kept when rates lost; Have ignores floor; floor claimed with no rates; latest -> first non-NULL; failed fetch zeroes the floor; zero-time not ignored; empty Need not carrying previous. Lint 0 issues, narrow loop green. Not run: covered full suite (V).
