---
id: SCENARIO-12
status: open
---

# SCENARIO-12: Amounts dated before the first rate stay native with a warning (folds SCENARIO-13)

Cadence: code-first (no mandatory test-first item: no write guard, no atomic adapter, no bug fix)
Acceptance test: `cmd/quarry/run_spend_unconverted_test.go` `Test_run_spend_lists_a_split_before_the_first_rate_in_its_own_currency_and_warns`
Acceptance test (SCENARIO-13, folded): `cmd/quarry/run_spend_unconverted_test.go` `Test_run_spend_without_rates_lists_each_currency_natively_and_warns_only_when_a_conversion_is_needed`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/report/ ./internal/cli/ ./cmd/quarry/ -run '(?i)unconverted|spend|cashflow|cash_flow'`
Mutation checks: `currency IN ('CAD','USD')` in the count → `Test_spending_does_not_count_a_third_currency_split_as_unconverted`; `count(DISTINCT transaction_id)` → `Test_spending_counts_two_unconverted_splits_of_one_transaction_once`; `FirstRate.IsZero()` arm choice in `unconvertedWarnings` → the no-rates vs before cli pins
Runs: A (1-2) | B1 (3) | B2 (4-5) | V (6-7)
Size: OWNS A RUN — 3 batches; report is a passthrough, the work is in duckstore + cli (as sized); folds 13

No new port: the unconverted facts ride extra fields on `store.Spending`/`store.CashFlow`, so no `report.Store` implementer changes (fakes keep zero values). One read per command already holds: the count runs on the same `openRead` connection inside `Spending`/`CashFlow`, one extra statement; `report.Store` has a single call per command.

## Rulings this plan applies (derived, not new copy)
- Native currency named in the "before" line = the other of CAD/USD than the reporting currency. Same-currency is the identity, so CAD mode can only leave USD unconverted and USD mode only CAD: a mix cannot occur, no ruling needed.
- FX warning only when `Transactions > 0`; native skips the count (store returns zero `Unconverted`); an empty window has no rows, so count is 0 and only the empty note prints.
- Arm: count > 0 and `FirstRate` zero -> the "no rates" line (no count, no currency); count > 0 and `FirstRate` set -> the "before" line (N via `humanize.Count(n, "transaction", "transactions")`, `is` for 1, `are` otherwise, date `time.DateOnly`).
- Count = distinct `transaction_id` in the window and accounts of that command's own view (`v_spending` for spend, `v_cash_flow` `flow IN ('income','expense')` for cashflow), where the target converted cell IS NULL and `currency IN ('CAD','USD')`. A third-currency split stays native and counts nothing (without the filter a post-rate EUR split would claim "dated before").

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_spend_unconverted_test.go` (new) `Test_run_spend_lists_a_split_before_the_first_rate_in_its_own_currency_and_warns` — `replaceStoreWithRates` + `rateOnJan2`, USD split before it, `spend` in CAD: USD row + USD Total, stderr and `warnings[]` carry the "before" line verbatim, exit 0
- [x] Step 2: same file `Test_run_spend_without_rates_lists_each_currency_natively_and_warns_only_when_a_conversion_is_needed` — `replaceStore` (no rates) rows: CAD+USD data CAD mode -> "no rates" line; all-CAD in CAD -> empty stderr and `warnings: []`. No stubs needed; both fail at the missing-warning assertion

