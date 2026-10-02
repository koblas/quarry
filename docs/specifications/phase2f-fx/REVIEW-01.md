# Review round 1: phase2f-fx

- **Range:** 69080355..f7c… (HEAD at gate)
- **Coverage gate:** 0 uncovered.
- **Mutation sample:** 20/181 sampled, 20 killed (866s).
- **spec-check --run:** OK.
- **Triggered:** arch-reviewer, correctness-reviewer ×2 (A: fx/money/store/snapshot/importer/config/run.go; B: report/cli/cmd), test-reviewer ×2 (same split), refactor-advisor. **Skipped:** api-reviewer (no HTTP), pipeline-reviewer (no .claude change).

## BLOCKER
- **correctness-A** `internal/fx/plan.go:36-38`. The forward span starts at `later(need.First, dayAfter(have.Last))`.
  - *Failure:* when the earliest transaction is after the last stored rate, the dates between them are never fetched, so fx_rates becomes two intervals. `coveredBy` (duckstore/rates.go:70-75) then claims one interval, so the hole is never asked for again. The ASOF join converts dates inside the hole at a stale rate, and no warning is printed.
  - *Fix (invariant):* fx_rates is always one interval. The forward span starts at `dayAfter(have.Last)` regardless of `need.First`. R6 is amended to say so.
  - *Test (test-A):* flip the `fx_test.go` row "have wholly before need asks for all of need". `syncWithCoveringRate` (cmd `run_sync_rates_floor_test.go:59-70`) seeds a 2017 rate plus a 2099 rate, a gap that the invariant now forbids. Reseed it with an adjacent covering span. Add a cmd sequence: sync a Jan 3 transaction, then sync a bundle whose earliest transaction is Jan 10. The second request must start at 2017-01-05.

## MAJOR
- **correctness-A** `internal/store/duckstore/history.go:193,226-228`. A rates fault clears `ratesFloor` only in memory. The carried import_runs rows keep the old `rates_checked_from`, and `readRuns` takes the newest non-NULL value, so the floor comes back on the next sync.
  - *Failure:* FXUSDCAD answers and IEXE0101 returns 500. Pre-2017 dates then never retry, so "run quarry sync again to fetch the rest" never works. The same root cause applies when the fx_rates table is absent but a floor is set.
  - *Fix:* read the floor from the newest run only, treating NULL as no floor. Or, on a rates fault or an absent table, NULL `rates_checked_from` on every carried row.
  - *Tests (test-A):* two builds (rates fault plus failing or partial fetch, then a healthy build) asserting that `Have.First` is not the old floor. Also the floor-present, table-absent case. Update the cases that pin the old "defers to an earlier run" behaviour.
- **arch** `cmd/quarry/run.go:36`. `newRatesSource` is a mutable package global that TestMain reassigns. Delete it. Build `duckstore.WithRates(fx.NewServer())` inline in `newServerFactory`, ahead of `storeOpts`, so a caller's `WithRates` still wins. Tests inject through the existing options or a run/env parameter. No test may reach the network: keep a test-wide fake default through the injected path, not a global.
- **test-A** `internal/fx/valet.go:98`. A per-observation decode fault (bad date, more than 6 decimals) never has its reason asserted. A mutant that drops `errNotRates` survived, so such a fault is classified as "cannot reach", which stops later spans.
  - *Fix:* add `Test_refresh_gives_each_failure_its_ruled_reason` rows that go through `valetVia` for a 7-decimal rate and an invalid date, each expecting the not-a-list reason.
- **test-A + correctness-A (MINOR)** `needSpan` / `planSpans`. When every transaction is after today, the request has First > Last. It is sent to Valet inverted, and `askedFrom` records a future floor.
  - *Fix (orchestrator ruling):* an inverted Need is empty. No request is made, the floor is unchanged, and the existing Rates-line arms apply. No new copy.
  - *Tests:* a `needSpan` case, a Refresh row, and a cmd sync with only future-dated transactions.

## MINOR / NIT: folded into the fix pass
- **arch:** `maxRate` (`fx/valet.go:26`) and `maxStoredRate` (`duckstore/rates.go:19`) are the same limit. Declare it once, in money or store.
- **correctness-A:** the `observe` repeat-date check is O(n²). Use a seen-map.
- **correctness-B:** `report/anomalies.go:142` drops `ok` silently. Add a one-line reason.
- **test-A:**
  - `history_test.go`: the test name claims a sixth query. Rename it, or name the `passQueries` constants.
  - `rates_test.go:~329`: move the `if c.noTxns` branch into case data.
- **refactor:**
  - The duplicated CAD/USD switch and the NULL-arm SQL in `spendingSource`, `cashFlowSource`, `unconvertedView` and `convertedTo`: replace with one `convertedColumn` and one `keepOwnCurrency` helper.
  - The first-rate SQL appears twice. Reuse `firstRateQuery`.

## MINOR / NIT: deferred to STATE.md Open debts
- **correctness-B:** recurring Per year multiplies int64 without an overflow check. It needs more than a quadrillion dollars to trigger.
- **refactor:**
  - Parallel conversion shapes: Anomaly `Listed*` vs Series `Native*` vs accounts accessor. Proposed catalog entry.
  - A shared unconverted helper; move `chargeIn`/`isCADOrUSD`.
  - Rename `Unconverted.Transactions`.
  - Pass `fillSeries` a typed target.
  - `slices.Concat` for warnings.
  - Named accounts warning helpers.
  - A `store.Converted` type.
- **arch:** move the `accountsFXWarnings` "first rate after today" decision into report.
- **test-A style MINORs:** in-body `if`s at `charges_fx_test.go:84` and `unconverted_test.go:297`; `fetchFailureCases` `earlier`; fakes spread across cmd; zone-probe skip; status Rates text never asserted with data at cmd level; `rates_test.go` file size; unkeyed fields; a test name drift.
- **test-B style MINORs (14):** dead NativeCurrency tier, in-body if/else, missing inner `t.Run`, unjustified white-box file, the sql row in the refusals table, multi-behaviour tests and names, a single-currency read test, package-level case tables, duplicated helpers, redundant assertions.
- **correctness-B "test gap" (bad reporting.currency for the five commands):** already pinned by `Test_run_read_commands_refuse_a_bad_reporting_currency`. No action.

## Verdict: BLOCKED (1 BLOCKER, 5 MAJOR)
