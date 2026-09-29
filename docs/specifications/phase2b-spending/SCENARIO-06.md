---
id: SCENARIO-06
status: open
---

# SCENARIO-06: v_cash_flow keeps only real income and spending (absorbs SCENARIO-08)

Cadence: code-first — store-DDL views; no write-safety guard or atomic adapter touched
Acceptance test: `cmd/quarry/run_sql_views_test.go` `Test_run_sql_cash_flow_keeps_only_real_income_and_spending`
Acceptance test (SCENARIO-08, folded): `cmd/quarry/run_sql_views_test.go` `Test_run_sql_spending_nets_refunds_against_their_category`
Narrow loop: `go test ./internal/store/duckstore/ ./cmd/quarry/ -run 'cash_flow|spending|query_|replace_|run_sql'`
Mutation checks (one Bash call each: copy, edit, `-run`, restore, `diff`), all in `schema.go`:
  transfer `from_split_id` arm dropped → `Test_cash_flow_leaves_out_what_quicken_reports_leave_out` (from-leg row);
  `to_split_id` arm dropped → same (to-leg row); transfer key replaced by `s.transfer_account_id IS NULL` → same (unmatched-leg row);
  system-kind predicate dropped → same (system row); `excluded_from_reports` predicate dropped → same (excluded row);
  `in_reports` predicate dropped → same (not-in-reports row); zero-uncategorized drop removed → same (zero row);
  `IS DISTINCT FROM 'system'` → `<> 'system'` → `Test_cash_flow_takes_an_uncategorized_splits_flow_from_its_sign`;
  uncategorized sign arms swapped → same; flow read from parent's kind → `Test_cash_flow_takes_flow_from_the_splits_own_category_kind`;
  payee `LEFT JOIN` → `JOIN` → `Test_cash_flow_names_the_month_category_and_payee` (no-payee split); `CAST(... AS DATE)` on month dropped → same (column type `DATE`);
  v_spending negation dropped → `Test_spending_holds_expense_flow_with_its_sign_flipped`; `WHERE flow = 'expense'` dropped → same (income split);
  v_spending adds `AND category_id IS NOT NULL` → same (negative uncategorized split); v_spending reads `splits` with its own predicates minus `in_reports` → `Test_spending_holds_exactly_the_cash_flow_expense_rows`
