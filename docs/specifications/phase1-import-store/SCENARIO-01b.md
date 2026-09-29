---
id: SCENARIO-01b
status: done
---

# SCENARIO-01b: sync checks balances and split sums before swapping the store in

Cadence: test-first — write-safety guard: `Import` must not call `Store.Replace` when a check fails (P1-2, clobber of `quarry.duckdb`)
Acceptance test: `cmd/quarry/run_validation_test.go` `Test_run_checks_balances_and_split_sums_before_swapping_the_store_in`
Acceptance test (SCENARIO-06, folded): `internal/importer/validation_test.go` `Test_import_checks_closed_and_inactive_accounts_like_any_other`
Narrow loop: `go test ./internal/importer/ ./internal/snapshot/ ./internal/cli/ ./internal/quicken/v9/v9fixture/ && go test ./cmd/quarry/ -run 'Test_run_checks|Test_run_imports|Schema|Success|Help'`
Mutation checks: early return before `srv.store.Replace` in `(*Server).Import` → `Test_import_does_not_replace_the_store_when_a_check_fails`; balance equality → `Test_import_reports_an_account_whose_reconciled_sum_differs_from_its_statement`; split-sum equality → `Test_import_reports_a_transaction_whose_splits_do_not_sum_to_its_amount`; `Store: &result` on validation failure in `SyncAndImport` → `Test_sync_and_import_keeps_the_store_result_when_validation_fails`

Size: OWNS A RUN (absorbs FOLD SCENARIO-06). Multi-package (store, importer, snapshot, cli, v9fixture), one behaviour.

