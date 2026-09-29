---
id: SCENARIO-14
status: done
---

# SCENARIO-14: A failed build never replaces the store (absorbs SCENARIO-13)

Cadence: test-first — write-safety guard (partial never replaces the store; pre-swap ctx check) and atomicity adapter (`duckstore` temp-then-rename)
Acceptance test: `cmd/quarry/run_store_faults_test.go` `Test_run_never_replaces_the_store_when_the_build_fails`
Acceptance test (SCENARIO-13, folded): `cmd/quarry/run_import_test.go` `Test_run_refuses_an_unmappable_value_and_keeps_the_snapshot`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/importer/ ./internal/snapshot/ ./cmd/quarry/ -run 'replace|Unmappable|sync_and_import|never_replaces|unmappable'`
Mutation checks: (each `Replace` failure exit calls `removePartial` itself — no single deferred cleanup) drop the permission tag in `Replace` → S1 row; drop the disk-full tag → S2 row; drop the I2 arm in snapshot → I2 row (prints S3); delete the pre-swap ctx check, or move it after `os.Rename` → I2 row (exit 0, store bytes differ); drop `removePartial` at the AppendRows / CheckpointClose / pre-swap exit → S3 / S2 / I2 row respectively (one at a time); drop the S4 arm → `Test_run_refuses_an_unmappable_value_and_keeps_the_snapshot`; add an unconditional `ctx.Err()` check after a successful `Import` → `Test_sync_and_import_completes_normally_when_the_context_ends_after_a_successful_import`

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_store_faults_test.go` (new) `Test_run_never_replaces_the_store_when_the_build_fails` — one table, four rows through `runWith`; each row seeds `quarry.duckdb` with sentinel bytes (pattern `run_validation_test.go:213-221`) and asserts exit 1, empty stdout, the exact spec *Refusals* stderr line, no `.partial`/`.wal` in the store dir, store byte-identical. Rows, one fault site each: S1 real `chmod 0500` on the store dir (create `snapshots/` 0700 first); S3 fake DB whose `AppendRows` calls the real one twice on one table (real duplicate-key shape); S2 fake `CheckpointClose` returning the driver IO "No space left on device" error wrapped as `CheckpointClose` wraps it (`platform/duckdb/duckdb.go:160-164`), after real appends; I2 fake calls the real `CheckpointClose`, then cancels the run's ctx
- [x] Step 2: wire the seam for real (pure wiring, no behaviour): `internal/store/duckstore/duckstore.go:23-31,81` exported `DB` port (`Exec`, `AppendRows`, `CheckpointClose`, `Close`; `Decimal`, `os.Rename`, `os.Remove` stay concrete), `Option`, `WithCreate`, variadic `New`, `build` takes `DB`, `*duckdb.DB` default behind a compile-time guard; `cmd/quarry/run.go:37-57,67-78` `newServer` → `newServerFactory(storeOpts ...duckstore.Option)`, `run` delegates to `runWith(…, newServerFactory())`. Expected: S1, S2 red (S3 frame printed), I2 red (exit 0, store bytes differ), S3 **green on arrival** (frame and removal exist) — report it, do not manufacture red
- [x] Step 3: `cmd/quarry/run_import_test.go:159-180` `Test_run_refuses_an_unmappable_value_and_keeps_the_snapshot` — seed an existing store, assert the exact S4 line and the store byte-identical; red on the S3 frame

