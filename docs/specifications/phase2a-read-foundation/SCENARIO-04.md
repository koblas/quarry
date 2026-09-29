---
id: SCENARIO-04
status: done
---

# SCENARIO-04: accounts lists open accounts with native-currency balances

Size verdict: OWNS A RUN (L; four packages: `store`, `duckstore`, `report`, `cli`). Absorbs FOLD SCENARIO-05 (`--all` is one flag and one filter branch in the same command).
Cadence: code-first (no write-safety guard, atomic adapter or bug fix touched)
Acceptance test: `cmd/quarry/run_accounts_test.go` `Test_run_accounts_lists_open_accounts_with_their_balances`
Acceptance test (SCENARIO-05, folded): `cmd/quarry/run_accounts_test.go` `Test_run_accounts_all_lists_closed_accounts`
Narrow loop: `go test ./internal/store/... ./internal/importer/ ./internal/report/ ./internal/cli/ -run 'Account|Investment|Balance' && go test ./cmd/quarry/ -run 'run_accounts'`
Mutation checks: `<= current_date` in the view → `Test_accounts_counts_transactions_dated_today_but_not_tomorrow`; investment predicate in the view → `Test_accounts_reads_each_accounts_balance` (retirement account WITH a transaction); `source_id` tiebreak and `lower(name)` → `Test_accounts_sorts_by_name_then_source_id`; closed filter in `(*report.Server).Accounts` → `Test_accounts_leaves_closed_accounts_out_unless_asked`

## Surface (specification.md → Surface & Copy → `accounts`, verbatim)
`quarry accounts [--all]`: Short, Long and `--all` help exactly as the spec. stdout: header `Account  Type  Currency  Balance  Status` padded per column, two spaces apart, Balance right-aligned (`formatMoney`, or `not imported` for brokerage/retirement), header always (zero rows too), no totals, no trailing spaces when Status is empty. Status: `""` open+active, `inactive` open+not active, `closed` when closed (closed wins over inactive). Default leaves closed out; `--all` includes them. Exit 0, stderr empty. Store fault → exit 1 `quarry: <wrapped error>` (R1–R3 text is S15–17). H1 via the existing factory, naming `accounts`.

## Existing surface (survey)
- Investment predicate today: `investmentTypes` (`internal/importer/statements.go:10-13`), used at `statements.go:57` and `validate.go:116` (feeds `import_runs.investment_accounts`). Type values come from `accountTypeMap` (`internal/importer/accounts.go:13-24`). `duckstore` cannot import `importer` → the predicate moves down to `internal/store`.
- `schemaDDL` is a const (`internal/store/duckstore/schema.go:6-100`), executed once in `build` (`duckstore.go:266-268`). Read pattern to mirror: `status.go:15-68` (`openRead`, `QueryRows`, `defer Close`). Fault seams: `WithOpenReadOnly`, `spyReadDB`, `ioFault` (`status_test.go:170-231`), `minimalRows()` (`duckstore_test.go:24`).
- `report.Store` (`internal/report/store.go:9-14`) implementers (grep; LSP not needed for two): `*duckstore.Store` (guard `cmd/quarry/run.go:27` — no edit) and `fakeStore` (`internal/report/status_test.go:16-22`).
- cli: `newStatusCommand` (`internal/cli/status.go:11-48`) is the command pattern; `root.go:7-8` doc + `root.go:26-27` `AddCommand`; `formatMoney` (`render.go:211`). No test asserts the store's table list (checked `run_schema_test.go`, `duckstore_test.go`), so the new view bumps no count.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_accounts_test.go` (new) — fixture via `v9fixture.NewBuilder` (pattern `run_status_test.go:137-158`): open CAD `Chequing`, open USD `US Chequing`, open inactive account, `RETIREMENTIRA` account WITH a transaction, closed account, plus a transaction dated far in the future (e.g. +1 year, not +1 day: the importer writes UTC calendar dates, `current_date` is local). `Test_run_accounts_lists_open_accounts_with_their_balances`: `sync` then `accounts`; exit 0, stderr empty, stdout equals the exact table (retirement `not imported`, future amount absent, inactive listed as `inactive`, closed absent). `Test_run_accounts_all_lists_closed_accounts`: `accounts --all` lists the closed account with Status `closed`.
- [x] Step 2: signature-only stubs so Step 1 compiles and fails at its assertion — `store.AccountBalance`/`store.AccountList` types, `report.Store.Accounts`, `(*duckstore.Store).Accounts`, `(*report.Server).Accounts(ctx, includeClosed bool)`, `fakeStore.Accounts`, `newAccountsCommand(newReport ReportFactory)` registered at `root.go:27`.

