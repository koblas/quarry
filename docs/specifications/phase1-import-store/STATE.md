# phase1-import-store — current state

Scenarios complete: SCENARIO-01a (+ folded 04, 05), SCENARIO-01d. Last updated by SCENARIO-01d.

## Binding decisions
- Store ADR: `docs/adr/001-shared-store-package.md`. `internal/store` = row types only, no driver; `internal/store/duckstore` = DDL + builder + atomic swap — keeps DuckDB out of snapshot/cli/importer test binaries (verified via `go list -deps -test`). (SCENARIO-01a)
- `importer.Store` is one method: `Replace(ctx, store.Rows) (string, error)`. `importer.Server.Import(ctx, snapshotPath) (store.Result, error)` maps a v9 snapshot end to end; validation runs on `store.Rows` in memory before `Replace`, so a failed check never creates a partial. (SCENARIO-01a)
- Sequencing ADR: `docs/adr/002-sync-sequences-import-in-snapshot.md`. `snapshot.Importer` is a one-method port shaped to `(*importer.Server).Import` (no adapter needed); `snapshot.WithImporter`, `snapshot.WithStorePath(filepath.Join(dir, duckstore.FileName))` — S1–S3 copy uses the dir, S4/V1/I2 the file; `snapshot` never spells `quarry.duckdb` itself. `(*Server).SyncAndImport(ctx, bundlePath) (Outcome, error)` calls `Sync` unchanged, skips the import (`Outcome.Store == nil`) on a mismatch or any other `Sync` failure. An import failure comes back as a store refusal (`storeRefusalError`) that wraps the importer's error via `Unwrap`, never through `FailureOutcome` — the snapshot stays committed. `(Outcome).StdoutWriteRefusal` is the O1/O1b choice: O1 while `Store == nil`, O1b once it is set. 01b's V1 must set `Store` (with a not-built flag) so V1's stdout failure gets O1b too. `cli.ServerFactory` and `cli`'s shape are unchanged; `cli` only renders `Outcome`. 03's `--from` entry reuses `SyncAndImport`/`Outcome` with no new cli wiring. (SCENARIO-01d)
- Money is int64 cents in `store.Rows`, parsed exactly from `typeof(col)` + `CAST(col AS TEXT)` — never floats. `parseMoney` classifies by `typeof()` alone; a NULL amount is checked by the caller before `parseMoney` runs. 01b's statement-balance parsing must reuse `parseMoney`, not reimplement it. (SCENARIO-01a, checkpoint fix)
- IDs are `<prefix>-<Z_PK>` VARCHAR (acct/cat/payee/tag/txn/split), derived from each row's own Z_PK. 01c's transfer ids derive from split ids. (SCENARIO-01a)
- `*importer.UnmappableError{Reason}` carries the ruled `<reason>` text verbatim; `errors.As`/`errors.Is` both reach it through 01d's `storeRefusalError` wrap. S4 offenders accumulate across every mapping step (`s4ClassOrder`: 7, then 1-6, then 11, then 8-10) in `internal/importer/offenders.go`. 01d renders `Reason` in the interim S3 frame; SCENARIO-14 swaps the frame, not the reason text. (SCENARIO-01a)
- P1-5d: a reference to a **deleted** row is dropped silently with whatever depends on it; a reference to a row that **does not exist at all** is treated as NULL (reason 10 on required, stored NULL on nullable). `mapAccounts`/`mapCategories`/`mapTags`/`mapPayees`/`existingTransactionPKs` each expose an `exists` set for this. (SCENARIO-01a, checkpoint fix)
- `v9fixture.Builder` writes real SQL NULL for every zero ref and every empty required string; `TagRow.Type` is `*int64`. `Builder.WithoutEntity(name)` omits a `Z_PRIMARYKEY` row. `OpenBundle`/`ExtraSchemaBundle` now seed typed accounts (`ZTYPENAME`/`ZCURRENCY`) plus `Z_PRIMARYKEY` rows for CashFlowTransaction/CategoryTag/UserTag (`requireEntityPrimaryKeys`), so both import cleanly; `MissingSchemaBundle` stays untyped on purpose (schema mismatch skips import anyway). (SCENARIO-01a; typing SCENARIO-01d)
- Every account/category boolean is read via `COALESCE(col, 0)` — a NULL imports as `false`. 01b/06's account labelling must assume this default. (SCENARIO-01a)
- `(*duckdb.DB).Create`/`CheckpointClose`/`AppendRows`/`Decimal`, and `IsDiskFull`/`IsPermission`, live in `internal/platform/duckdb`. `QueryRows(ctx, query, args, row)` is the shared multi-row read shape on both `platform/sqlite` and `platform/duckdb` — 01b's reconcile-record reads should use it. (PREP-c, carried forward)