### Build
- [ ] Step 3 (B1, duckstore): `internal/store/store.go:526-535,579-585` new `store.Unconverted{Transactions int; FirstRate time.Time}` (FirstRate zero = no rates) as field on `Spending` and `CashFlow`; new `internal/store/duckstore/unconverted.go` `unconvertedQuery(view, convertedCol, flowFilter, accounts)` (one statement: distinct-transaction count + `min(fx_rates.date)`, same `$1/$2` window and `accounts.and("account_id")`); call it in `spending.go:118-` after the rows query and before the multi-tag query (`:147`), in `cashflow.go:67-` after the rows query, both skipped for `money.Native`. Do NOT touch `spendingSource`/`cashFlowSource`. Tests `internal/store/duckstore/unconverted_test.go` (reuse `fxSpendRows`/`expense`/`newStoreWithRates`, `views_fx_test.go`): CAD mode USD pre-rate -> count + first rate; USD mode CAD pre-rate; no rates -> count, zero FirstRate; two splits of one transaction -> 1; post-rate split and weekend split -> 0; `--account` on a CAD-only account -> 0 though another account holds a pre-rate USD split; window excluding the pre-rate split -> 0; third currency -> 0; native with pre-rate USD -> zero struct; `--by tag` multi-tag split counted once; consistency: `Unconverted.Transactions > 0` iff Rows hold a currency other than the target (both reads); cashflow count includes a pre-rate income split spend's leaves out; fault test per read: `spyReadDB{passQueries: 1, queryFault}` -> `assertOtherFault` (second query is the count; query order is rows, count, multi-tag, transactionRange)
- [ ] Step 4 (B2, report + cli): `internal/report/spending.go:30-48,69-80`, `cashflow.go:25-40,56-70` carry `Unconverted` into `report.Spending`/`report.CashFlow`; report tests in `spending_test.go`/`cashflow_test.go` (value passes through; fake counts one store read per `Spend`/`CashFlow`, extend `fakeStore` with a read counter like `accountsReads`). New `internal/cli/fx_warning.go` `unconvertedWarnings(currency money.Currency, u store.Unconverted) []string` (both lines as consts, helper for the other currency); wire into `spend.go:90-101` and `cashflow.go:105-111` directly after `leftOutWarnings`, before the multi-tag note and the empty note. cli tests (`internal/cli/spend_unconverted_test.go`, `cashflow_unconverted_test.go`, `fakeReportStore`): both lines verbatim in text (stderr, `quarry: warning: ` prefix) and `--json` `warnings[]` (config warnings first, as 16); n=1 (`is`), n=2 (`are`), n=1000 (`1,000 transactions`); USD mode names CAD/USD swapped; no-rates line carries no count; Transactions 0 -> no warning; order: left-out account, FX, then multi-tag note (spend `--by tag`)
- [ ] Step 5 (B2, cmd cross cells): `cmd/quarry/run_spend_unconverted_test.go` + new `run_cashflow_unconverted_test.go`, one case per cell, each text and `--json`: before-first in CAD mode and in USD mode; two transactions -> `2 transactions ... are`; `--account` USD account warns with its count, `--account` CAD-only account silent; spend `--by tag`/`--by month`/`--by payee` warn once; cashflow before-first (income+expense) and no-rates; `--currency native` with pre-rate USD silent; unrated + empty window -> empty note only (spend and cashflow); all-USD data in USD silent, all-CAD data in USD mode warns. Flip `run_cashflow_fx_test.go:266-335` `..._without_a_warning` (rename, assert the "before" line on stderr/`warnings[]` for CAD and USD, native stays silent). Bump the goldens that now see the "no rates" line because they run USD data in default CAD on an unrated `replaceStore`: `run_spend_test.go:17-50` (`:39`), `run_spend_json_test.go:16-80` (`:35`, `warnings` at `:75`), `run_spend_by_test.go:16-47` (`:36`) and `:49-80` (by-tag: no-rates line then multi-tag note), plus any the toolchain lists. Covered already, no new case: future-dated, weekend, closed account (edge matrix asserts empty stderr, `run_spend_fx_test.go:202-274`); invariant before-first cells (`run_cashflow_invariant_test.go:87`, s07/s08 already pre-rate, stderr unasserted)

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments (`store.Unconverted` + the two fields, `unconvertedQuery`, `unconvertedWarnings`) within the `clean-architecture` budget

### Verify
- [ ] Step 7: full verification + `spec-check.py phase2f-fx`; tick SCENARIO-12 and tick SCENARIO-13 "delivered by SCENARIO-12" (acceptance test last on the line); rewrite STATE.md: drop the Interim line, move the `current_date` debt to 19, note the unruled items below as decided