### Build
- [x] Step 3: `internal/store/store.go:8-20` — `AccountTypeBrokerage`/`AccountTypeRetirement` consts, `InvestmentAccountTypes`, `IsInvestmentAccount(accountType) bool`; `AccountBalance{Account; Balance *int64}` (nil = not computable), `AccountList{AsOf time.Time; Accounts []AccountBalance}`. Importer uses them: `accounts.go:20-23` map values, delete `statements.go:10-13`, call sites `statements.go:57`, `validate.go:116`. Test `internal/store/store_test.go` (new) `Test_IsInvestmentAccount` (brokerage, retirement, chequing, `""`); existing importer investment-count tests stay green unchanged.
- [x] Step 4: `internal/store/duckstore/schema.go:6-100` — `v_account_balances` view DDL derived from `store.InvestmentAccountTypes` (the `IN (...)` list generated, never typed), run by `build` after the tables (`duckstore.go:267`). Columns: `id, source_id, name, type, currency, institution, closed, active, balance` (DECIMAL(18,2); NULL for investment types even with transactions; `0.00` via LEFT JOIN + COALESCE for none), counting only `date <= current_date`. `internal/store/duckstore/accounts.go` (new) `accountsQuery` + `(*Store).Accounts`: one statement, one-row `current_date AS as_of` driver LEFT JOINed to the view so a zero-account store still yields `AsOf`; `ORDER BY lower(name), name, source_id` (copy ruling); balance converted to BIGINT cents in SQL (SUM widens to DECIMAL(38,2), which the driver will not scan into int64). Tests `accounts_test.go` (new): `Test_accounts_reads_each_accounts_balance` (CAD+USD sums, retirement with a txn → nil, account with no txns → 0, negative sum, institution NULL/non-NULL, `AsOf` = today local); `Test_accounts_counts_transactions_dated_today_but_not_tomorrow` (dates from `time.Now()` local, no `t.Parallel` with zone swaps); `Test_accounts_sorts_by_name_then_source_id` (two same-name accounts, higher source_id inserted first; plus a case-only pair: "visa" inserted before "Zed" must sort after "Chequing" and before "Zed", so byte order fails); `Test_accounts_reads_as_of_with_no_accounts`; fault tests mirroring status: `Test_accounts_returns_the_open_fault`, `..._returns_the_query_fault`, `..._closes_the_connection_on_success_and_on_a_query_fault`, `..._fails_on_a_missing_store_without_creating_it`.
- [x] Step 5: `internal/report/store.go:9-14` + `internal/report/accounts.go` (new) `(*Server).Accounts` — Store returns every account; report drops closed ones unless `includeClosed`, keeping order and `AsOf`; store error returned unchanged (as `report.go:46-50`). Tests `internal/report/accounts_test.go` (new): `Test_accounts_leaves_closed_accounts_out_unless_asked` (both arms, closed+inactive included), `Test_accounts_returns_the_store_fault`.
- [x] Step 6: `internal/cli/accounts.go` (new) `newAccountsCommand` — Short/Long/`--all` copy verbatim; factory error, store error, write error each → `runtimeError` (pattern `status.go:21-46`). `internal/cli/render_accounts.go` (new) `renderAccounts(store.AccountList) string` + `accountStatus`. Tests `render_accounts_internal_test.go` (new) `Test_renderAccounts`: spec example bytes exactly; header-only for zero accounts; Balance column width driven by `not imported` and right-aligned header; no trailing spaces on empty-Status rows; non-ASCII name padded by rune count; closed+inactive → `closed`. cmd-level fault tests in `run_accounts_test.go`: `Test_run_accounts_refuses_when_home_is_unset` (H1 names `accounts`), `Test_run_accounts_fails_without_creating_a_store_when_none_exists`, `Test_run_accounts_reports_exit_1_when_stdout_cannot_be_written` (pattern of the status equivalents).

### Sweep
- [x] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on new symbols (1-2 lines unexported, ~4 exported); `root.go:7-8` doc names `accounts`; `duckstore/doc.go` mentions the view if it lists read contents.

### Verify
- [x] Step 8: full verification per `.claude/rules/agent-briefs.md` → *Verification* (`git add` new files first), `.claude/scripts/spec-check.py phase2a-read-foundation`; tick SCENARIO-04 with its acceptance test and SCENARIO-05 as `— delivered by SCENARIO-04 — \`cmd/quarry/run_accounts_test.go\` \`Test_run_accounts_all_lists_closed_accounts\`` (test reference last); `status: done`; fold Handoff into STATE.md.

## Handoff

**Binding decisions:**
- One investment predicate: `store.IsInvestmentAccount` / `store.InvestmentAccountTypes` feed both `import_runs.investment_accounts` (importer) and the view's NULL balance — changing one without the other makes `status` and `accounts` disagree.
- `v_account_balances` columns `id, source_id, name, type, currency, institution, closed, active, balance` are a public contract from S08 (`quarry sql` users query it) and match S07's JSON keys; `balance` NULL = investment, never "no transactions".
- `as_of` comes from `current_date` in the same statement as the balances and is returned even for zero accounts - S07 renders `store.AccountList.AsOf`, it does not query again.
- `duckstore.Accounts` returns every account sorted `lower(name), name, source_id`; closed filtering lives in `(*report.Server).Accounts` - S07's all-closed note counts what report hid, so do not push the filter into SQL.
- `FormatVersion` stays 2: format 2 is unshipped. A dev store built before this scenario lacks the view (catalog error until re-sync).

**Left unbuilt:**
- `accounts --json` (persistent root flag: prints the human table until S07, which adds `jsonOut` to `newAccountsCommand`), all-closed note + `warnings[]` (S07 absorbs S06); `accounts takes no arguments` U8 (S18); R1–R3 text (S15–17); O2 text (S21).

**Traps:**
- `SUM(DECIMAL(18,2))` is DECIMAL(38,2): cast to BIGINT cents in SQL before scanning.
- Importer dates are UTC calendar days, `current_date` is process-local: boundary tests belong at duckstore level with `store.Transaction.Date` written directly; cmd fixtures use far-future dates.
- A retirement account with no transactions reads "not imported" whether or not the predicate works: fixtures give it a transaction.
