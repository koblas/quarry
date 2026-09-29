---
id: SCENARIO-01
status: open
---

# SCENARIO-01: sync records which transactions Quicken leaves out of reports

Cadence: code-first — no bug fix, write-safety guard or atomicity adapter touched
Acceptance test: `cmd/quarry/run_sync_reports_test.go` `Test_run_sync_records_which_transactions_are_excluded_from_reports`
Acceptance test (SCENARIO-02, folded): `cmd/quarry/run_sync_reports_test.go` `Test_run_sync_records_which_accounts_are_used_in_reports`
Acceptance test (SCENARIO-03, folded): `cmd/quarry/run_sync_reports_test.go` `Test_run_sync_dates_each_transaction_by_its_register_date`
Acceptance test (SCENARIO-04, folded): `cmd/quarry/run_sync_reports_test.go` `Test_run_sync_stores_uncategorized_splits_with_no_category`
Narrow loop: `go test ./internal/importer/ ./internal/store/duckstore/ ./cmd/quarry/ -run 'Sync_records|Sync_dates|Sync_stores|Import|Replace|Report|Register|Uncategorized'`
Mutation checks: kind test in the Uncategorized match (`categories.go`) → `Test_import_keeps_an_expense_category_named_uncategorized`; full_path (not name) in the same match → `Test_import_keeps_a_system_subcategory_named_uncategorized`
Runs: A (1-2) | B1 (3-5) | B2 (6) | V (7-8)
Size: OWNS A RUN — 4 batches, 1 feature package (importer; duckstore/store/v9fixture plumbing); absorbs FOLD 02, 03, 04

User-visible contract: `quarry sync` output, stderr and exit codes unchanged. Store gains `transactions.excluded_from_reports BOOLEAN NOT NULL`, `transactions.posted_date DATE` (nullable), `accounts.in_reports BOOLEAN NOT NULL`; `store_info.format_version` = 3, so a format-2 store reads as R2 (already built).

