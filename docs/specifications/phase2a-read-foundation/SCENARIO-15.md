---
id: SCENARIO-15
status: open
---

# SCENARIO-15: read commands refuse when there is no store (absorbs 16, 17)

Size: OWNS A RUN; absorbs SCENARIO-16 (R2) and SCENARIO-17 (R3a) as FOLDs.
Cadence: code-first — no write-safety guard, atomicity or bug fix touched (a read already never creates the store).
Acceptance test: `cmd/quarry/run_read_refusals_test.go` `Test_run_read_commands_refuse_when_there_is_no_store`
Acceptance test (SCENARIO-16, folded): `cmd/quarry/run_read_refusals_test.go` `Test_run_status_refuses_a_store_built_by_another_version`
Acceptance test (SCENARIO-17, folded): `cmd/quarry/run_read_refusals_test.go` `Test_run_accounts_refuses_a_store_that_is_not_a_duckdb_file`
Narrow loop: `go test ./internal/platform/duckdb/ ./internal/store/duckstore/ ./internal/report/ -run 'open_read|refuse|fault|closes|interrupt|missing|predicates' && go test ./cmd/quarry/ -run 'refuse|Test_run_status|Test_run_accounts|Test_run_sql'`
Mutation checks: `errors.Is(err, store.ErrQueryInterrupted)`-first arm of `report.storeRefusal` → `Test_query_keeps_an_open_interrupted_by_its_context_as_interrupted`; catalog check of `store_info` in `openRead` → `Test_run_status_refuses_a_store_built_by_another_version`

