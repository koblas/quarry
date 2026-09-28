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

**PENDING COPY RULING — orchestrator: do not dispatch developer until it lands** (see report). Step 1's expected stdout (row order for 3 mismatches) and Step 2 both depend on it. Row-format spacing otherwise verified byte-for-byte against spec :239-240: account-label field left-padded to the block's widest label + 2 literal spaces; date fixed 10 chars + 2 spaces; each of quarry/Quicken/difference right-aligned to its own column's widest value, one separator space after its fixed word. Splits row's label reuses the same helper with `closed=false, active=true` (SplitMismatch carries neither field). No column headers.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_validation_test.go` (new test, appended) `Test_run_leaves_the_previous_store_byte_identical_after_a_failing_sync` — write sentinel bytes directly to `quarry.duckdb` (no prior sync), capture them; sync once against a fixture with 3 mismatched accounts: plain (neither suffix), closed+not-active (proves `inactive` suppressed by `closed`, not merely absent), open+inactive; differing amount widths incl. one negative. Assert full V1 stdout block (`Store NOT REBUILT`, Rows, Balances DIFFER + 3 `!` rows, Splits non-DIFFER), restored stderr clause, `quarry.duckdb` bytes unchanged, exit 1. Fails today: stdout empty (interim V1).

### Build
- [ ] Step 2 — **blocked on the copy ruling**: `internal/importer/validate.go:16-45` `checkBalances`, `:49-80` `checkSplits` — sort `BalanceCheck.Mismatched`/`SplitCheck.Mismatched` per the ruling; if ruled numeric-source-id, first add `SourceID int64` to `store.BalanceMismatch`/`store.SplitMismatch` at `internal/store/store.go` and set it in both `check*` functions. `internal/importer/validation_test.go` (new cases, black-box through `importer.Server.Import`) `Test_import_sorts_balance_mismatches_for_display`, `Test_import_sorts_split_mismatches_for_display` — feed accounts/transactions in reversed insertion order, assert output order.
- [ ] Step 3: `internal/snapshot/import.go:20-23` `Outcome` — add `StoreExisted bool` (doc: meaningful only when `Store != nil && !Store.Built`; note on `Store`'s own doc that `Path` is populated even on a V1 failure, though `store.Result`'s own contract for direct `Import` callers is unchanged); `:61-70` `SyncAndImport`'s `store.ErrValidationFailed` branch — set `result.Path = s.storePath` (on the local copy) and `StoreExisted: s.previousStoreExists()`; new unexported `previousStoreExists() bool` (`os.Stat(s.storePath)`; only `errors.Is(err, fs.ErrNotExist)` is false — any other stat error is treated as "existed", since "unchanged" holds either way). `internal/snapshot/sync_and_import_test.go` (new) `Test_sync_and_import_reports_whether_a_previous_store_existed` (ENOENT vs a pre-created file) and `Test_sync_and_import_treats_a_stat_fault_as_a_previous_store` (`WithStorePath` pointing through a regular file, so stat fails ENOTDIR — fault test per `build.md` → Planning); update `:130-148`/`:151-228`'s `result`/`want` literals for the now-populated `Path` field so this batch stays green (leave the clause text itself for Step 8).
- [ ] Step 4: `internal/cli/render.go` — `formatMoney(cents int64) string` (thousands separator, 2 decimals, leading `-`) and `accountLabel(name, currency string, closed, active bool) string` (`Name (CUR[, closed][, inactive])`); `internal/cli/render_internal_test.go` `Test_formatMoney` (0 / positive / negative / thousands-boundary) and `Test_accountLabel` (neither; closed+not-active — the discriminating case; open+inactive).
- [ ] Step 5: `internal/cli/render.go` — `balanceMismatchRows([]store.BalanceMismatch) []string`, `splitMismatchRows([]store.SplitMismatch) []string` (block-wide column widths per the derivation above; empty payee → `(no payee)`); `Test_balanceMismatchRows`, `Test_splitMismatchRows` covering single-row (no padding needed), 2+-row (widest label, widest amount, negative-vs-positive alignment), and the empty-payee branch.
- [ ] Step 6: `internal/cli/render.go:106-159` — `balancesDifferPhrase`/`splitsDifferPhrase` (`DIFFER for X of Y <noun>`, noun agreeing with Y per spec :252; reuse `balancesPhrase`'s never-reconciled/investment extras) and `renderStoreFailure(result store.Result, storeExisted bool, home string) string` (Store NOT REBUILT/NOT BUILT via `result.Path`, Rows via `rowsPhrase`, each of Balances/Splits choosing its DIFFER form + rows or the existing success phrase independently per check); `Test_renderStoreFailure` covering NOT REBUILT vs NOT BUILT text and a balances-only-failed case (Splits line stays non-DIFFER).
- [ ] Step 7: `internal/cli/sync.go:60-110` `RunE` — derive `validationFailed := outcome.Store != nil && !outcome.Store.Built`; only the `--json` path still returns early for it (interim, per scope); on the human path render `renderSuccess` + `renderStoreFailure(*outcome.Store, outcome.StoreExisted, home)`, write it, then (same as the mismatch path) return the error after — keep the `Manifest.Warnings` loop running before that return so W1 still prints before the V1 line. New fault test (reuse the Phase-0 O1 tests' failing-writer helper) `Test_run_reports_the_o1b_refusal_when_stdout_fails_during_a_v1_render` — a V1 fixture with a writer that errors, exit 1, O1b text.
- [ ] Step 8: `internal/snapshot/import.go:93-98` `validationFailedRefusal` — restore `each difference is listed on stdout; ` (verbatim, spec :250). Update `internal/snapshot/sync_and_import_test.go:151-228` (`Test_sync_and_import_reports_the_v1_refusal_for_a_failed_check`)'s `want` strings to include the restored clause.
- [ ] Step 9: `cmd/quarry/run_validation_test.go:97-136` `Test_run_refuses_a_balance_mismatch_and_leaves_no_store` — update to assert the full NOT BUILT stdout block and the restored stderr clause (single-account mismatch; keeps first-run/no-previous-store coverage).
- [ ] Step 10: `cmd/quarry/run_validation_test.go` (new test) `Test_run_lists_mismatched_splits_in_the_v1_stdout_block` — first-run fixture: balances match, one transaction with mismatched splits and no payee; assert Balances line non-DIFFER, Splits DIFFER line + `!` row with `(no payee)`.

### Sweep
- [ ] Step 11: fix what `go build ./... && golangci-lint run ./...` reports; doc comments on `Outcome.StoreExisted`, `formatMoney`, `accountLabel`, `renderStoreFailure`.

### Verify
- [ ] Step 12: full verification (`agent-briefs.md` → Verification) + `.claude/scripts/spec-check.py phase1-import-store` → tick SCENARIO-09 and SCENARIO-11 (folded) with their acceptance tests.

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