## Handoff

**Binding decisions:**
- `store.Unconverted{Transactions, FirstRate}` rides `store.Spending`/`store.CashFlow` and is filled by the same `Spending`/`CashFlow` call on the same connection; `FirstRate` zero means the store has no rates. 17-19 reuse the type or add their own count; they never add a second port call.
- The count duplicates the NULL predicate of `spendingSource`/`cashFlowSource` on purpose (those own the arm, must not regroup); the Rows-vs-count consistency test is what keeps the two from drifting.
- Warning text is built in `internal/cli/fx_warning.go`; 18 swaps the noun to `charges`, 17 to `series` (spec "Report warnings").
- `current_date` zone debt is NOT closed here: spend/cashflow use an ASOF on the split's date, only `v_account_balances` (`schema.go:150,153`) and `accounts.go:15` read `current_date`. 19 owns it with a zone-pinned view test.

**Unruled, needs a ruling before B1 (plan assumes the bracketed default):**
1. Third-currency (EUR) splits: no FX warning, stay native [assumed; restricting to CAD/USD avoids a false "dated before"].
2. Where the FX line sits against spend `--by tag`'s multi-tag note: left-out, FX, multi-tag, empty [assumed; spec rules only after left-out and before empty].
3. The count is distinct transactions of each command's own view, so spend (expense splits) and cashflow (income and expense) can report different N for one store [assumed OK].
4. `fillSeries` still adds a zero USD row in every CAD period when one USD split predates the first rate. Not changed (would regroup); the new warning now explains the row. Stays with the final product-vision pass.

**Left unbuilt:** recurring/anomalies/accounts warnings (17-19); the partial-fetch "convert at the <last> rate" wording (unowned debt).

**Traps:**
- Existing USD goldens on an unrated `replaceStore` in default CAD now print the "no rates" line (step 5 lists them); do not "fix" by seeding rates, which changes what they test.
- `spyReadDB.passQueries` counts the rows query as the first read; the count is second, before multi-tag.
- `-run` is case-sensitive; keep `(?i)`.

## Phase report

Run A (steps 1-2) done; both ticked. Nothing else built.

- Files: `cmd/quarry/run_spend_unconverted_test.go` (new, only file touched). No stubs needed.
- Test 1 `Test_run_spend_lists_a_split_before_the_first_rate_in_its_own_currency_and_warns`: red at `stderr` (text) and `warnings[]` (json) assertions; actual `""` / `[]`. Seed: `replaceStoreWithRates` + `rateOnJan2`, TWO USD splits on 2026-01-01 (N=2, "are"), one post-rate USD split (converted, must not count), one CAD split. Text pins stdout rows (USD rows + CAD/USD Totals, by category: no periods, so no fillSeries rows) and stderr; json pins `totals` and `warnings[]`. Line const `beforeFirstRateLine` is the spec text with N=2.
- Test 2 `Test_run_spend_without_rates_lists_each_currency_natively_and_warns_only_when_a_conversion_is_needed`: subtest "CAD and USD data warns..." red at stderr / `warnings[]` (actual empty); its stdout rows assertion already passes (native listing exists). Subtest "all-CAD data in CAD ... stays quiet" is GREEN ON ARRIVAL by design: it is the control arm guarding against over-warning once the no-rates warning exists.
- Helpers added in that file: `unconvertedDoc`, `runSpendUnconverted(t)` (json doc + stderr); consts `beforeFirstRateLine`, `noRatesLine`. Did not extend `spendReport` (adding `Warnings` would break existing struct equality: nil vs `[]`).
- Deliberately not asserted (pending ruling): warning order vs multi-tag note, EUR, fillSeries zero rows.
- Lint on `cmd/quarry`: 0 issues. Not yet committed beyond this run's commit; no production code touched.
- Next (B1): store.Unconverted + duckstore count (step 3). Tests use no network (`TestMain` swaps `newRatesSource`; these use `replaceStore*` direct).

