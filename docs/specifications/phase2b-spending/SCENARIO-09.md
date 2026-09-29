---
id: SCENARIO-09
status: open
---

# SCENARIO-09: spend shows this year's spending by category in each currency (absorbs SCENARIO-05, SCENARIO-07, U9-example fold)

Cadence: code-first. This is a read-only command, so no write-safety guard, atomic adapter or bug fix is touched.
Acceptance test: `cmd/quarry/run_spend_test.go` `Test_run_spend_shows_this_years_spending_by_category_in_each_currency`
Acceptance test (SCENARIO-05, folded): `cmd/quarry/run_read_refusals_test.go` `Test_run_spend_refuses_a_store_built_by_an_older_quarry`
Acceptance test (SCENARIO-07, folded): `cmd/quarry/run_spend_test.go` `Test_run_spend_leaves_out_accounts_quicken_does_not_use_in_reports`
Narrow loop: `go test ./internal/report/ ./internal/store/duckstore/ ./internal/cli/ ./cmd/quarry/ -run 'spend|spending|window|reads_|read_commands|help_prints|usage_hint'`
Mutation checks: `now.Date()` → `now.UTC().Date()` in `report.DefaultWindow` → `Test_default_window_runs_from_january_first_to_today_in_the_instants_own_zone`; until `<=` → `<` in the duckstore spending query → `Test_spending_counts_the_windows_first_and_last_day_only`; since `>=` → `>` → same test
Runs: A (1-2) | B1 (3-5) | B2 (6) | V (7-8)
Size: OWNS A RUN — 4 batches, 1 feature package (report; the duckstore adapter and cli/cmd wiring do not count)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: acceptance tests, all three at cmd level through `runWith`:
  - `run_spend_test.go` (new) `Test_run_spend_shows_this_years_spending_by_category_in_each_currency`. Uses `replaceStore(t, home, rows)` and `env := defaultEnv(...)` with a fixed `env.Now` in a non-UTC `time.FixedZone`. Fixture has CAD and USD spending this year, an uncategorized negative split, a 2025-12-31 row, and a row dated the day after "today". Build the expected stdout from the fixture's own cells, never from the spec's sample table.
  - `run_spend_test.go` `Test_run_spend_leaves_out_accounts_quicken_does_not_use_in_reports`. Same shape, with one account `NotInReports: true`.
  - `run_read_refusals_test.go:76-89` sibling `Test_run_spend_refuses_a_store_built_by_an_older_quarry`. Uses `writeStoreFixture(phaseOneImportRunsDDL + CREATE store_info + INSERT a row with literal format_version 2)`. Expects R2 with `--from 20260927T143005Z` and exit 1. The literal `2` row closes STATE's open debt.
- [x] Step 2: stubs so all three fail at their assertions, not with "unknown command":
  - `internal/cli/run.go:22-29`: `Env.Now func() time.Time`.
  - `cmd/quarry/run.go:114-124`: `defaultEnv` sets `Now: time.Now`.
  - `internal/store/store.go:309-314`: `store.Window`, `store.SpendingGroup` + `SpendByCategory`, `store.SpendingParams` (Window, By, AccountIDs), `store.Spending` / `SpendingRow` / `SpendingTotal`.
  - `internal/report/store.go:10-18`: `Store.Spending(ctx, store.SpendingParams)`, stubbed in `duckstore`, `internal/cli/fakes_test.go:9-16`, `internal/report/fakes_test.go:9-16`.
  - `internal/report/spending.go` (new): `SpendRequest`, `Spending`, `DefaultWindow`, `(*Server).Spend`.
  - `internal/cli/spend.go` (new): `newSpendCommand` with Use/Short/Long/Example verbatim from spec, `Args: noArgs`, and a RunE that returns nil. Register it at `internal/cli/root.go:27-30`.
  - Update the two tests that registration breaks: `cmd/quarry/run_status_test.go:98-103` (Available Commands gains `  spend       Show spending by category, payee, tag or month` between help and sql) and `cmd/quarry/run_usage_test.go:178` (`spend` → `spending`).

