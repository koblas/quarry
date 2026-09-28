---
id: PREP-c
status: done
---

# PREP-c: DuckDB platform package, SQLite multi-row reads, v9 row fixtures

Size: OWNS A RUN (seam if it overruns: `platform/duckdb` ⟂ `platform/sqlite` + `v9fixture`). Infrastructure — no spec tick.
Cadence: test-first — `duckdb.Create` refuses an existing path (exclusive-create) and `(*DB).CheckpointClose` guarantees no `.wal` (atomicity claim feeding 01a's rename).
Acceptance test: `internal/platform/duckdb/duckdb_test.go` `Test_bulk_inserted_decimals_read_back_exactly_after_checkpoint_close`
Acceptance test (sqlite): `internal/platform/sqlite/sqlite_test.go` `Test_query_rows_scans_every_row_in_order`
Acceptance test (v9fixture): `internal/quicken/v9/v9fixture/builder_test.go` `Test_builder_seeds_every_row_kind_and_reads_back`
Narrow loop: `go test ./internal/platform/duckdb/ ./internal/platform/sqlite/ ./internal/quicken/v9/v9fixture/`
Mutation checks: existing-path refusal in `duckdb.Create` → `Test_create_refuses_an_existing_path`; unexported no-WAL check behind `(*DB).CheckpointClose` → `Test_no_wal_check_fails_when_a_wal_file_remains` (white-box, `.wal` planted beside a closed file)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `go.mod` — `go get github.com/duckdb/duckdb-go/v2@v2.10505.0` unsandboxed (GOMODCACHE is this worktree's `.devenv/`, so it downloads; bindings for every platform are large — not a hang)
- [x] Step 2: new `internal/platform/duckdb/{doc.go,duckdb.go}` — signature-only stubs importing the driver: `Create`, `OpenReadOnly`, `(*DB).Exec`, `(*DB).AppendRows`, `(*DB).QueryRows`, `(*DB).CheckpointClose`, `(*DB).Close`, `Decimal` (returns value + error), `IsDiskFull`, `IsPermission`, `ErrExists`, `ErrWALRemains`
- [x] Step 3: `go mod tidy` (after Step 2, or it drops duckdb-go) — expected end state: duckdb-go direct; `pflag` `// indirect`; `yaml.v3` stays as `// indirect` (testify + duckdb-go need it)
- [x] Step 4: `duckdb_test.go` acceptance test — Create fresh file, DDL with `DECIMAL(18,2)`, AppendRows (negative, zero, `9999999999999999.99`), control: `.wal` present before close; CheckpointClose → no `.wal`; OpenReadOnly → values read back as `CAST(... AS VARCHAR)` strings equal inputs. Red at assertion
- [x] Step 5: `internal/platform/sqlite/sqlite.go:118-128` stub `(*DB).QueryRows` + its acceptance test; `v9fixture/builder.go` stub `Builder` + `builder_test.go` acceptance test. Both red at assertion

### Build
- [x] Step 6: `duckdb.go` `Create` — used the plan's own anticipated fallback: DuckDB rejects a pre-created 0-byte file (empirically confirmed), so `Create` is `os.Stat` existence check → `ErrExists`, then `sql.Open` + `PingContext` (which creates the file), then `chmod 0600`. Pool pinned to 1 conn. Tests: `Test_create_refuses_an_existing_path` (mutation-verified red first — see Handoff), `Test_create_makes_the_file_owner_only`, fault: `Test_create_fails_with_a_permission_error_in_a_read_only_directory` → `IsPermission` true
- [x] Step 7: `(*DB).CheckpointClose` — `CHECKPOINT`, close, then unexported `checkNoWAL` → `ErrWALRemains`. Tests: `Test_no_wal_check_fails_when_a_wal_file_remains` (white-box `checkpoint_internal_test.go`, mutation-verified red first — see Handoff), `Test_no_wal_check_passes_when_no_wal_file_exists`; fault covered by the acceptance test's real CHECKPOINT-then-close path
- [x] Step 8: `(*DB).AppendRows(ctx, table, rows [][]any)` via driver Appender on `conn.Raw`; `Decimal(unscaled int64, width, scale) (any, error)` → driver `Decimal` (no float64); rejects `|unscaled| ≥ 10^width` itself (empirically confirmed the Appender accepts an overflowing value silently otherwise). Appender `Close` error always returned; ctx checked per row; conn released via `defer`. Tests: `Test_append_rows_reports_a_duplicate_primary_key`, `Test_append_rows_reports_a_wrong_column_count`, `Test_append_rows_stops_when_the_context_is_cancelled`, bound: `Test_decimal_refuses_an_unscaled_magnitude_of_10_to_the_width` / `Test_decimal_accepts_an_unscaled_magnitude_one_below_10_to_the_width` / `Test_decimal_refuses_a_negative_unscaled_magnitude_of_10_to_the_width`
- [x] Step 9: `(*DB).Exec`, `(*DB).QueryRows` (same callback contract as sqlite's), `OpenReadOnly` — faults: `Test_exec_fails_on_a_syntax_error`, `Test_query_rows_fails_on_a_query_error`, `Test_query_rows_fails_on_a_scan_type_error`, `Test_query_rows_stops_once_the_callback_errors` (`errors.Is` holds), `Test_open_read_only_fails_on_a_missing_path`
- [x] Step 10: `IsDiskFull` / `IsPermission` — `errors.Is` for os errors (`syscall.ENOSPC`/`EDQUOT`/`os.ErrPermission`); driver `*duckdbdriver.Error` with `Type == ErrorTypeIO` (empirically the only type real IO faults produce, including permission-denied) plus case-insensitive match of `syscall.EACCES/EPERM/ENOSPC/EDQUOT.Error()` against the driver's message. Table test `Test_IsDiskFull_and_IsPermission_classify_error_shapes` (os `*PathError`/`*LinkError`, constructed driver IO errors in the real message shape, unrelated IO error, non-IO-typed driver error, non-duckdb error, nil) plus real-driver rows `Test_open_read_only_on_a_permission_denied_file_classifies_as_permission` (mode-0000) and its control `Test_open_read_only_on_an_owner_readable_file_succeeds` (mode-0400), and `Test_create_on_an_existing_path_reports_ErrExists_not_a_permission_fault`
- [x] Step 11: `sqlite.go` `(*DB).QueryRows(ctx, query, args, row func(scan func(dest ...any) error) error) error` — closes rows, surfaces `rows.Err`, callback error stops iteration. Faults in `internal/platform/sqlite/query_rows_test.go` (new file, kept `sqlite_test.go` under the ~400-line split threshold): query error, scan type error, callback error (`errors.Is`), ctx cancelled mid-iteration
- [x] Step 12: new `internal/quicken/v9/v9fixture/builder.go` — `Builder` with row structs (`AccountRow`, `TransactionRow`, `EntryRow`, `ReconcileRow`, `TagRow`, `PayeeRow`) and methods returning assigned `Z_PK`; `Seed(tb, *sql.DB)`, `WriteBundle(tb, dir) Bundle`. `Seed` also writes one `Z_PRIMARYKEY` row per of the five overridable entity kinds, `Z_MAX` computed from the rows actually added under that entity. `Z_15USERTAGS`'s own columns (`Z_15CASHFLOWTRANSACTIONENTRIES`/`Z_76USERTAGS`) are literal per the DDL, not entity-derived — `LinkUserTag`'s doc comment says so. Acceptance test reads every kind back via `sqlite.QueryRows`, asserts overridden entity numbers land, `typeof(ZAMOUNT)` is `real` for `"12.34"` and `integer` for `"12.00"` (SQLite's NUMERIC-affinity collapse — also visible on `ZRECONCILERECORD.ZENDINGBALANCE`), and a `"12.345"` value is storable verbatim. `OpenBundle` untouched

### Sweep
- [x] Step 13: `go build ./... && golangci-lint run ./...` — 0 issues; doc comments per `clean-architecture` (`doc.go` for `platform/duckdb`); `go doc`/`go doc -all` read as the contract for both new packages

### Verify
- [x] Step 14: full verification per `.claude/rules/agent-briefs.md` → *Verification* (`-race` on the three packages, clean); `go list -deps -test ./internal/snapshot/ ./internal/cli/ ./cmd/quarry/ ./internal/quicken/v9/... ./internal/platform/sqlite/ | grep -c duckdb-go` → `0` (grep exit 1, expected), control `./internal/platform/duckdb/` → `5`; no spec tick, `status: done`

## Handoff

**Empirical answers to the plan's open questions** (small throwaway probes against the real driver, not left to guesswork):
- DuckDB refuses to open a pre-created 0-byte file ("exists, but it is not a valid DuckDB database file"), confirming the plan's own fallback: `Create` is `os.Stat` existence check → `ErrExists`, then `sql.Open`+`PingContext` (which creates the file itself, mode 0644), then `chmod 0600`.
- The Appender does **not** reject an overflowing `DECIMAL(18,2)` value — appending unscaled `10^18` (one digit over) succeeded silently and read back as `10000000000000000.00`. `Decimal()` must self-validate; there is no driver-side backstop and no need for a prepared-insert fallback.
- Appender faults (duplicate PK, etc.) surface at `Close`/`Flush`, not at `AppendRow`; a direct `Close()` with no prior explicit `Flush()` already returns the real error, so `AppendRows` only needs to check `app.Close()`'s return.
- `CHECKPOINT` itself removes the `.wal` file (before `Close`, even) on this driver version — so a real end-to-end `.wal`-remains fault cannot be constructed; `checkNoWAL` is verified white-box only (mutation-tested).
- Real driver IO faults (permission denied, missing read-only path, file-not-a-database) all report `duckdbdriver.ErrorTypeIO` with a message ending in the OS's own `strerror` text (e.g. "Permission denied") — confirming the plan's classification approach (`Type == ErrorTypeIO` + case-insensitive `syscall.Errno.Error()` substring match). No real ENOSPC/EDQUOT fixture exists in this repo, so those two rows in the table test use a constructed `*duckdbdriver.Error` in the same real shape.
- `access_mode=READ_ONLY` on the DSN query string is the read-only knob (no URL escaping needed — the connector splits the DSN on the first literal `?`, not via `url.Parse` on the whole string).
- DuckDB's `InstanceCache` refuses a second connection to the same path with a different config while the first is still open ("Can't open a connection to same database file with a different configuration") — tests that open both a writer and a read-only `*DB` on one path must close the writer first.
- SQLite's NUMERIC affinity collapses a whole-valued decimal *string* to INTEGER storage on write for **any** `DECIMAL`-declared column, not just `ZAMOUNT` — `ZRECONCILERECORD.ZENDINGBALANCE` shows the same behavior in the v9fixture acceptance test.

**Binding decisions** — a later scenario must not contradict these without saying so:
- Bulk insert = driver Appender behind `(*DB).AppendRows`; money enters DuckDB only as `duckdb.Decimal(unscaled cents, 18, 2)` — P1-7 forbids float64 in the path.
- Money leaves DuckDB only as `CAST(... AS VARCHAR)` or the driver Decimal type — scanning DECIMAL into float64 silently breaks P1-7.
- `CheckpointClose` is the only close before a rename; it returns `ErrWALRemains` — 01a's swap (P1-2) depends on "no `.partial.wal` before rename".
- `duckdb-go` is imported only by `internal/platform/duckdb` (and 01a's store adapter) — confirmed via `go list -deps -test`: 0 hits on snapshot/cli/cmd/v9/sqlite, 5 on `platform/duckdb` itself.
- `IsDiskFull` → S2, `IsPermission` → S1; classification lives here, not string-matched by callers.
- v9 money fixtures are strings; SQLite NUMERIC affinity picks the storage class (whole → INTEGER, fractional → REAL) on any `DECIMAL`-declared column, not just `ZAMOUNT`.
- `v9fixture.Builder`'s `Z_PRIMARYKEY` rows cover only its five overridable entity kinds (CategoryTag/UserTag/CashFlowTransaction/SmartCashFlowTransaction/InvestmentTransaction) — `ZACCOUNT`/`ZCASHFLOWTRANSACTIONENTRY`/`ZRECONCILERECORD`/`ZUSERPAYEE` rows carry no matching `Z_PRIMARYKEY` entry (01a's importer resolves entity numbers only for the five kinds per P1-5b; extend `Seed` if that changes).

**Left unbuilt** — named so nobody assumes it exists:
- `internal/store`, the importer, its `Source` port, snapshot→manifest resolution — SCENARIO-01a.
- Store file mode 0600 after rename — 01a asserts it (Create only sets it on the partial).
- `ZFINANCIALINSTITUTION` / `accounts.institution` seeding in `v9fixture.Builder` — 01a if needed.
- Upgrading `OpenBundle` / Phase 0 fixtures to typed accounts — 01a (P1-9 trap).

**Traps** — things that look right and are not:
- `Z_15USERTAGS` columns are `Z_15CASHFLOWTRANSACTIONENTRIES` / `Z_76USERTAGS` whatever the UserTag entity number — the importer, and `v9fixture.LinkUserTag`, name them literally.
- Payees are `ZTRANSACTION.ZUSERPAYEE` → `ZUSERPAYEE.ZNAME`, not `ZBPFIPAYEE`.
- Appender errors surface at `Close`, and it ignores ctx; holding its `sql.Conn` with a 1-conn pool deadlocks any other query.
- The Appender does not reject an overflowing Decimal — `duckdb.Decimal()` is the only guard; never build a `duckdbdriver.Decimal` directly.
- `go mod tidy` before any `.go` file imports duckdb-go removes it (confirmed — had to `go get` twice). GOMODCACHE is per-worktree (`.devenv/`), so module fetches need network.
- Two `*duckdb.DB` on the same path with different configs (e.g. a writer still open, then `OpenReadOnly` on the same path) fail with a `Connection Error`, not a permission/IO fault — close the first before opening the second.
