---
id: SCENARIO-01
status: open
---

# SCENARIO-01: sync stamps the store's format and records what status needs

Size verdict: OWNS A RUN (one `When`, M; no fold or split seam - S02 needs every column this writes).
Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched; `Replace`'s swap/cleanup is unchanged)
Acceptance test: `cmd/quarry/run_store_info_test.go` `Test_run_stamps_the_store_with_its_format_and_the_build`
Narrow loop: `go test ./internal/store/... ./internal/importer/ ./internal/snapshot/ ./cmd/quarry/ -run 'StoreInfo|ImportRun|Replace|TransferTotals|TakenAt|BuildVersion|SnapshotRef|Stamps'`
Mutation checks: `checkTransferTotals` comparison (drop the one-sided term) -> `Test_checkTransferTotals_refuses_rows_that_are_not_paired_plus_one_sided`; `importVerified` reading the manifest's `TakenAt`/`Source` (use `time.Now()` / `""`) -> `Test_import_from_passes_the_recorded_taken_at_and_source_to_the_importer`; `(devel)` fallback in duckstore -> `Test_replace_records_devel_when_no_quarry_version_is_given`

Existing (do not re-plan): `import_runs` table + `importRunRows` (`duckstore.go:346`), `Validation.Balances.{NeverReconciled,InvestmentAccounts}` and `Transfers.{Paired,CrossCurrency}` already computed (`store.go:186,227`), `Manifest.Snapshot.{Source,TakenAt}` (`manifest.go:19-26`), `OpenReadOnly` for test reads. Read side, views, lockdown: SCENARIO-02+.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_store_info_test.go` `Test_run_stamps_the_store_with_its_format_and_the_build` - `run(["sync","--quicken",dir])` on a v9fixture bundle (never-reconciled account, brokerage account, one paired cross-currency transfer, one one-sided leg); reads the store back via `duckdb.OpenReadOnly` (pattern: `run_import_runs_test.go:20-72`). Asserts: one `store_info` row, `format_version` 2, `quarry_version` non-empty, `built_at` within a before/after wall-clock window and `>= import_runs.finished_at`; `snapshot_taken_at` = manifest `taken_at` (UTC), `source_path` = manifest `source`, and the four count columns. SQL-only, compiles with no stubs: red = "table store_info does not exist".

### Build
- [ ] Step 2: `internal/store/store.go:106-124` `SnapshotRef` (+`TakenAt time.Time`, +`Source string`), `ImportRun` (+`BalancesNeverReconciled`, `InvestmentAccounts`, `TransfersPaired`, `TransfersCrossCurrency`) - exchange types only; doc says TakenAt/Source are zero unless filled from a manifest.
- [ ] Step 3: `internal/importer/importer.go:112-152` `Import`, `newImportRun` + new unexported `checkTransferTotals(transfers []store.Transfer, check store.TransferCheck) error` in `transfers.go` - call after `pairTransfers` (line ~104); fill the four new run fields from `validation`. Tests: extend `import_runs_test.go:14-45` (never-reconciled + investment account + paired cross-currency + one-sided; fix the exact `ImportRun` literal); `Test_checkTransferTotals_refuses_rows_that_are_not_paired_plus_one_sided` (white-box, both directions; matching control arm) - the only route to the branch, `pairTransfers` builds the invariant.
- [ ] Step 4: `internal/snapshot/import.go:95-97` `importVerified` - fill `TakenAt` (`time.Parse(RFC3339)` then `.UTC()`; unparseable -> zero, import proceeds) and `Source` from `manifest.Snapshot`, as the existing `Path/SHA256` are (recomputed values stay for those). Tests in `import_from_test.go` (near :185) and `sync_and_import_test.go:105-118`: `Test_import_from_passes_the_recorded_taken_at_and_source_to_the_importer` (far-past recorded `taken_at` with a non-UTC offset, edited `source`; asserts UTC instant), `Test_sync_and_import_passes_the_manifests_taken_at_and_source_to_the_importer`, `Test_import_from_imports_with_a_zero_taken_at_when_the_manifest_time_is_unparseable`.
- [ ] Step 5: `internal/store/duckstore/schema.go:68-89` + `duckstore.go` (`Store`/`New`/options :58-80, `Replace` :107-154, `build` :214, `importRunRows` :346) - DDL: `store_info` per P2a-4 and six `import_runs` columns nullable per P2a-5; exported `const FormatVersion = 2`; `WithQuarryVersion(v string) Option` (empty keeps default `(devel)`, never stored empty); `Replace` stamps `builtAt := time.Now().UTC()` after the ctx/create step and before `build`, passes `(quarryVersion, builtAt)` into `build`, which appends `store_info` last (after `import_runs`); zero `TakenAt` and empty `Source` append as NULL (small `nullableTime` beside `nullableStr`). Tests (`duckstore_test.go`): extend `minimalRows`/round-trip (:23-95, non-zero TakenAt/Source + new counts); `Test_replace_writes_one_store_info_row_with_the_format_version_and_build_time`; `Test_replace_records_the_quarry_version_it_is_given`; `Test_replace_records_devel_when_no_quarry_version_is_given` (no option and `WithQuarryVersion("")`); `Test_replace_stores_null_when_the_snapshot_has_no_taken_at_or_source`; fault: fake `DB` (via `WithCreate`, pattern :342) whose `AppendRows("store_info")` fails -> `Replace` errors, partial removed, prior store byte-identical. Update the `build(...)` call at `duckstore_internal_test.go:25`.
- [ ] Step 6: `cmd/quarry/run.go:38-60` `newServerFactory` + new unexported `buildVersion(info *debug.BuildInfo) string` (nil or empty `Main.Version` -> `""`) - `info, _ := debug.ReadBuildInfo()`; prepend `duckstore.WithQuarryVersion(buildVersion(info))` ahead of `storeOpts` so fault tests still override. Test: `Test_buildVersion_reads_the_main_module_version` (nil, empty, `v1.2.3`).

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; update `doc.go` of `duckstore` (store_info, FormatVersion) and doc comments on the new exported symbols within budget; bump exact-shape assertions the new columns break (`fakeStore`-based `ImportRun` literals).

### Verify
- [ ] Step 8: full verification per `.claude/rules/agent-briefs.md` + `.claude/scripts/spec-check.py phase2a-read-foundation` -> tick SCENARIO-01 with its acceptance test.

## Handoff

**Binding decisions**:
- `built_at` = `duckstore.Replace`'s own `time.Now().UTC()` taken just before `build` = when the store file was written; it is always `>= import_runs.finished_at` (Phase 1 stamps that before `Replace`, so it excludes write/checkpoint). `status` shows `built_at`, never `finished_at`; no injected clock.
- `format_version` is duckstore's exported `FormatVersion = 2`; S02's read-side check compares against that symbol, not a literal. `quarry_version` is set via `duckstore.WithQuarryVersion`, wired from `cmd/quarry` (`buildVersion`); the `(devel)` default lives once, in duckstore, so no caller can store an empty string.
- Transfers invariant failure = returned error (`checkTransferTotals`), not a panic (a Go panic exits 2 = the usage code, with a stack trace) and no new copy: it flows `Import` -> `importFailureRefusal` default arm (S3, `import.go:~120`). The call-site branch carries `// unreachable:` (pairTransfers appends one row and bumps exactly one of Paired/OneSided per iteration, `transfers.go:~62-90`); the check function itself is unit-tested. No product-vision ruling needed.
- `SnapshotRef.TakenAt`/`Source` are filled only in `importVerified` from the manifest (recorded under `--from`); `Source` is stored verbatim; unparseable `taken_at` -> zero -> NULL `snapshot_taken_at` (no refusal; `--from` never validated it). S02's `status` must render a NULL `snapshot_taken_at`/`source_path` rather than assume non-NULL; the columns are nullable by spec.
- `store_info` is appended last in `build` (after `import_runs`): a store carrying it is complete. `Counts` does not count `store_info` either.

**Left unbuilt**: read side (`duckstore` open/read port, `internal/report`), `v_account_balances`, read lockdown, R1-R3 refusals, `status`/`accounts`/`sql` - SCENARIOs 02+.

**Traps**:
- The test binary's `debug.ReadBuildInfo().Main.Version` is `""` or `(devel)` either way, so no cmd-level test can tell "wired" from "not wired"; only the duckstore option test and `buildVersion` test carry the proof. Do not claim a wiring mutation.
- DuckDB `TIMESTAMP` is microsecond UTC; assert `built_at >= finished_at`, never `>`.
- Acceptance test reads the store via `OpenReadOnly` only after `run` returns (sync's connection is closed; InstanceCache refuses a second differently-configured open in-process while one is live).
- Existing `fakeStore` captures `Rows` by value; assert the new `ImportRun` fields field-wise around the wall-clock times, never whole-`Rows` equality.
