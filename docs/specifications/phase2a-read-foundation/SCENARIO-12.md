---
id: SCENARIO-12
status: done
---

# SCENARIO-12: sql refuses to change the store (absorbs 13, 14, 20)

Size: OWNS A RUN. It absorbs SCENARIO-13, 14 and 20 as FOLDs. Cadence: test-first, because the read lockdown in `duckdb.OpenReadOnly` is a write-safety guard.
Acceptance test: `cmd/quarry/run_sql_test.go` `Test_run_sql_refuses_to_change_the_store`
Acceptance test (SCENARIO-13, folded): `cmd/quarry/run_sql_test.go` `Test_run_sql_refuses_to_write_another_file`
Acceptance test (SCENARIO-14, folded): `cmd/quarry/run_sql_test.go` `Test_run_sql_reports_a_bad_query`
Acceptance test (SCENARIO-20, folded): `cmd/quarry/run_sql_test.go` `Test_run_sql_reports_a_query_interrupted_by_sigint`
Narrow loop: `go test ./internal/platform/duckdb/ ./internal/store/duckstore/ ./internal/cli/ ./cmd/quarry/ -run 'open_read_only|query|sql'`
Mutation checks: `access_mode=READ_ONLY` → `Test_run_sql_refuses_to_change_the_store`; `enable_external_access=false` → `Test_run_sql_refuses_to_write_another_file` (run `-run` on that test ONLY: with access on, the INSTALL row downloads); `autoload_known_extensions=false`, `autoinstall_known_extensions=false` → their `Test_open_read_only_locks_down_the_session` rows; `lock_configuration=true` → `Test_run_sql_refuses_to_change_a_setting`; the `ctx.Err()`-first check → `Test_run_sql_reports_a_query_interrupted_by_sigint`; the Q1 predicate's message arm → `Test_query_error_predicates_classify_driver_errors` SET row, and its type arm → that test's Catalog `"read-only mode"` row

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_sql_test.go` `Test_run_sql_refuses_to_change_the_store`. A built store runs `CREATE TABLE`. Expect Q1 on stderr, empty stdout, exit 1, and the store's sha256 unchanged. The test goes red only on the stderr line; the bytes-unchanged half passes on arrival because READ_ONLY already exists. Report it that way.
- [x] Step 2: `run_sql_test.go` adds the lockdown reds that exist today, one named test per SCENARIO-13 row:
  - `Test_run_sql_refuses_to_write_another_file`: COPY to a tmp path. Expect Q2 and assert the file does not exist.
  - `Test_run_sql_refuses_to_read_another_file`: `read_csv` of a tmp CSV the test wrote. Expect Q2.
  - `Test_run_sql_refuses_to_attach_another_database`: Expect Q2 and assert the target is not created.
  - `Test_run_sql_refuses_to_install_an_extension`: `INSTALL httpfs`. Expect Q2.
  - `Test_run_sql_refuses_to_change_a_setting`: `SET enable_external_access=true`. Expect the exact Q3 line from the spec's Refusals.
- [x] Step 3: `internal/platform/duckdb/exec_query_test.go` after :86-99 `Test_open_read_only_locks_down_the_session`. It reads `current_setting` for each of the 5 DSN parameters, one row each. 4 of the 5 rows are red today.
- [x] Step 4: `run_sql_test.go:60-71` rewrites `Test_run_sql_reports_a_query_error` as `Test_run_sql_reports_a_bad_query`. It asserts the exact stderr, which is the Binder message's first line only. Also add `Test_run_sql_reports_a_query_interrupted_by_sigint`. Its constraints:
  - package main, no `t.Parallel`.
  - `signalContext` is registered first. Then send exactly one `syscall.Kill(os.Getpid(), SIGINT)` after a short delay.
  - The query is `SELECT count(*) FROM range(1000000000000)` (an integer literal).
  - Expect Q4 and exit 1, within a deadline, so a broken interrupt fails instead of hanging.
- [x] Step 5: `internal/store/query.go:36-45` gets signature-only stubs: `ErrReadOnlyQuery` (Q1), `ErrExternalAccess` (Q2), `ErrQueryInterrupted` (Q4), `*QueryError{Reason}` (Q3). Also stub `duckdb.IsReadOnlyViolation` and `duckdb.IsAccessDisabled`. Confirm that every test from Steps 1-4 fails at its assertion.

### Build
- [x] Step 6: `internal/platform/duckdb/duckdb.go:70-87` `OpenReadOnly` builds its DSN from one named constant: `access_mode=READ_ONLY&enable_external_access=false&autoload_known_extensions=false&autoinstall_known_extensions=false&lock_configuration=true`, with `lock_configuration` last. This turns Steps 2-3 green. Update the doc comment to name the lockdown.
- [x] Step 7: `duckdb.go:224-231`, beside `isDriverIOError`, adds `IsReadOnlyViolation` and `IsAccessDisabled`:
  - `IsReadOnlyViolation` is true for `ErrorTypeInvalidInput` whose Msg contains `read-only mode`.
  - `IsAccessDisabled` is true for `ErrorTypePermission`.
  - `faults_test.go` gets `Test_query_error_predicates_classify_driver_errors`. It uses real driver errors from a locked-down open:
    - CREATE is true for Q1.
    - SET is InvalidInput but false for Q1.
    - `SELECT * FROM "read-only mode"` is a Catalog error and false for Q1.
    - `read_csv` and `LOAD httpfs` are true for Q2.
    - A plain `errors.New` is false for both.
- [x] Step 8: `internal/platform/duckdb/table.go:84-86` handles the Q5 case where the driver refuses a type. When `rows.Err()` matches `unsupported data type: <T>: index: <i>` and `i` is a valid column index, return `&UnprintableValueError{Column: Columns[i].Name, Type: <T>}`. Otherwise return the driver error unchanged.
  - `table_test.go:66-73` becomes `Test_query_table_refuses_a_type_the_driver_cannot_read`. VARIANT is in column 1 and a distinctly named column is at 0, which catches an off-by-one.
  - Add one internal test for the unmatched and out-of-range cases, which return the error unchanged.
- [x] Step 9: `internal/store/duckstore/query.go:12-31` `Query` gets the classifier, in this order:
  1. `ctx.Err() != nil` at either the open or the query stage gives `ErrQueryInterrupted`.
  2. `*duckdb.UnprintableValueError` stays mapped to the store's `*UnprintableValueError` (existing).
  3. Q1, then Q2.
  4. Any other query-stage error becomes `*store.QueryError` with Reason = the first line of `err.Error()`, keeping the type prefix and paths verbatim.

  A non-ctx open fault stays `run query: %w`.

  Tests in `query_test.go`:
  - `:59-68` is rewritten so it asserts the first-line Reason; the Binder message has a second line.
  - Real-store rows for Q1, Q2 and the SET case of Q3.
  - A fake `ReadDB` returning `errors.Join(context.Canceled, &duckdbdriver.Error{Type: ErrorTypeInterrupt, Msg: "INTERRUPT Error: Interrupted!"})` under a cancelled ctx gives Q4. A live ctx with the same error gives Q3 `context canceled`; this is the control arm.
  - A fake open fault under a cancelled ctx gives Q4. `:46-57` keeps the live-ctx open fault as `run query:`.
- [x] Step 10: `internal/cli/sql.go:57-64` `queryFailure` builds the ruled Q1 to Q4 copy verbatim from the spec's Refusals. `cmd/quarry` adds the `quarry: ` prefix. The Q5 arm is unchanged. Any other error passes through unchanged.
  - `sql_test.go` gets `Test_sql_reports_each_query_refusal`, one row per sentinel plus `*QueryError`.
  - `:87-95` `Test_sql_returns_the_query_fault` still pins the default arm.

### Sweep
- [x] Step 11: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`.
  - Add doc comments on the new sentinels, `QueryError`, both predicates and the DSN constant.
  - Update `Query`'s doc (`query.go:12-15`) to list the refusals.
  - Reword the `// unreachable:` reasons at `table.go:54` and `:71`: the only way in is a ctx cancel racing the call, which the classifier reports as Q4. This closes the STATE open debt.