Runs: A (1-2) | B1 (3) | B2 (4) | V (5-6)
Size: OWNS A RUN — 2 batches, 1 feature package (duckstore; cmd test only); absorbs SCENARIO-08. B split in two for the mutation count, not the code size

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_sql_views_test.go` (new) both acceptance tests + a shared `store.Rows` fixture; `cmd/quarry/run_helpers_test.go:68` (append) `replaceStore(t, home, rows)` — `duckstore.New(storeDirUnder(home)).Replace` (MkdirAll is inside `Replace`). No cmd test builds through `Replace` today (all sync a v9fixture); this is the first. Fixture: one CAD in-reports account, one `ImportRun` (sql needs none; later reads do), categories Groceries (expense), Salary (income), Adjustment (system), Returns (expense); splits: expense (the Groceries −100.00 below), income, a paired transfer whose two legs are real splits, an unmatched leg (`to_split_id` NULL, `transfer_account_id` NULL), system split, split on an `ExcludedFromReports` transaction, uncategorized +/−; for 08: Groceries −100.00 and +30.00, Returns −10.00 and +25.00. 06 runs `quarry sql "SELECT split_id, flow, amount FROM v_cash_flow ORDER BY split_id"`, 08 runs `SELECT category, sum(spent) ... FROM v_spending GROUP BY category ORDER BY category`; each asserts exact stdout, empty stderr, exit 0
- [ ] Step 2: `internal/store/duckstore/schema.go:129` (append) `cashFlowViewDDL`, `spendingViewDDL` stubs — full P2b-6 / P2b-8 column lists, `WHERE false`; `duckstore.go:389` concatenate both after `accountBalancesViewDDL()`; `query_test.go:28-33` `storeRelations()` gains `v_cash_flow`, `v_spending` (keeps `SHOW TABLES` test green). Expected red at end of A: both acceptance tests at their stdout assertion, and `Test_query_prints_every_column_of_each_table_and_view/v_cash_flow` + `/v_spending` (empty rows) — run A does not fix these

### Build
- [ ] Step 3: `schema.go` `cashFlowViewDDL` real body + `COMMENT ON VIEW v_cash_flow IS` the spec sentence verbatim ("excludes accounts where accounts.in_reports is false, as Quicken reports do."); `duckstore_test.go:40-43` `minimalRows` gains `split-4` on `txn-1`, `cat-1`, not in `transfers` (split-1 is a transfer leg) — `SELECT *` over `v_cash_flow` returns a row. New `internal/store/duckstore/views_test.go` through `st.Query`:
  `Test_cash_flow_leaves_out_what_quicken_reports_leave_out` (table: paired from-leg, paired to-leg, unmatched leg, system category, excluded transaction, account not in reports, zero uncategorized — each row one variable away from a control expense split that stays; the fixture carries a NULL `to_split_id`),
  `Test_cash_flow_keeps_closed_accounts_hidden_categories_and_investment_accounts`,
  `Test_cash_flow_takes_flow_from_the_splits_own_category_kind` (expense child under income parent and the reverse),
  `Test_cash_flow_takes_an_uncategorized_splits_flow_from_its_sign`,
  `Test_cash_flow_names_the_month_category_and_payee` (exact `Columns` names+types in P2b-6 order, `month` a first-of-month `DATE`, `category` full_path, NULL category, a split with no payee present),
  `Test_cash_flow_view_carries_its_reports_note` (`duckdb_views()` comment); run this batch's 12 mutations
- [ ] Step 4: `schema.go` `spendingViewDDL` real body — exactly P2b-8 (`SELECT … -amount AS spent FROM v_cash_flow WHERE flow = 'expense'`, nothing else). Tests in `views_test.go`:
  `Test_spending_holds_expense_flow_with_its_sign_flipped` (columns in P2b-8 order, `spent` `DECIMAL(18,2)`; refund negative; income and positive uncategorized absent; negative uncategorized present),
  `Test_spending_holds_exactly_the_cash_flow_expense_rows` (fixture carrying every left-out class, not-in-reports account included); run this batch's 4 mutations

### Sweep
- [ ] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on both DDL consts (≤2 lines each); `internal/store/duckstore/doc.go:5-6` names the two new views beside `v_account_balances`

### Verify
- [ ] Step 6: full verification + `spec-check.py phase2b-spending` → tick SCENARIO-06 and SCENARIO-08 (`delivered by SCENARIO-06 —` before its test); STATE.md rewrite

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `v_cash_flow` columns exactly P2b-6 order; `account_id`, `date`, `currency` come from `transactions` (`t.currency` = account currency at import, `importer/transactions.go:185`); `month` = `CAST(date_trunc('month', t.date) AS DATE)`; `category` = `categories.full_path` via LEFT JOIN, `payee` = `payees.name` via LEFT JOIN — SCENARIO-09..12 group on these names and `month` type.
- `v_spending` = P2b-8 verbatim over `v_cash_flow`, no predicate of its own — SCENARIO-23's spend = cashflow-spent invariant depends on one predicate owner.
- Transfer exclusion keys on `transfers.from_split_id`/`to_split_id` via `NOT EXISTS` — never `transfer_account_id` (NULL on unmatched legs).
- `COMMENT ON VIEW v_cash_flow` carries the describe_schema note (DuckDB v1.5.5 supports it, verified by probe; read via `duckdb_views().comment`). No column comments — out of scope.
- No `FormatVersion` bump (STATE: 3 is the only 2b bump).
- `replaceStore(t, home, rows)` in `cmd/quarry/run_helpers_test.go` is the cmd-level fixture path for view- and report-level tests from here on; v9fixture sync stays for importer-shaped tests.

**Left unbuilt** — named so nobody assumes it exists:
- `report.Store` spending/cash-flow port methods, aggregates (P2b-9), windows — SCENARIO-09/13/20. No tag column on the views (tags join at query time, P2b-8).

**Traps** — things that look right and are not:
- `date_trunc('month', <DATE>)` returns `TIMESTAMP` on DuckDB v1.5.5 (probed) — cast to `DATE`.
- `s.id NOT IN (SELECT to_split_id …)` with any NULL `to_split_id` is NULL for every row and empties the view — use `NOT EXISTS`.
- `c.kind <> 'system'` is NULL for uncategorized splits and drops them all — use `IS DISTINCT FROM`.
- Category kind strings live only in `internal/importer` (`categories.go:14`); duckstore uses SQL literals — do not import importer.
- `minimalRows`' transfers name `split-2`/`split-3`, which are not in `splits`; the rule fixture needs real legs.

## Phase report