Contract: `quarry sync` exit 0 on all-pass; stdout = Phase 0 block + `Store`, `Rows`, `Balances`, `Splits` (spec :179-200; Transfers line is 01c's). On a failed check: exit 1, stdout empty, stderr one interim line (copy ruling pending — see Handoff), `quarry.duckdb` untouched, snapshot kept.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_validation_test.go` `Test_run_checks_balances_and_split_sums_before_swapping_the_store_in` — fixture: reconciled chequing (one non-reconciled txn so an all-txn sum would differ; an older statement with a different balance; a deleted newer statement), closed reconciled account, open inactive reconciled account, never-reconciled savings, brokerage; every txn's entries balance. Assert exact stdout incl. `Balances  3 accounts match …; 1 never reconciled and 1 investment account not checked` and `Splits    all N transactions equal the sum of their splits`, store file present, exit 0
- [x] Step 2: `internal/store/store.go` — signature-only `Validation`, `BalanceCheck`, `BalanceMismatch`, `SplitCheck`, `SplitMismatch`, `(Validation).Failed`, `ErrValidationFailed`; `Result.Built`, `Result.Validation`. Run: red at the stdout assertion

### Build
- [x] Step 3: `internal/quicken/v9/v9fixture/builder.go:78-83,328-331` `ReconcileRow` — `EndDate *time.Time` via `nullableTime`, `EndingBalance` via `nullableString`; update `builder_test.go:40`
- [x] Step 4: new `internal/importer/statements.go` + `reasons.go` statement reasons — read non-deleted `ZRECONCILERECORD` (`CAST(ZENDDATE AS REAL)`, `typeof`/`CAST(… AS TEXT)` balance via `parseMoney`), pick newest per imported non-investment account (NULL date ranks newest, then `ZENDDATE`, then `Z_PK`); only that record is parsed; S4 reasons 5, 6 (statement subject), 11, 10 `has no balance`/`has no date` into `offenders`. Tests `internal/importer/statements_test.go`: newest by date; `Z_PK` tie-break; deleted newer record ignored; older record's bad balance ignored; records on investment / deleted / missing accounts skipped; one test per reason form (NULL balance, NULL date, 3dp, too large, text). Faults: add a `ZRECONCILERECORD` row + table entry to `import_faults_test.go:45-70` and `:115-125`
- [x] Step 5 (test-first): new `internal/importer/validate.go` pure `validate(store.Rows, statements)`; wire into `importer.go:94-113` after `off.firstError()` — on `Failed()` return `Result{Built:false, Counts, Validation}` + `store.ErrValidationFailed` (unwrapped or innermost — `causeText` walks to it) and never call `Replace`; else `Built:true`. Tests `internal/importer/validation_test.go`: `Test_import_does_not_replace_the_store_when_a_check_fails` (fake Store `store_fake_test.go` saw no `Replace`), `Test_import_reports_an_account_whose_reconciled_sum_differs_from_its_statement` (bound: ±1 cent vs equal control), `Test_import_reports_a_transaction_whose_splits_do_not_sum_to_its_amount` (±1 cent; plus a zero-split transaction), never-reconciled list excludes investment accounts, investment count, `Test_import_checks_closed_and_inactive_accounts_like_any_other` (closed/active stored; both in `Checked`)
- [x] Step 6: rebalance success-path fixtures the split gate now fails — `entities_test.go:16`, `coverage_test.go:13,34,52`, `dangling_references_test.go:15,46,76,95,112,130,164,183`, `transactions_test.go:104,188` (confirm with narrow loop; S4-refusal tests are unaffected). Add a balancing entry; do not weaken what each test proves. A P1-5d case whose only entry is dropped legitimately becomes V1: assert `ErrValidationFailed` there instead
- [x] Step 7 (test-first): `internal/snapshot/import.go:59-64` — `errors.Is(err, store.ErrValidationFailed)` → `Outcome{Manifest, Store: &result}` + interim refusal (ruled copy); doc at `:17-23` says `Store` is set on a failed check. Tests in `sync_and_import_test.go`: `Test_sync_and_import_keeps_the_store_result_when_validation_fails` (fake Importer; asserts `Store` non-nil, error text, and `StdoutWriteRefusal` gives O1b)
- [x] Step 8: `internal/cli/render.go:106-114` `renderStore` + new `balancesPhrase`, `splitsPhrase` — every clause/plural form (spec :190-198): `N accounts match`/`1 account matches`/`no accounts to check`; never-reconciled and investment clauses omitted at 0, joined ` and `, singular `1 investment account not checked`; Splits `all N … their`/`the 1 transaction equals … its`/`no transactions to check`; comma-grouped. Table tests in `render_internal_test.go`; update `:225-234`
- [x] Step 9: help — `internal/cli/root.go:15-17` root Long, `internal/cli/sync.go:32-43` Short + Long verbatim from spec :142-169 (stop before the `With --from` paragraph; Example unchanged). `cmd/quarry/run_usage_test.go` `Test_run_prints_the_sync_help` asserting Short and the new Long paragraph

### Sweep
- [x] Step 10: exact-stdout bumps — `cmd/quarry/run_import_test.go:88-100` (`no accounts to check; 2 never reconciled`, `all 2 transactions …`), `run_schema_test.go:100-114`, `run_success_test.go:50-64` (`no transactions to check`); fix what `go build ./... && golangci-lint run ./...` reports; doc comments on every new exported symbol (`store` types, `ErrValidationFailed`)

### Verify
- [x] Step 11: full verification per `.claude/rules/agent-briefs.md` + `.claude/scripts/spec-check.py phase1-import-store` → tick SCENARIO-01b and SCENARIO-06 (folded, delivered by SCENARIO-01b) with their acceptance tests; rewrite STATE.md (drop the "unchecked store" line from Left unbuilt)

## Handoff

**Binding decisions:**
- Check results live on `store.Result` (`Built`, `Validation{Balances, Splits}`); mismatch/never-reconciled entries carry every display field of spec :216,227-228 — 02 and 09 render from them, never recompute or re-read `Rows`.
- `Import` never calls `Replace` when `Validation.Failed()`; it returns the result plus `store.ErrValidationFailed`. `SyncAndImport` keys on `errors.Is` and sets `Outcome.Store`, so O1b already covers V1 (ADR-002).
- On V1 `Result.Path` is empty (Replace never ran) — 09's `NOT REBUILT`/`NOT BUILT` line takes the path from snapshot's `storePath`.
- Newest statement = non-deleted, NULL `ZENDDATE` ranks newest (refuses reason 10 rather than guess), then `ZENDDATE`, then `Z_PK`. Only that record is parsed; records on investment, deleted or missing accounts are skipped silently.
- 01b renders `Splits    no transactions to check` (existing zero-transaction fixtures hit it); 01c still owns SCENARIO-12's tick.

**Left unbuilt:**
- V1 stdout block (`NOT REBUILT`/`NOT BUILT`, `!` rows), V1 stderr line — SCENARIO-09 (replaces 01b's interim line). `sync.go:74-76` still returns before writing stdout on any non-mismatch error.
- `--json` `balances`/`splits`/`never_reconciled` — SCENARIO-02. Transfers line — 01c. `--from` paragraph + Example — 03.

**Traps:**
- `causeText` (`snapshot/destination_refusal.go:15`) prints the innermost error — wrapping `ErrValidationFailed` with extra text as `%w: …` loses the text.
- SQLite `ORDER BY … DESC` puts NULLs last; NULL-newest needs an explicit `ZENDDATE IS NULL DESC` term. `ZENDDATE` is TIMESTAMP — cast to REAL.
- Copy pending a product-vision ruling before this developer runs: interim V1 stderr (proposed S3 frame with cause `validation failed`; its `--from` advice is misleading); reason 6 statement form (`has a balance of <v>, which is too large…`?); undated newest statement with a balance fault (fallback subject `a statement for "X" (source id N) …`); `1 investment account not checked`.