### Verify
- [x] Step 12: full verification plus `spec-check.py phase2a-read-foundation`.
  - Tick SCENARIO-12 with its acceptance test.
  - Tick 13, 14 and 20, each ending "delivered by SCENARIO-12 — `<file>` `<test>`".
  - Remove the VARIANT and `table.go:54,71` debts from STATE.

## Handoff

**Binding decisions:**
- There is one read DSN constant in `duckdb.OpenReadOnly`. Every read-only open in the process uses it. A differently-configured open of a live path fails with `Connection Error: Can't open a connection to same database file with a different configuration`. Under `lock_configuration`, nothing on the read path may `SET` after open, so any future session setting (S15's catalog check included) goes into the DSN.
- Classifier order in `(*duckstore.Store).Query`:
  1. `ctx.Err()`, at the open and the query stage, gives Q4.
  2. Q5.
  3. Q1, then Q2.
  4. S18's U5 empty-query arm goes here.
  5. Q3.

  A non-ctx open fault stays `run query: open … read-only: …` for S15 to replace with R1-R3. S18 inserts U5 after `ctx.Err()`.
- The empty-query arm is left to SCENARIO-18. U5 is exit 2 with usage copy, and S18 owns every U5 arm; landing only the driver-detected half here would split U5's surface. Interim output, replacing STATE's line: `sql ""`, `"   "`, `";"` and `"-- note"` exit 1 with `quarry: query failed: empty query`. The driver's `errEmptyQuery` is a plain unexported `errors.New("empty query")`, so S18 must match it by message.
- Q2 = DuckDB `Permission Error` (a configuration refusal), through `duckdb.IsAccessDisabled`. `duckdb.IsPermission` is IO/OS permission, and S15's R3b uses that one, never `IsAccessDisabled`.
- Q3's Reason keeps the type prefix and paths verbatim. R3 strips the prefix and `~`-replaces paths. They must not share a normalising helper.
- Q5 for a type the driver refuses is mapped inside `QueryTable`, the only place that has the column list. Its `Type` is the driver's `<T>`.

**Left unbuilt:** U5 empty-query arm (S18); R1-R3 open refusals (S15); `--json` and truncation warning (S09).

**Traps:**
- The interrupt error is `errors.Join(context.Canceled, *Error{Interrupt})`, so its first line is `context canceled`. Without the `ctx.Err()` check it would print as Q3.
- `INSTALL httpfs` fails as a Permission Error on `~/.duckdb/extensions` before any network access. With external access re-enabled it downloads, so never mutate that parameter while running the full test set.
- The 13 `OpenReadOnly` caller sites in cmd inspectors and duckstore tests run no SET, ATTACH, COPY, INSTALL, LOAD or file-reading function. The grep hits were test names only, so they survive the lock.
- `CREATE TEMP TABLE`/`VIEW` succeed in memory; that is acceptable per P2a-11.

**Copy gaps (orchestrator decides whether product-vision rules on them):**
- The interim `quarry: query failed: empty query` (exit 1) until S18 lands.
- A Q3 line can carry an absolute path, for example a Permission-free IO fault naming a file. P2a-8 says paths are `~`-abbreviated on stderr, and the Q3 row does not say which rule applies.