### Build
- [x] Step 3: `internal/report/spending.go` `DefaultWindow`: January 1 of now's year through now's day, both read in now's own zone and returned as civil days.
  - `Test_default_window_runs_from_january_first_to_today_in_the_instants_own_zone`: 2025-12-31 22:00 at UTC-5 (already 2026-01-01 in UTC) must give 2025-01-01..2025-12-31; a mid-year instant is the control.
- [x] Step 4: `internal/store/duckstore/spending.go` (new) `(*Store).Spending`. Shape follows `accounts.go:11-58`: one statement over `v_spending`; since and until bound as DATE parameters; category grouping; zero-net rows dropped; rows sorted with NULL first, then `lower(category)`, category, currency; per-currency Totals read from `v_spending` itself, never summed from rows; an unsupported `By` returns an error. Also fill the two fakes: record the params and return canned output. Tests in `spending_test.go` (new):
  - `Test_spending_counts_the_windows_first_and_last_day_only` (since-1, since, until, until+1; only the middle two counted)
  - `Test_spending_sorts_categories_ignoring_case_with_uncategorized_first` (one category in both currencies; case order differs from byte order)
  - `Test_spending_omits_a_category_that_nets_to_zero_and_keeps_it_in_the_total`
  - `Test_spending_totals_each_currency_cad_before_usd`
  - `Test_spending_refuses_a_grouping_it_does_not_know`
  - Faults, mirroring `accounts_test.go:130-192`: open fault, query fault as another fault, scan fault, connection closed on success and on fault, missing store not created.
- [x] Step 5: `internal/report/spending.go` `(*Server).Spend`: DefaultWindow(req.Now) → `SpendingParams{By: SpendByCategory}` → store, with the refusal from `readRefusal(ctx, "spend", err)`. Tests:
  - `internal/report/spending_test.go` (new) `Test_spend_reads_this_years_spending_by_category` (asserts the params the fake recorded and the returned Window)
  - `Test_spend_returns_the_store_fault`
  - Add `spend` rows to `refusal_test.go:88-106` (store refusal copy), `:127-157` (interrupt → `spend interrupted`) and `:159-` (non-refusal fault unchanged).