Copy gaps (need a scoped product-vision ruling before the developer implements; interim in brackets):
- G1: open fault with no `*duckdb.Error` in its tree — `<reason>` undefined [first line of `err.Error()`, path-normalised].
- G2: `status`/`accounts` interrupted during open → R3 `...: context canceled; run quarry sync to rebuild it` (wrong advice) [interim as written; only `sql` has Q4].
- G3: store deleted between stat and open → driver `Cannot open database "…" in read-only mode: database does not exist` [R3 fallback, not R1].
- G4: `store_info` present with 0 or 2+ rows [R2].

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_read_refusals_test.go` (new) `Test_run_read_commands_refuse_when_there_is_no_store` — outline rows `status`, `accounts`, `sql "SELECT 1"`, `sql ";"`; each: exact R1 stderr (home abbreviated), stdout empty, exit 1, home dir still empty
- [ ] Step 2: same file, folded tests: `Test_run_status_refuses_a_store_built_by_another_version` (fixture: `os.MkdirAll(storeDirUnder(home))`, then `duckdb.Create` at `storeDirUnder(home)/quarry.duckdb`, Phase-1-shape `import_runs(id, snapshot_path …)` with one row `…/20260927T143005Z.sqlite`, no `store_info`, `CheckpointClose` before running) and `Test_run_accounts_refuses_a_store_that_is_not_a_duckdb_file` (text bytes at the store path) — exact R2 / R3a lines
- [ ] Step 3: `internal/store/query.go:61-67` neighbour — `store.OpenError{Fault, Path, SnapshotPath, Reason, Err}` + `Unwrap` + `OpenFault` consts (missing, other-format, not-duckdb, permission, locked, other) as signature-only; run Step 1-2 red at their stderr assertions

### Build
- [ ] Step 4: `internal/platform/duckdb/duckdb.go:222-243` — `IsNotDatabase`, `IsLocked` (driver IO error + probed text) and `DriverMessage(err) (string, bool)` (first line of `Msg`, `^[A-Za-z ]+ Error: ` stripped; `errors.As` tree walk). Tests in `faults_test.go:98-123` neighbour: `Test_open_read_only_on_a_non_duckdb_file_classifies_as_not_a_database` (text file and 0-byte file), `Test_open_read_only_on_a_file_another_process_holds_classifies_as_locked` (env-gated, skip-by-default helper test re-execs the test binary; child holds `duckdb.Create` on the file, prints READY, waits on stdin), predicate rows in `Test_query_error_predicates_classify_driver_errors` (`faults_test.go:136-168`) incl. prefix strip and multi-line Msg
- [ ] Step 5: `internal/store/duckstore/duckstore.go:146-152` `openRead` — `os.Stat` (only `fs.ErrNotExist` → missing; any other stat fault falls through to the open) → `s.openReadOnly` → classify fault (not-duckdb, permission via `duckdb.IsPermission`, locked, else other with Reason: `DriverMessage`, `filepath.EvalSymlinks(Path)` occurrences → `Path`) → catalog check (`duckdb_columns()` for `store_info.format_version`; then `format_version` = `FormatVersion`, exactly one row) → other-format with `SnapshotPath` read only when `import_runs.snapshot_path` exists and yields one non-empty value, else `""`; Close on every refusal after a successful open; no `SET`. Tests in new `internal/store/duckstore/open_test.go`: `Test_open_read_classifies_each_store_it_cannot_read` (real files: missing + nothing created; not-duckdb; chmod-0000 file → permission; chmod-0000 dir → not missing, expected permission (unprobed: report if it lands in other); injected locked shape), `Test_open_read_refuses_a_store_of_another_format` (no store_info; format FormatVersion-1 and +1, FormatVersion as control; SnapshotPath rows: present / no import_runs / no snapshot_path column / zero rows / empty), `Test_open_read_reports_other_faults_with_the_drivers_first_line` (store dir reached through a symlink so both path forms differ on every OS; Msg naming the resolved path), fault tests: catalog query fault → other, `format_version` read fault → other, snapshot-path read fault → other-format with `""`, `Test_open_read_closes_the_connection_when_it_refuses_the_format`
- [ ] Step 6: fake-opener churn — stat-first and the catalog query break these; give each a file at `Path` (or `newBuiltStore`) and let `spyReadDB` (`status_test.go:143-182`) fault only the read's own query: `status_test.go:132-141` (assert `OpenError` missing), `:187-197`, `:199-212`, `:214-240`; `accounts_test.go:126-136`, `:138-151`, `:153-163`, `:165-193`, `:195-200`; `query_test.go:47-58` (drop the `run query:` EqualError), `:106-118`, `:120-132`, `:134-145`, `:158-185`
- [ ] Step 7: `internal/store/duckstore/query.go:13-24` — keep the `ctx.Err()`-first arm; return the `*store.OpenError` unwrapped (drop `run query: %w`); doc lines on `Query`, `Status` (`status.go:30-33`), `Accounts` (`accounts.go:21-23`) name `*store.OpenError`
- [ ] Step 8: `internal/report/refusal.go` (new) `RefusalError{msg, cause}` + unexported `storeRefusal(err, home)` — `errors.Is(err, store.ErrQueryInterrupted)` → err unchanged FIRST; `errors.As` `*store.OpenError` → R1/R2 (`SnapshotID(SnapshotPath)`, fallback when `""`)/R3a/b/c/R3 copy verbatim from spec, path via `homepath.Abbreviate(home, Path)`, Reason's `Path` occurrences → that `~` form; else err unchanged. Wire into `report.go:46-50` `Status`, `accounts.go:23-30` `Accounts`, `query.go:17-29` `Query`. Tests `internal/report/refusal_test.go`: `Test_status_refuses_with_the_store_refusal_copy` (one row per fault, R2 both forms, path outside home stays absolute, Reason path abbreviated), `Test_accounts_refuses_with_the_store_refusal_copy` and `Test_query_refuses_with_the_store_refusal_copy` (one row each), `Test_query_keeps_an_open_interrupted_by_its_context_as_interrupted` (store returns `ErrQueryInterrupted` wrapping an `OpenError`)
- [ ] Step 9: `cmd/quarry/run_status_test.go:60-73`, `cmd/quarry/run_accounts_test.go:69-82` — delete (subsumed by Step 1)

### Sweep
- [ ] Step 10: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; `internal/report/doc.go:1-4` now says report also builds the store-refusal copy; doc comments on `OpenError`, `RefusalError`, new predicates

### Verify
- [ ] Step 11: full verification + `spec-check.py phase2a-read-foundation` → tick SCENARIO-15; tick 16 and 17 with `— delivered by SCENARIO-15 —` before their test refs; STATE.md: remove the R1-R3 "Left unbuilt" line and the `run query: %w` note

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `openRead` order is stat → open → classify → catalog format check; every read (and every Phase 2 read after 2a) gets R1-R3 from it — P2a-1 "one shared open".
- R copy lives in `report` (`storeRefusal`, like `snapshot.RefusalError`), not cli; cli passes it through (`queryFailure` falls through for it) — P2a-2 "one builder" for three commands.
- `storeRefusal` checks `store.ErrQueryInterrupted` before `*store.OpenError`: Query's interrupted-open wrap carries the OpenError, so the reverse order prints R1/R3 on SIGINT.
- Format check reads the catalog (`duckdb_columns()`), never a bare `SELECT` from `store_info` (a Catalog error otherwise); it is a SELECT, so the configuration lock is untouched.

**Left unbuilt** — named so nobody assumes it exists:
- G1-G4 copy rulings; `sql` U5 for `";"` with a store (S18) — with no store it is R1 by construction.

**Traps** — things that look right and are not:
- Driver messages name the store by its resolved path (`/private/var/…` on macOS) while `$HOME`/Path is `/var/…`, a substring of it: replace resolved → Path first, then Path → `~`; the reverse yields `/private~/…`. On Linux `t.TempDir()` has no symlink — the Reason test must symlink the store dir itself.
- A same-process lock is not constructible: a second open of a live path fails with `Connection Error: Can't open a connection … different configuration` (InstanceCache), not `Could not set lock on file`. R3c needs another process (probed darwin, DuckDB v1.5.5).
- `chmod 0000` on the store file: `os.Stat` still succeeds; the fault is the driver's `IO Error: Cannot open file "…": Permission denied`. An empty file is `not a valid DuckDB database file` (R3a).
- Q-sentinels wrap `%w: %w`: take the Reason from `errors.As` to the driver error, never a single-chain `errors.Unwrap` walk.
- Fixtures writing at the store path must `CheckpointClose` before any read (InstanceCache).