Existence notes: no curated Z-column list exists outside the query SQL — both columns are already in `reference.sql:79,83`, so the schema fingerprint needs no step. No `SELECT *` / positional reads over `accounts`/`transactions` in tests (only `query_test.go:57`, which is column-agnostic).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_sync_reports_test.go` (new) — the four acceptance tests above, each `v9fixture.Builder` → `syncBundle` → `duckdb.OpenReadOnly` + `stringMap` (`run_import_test.go` precedent). Fixtures: 01 one txn `ExcludeFromReports` 1, one unset; 02 accounts with `UsedInReports` 0 / 1 / nil; 03 txn entered 2026-06-01 + posted 2026-05-31, txn entered-only; 04 split on a `Type: 0` category named `Uncategorized`. Splits sum to their transaction so sync exits 0.
- [x] Step 2: plumbing so all four compile and fail at their assertion (importer untouched): `internal/store/store.go:10-19` `Account.NotInReports bool`; `store.go:70-81` `Transaction.ExcludedFromReports bool`, `Transaction.PostedDate *time.Time`; `v9fixture/builder.go:38-46` `AccountRow.UsedInReports *int64`, `:51-62` `TransactionRow.ExcludeFromReports *int64`, `Seed` INSERTs `:297` and `:310-314` gain `ZUSEDINREPORTS` / `ZEXCLUDEFROMREPORTS`; `duckstore/schema.go:13-22` `in_reports`, `:42-53` `excluded_from_reports`, `posted_date` — each LAST in its table; `duckstore.go:460-466` `accountRows` (writes `!a.NotInReports`), `:492-505` `transactionRows` (both new values appended last). Expected red: in_reports true for the off account; date = posted; posted_date NULL; category_id set; excluded false.

### Build
- [x] Step 3: `duckstore.go:24` `FormatVersion` 2 → 3 + `duckstore_test.go:24-60` `minimalRows` / `:66-101` `Test_replace_swaps_in_a_store_that_reads_back_every_row` — assert the three new columns read back (a true exclude flag, an account not in reports, a posted_date value and a NULL one); tests already compare against the constant.
- [x] Step 4: `accounts.go:40-46` `accountsQuery` + `:59-98` `mapAccounts`; `transactions.go:41-48` `transactionsQuery` + `:95-196` `mapTransactions` — read `ZUSEDINREPORTS` (NULL → in reports) and `ZEXCLUDEFROMREPORTS` (NULL → false) as `COALESCE(...) <> 0` in SQL. Tests in `accounts_test.go` / `transactions_test.go`: `Test_import_marks_an_account_quicken_leaves_out_of_reports` (0 / 1 / NULL / 2 rows), `Test_import_marks_a_transaction_excluded_from_reports` (1 / 0 / NULL / 2 rows).
- [x] Step 5: `transactions.go:47` ORDER BY → `COALESCE(t.ZENTEREDDATE, t.ZPOSTEDDATE)`; `:121-131` date = entered, posted only when entered NULL; set `PostedDate` whenever posted is present; doc comment `:81-87`. Tests: `Test_import_dates_a_transaction_by_its_entered_date` (entered/posted in different months; entered NULL → posted; posted NULL → `PostedDate` nil), `Test_import_orders_transactions_by_register_date` (two txns whose posted order is the reverse of their entered order — drop it if `fake.Rows.Transactions` order proves unobservable, and say so). Bump `import_test.go:69-72` exact `[]store.Transaction` literal with `PostedDate: &posted`.
- [x] Step 6: `categories.go:39-90` `mapCategories` also returns the PK set of categories with kind `system` AND full_path exactly `Uncategorized` (category row still emitted); `importer.go:72,94` thread it; `splits.go:19-25,80-83` `mapSplits` stores `CategoryID` nil for those PKs. Tests in `categories_test.go`: `Test_import_stores_a_split_on_uncategorized_with_no_category` (also asserts the `Uncategorized` row is in `Categories`), `Test_import_keeps_an_expense_category_named_uncategorized`, `Test_import_keeps_a_system_subcategory_named_uncategorized` (`Parent:Uncategorized`, system kind). Run both mutations on the `Mutation checks:` line individually.

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on the three new store fields, `mapCategories`' new return, `mapSplits`, `FormatVersion` (no history), `TransactionRow`/`AccountRow` field docs.

### Verify
- [ ] Step 8: full verification (`agent-briefs.md` → *Verification*) + `spec-check.py phase2b-spending`; tick SCENARIO-01 with its acceptance test and SCENARIO-02/03/04 each with `delivered by SCENARIO-01 —` before its own test (test last on the line); create `docs/specifications/phase2b-spending/STATE.md` seeded from 2a STATE.md's *Phase 2b must inherit* + binding decisions 2b reads (read seam, `storeRelations()`, refusal/read-command shape, date traps) + this Handoff.

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `store.Account.NotInReports bool` ↔ column `accounts.in_reports` (writer inverts) — zero value must mean "in reports": 31 `store.Account`/`AccountBalance` test literals rely on it. Run 2's `Accounts` read maps `in_reports` back to `NotInReports`.
- `store.Transaction.ExcludedFromReports bool` ↔ `transactions.excluded_from_reports`; `store.Transaction.PostedDate *time.Time` ↔ nullable `transactions.posted_date` — set whenever Quicken has a posted date, even when it equals `date`.
- `transactions.date` = UTC day of `ZENTEREDDATE`, else `ZPOSTEDDATE`; importer ORDER BY uses the same COALESCE order. Balance validation stays date-free.
- Quicken flag columns read as `COALESCE(col, default) <> 0`: any non-zero is set (a value like 2 never faults the scan).
- Uncategorized = category with kind `system` AND full_path exactly `Uncategorized`; its splits get `category_id NULL`, its category row stays in `categories`. Run 3's view keys on `category_id IS NULL`, never on the name.
- `FormatVersion = 3` is the only 2b bump — format 3 is unshipped, so runs 2 and 3 add columns/views without bumping again (2a precedent).

**Left unbuilt:**
- `in_reports` in `(*duckstore.Store).Accounts` / `v_account_balances`, accounts Status + `--json` `in_reports` — run 2 (SCENARIO-26).
- `v_cash_flow`, `v_spending`, their `storeRelations()` entries and `minimalRows` rows — run 3 (SCENARIO-06).

**Traps:**
- The build Appender is positional: a column added mid-`CREATE TABLE` while the writer appends at the end silently writes values into the neighbouring column. New columns go last in both.
- `txnRef.Date` now carries the register date, so dated refusal subjects and the split-mismatch listing (`validate.go:92,175`) show entered dates — copy format unchanged, no ruling needed.
- A dev store synced after this run lacks the 2b views until run 3 lands; re-sync after run 3.
- Existing importer tests set only `PostedDate`; with entered NULL they still date by posted, so only exact-struct asserts on `Transactions` need a `PostedDate` bump.

## Phase report

Run B2 (step 6) done. `importer/categories.go`: `mapCategories` returns a third map (PKs of system-kind categories whose full_path is exactly `Uncategorized`, const `uncategorizedPath`; category row still emitted); `importer.go:72,94` thread it; `splits.go` `mapSplits` takes `uncategorized` and stores `CategoryID` nil for those PKs. Tests in `categories_test.go`: `Test_import_stores_a_split_on_uncategorized_with_no_category`, `Test_import_keeps_an_expense_category_named_uncategorized`, `Test_import_keeps_a_system_subcategory_named_uncategorized`, helper `categorizedSplit`.

Mutations (each restored, diffed identical): drop `kind == "system"` -> `Test_import_keeps_an_expense_category_named_uncategorized` red ("Expected value not to be nil"); `fullPath` -> `r.name.String` -> `Test_import_keeps_a_system_subcategory_named_uncategorized` red (same message).

Green now: all four `run_sync_*` acceptance tests, `./internal/importer`. Not run: lint, full suite, coverage gate (run V). Steps 1-6 done; do not redo. Note: the plan's Narrow loop `-run` pattern is case-sensitive and matches no `Test_import_*` names; use lowercase (`import|replace|run_sync`).