## Left unbuilt
- Pre-swap ctx check (I2), S1/S2 classification, EDQUOT routing — SCENARIO-14. `duckstore.Replace` does not check ctx before its rename.
- Stale `quarry.duckdb.wal` removal, `.partial` leftover sweep — SCENARIO-20.
- `transfers`, `import_runs` tables; `not_imported` count — 01c, 08, 01c/S21. `store.Counts.Transfers` is always 0 until 01c.
- Reconcile-record reads, S4 reason 5, statement required-NULL reasons — 01b. **Until 01b lands, production (`cmd/quarry` is now wired end-to-end) swaps in an unchecked store: no balance or split-sum gate exists yet, on every real `quarry sync`.**
- `--json` `store` key and `store: null` on mismatch — SCENARIO-02 (SCENARIO-18's acceptance test is 02's; 01d builds the skip-on-mismatch but does not tick 18).
- Balances/Splits/Transfers lines, V1 — 01b/01c; `; N investment transactions not imported` — 01c.
- sync help text (01b), `--from` flag (03). 01d did not touch `internal/cli/sync.go`'s Short/Long/Example.
- `snapshot.Server` method for `--from` (`Manifest` decode + snapshot→manifest resolution, reusing `buildManifest`'s integrity/hash/schema steps) — SCENARIO-03.

## Traps
- `WithEntity`/`WithoutEntity` overrides are the only proof entity numbers are resolved from `Z_PRIMARYKEY`, never hard-coded.
- SQLite NUMERIC affinity stores a whole-valued decimal string as INTEGER, not TEXT/REAL — `parseMoney` branches on `typeof()`, not on the source column's declared type.
- `mattn/go-sqlite3` auto-converts a TIMESTAMP-declared column to a Unix-epoch `time.Time` unless the query casts it — `ZPOSTEDDATE`/`ZENTEREDDATE` need `CAST(... AS REAL)`.
- Payees are `ZTRANSACTION.ZUSERPAYEE` → `ZUSERPAYEE.ZNAME`, not `ZBPFIPAYEE`.
- DuckDB's `InstanceCache` refuses a second connection to the same path with a different config while the first is open — close a writer `*duckdb.DB` before `OpenReadOnly` on the same path.
- Appender faults (constraint violations) surface at `Close`/`Flush`, not at `AppendRow`.
- `go mod tidy` run before any `.go` file imports `duckdb-go` removes it from `go.mod` again.
- `buildManifest` (snapshot package) still reads with concrete `sqlite`/`os` calls, no read port — 01d/03 must not grow a second ad hoc snapshot reader; extract/reuse its integrity/hash/schema steps instead.
- `uncovered-diff.py` is blind to untracked files (`git diff <start>` doesn't see them) — `git add` before trusting its output.
- `MissingSchemaBundle` has no `Z_PRIMARYKEY` rows: it fails at S4 reason 7 if the mismatch skip is ever removed, and creates no store — a cmd-level "no store" assertion cannot catch that on its own; `sync_and_import_test.go`'s fake-`Importer` test is the proof the skip itself works.
- O1b and the S3 refusal both tell the user to run `--from`, which does not exist until SCENARIO-03 — ruled copy; do not "fix" it early.
- `cmd/quarry`'s test binary now links DuckDB (first package outside `duckstore`/`platform/duckdb` to do so); linux-small CI has OOMed linking it (`devenv.nix`). macOS Verify will not show it.
- `go list -deps` without `-test` misses test-only imports; the confinement check needs `-test`.

## Open debts
- `--json` stdout gains top-level `store` key (P1-11); `Test_run_prints_the_manifest_as_json_with_the_json_flag` needs to change to "stdout minus `store` equals the manifest" — owned by SCENARIO-02.
- `duckstore.Replace`'s `CheckpointClose`-failure branch, and `build`'s schema-exec failure via a real fault, have no black-box trigger through the current API — declared unreachable with a stated reason each; SCENARIO-14's fault work may want a seam to exercise them for real.
