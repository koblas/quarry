---
id: SCENARIO-09
status: open
---

# SCENARIO-09: a balance mismatch leaves the previous store unchanged

Cadence: code-first
Acceptance test: `cmd/quarry/run_validation_test.go` `Test_run_leaves_the_previous_store_byte_identical_after_a_failing_sync`
Acceptance test (SCENARIO-11, folded): `cmd/quarry/run_validation_test.go` `Test_run_lists_mismatched_splits_in_the_v1_stdout_block`
Narrow loop: `go test ./cmd/quarry/... ./internal/cli/... ./internal/importer/... ./internal/snapshot/...`
Mutation checks: none (code-first; no bug fix, write-safety guard or atomicity/exclusive-create adapter touched)

**Copy ruling landed** (row order sorted once in the core; a numeric `SourceID` on both mismatch structs) — see the STATE.md handoff below. Row-format spacing verified byte-for-byte against spec :239-240: account-label field left-padded to the block's widest label + 2 literal spaces; date fixed 10 chars + 2 spaces; each of quarry/Quicken/difference right-aligned to its own column's widest value, one separator space after its fixed word. Splits row's label reuses the same helper with `closed=false, active=true` (SplitMismatch carries neither field). No column headers.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_validation_test.go` (new test, appended) `Test_run_leaves_the_previous_store_byte_identical_after_a_failing_sync` — write sentinel bytes directly to `quarry.duckdb` (no prior sync), capture them; sync once against a fixture with 3 mismatched accounts: plain (neither suffix), closed+not-active (proves `inactive` suppressed by `closed`, not merely absent), open+inactive; differing amount widths incl. one negative. Assert full V1 stdout block (`Store NOT REBUILT`, Rows, Balances DIFFER + 3 `!` rows, Splits non-DIFFER), restored stderr clause, `quarry.duckdb` bytes unchanged, exit 1. Fails today: stdout empty (interim V1).

