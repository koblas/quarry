---
id: SCENARIO-13
status: done
---

# SCENARIO-13: sync keeps earlier builds in import_runs (absorbs SCENARIO-14)

Cadence: test-first — `duckstore.Replace` is the temp-then-rename adapter (atomicity claim): its rename destroys the only copy of the previous `import_runs`, and this scenario edits its sequence
Acceptance test: `cmd/quarry/run_import_runs_test.go` `Test_run_sync_from_keeps_the_earlier_build_in_import_runs`
Acceptance test (SCENARIO-14, folded): `cmd/quarry/run_status_test.go` `Test_run_status_reports_the_latest_build_when_import_runs_holds_several`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/importer/ ./cmd/quarry/ -run 'replace|status|open_read|import_run|history'`
Mutation checks: a carried NULL count written as 0 (row rebuilt through `importRunRows`) → `Test_replace_carries_null_for_columns_an_older_store_lacks` | new id = count+1 → `Test_replace_numbers_the_new_run_after_the_highest_carried_id` | `statusQuery` without `ORDER BY r.id DESC` → `Test_status_reads_the_latest_import_run` | `snapshotPathQuery` without its `ORDER BY` → `Test_open_read_names_the_latest_import_runs_snapshot` | history read moved below `os.Rename` → `Test_replace_reads_the_previous_history_before_it_creates_the_build_file` | a history fault returned as `Replace`'s error → `Test_replace_restarts_history_when_the_previous_store_cannot_be_read`
Runs: A (1-3) | B1 (4-5) | B2 (6-7) | V (8-10)
Size: OWNS A RUN — 4 batches, 1 feature package (importer) + store/duckstore; absorbs 14; owns surfaces #7, #8

Contract (specification.md SCENARIO-13, SCENARIO-14, P2c-9, P2c-10): `quarry sync` and `quarry sync --from <id>` keep stdout, stderr and exit code as today (the CF2 line is SCENARIO-15's); `quarry status` and `status --json` stay byte-identical and describe the highest-id run; R2's `--from <id>` hint names the highest-id run's snapshot; the zero-run R3 line stays `... expected exactly one import run, found 0; run quarry sync to rebuild it`, exit 1.

Port survey: `importer.Store` has one method, `Replace`, one production call (`importer.go:129`), two implementers (`duckstore.go:279`, `importer/store_fake_test.go:21`; grep — see Traps on LSP). The history read reuses `ReadDB` (`QueryRows`, `Close`) through `s.openReadOnly`: no new port, no new option.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_import_runs_test.go` `Test_run_sync_from_keeps_the_earlier_build_in_import_runs` — two bundles with different data (equal data can hash alike); store built from A only (sync A, copy the store file aside, sync B, put the copy back), then `sync --from <B>`; ids 1 and 2 carry each manifest's sha256, and row 1 reads the same in every column (timestamps included) as before the When. Red today: one row
- [x] Step 2: `cmd/quarry/run_status_test.go` `Test_run_status_reports_the_latest_build_when_import_runs_holds_several` — Given built with `editStore` (`run_read_refusals_test.go:190-197`): a second row as `:222` builds one (`SELECT * REPLACE (2 AS id, '<other snapshot path>' AS snapshot_path)`; vary the path only, so `(just now)` holds; two real syncs would pass today with one row); stdout is the `:45-54` form with only the Snapshot id changed. Red today: `found 2`
- [x] Step 3: `internal/store/store.go:199-209` `Result.HistoryFault *OpenError` + new `store.Replaced{Path, HistoryFault}`; `importer/ports.go:21-26`, `duckstore.go:275-325`, `importer.go:128-134`, `importer/store_fake_test.go:9-28` `Replace` returns `store.Replaced` — signature only, behaviour unchanged; `.Path` at each caller the build lists (about 20 in `duckstore_test.go`)

