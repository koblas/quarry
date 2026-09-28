---
id: SCENARIO-08
status: done
---

# SCENARIO-08: A one-sided transfer is kept and warned about (absorbs SCENARIO-19)

Size verdict: OWNS A RUN. If it overruns, SPLIT at the 08/19 seam: W2 needs no port change, and import_runs needs nothing from W2.
Cadence: code-first. No mandatory item is touched. duckstore's temp-then-rename and partial-removal paths stay unchanged, and the new `import_runs` append rides the existing build-failure tests (`duckstore_test.go:132-206`).
Acceptance test: `cmd/quarry/run_transfers_test.go` `Test_run_keeps_and_warns_about_one_sided_transfers`
Acceptance test (SCENARIO-19, folded): `cmd/quarry/run_import_runs_test.go` `Test_run_records_an_import_runs_row_for_the_build`
Narrow loop: `go test ./internal/store/... ./internal/importer/ ./internal/snapshot/ ./internal/cli/ && go test ./cmd/quarry/ -run 'OneSided|ImportRuns|Transfers'`
Mutation checks: one-sided comparator uses `strings.Compare` on ids, or drops the split-id key → `Test_import_orders_one_sided_transfers_by_date_account_and_transaction`; `(Outcome).Warnings` puts import warnings before manifest warnings → `Test_outcome_warnings_put_w1_before_w2`; W2 emitted on an unbuilt result (per ruling R2) → `Test_outcome_warnings_omit_w2_when_the_store_was_not_built`
Rulings needed before Step 9 (orchestrator → scoped `product-vision`). Each ruling flips one condition:
- R1: on a V1 run, do `?` rows follow the V1 block's Transfers line? Recommend **yes**, because V1 "stdout carries the full block" and the Transfers line already counts them.
- R2: on a V1 run, does W2 print on stderr and enter warnings? Recommend **no**. W2 is ruled under "exit 0 / Store built", and its copy "quarry keeps them" is false when nothing was swapped in.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_transfers_test.go` `Test_run_keeps_and_warns_about_one_sided_transfers` — one bundle holding all three Outline legs plus one pair. It checks: exact stdout via `syncBlock` with the Transfers line followed by the three `?` rows in core order; stderr exactly the plural W2 line; exit 0; three `transfers` rows with `to_split_id IS NULL`.
- [x] Step 2: `cmd/quarry/run_import_runs_test.go` `Test_run_records_an_import_runs_row_for_the_build` — after a successful sync, `SELECT … FROM import_runs` returns exactly one row. Its `snapshot_path` / `snapshot_sha256` / `schema_fingerprint` equal the manifest JSON read from disk. It also checks row counts, balances_checked, zero mismatches, transfers_one_sided, investment_transactions_not_imported, and `started_at ≤ finished_at`.
- [x] Step 3: `internal/store/duckstore/schema.go:57-68` — stub `CREATE TABLE import_runs` (full column set, `id BIGINT PRIMARY KEY`) so Step 2 fails on its row count, not on a missing table. Both tests red at their assertions.

### Build
- [x] Step 4: `internal/store/store.go:94-104` `Rows`, `:194-207` `OneSidedTransfer` — add `Rows.ImportRuns []ImportRun`, new `ImportRun` and `SnapshotRef{Path, SHA256, SchemaFingerprint}`, and on `OneSidedTransfer` add `Date`, `Account`, `Currency`, `Payee`, `Amount`. Doc comments only, no logic.
- [x] Step 5: `internal/importer/validate.go:11-18,68-107` + `transfers.go:22-99`. Fill each one-sided leg's date, account, currency, payee and amount (the **split's** amount). Sort once in the core by (date, account name, account source id, transaction source id, split source id), all ids numeric. Extract one comparator shared with `checkSplits`. Update the exact-`TransferCheck` asserts at `transfers_test.go:140-192`. New `Test_import_orders_one_sided_transfers_by_date_account_and_transaction`: insert a later date first, include a same-date pair whose name order disagrees with account source-id order, include a 9-vs-10 source-id tie, and include two one-sided legs in one transaction (STATE trap 46).
- [x] Step 6: `internal/snapshot/ports.go:55-61` `Importer.Import`, `internal/importer/importer.go:41-48`, `internal/snapshot/import.go:65`. The port takes `store.SnapshotRef`, which `SyncAndImport` fills from `manifest.Snapshot.Path/SHA256` and `manifest.Schema.Fingerprint`. Rewrite the importer tests mechanically with `gofmt -r 'x.Import(a, b) -> x.Import(a, store.SnapshotRef{Path: b})' -w internal/importer/*_test.go`. Hand-edit `fakeImporter` (`sync_and_import_test.go:20-31`) to record the whole ref. New `Test_sync_and_import_passes_the_manifests_hash_and_fingerprint_to_the_importer`.
- [x] Step 7: `internal/importer/importer.go:105-128` — after validation passes and before `Replace`, set `rows.ImportRuns` to one run: StartedAt = Import entry (UTC), FinishedAt = just before `Replace`, the ref's three fields, `Counts`, and the validation/not-imported numbers. New `Test_import_hands_the_store_one_import_run_describing_the_build` (time bracket, not equality). The existing V1 tests already prove `Replace` is never called, so no run is recorded.
- [x] Step 8: `internal/store/duckstore/duckstore.go:81-116` `build` + new `importRunRows` — append `import_runs` last. Tests: `minimalRows` (`duckstore_test.go:20-49`) gains a run, `Test_replace_swaps_in_a_store_that_reads_back_every_row` reads it back, and the append-fault table (`:132-156`) gains an `import_runs` case (duplicate id).
- [x] Step 9: `internal/snapshot/import.go:19-27` — new `(Outcome).Warnings()`: `Manifest.Warnings` followed by the W2 text (no `quarry: warning: ` prefix) when one-sided legs exist, gated on R2. Copy is verbatim from spec W2, plural and singular, with the count as `%d` (V1-stderr style). Tests: `Test_outcome_warnings_put_w1_before_w2`, singular/plural, none when zero one-sided, and `Test_outcome_warnings_omit_w2_when_the_store_was_not_built` (or its inverse per R2).
- [x] Step 10: `internal/cli/render.go` — new `oneSidedRows` after `splitMismatchRows` (`:270-302`). Format: `  ? <date>  <label>  <payee|(no payee)>  <amount>  other account: …`, with three forms: `<name>` / `<name> (not in this file)` / `unknown`. Label is `accountLabel(name, cur, false, true)`. Columns are padded among the `?` rows only; amounts are right-aligned. Append the rows after Transfers in `renderStore` (`:108-116`), and in `renderStoreFailure` (`:336`) per R1. `internal/cli/sync.go:108-110` loops over `outcome.Warnings()`. Tests: `Test_oneSidedRows` covering the 3 forms, `(no payee)` and padding. Update `Test_renderStore_…` (`render_internal_test.go:265`). Per R1, update `Test_renderStoreFailure` (`:451-472`), whose zero-valued `OneSidedTransfer` would render a `0001-01-01` row, so give it real fields.

### Sweep
- [x] Step 11: fix what `go build ./... && golangci-lint run ./...` reports (missing `store` imports after the gofmt rewrite included); doc comments on every new exported symbol; `go doc ./internal/store` reads right.

### Verify
- [x] Step 12: full verification + `.claude/scripts/spec-check.py phase1-import-store` → tick SCENARIO-08 and SCENARIO-19 (FOLD → 08) with their acceptance tests; STATE.md: close "Interim W2" except `warnings[]` JSON → 02.

## Handoff

**Binding decisions:**
- W2 text lives only in `(snapshot.Outcome).Warnings()`, which returns manifest warnings first and then import warnings. The stderr loop prints it, and 02 serializes it into `warnings`. Reason: P1-11, the manifest is never rewritten.
- `Importer.Import(ctx, store.SnapshotRef)`. 03's `--from` fills it with the manifest SHA (verified by F5) and the **current-reference** fingerprint (P1-12).
- A numeric-link one-sided leg has `OtherAccount == nil` and renders `other account: unknown`. The Outline's first example rules this, which closes STATE's open item. 02's json `other_account: null`.
- A one-sided leg's `Amount` is its split's amount, not the transaction's, and 02's json `amount` inherits it. The label is `Name (CUR)`, following the Splits-row precedent.
- One-sided order is (date, account name, account source id, txn source id, split source id), all numeric, sorted once in the importer. Renderers and 02 never re-sort.
- `import_runs` columns: `id BIGINT PK` (1 in Phase 1; Phase 2 appends max+1), `started_at`/`finished_at TIMESTAMP` UTC, `snapshot_path` (absolute), `snapshot_sha256`, `schema_fingerprint`, and `<table>_rows` for each of the 8 `Counts` tables. It also has `balances_checked`, `balances_mismatched`, `splits_mismatched`, `transfers_one_sided` and `investment_transactions_not_imported`. `Counts` does not count `import_runs`.

**Left unbuilt:**
- `warnings[]` in `--json`, and `transfers.one_sided[]` — SCENARIO-02.
- `import_runs` history across rebuilds — Phase 2.

**Traps:**
- Appending W2 to `Manifest.Warnings` would make `--json` look done and break P1-11.
- `finished_at` is stamped before `Replace`, so it excludes the write, checkpoint and swap.
- `gofmt -r` also matches any other two-argument `.Import(` in the targeted files. Diff-check the rewrite.