### Build
- [x] Step 2 — the pending copy ruling landed in the spec (row order sorted once in the core: Balances/never-reconciled by account name then account source id numeric; Splits by date, account name, account source id, transaction source id numeric; a numeric `SourceID` field added to both mismatch structs, not emitted in JSON): `internal/importer/validate.go` `checkBalances`, `checkSplits` — sort `BalanceCheck.Mismatched`/`NeverReconciled`/`SplitCheck.Mismatched`; `SourceID int64` added to `store.BalanceMismatch`/`store.SplitMismatch` at `internal/store/store.go` and set in both `check*` functions. `internal/importer/validation_test.go` (new cases, black-box through `importer.Server.Import`) `Test_import_sorts_balance_mismatches_for_display`, `Test_import_sorts_split_mismatches_for_display` — feed accounts/transactions in reversed insertion order (and account/transaction source ids 9 and 10 on tied names, so a string comparison would invert them), assert output order.
- [x] Step 3: `internal/snapshot/import.go` `Outcome` — added `StoreExisted bool` (meaningful only when `Store != nil && !Store.Built`); `SyncAndImport`'s `store.ErrValidationFailed` branch sets `result.Path = s.storePath` (on the local copy) and `StoreExisted: s.previousStoreExists()`; new unexported `previousStoreExists() bool` (`os.Stat(s.storePath)`; only `errors.Is(err, fs.ErrNotExist)` is false — any other stat error is treated as "existed", per the landed ruling). `internal/snapshot/sync_and_import_test.go` (new) `Test_sync_and_import_reports_whether_a_previous_store_existed` (ENOENT vs a pre-created file) and `Test_sync_and_import_treats_a_stat_fault_as_a_previous_store` (`WithStorePath` pointing through a regular file, so stat fails ENOTDIR); updated the `Path`-bearing `result`/`want` literals in the two existing V1-refusal tests so this batch stayed green.
- [x] Step 4: `internal/cli/render.go` — `formatMoney(cents int64) string` (thousands separator, 2 decimals, leading `-`) and `accountLabel(name, currency string, closed, active bool) string` (`Name (CUR[, closed][, inactive])`); `internal/cli/render_internal_test.go` `Test_formatMoney` (0 / positive / negative / thousands-boundary) and `Test_accountLabel` (neither; closed+not-active — the discriminating case; open+inactive).
- [x] Step 5: `internal/cli/render.go` — `balanceMismatchRows([]store.BalanceMismatch) []string`, `splitMismatchRows([]store.SplitMismatch) []string` (block-wide column widths; empty payee → `(no payee)`); `Test_balanceMismatchRows`, `Test_splitMismatchRows` covering single-row (no padding needed), 2+-row (widest label, widest amount, negative-vs-positive alignment), and the empty-payee branch.
- [x] Step 6: `internal/cli/render.go` — `balancesDifferPhrase`/`splitsDifferPhrase` (`DIFFER for X of Y <noun>`, noun agreeing with Y; reuse `balancesPhrase`'s never-reconciled/investment extras via the extracted `balancesExtrasPhrase`) and `renderStoreFailure(result store.Result, storeExisted bool, home string) string` (Store NOT REBUILT/NOT BUILT via `result.Path`, Rows via `rowsPhrase`, each of Balances/Splits choosing its DIFFER form + rows or the existing success phrase independently per check); `Test_renderStoreFailure` covering NOT REBUILT vs NOT BUILT text and a balances-only-failed case (Splits line stays non-DIFFER).
- [x] Step 7: `internal/cli/sync.go` `RunE` — derive `validationFailed := outcome.Store != nil && !outcome.Store.Built`; only the `--json` path still returns early for it (interim, per scope); on the human path renders `renderSuccess` + `renderStoreFailure(*outcome.Store, outcome.StoreExisted, home)`, writes it, then (same as the mismatch path) returns the error after — the `Manifest.Warnings` loop still runs before that return so W1 still prints before the V1 line. New fault test (reuses the Phase-0 O1 tests' `failingWriter` helper) `Test_run_reports_the_o1b_refusal_when_stdout_fails_during_a_v1_render` — a V1 fixture with a writer that errors, exit 1, O1b text.
- [x] Step 8: `internal/snapshot/import.go` `validationFailedRefusal` — restored `each difference is listed on stdout; ` verbatim. Updated `internal/snapshot/sync_and_import_test.go`'s `Test_sync_and_import_reports_the_v1_refusal_for_a_failed_check` `want` strings to include the restored clause.
- [x] Step 9: `cmd/quarry/run_validation_test.go` `Test_run_refuses_a_balance_mismatch_and_leaves_no_store` — updated to assert the full NOT BUILT stdout block and the restored stderr clause (single-account mismatch; keeps first-run/no-previous-store coverage).
- [x] Step 10: `cmd/quarry/run_validation_test.go` (new test) `Test_run_lists_mismatched_splits_in_the_v1_stdout_block` — first-run fixture: balances match, one transaction with mismatched splits and no payee; assert Balances line non-DIFFER, Splits DIFFER line + `!` row with `(no payee)`.

### Sweep
- [x] Step 11: fixed what `go build ./... && golangci-lint run ./...` reported (one De Morgan simplification in `sync.go`'s RunE guard); doc comments added on `Outcome.StoreExisted`, `formatMoney`, `accountLabel`, `renderStoreFailure`, and every other new symbol.

### Verify
- [x] Step 12: full verification (`agent-briefs.md` → Verification) + `.claude/scripts/spec-check.py phase1-import-store` → ticked SCENARIO-09 and SCENARIO-11 (folded) with their acceptance tests.

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `store.Result.Path` is populated on a V1 failure too, but only by `snapshot.SyncAndImport` (on its own copy) — `importer.Server.Import`'s own contract (empty `Path` when `Built` is false) is unchanged. SCENARIO-02's `--json` `store.path` on a failed build should read this same populated field, not re-derive it.
- `Outcome.StoreExisted` is the single source for the NOT REBUILT/NOT BUILT choice — SCENARIO-01c/08/20, which extend the V1 block, read it rather than stat the filesystem again.
- `accountLabel(name, currency, closed, active)` is reused for Splits rows via `closed=false, active=true` — do not fork a second label formatter.

**Left unbuilt** — named so nobody assumes it exists:
- Sort order for `Mismatched` lists — pending the copy ruling; Step 2 is not implemented until it lands.
- `BalanceCheck.NeverReconciled` sort order — SCENARIO-02 (09 only counts it).
- Transfers line in the V1 block — SCENARIO-01c.
- `--json` V1 (`"built": false`, `mismatched[]`) — SCENARIO-02; until then V1 + `--json` stays interim: empty stdout, and the restored "each difference is listed on stdout" clause is false for that path (it names a stdout block `--json` V1 does not yet print).

**Traps**:
- The V1 and success blocks share `rowsPhrase`/`balancesPhrase`/`splitsPhrase` for whichever check did *not* fail — only the failed check's line switches to its DIFFER form.
- `validationFailedRefusal`'s clause text (singular/plural, X-of-Y forms) is unchanged by this scenario; only the trailing clause is restored.