### Build
- [x] Step 4: `duckstore/status.go:12-27,57-62` `statusQuery`, `Status` + `duckstore.go:175,244-261` `snapshotPathQuery`, `snapshotPath` — latest row. `status_test.go:100-127` becomes `Test_status_reads_the_latest_import_run` (three runs inserted in id order 1, 3, 2 by SQL as `:62-73` does, each its own snapshot and counts) and `Test_status_refuses_a_store_without_an_import_run` (`found 0`); `open_test.go:204-229` becomes `Test_open_read_names_the_latest_import_runs_snapshot` (same 1, 3, 2 shape with distinct paths; the other rows stay); `cmd/quarry/run_read_refusals_test.go:215-242` loses its two-runs row; `cmd/quarry/run_status_json_test.go` `Test_run_status_json_reports_the_latest_build_when_import_runs_holds_several`. Step 2 turns green
- [x] Step 5: new `duckstore/history.go` (previous runs read) + `duckstore.go:279-288` `Replace` (read after the sweep, before `s.create`), `:388-429` `build`, `:544-560` `importRunRows` (carried rows first, then the new run numbered after them). New `history_test.go`: `Test_replace_carries_the_previous_import_runs_unchanged` (every column as text, before against after), `Test_replace_carries_null_for_columns_an_older_store_lacks` (Phase 1's 19-column table, no `store_info`; six columns NULL, `HistoryFault` nil), `Test_replace_numbers_the_new_run_after_the_highest_carried_id` (ids 2 and 5 give 6; controls: empty `import_runs` gives 1, no store gives 1), `Test_replace_reads_the_previous_history_before_it_creates_the_build_file` (order through `WithOpenReadOnly` + `WithCreate`: opened, closed, created), `Test_replace_starts_history_silently_when_no_store_exists` (nil fault, no read open; also when the file vanishes between the stat and the open, as `open_test.go:161`). Controls that must stay green: `duckstore_test.go:144`, `:376`. `duckstore_test.go:324` no longer collides: re-point it at `faultDB` (`:454-499`). Step 1 turns green
- [x] Step 6: `duckstore/history.go` fault paths — every failure is `openFault(path, err)` on `Replaced.HistoryFault`; the build goes on with no carried row and id 1. `Test_replace_restarts_history_when_the_previous_store_cannot_be_read`, one row per shape: a real non-DuckDB file, permission and locked as `open_test.go:71-160` builds them, catalog-query fault, rows-query fault, scan fault, a DuckDB file with no `import_runs`, the `(id, snapshot_path)` table of `open_test.go:32`, NULL in a required column, a NULL id, carried ids that are not unique (Reason text: copy ruling owed before run B2, do not invent it); each asserts the Fault, one row with id 1, the store swapped, the read connection closed once. `Test_replace_reads_history_beside_a_stale_wal` pins what the read does with a `quarry.duckdb.wal` beside the store (carried or fault — observe, then pin; `:551`, `:577`, `:602` stay green). `Test_replace_does_not_swap_when_the_context_ends_during_the_history_read` (`spyReadDB.onQuery`)
- [x] Step 7: `importer/importer.go:128-134,137-154` — `importRunID` removed, `newImportRun` leaves `ID` unset, `Result.HistoryFault` taken from `Replaced`; `store_fake_test.go:9-28` fake returns a settable fault; `import_runs_test.go:14-48` (`ID: 1` goes); `Test_import_reports_the_history_fault_the_store_returns` with a nil control

### Sweep
- [x] Step 8: `docs/initial-prd.md:127,146,163` — P2c-13's quoted text verbatim: `:127` both cells; `:146` second sentence replaced, then P2c-13's closing sentence (`A snapshot never built ...`) after it (the rejected-snapshot sentence stays); `:163` drops `Quicken version, `
- [x] Step 9: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments within budget on `Replace` (`duckstore.go:275-278`), `Status` and `errImportRunCount` (`status.go:12,29-32`), `snapshotPath` (`:244-245`), `store.Rows`, `store.ImportRun` (`store.go:132,155-157`), `OpenError.SnapshotPath` (`open.go:18-21`), `Import` (`importer.go:42-48`), `duckstore/doc.go`

### Verify
- [x] Step 10: full verification + `.claude/scripts/spec-check.py phase2c-snapshots-config` → tick SCENARIO-13, and SCENARIO-14 as delivered by SCENARIO-13, each with its acceptance test; rewrite this feature's `STATE.md` only (other features' STATE files are the orchestrator's at SHIP)

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- The carry lives in `duckstore.Replace` and never passes through `store.ImportRun` — that type holds the four nullable counts as `int` and `importRunRows` writes `int64`, so a typed round trip turns NULL into 0
- The read is `s.openReadOnly` (the lockdown DSN, the one read config in the process) without `checkFormat`, closed before `s.create` — amends 2a's "`Replace` never opens the final path" to "never opens it for writing"; the held-reader test (`duckstore_test.go:144`) is its control
- A previous store of another format (R2's case) is read for history, not a fault: Phase 1 stores have no `store_info` and are the only ones lacking columns. `HistoryFault.Fault` is NotDuckDB, Permission, Locked or Other, never Missing (CF1, silent) or OtherFormat — so `UnreadableReason` is never empty for SCENARIO-15
- The store numbers runs: `rows.ImportRuns` are numbered consecutively from the highest carried id + 1 (from 1 with none); an `ImportRun.ID` handed to `Replace` is ignored — the importer cannot know the maximum
- SCENARIO-15's seam is `store.Result.HistoryFault *store.OpenError`, nil when history was carried or no store existed; 13 changes no CLI or JSON output
- `FormatVersion` stays 3 — no DDL change (P2c-11); a pre-2c binary reading a store with two runs gets R3 `found 2`, not R2
- History SELECT column names come from the fixed list in code, never from the previous store's catalog; no new ctx gate (the one before `os.Rename` still refuses the swap)

**Left unbuilt** — named so nobody assumes it exists:
- CF2 line in `Outcome.Warnings` and `warnings[]` — SCENARIO-15
- The store's-snapshot read port (latest row, `EvalSymlinks`) — SCENARIO-16
- Copy rulings (`product-vision`): the Reason for carried ids that are not unique (needed by step 6); CF2's `<reason>` for a DuckDB file whose `import_runs` is absent, lacks a required column or holds NULL there is driver or scan text by R3 precedent (`run_read_refusals_test.go:210-211`), to confirm before SCENARIO-15; `expected exactly one import run` now only ever says `found 0`, kept byte-identical

**Traps** — things that look right and are not:
- `spyReadDB` routes by query text (`fakes_test.go:43`): a history read reusing `columnExistsQuery` takes faults from `checkFaults` and is not counted by `passQueries`
- Keep `HistoryFault` a concrete `*store.OpenError`; a nil one assigned to an `error` is non-nil
- After step 5 a fixture cannot choose ids through `rows.ImportRuns`, and every test calling `Replace` twice in one directory holds two rows — build multi-run fixtures by SQL
- A connection opened with another config on the store path (`sql.Open("duckdb", path)`) makes the history open fail and history restart silently; close it before `Replace`
- A carried row the new DDL rejects would fail the build as S3, whose `sync --from` remedy re-reads the same history and loops: every such shape must end as a `HistoryFault` at the read, never at the append
- The history read runs before `removeStaleWAL`; what a read-only open does beside a stale WAL was not established at plan time (step 6 pins it)
- `LSP` in this session answers from another worktree (`../agent-…` paths): treat its results as unverified and grep
- `Outcome.Warnings()` has no home; `importVerified` (`snapshot/import.go:91-111`) has `s.home` — SCENARIO-15 needs it for `UnreadableReason(at)`

## Phase report

Run B2 (steps 6-7) done, plus step 8 (PRD `:127,:146,:163`, P2c-13 text and :146 order) - V must not redo it. Narrow loop, `go build`, `golangci-lint run ./...` 0 issues, duckstore `-race` green. Full suite/coverage not run (V; narrow coverage shows no uncovered added line in `history.go`/`importer.go`).

Files:
- `duckstore/history.go`: `readHistory` (stat/open fault -> `openFault`, as before), `readRuns(ctx, db) (history, error)` (one `runColumnsQuery` for the table's columns: none -> `errRunsMissing`; optional columns from that set, else `NULL`; duplicate id -> `errRunsRepeatID`), `historyFault(path, err)`. `import_runs` DDL has only PK + NOT NULL, so no other constraint needs a read-time check.
- CONTRACT for SCENARIO-15: `Replaced.HistoryFault` / `Result.HistoryFault` (`*store.OpenError`) renders through `UnreadableReason(at)` only, verbatim. Open-time faults (`s.openReadOnly` failed): `openFault`, so NotDuckDB/Permission/Locked/Other give the R3 phrases. Any fault after a successful open (columns query, SELECT, scan, duplicate id) is `Fault: OpenFaultOther`, `Reason` = the ruled phrase (`its import_runs table repeats an id` / `it has no import_runs table` / `its import_runs table is incomplete`), `Err` = cause (driver text: never print `Error()`). Never Missing/OtherFormat; no new `OpenFault` constant (`exhaustive`). `store.OpenError` doc says so.
- `store/store.go`: `ImportRun.ID` doc (store numbers it), `Result.HistoryFault` doc. `importer.go`: `importRunID` gone, `newImportRun` leaves `ID` 0, `Result.HistoryFault = replaced.HistoryFault`.
- Tests: new `duckstore/history_faults_test.go`; `importer/import_runs_test.go` (`ID: 1` gone, `Test_import_reports_the_history_fault_the_store_returns` + nil control), `store_fake_test.go` (`historyFault`).
- `docs/initial-prd.md` `:127`, `:146`, `:163` amended.

Red seen: no-table (`Table with name import_runs does not exist!`), missing columns (Binder Error), NULL id/path/sha (`sql: Scan error ...`), columns/rows/scan spy rows (driver text / no fault), duplicate ids (`require.NoError` on PK append), importer fault (`Not same ... nil`), `ID:0` expectation. Green on arrival: `Test_replace_restarts_history_when_the_previous_store_cannot_be_read` (not DuckDB, permission, locked: B1's `openFault`), both stale-WAL tests, ctx-during-read (create/exec refuse a cancelled ctx before the swap), importer nil control.

Mutation (reverted, `diff` clean): `if historyFault != nil { return store.Replaced{}, historyFault }` after `readHistory` reddened `Test_replace_restarts_history_when_the_previous_store_cannot_be_read` (3 rows: `Received unexpected error: open store ...`), and every `Test_replace_names_*` test plus `Test_replace_sweep_never_matches_the_live_stores_own_files`.

Stale WAL (observed, pinned): a regular `quarry.duckdb.wal` is no fault. A real uncheckpointed WAL is replayed by the read-only open and its committed rows carried (ids 1,7 -> new run 8); a garbage file is ignored (ids 1,2 carried). A DIRECTORY at the WAL path gives `OpenFaultOther` ("Failure while replaying WAL ... Is a directory") but then `removeStaleWAL` fails and Replace errors (`duckstore_test.go:629`) - CF2 never prints; not pinned, for the orchestrator to note.

V: step 9 doc-comment budgets (`Replace`, `Status`, `errImportRunCount`, `snapshotPath`, `store.Rows`, `store.ImportRun`, `OpenError.SnapshotPath`, `Import`, `duckstore/doc.go`) and step 10. Trap for SCENARIO-15: the phrase depends on `UnreadableReason`, not `Reason`'s raw text for Path-bearing faults.
