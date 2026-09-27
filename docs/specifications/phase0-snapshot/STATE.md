# phase0-snapshot — current state

Scenarios complete: SCENARIO-01a, SCENARIO-03 (folded). Last updated by SCENARIO-01a.

## Binding decisions
- `internal/quicken/v9` (+ `v9fixture`) is a data package; production `internal/snapshot` never imports it. The reference schema arrives via `WithReference(label, sqlschema.Schema)`; only tests and (later) `cmd/quarry` call `v9.Reference`/`v9.ReferenceLabel`. (SCENARIO-01a)
- BR-6 scope lives in unexported `snapshot.scopeSchema`; `sqlschema` stays generic (no Quicken knowledge). Fingerprint and diff both run on scoped schemas. (SCENARIO-01a)
- `Source` (`Open`/`Probe`/`Backup`/`Close`) and `Destination` (`Prepare`/`Backup`/`WriteManifest`/`CommitSnapshot`/`CommitManifest`) are the only two ports in `internal/snapshot`. Production adapters are unexported (`newSQLiteSource`, `newDirDestination`); `internal/snapshot/export_test.go` re-exports them as `NewSQLiteSource`/`NewDirDestination` for the `snapshot_test` package only, so fault tests can wrap the real `Destination` and inject one failing method while the rest of the pipeline runs for real, without widening the package's real API. Both are injectable in production via `WithSource`/`WithDestination`. (SCENARIO-01a)
- `Manifest.Encode()` is the only encoder (2-space indent, trailing `\n`); the four diff lists (`missing_tables`, `missing_columns`, `unexpected_tables`, `unexpected_columns`) are always non-nil empty slices, never `null`. (SCENARIO-01a)
- `Sync`'s order is: reference check → `Source.Open` → `Source.Probe` → `Destination.Prepare` → `Destination.Backup` → integrity check → `ZACCOUNT` count → SHA-256 → schema diff → `Encode` → `WriteManifest` → `CommitManifest` → `CommitSnapshot`. Manifest commits before snapshot (BR-8: a snapshot without a manifest is never accepted). Nothing touches the snapshots directory before the probe succeeds. (SCENARIO-01a)
- `Sync` takes an already-resolved bundle directory path and joins `"data"` itself. Discovery, `~/` expansion and R4–R7 path validation belong to 04/05. (SCENARIO-01a)
- Connection-level pragmas the mattn/go-sqlite3 driver always runs on `Open` (`busy_timeout`, `locking_mode=NORMAL`, `synchronous=NORMAL`) do not write to the file's persistent state (they are per-connection settings) — reported per BR-1, not a blocker. `mode=ro` in the URI restricts the always-passed `READWRITE|CREATE` C-level open flags; dropping it is exactly what `Test_open_read_only_refuses_writes` and the SCENARIO-03 injected-checkpoint control catch. (SCENARIO-01a)

## Left unbuilt
- Error classification, refusal copy and exit codes: R8 (06), R9 + `WithBusyTimeout` (07), R10/R11/R12 (08), R13 (13), M1/W1 + outcome policy (10/09). `platform/sqlite.IntegrityCheck` and the `ZACCOUNT` count are built and run in BR-8 order inside `snapshot.buildManifest`, and are exercised end-to-end (`Test_sync_wraps_an_error_when_the_snapshot_fails_integrity_check` uses a fixture whose `ZACCOUNT` stays queryable while only `integrity_check` fails) — but they return generic wrapped errors. SCENARIO-08 owns turning that into R10/R11/R12's refusal copy, classification and exit codes, not the check mechanics themselves.
- R14 (disk-full/write-error during backup or manifest write) unit test: **unowned**, orchestrator-assigned to SCENARIO-13. The port is fakeable now (`WithDestination`) and `Test_sync_wraps_an_error_when_writing_the_manifest_fails` already proves `Sync` wraps a `Destination`-level failure; SCENARIO-13's remaining work is R14's disk-full-specific classification and refusal copy, not new plumbing.
- `Manifest.Warnings` is always populated as `[]string{}`; nothing appends to it yet. Owed to SCENARIO-09 (`M1`/`W1` + outcome policy).
- `_2`/`_3` collision suffix (11); leftover `.partial` sweep (12); `Schema` table/column counts for the human `Schema` line (01b).
- `cmd/quarry` wiring, CLI surface, help text, `--json` flag, all Surface & Copy strings: 01b onward.

## Traps
- mattn/go-sqlite3's `Backup` is called on the **destination** connection: `destConn.Backup("main", srcConn, "main")`, not the other way around.
- A `database/sql.DB` pinned to `SetMaxOpenConns(1)`: never call a method that needs a pool connection (e.g. `db.ExecContext`) while a `*sql.Conn` checked out from that same pool is still open — it deadlocks waiting for the single connection to be released. Run the follow-up statement through the checked-out `*sql.Conn` itself.
- SQLite creates db files `0644`; pre-create the partial `0600` via `atomicfile.Create` (`O_EXCL`) before backing up into it.
- The backup API copies the WAL flag from the source header, so the destination needs `PRAGMA journal_mode = DELETE` before hashing, or it grows its own `-wal`/`-shm`.
- `atomicfile.Commit` is hard-link-then-remove, not `os.Rename` — that is what makes it refuse to clobber an existing destination atomically.
- Corrupting a SQLite file to fail `integrity_check` needs real page content: a fresh/empty table's root page tolerates bit-flips without complaint. Insert enough rows to spill past page 1, then flip bytes in a later page. To fail `integrity_check` *only* (not also the accounts count or schema read), corrupt an index's pages rather than the queried table's: build the queried table first and small, add a large separately-indexed filler table after it, then flip bytes in the file's last page.
- `dirDestination.WriteManifest`'s `f.Write(data)` failure branch is permanently unreachable through the real adapter: no portable, test-constructible input makes a `Write` to a file that call just created fail (only disk exhaustion or an I/O fault would). It is marked `// unreachable` in code, not tracked as an open debt — no future scenario is expected to make it reachable or replace the marker.

## Open debts
- Connection-level driver pragmas (see Binding decisions) were reported, not ruled on by product-vision; carried forward for the final surface review in case a future driver upgrade changes what `Open` runs unconditionally.