- [ ] Step 6: `internal/cli/spend.go` RunE follows `accounts.go:19-42`: `openReport` → `srv.Spend(ctx, SpendRequest{Now: now()})` → emit. `--json` prints the text table until SCENARIO-16. Never pass a nil `renderJSON`.
  - `internal/cli/render_spend.go` (new) `renderSpending`: caption `Spending <since> to <until> in all accounts`, a blank line, the `Category  Currency  Spent` header, `(uncategorized)` for NULL, `formatMoney` right-aligned, `Total` rows last, no trailing spaces. Reuses `padRight`/`padLeft`/`accountsColumnGap` (`render_accounts.go:3-64`).
  - Render tests in `render_spend_internal_test.go` (new) `Test_renderSpending`, table cases: widths from the widest cell; a negative (net-refund) row; empty window gives caption + blank + header only.
  - cli tests in `spend_test.go` (new), styled after `accounts_test.go:34-146`: `Test_spend_reads_the_window_from_the_env_clock`, `Test_spend_returns_the_report_fault`, `Test_spend_returns_the_report_factory_fault`, `Test_spend_reports_a_failed_stdout_write`.
  - Add a `spend` row to each of: cmd `run_read_refusals_test.go:46-73` (R1, no store), `:139-148` (I1), and `run_read_usage_test.go:23-39` (U8 `quarry: spend takes no arguments`).

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`. Add doc comments on every new exported symbol. Update the subcommand list in the `newRootCommand` doc (`root.go:7-9`) and the `Env` doc (`run.go:22-23`).

### Verify
- [ ] Step 8: full verification + `spec-check.py phase2b-spending`. Tick SCENARIO-09, and tick 05 and 07 as "delivered by SCENARIO-09", each with its acceptance test. Rewrite STATE.md, which drops the FormatVersion literal debt.

## Handoff

**Binding decisions:**
- **Clock via `cli.Env.Now`** (`time.Now` in `defaultEnv`). This deviates from the clean-architecture skill's "no injected clock": a test must choose the instant's zone without mutating `time.Local`, which a synctest bubble cannot do. The window rule lives in `report` (`DefaultWindow`); cli only passes `env.Now()` down. SCENARIO-13's parser and SCENARIO-20 take `now` the same way.
- **Window reaches DuckDB as civil DATE parameters.** No query reads `current_date`. SCENARIO-13, 17 and 20 bind the same way.
- **One `store.SpendingParams` for every spend read.** SCENARIO-10, 11, 12 and 14 add enum values or fields, never a new signature. Its 3 implementers are duckstore, `internal/cli/fakes_test.go` and `internal/report/fakes_test.go`.
- **Totals come from `v_spending` per currency, independent of `By`.** SCENARIO-11 (a split with two tags counts once in Total) relies on it. No S09 test can tell the difference, so SCENARIO-11 owns the mutation.
- **Sort and zero-net omission live in the store query.** Totals keep zero-net rows. A currency whose rows all net to 0.00 still prints `Total <cur> 0.00`; this default awaits a copy ruling.

**Left unbuilt:**
- `SpendingParams.AccountIDs`: the field exists, but duckstore does not read it yet. SCENARIO-14 wires it.
- Owned by later scenarios:
  - `--by` flag, `SpendByPayee`/`Tag`/`Month`, and the S4 check: SCENARIO-10, 11, 12.
  - `--since`/`--until` and S1–S3: SCENARIO-13 (until then, cobra "unknown flag", exit 2).
  - `--account`, the named-accounts caption, and `account_filter`: SCENARIO-14.
  - `renderSpendingJSON`: SCENARIO-16.
  - E1/E2 warnings: SCENARIO-17 (an empty window prints only caption + header, with no stderr).

**Traps:**
- Binding a local-zone `time.Time` to a DATE comparison: go-duckdb converts it to UTC and can shift the day. Bind civil days.
- The spec's sample table is illustrative: its Category column is wider than any cell it shows. Derive widths from the cells.
- A nil `renderJSON` passed to `renderResult` makes `spend --json` panic.
- The Long and Example text name `--by`/`--since`/`--account` before those flags exist. They stay verbatim.
- Existing cli tests build `cli.Env` without `Now`. Every spend test must set it, because a nil `Now` panics by design.

## Phase report

Run B1 done (steps 3-5); steps 3-5 ticked. Narrow loop green for `internal/report`, `internal/store/duckstore`, `internal/cli`. The three cmd acceptance tests are still red by design (cli RunE is the stub; B2 fills it).

Files:
- `internal/report/spending.go`: `DefaultWindow` (`now.Date()` in now's zone, UTC-midnight days), `Spend` (DefaultWindow -> `store.Spending` with `SpendByCategory` -> `readRefusal(ctx, "spend", err)`).
- `internal/store/duckstore/spending.go`: `Spending` is one statement, `GROUPING SETS ((category, currency), (currency))` over `v_spending`. Since and until bind as `YYYY-MM-DD` text through `CAST(? AS DATE)`. HAVING drops zero-net detail rows and keeps every total. `ErrUnsupportedGrouping` (exported sentinel, err113) is returned for any other `By`, before the store is opened. `AccountIDs` is still unread (SCENARIO-14).
- Tests: `internal/report/spending_test.go` (new), `refusal_test.go` (spend rows: refusal copy, interrupt), `internal/store/duckstore/spending_test.go` (new: 5 behaviours + open/query/scan faults, close, missing store), `views_test.go` (`splitSpec.date` added, zero = 2026-03-15).
- Fakes: `internal/report/fakes_test.go` `fakeStore.spending` + `gotSpending *store.SpendingParams`; `internal/cli/fakes_test.go` `fakeReportStore.spending` + `gotSpending` (B2 uses both).

Mutations (all red, then restored byte-identical): until `<=`->`<` and since `>=`->`>` both redden `Test_spending_counts_the_windows_first_and_last_day_only` (actual 200 / 400 vs 600); `now.Date()`->`now.UTC().Date()` reddens both subtests of `Test_default_window_runs_from_january_first_to_today_in_the_instants_own_zone`.

Next (B2): step 6 (cli `spend.go` RunE, `render_spend.go`, render/cli tests, R1/I1/U8 rows in cmd tests). Then V. Lint was run for report and store only.