### Build
- [x] Step 4: `internal/store/store.go:157-158` `ErrStoreNotWritable`, `ErrDiskFull`, `ErrUnmappable` — sentinels beside `ErrValidationFailed`
- [x] Step 5: `internal/store/duckstore/duckstore.go:38-71` `Replace` — every failure exit calls `removePartial` (partial + `.wal`) and returns through one tagging helper: `duckdb.IsPermission` → `ErrStoreNotWritable`, `duckdb.IsDiskFull` → `ErrDiskFull`; pre-swap `ctx.Err()` check immediately before `os.Rename`; delete the `// unreachable` at `:55-59`. Tests red first in `duckstore_test.go`: `Test_replace_tags_a_read_only_store_directory_as_not_writable` (extend `:242-264`), `Test_replace_removes_the_partial_and_wal_when_the_checkpoint_fails` (disk-full shape → `ErrDiskFull`), `Test_replace_does_not_swap_when_the_context_ends_after_the_checkpoint` (`ErrorIs context.Canceled`, store byte-identical), `Test_replace_does_not_tag_an_unrelated_build_failure` (duplicate PK → neither sentinel); fault test: `WithCreate` creator returning an error
- [x] Step 6: `internal/importer/errors.go:3-14` `(*UnmappableError).Is` — matches `store.ErrUnmappable`; `Test_UnmappableError_matches_ErrUnmappable_and_keeps_its_reason` in `coverage_test.go:146` area
- [x] Step 7: `internal/snapshot/import.go:93-121` `SyncAndImport` + replace `storeBuildRefusal` with one `importFailureRefusal(ctx, manifest, err)` — order V1 (existing branch) → I2 (`ctx.Err() != nil`) → S4 → S1 → S2 → S3 default; every arm stays a `storeRefusalError` (Unwrap kept). Steps 1 and 3 go green. Tests red first in `sync_and_import_test.go` (fake importer, shapes as `Import` wraps them: `replace store: build store: …`): `Test_sync_and_import_reports_each_build_failure_with_its_refusal` (rows S1, S2, S3, S4 exact line; I2 via a fake that cancels ctx then fails; I2 beats a tagged disk-full and an `UnmappableError`), `Test_sync_and_import_reports_an_untagged_permission_fault_as_s3` (bare `fs.ErrPermission`), `Test_sync_and_import_completes_normally_when_the_context_ends_after_a_successful_import`; keep `:105-122` as the S3 control

### Sweep
- [x] Step 8: fix what `go build ./... && golangci-lint run ./...` reports; doc comments on `DB`, `WithCreate`, `Replace` (names the sentinels and the pre-swap check), `importFailureRefusal`, the three sentinels; `gofmt` `internal/snapshot/sync_faults_helpers_test.go` (Open debt, cheap)

### Verify
- [x] Step 9: full verification per `.claude/rules/agent-briefs.md` + the mutation checks above, one at a time + `.claude/scripts/spec-check.py phase1-import-store` → tick SCENARIO-14 with its acceptance test and SCENARIO-13 as "FOLD → 14, delivered by SCENARIO-14" with its test; STATE.md drops the CheckpointClose-unreachable debt

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Faults are classified in `duckstore` and cross into `snapshot` only as `store` sentinels — `snapshot` must not import `platform/duckdb` (links DuckDB into its test binary) nor `importer` (feature→feature).
- `snapshot` branches only on those sentinels, never on `fs.ErrPermission`/`ENOSPC` — a permission fault reading the snapshot is S3, not S1 ("cannot write to <store dir>" would be false).
- S1 is classified from the build error, no preflight probe — a probe is TOCTOU and a second write; `IsPermission` already covers PathError and driver IO shapes.
- S1/S2 text is the table's fixed literal (`permission denied`, `no space left on device`, EDQUOT included), never `causeText` — DuckDB's IO message is long and varies.
- Precedence: V1 → I2 → S4 → S1 → S2 → S3. I2 = a non-V1 import error while `ctx.Err() != nil`; a successful `Import` is never reclassified (P1-14).
- The only ctx gate is `Replace`'s check immediately before `os.Rename`; SCENARIO-20's sweep must not add a second one after it.
- `duckstore.WithCreate` + `DB` port and `cmd/quarry` `runWith`/`newServerFactory` are the fault seams; 03/15/20 reuse them rather than adding package vars.

**Left unbuilt** — named so nobody assumes it exists:
- Snapshot-side EDQUOT (`SQLITE_IOERR_WRITE` during `Source.Backup` → R14b) — not folded: Phase-0 copy on a `platform/sqlite` classifier with no Outline row; stays in Open debts as a standalone named-bug fix (copy already ruled).
- `.partial` leftover sweep, stale `quarry.duckdb.wal` removal — SCENARIO-20.

**Traps** — things that look right and are not:
- `UnmappableError` matches `ErrUnmappable` via `Is`, not `Unwrap`/multi-`%w` — `causeText` walks the single Unwrap chain and would print the sentinel instead of the reason.
- The duckstore tag must keep a single `Unwrap` to the cause — S3's `<reason>` is `causeText`.
- Fake `DB.Close` is called again after a failed `CheckpointClose` — make it tolerate a second call.
- S1 row: without `snapshots/` created before the chmod, Phase 0's `Prepare` refuses first and the row tests the wrong refusal.
- The cmd test imports `duckdbdriver` only for the S2 error shape (no real disk-full fixture); S3 uses a real duplicate-key failure.
