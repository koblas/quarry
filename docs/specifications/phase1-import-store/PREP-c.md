---
id: PREP-c
status: open
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
- [ ] Step 1: `go.mod` — `go get github.com/duckdb/duckdb-go/v2@v2.10505.0` unsandboxed (GOMODCACHE is this worktree's `.devenv/`, so it downloads; bindings for every platform are large — not a hang)
- [ ] Step 2: new `internal/platform/duckdb/{doc.go,duckdb.go}` — signature-only stubs importing the driver: `Create`, `OpenReadOnly`, `(*DB).Exec`, `(*DB).AppendRows`, `(*DB).QueryRows`, `(*DB).CheckpointClose`, `(*DB).Close`, `Decimal` (returns value + error), `IsDiskFull`, `IsPermission`, `ErrExists`, `ErrWALRemains`
- [ ] Step 3: `go mod tidy` (after Step 2, or it drops duckdb-go) — expected end state: duckdb-go direct; `pflag` `// indirect`; `yaml.v3` stays as `// indirect` (testify + duckdb-go need it)
- [ ] Step 4: `duckdb_test.go` acceptance test — Create fresh file, DDL with `DECIMAL(18,2)`, AppendRows (negative, zero, `9999999999999999.99`), control: `.wal` present before close; CheckpointClose → no `.wal`; OpenReadOnly → values read back as `CAST(... AS VARCHAR)` strings equal inputs. Red at assertion
- [ ] Step 5: `internal/platform/sqlite/sqlite.go:118-128` stub `(*DB).QueryRows` + its acceptance test; `v9fixture/builder.go` stub `Builder` + `builder_test.go` acceptance test. Both red at assertion

### Build
- [ ] Step 6: `duckdb.go` `Create` — `O_EXCL` create at 0600, then open that file with the driver (if DuckDB rejects the 0-byte file: existence check → `ErrExists`, open, `chmod 0600`; say which in report). Pool pinned to 1 conn. Tests: `Test_create_refuses_an_existing_path` (red first), `Test_create_makes_the_file_owner_only`, fault: Create in read-only dir → `IsPermission` true (os `*PathError` shape)
- [ ] Step 7: `(*DB).CheckpointClose` — `CHECKPOINT`, close, then unexported no-WAL check → `ErrWALRemains`. Tests: `Test_no_wal_check_fails_when_a_wal_file_remains` (red first; white-box — planting a `.wal` around a live DB does not work: DuckDB replays it on open or truncates it on checkpoint), fault: CHECKPOINT on closed DB errors
- [ ] Step 8: `(*DB).AppendRows(ctx, table, rows [][]any)` via driver Appender on a `sql.Conn`/`Raw`; `Decimal(unscaled int64, width, scale) (value, error)` → driver Decimal (no float64); rejects `|unscaled| ≥ 10^width` itself, since the driver may append an overflowing value silently. Return Appender `Close`/flush error (never deferred-ignored); check ctx per row; release the conn before returning (pool of 1 deadlocks otherwise). Tests: `Test_append_rows_reports_a_duplicate_primary_key`, `Test_append_rows_reports_a_wrong_column_count`, `Test_append_rows_stops_when_the_context_is_cancelled`, bound: `Decimal` refuses unscaled 10^18 (`10000000000000000.00`), accepts 10^18−1 (`9999999999999999.99`) as control
- [ ] Step 9: `(*DB).Exec`, `(*DB).QueryRows` (same callback contract as sqlite's), `OpenReadOnly` — faults: Exec syntax error, QueryRows query error, scan error, callback error returned (`errors.Is` holds), OpenReadOnly on missing path
- [ ] Step 10: `IsDiskFull` (ENOSPC or EDQUOT) / `IsPermission` (EACCES/EPERM) — `errors.Is` for os errors; driver error type + case-insensitive match of `syscall.X.Error()` against DuckDB's `strerror` text. Table test: one row per real shape (os `*PathError`, driver IO error in the driver's actual message format — read the driver's error type first), negatives for unrelated IO error and nil. Real-shape driver permission row: `OpenReadOnly` on a mode-0000 file (0400 opens fine; `Create` on an existing file is `ErrExists`)
- [ ] Step 11: `sqlite.go` `(*DB).QueryRows(ctx, query, args, row func(scan func(dest ...any) error) error) error` — closes rows, surfaces `rows.Err`, callback error stops iteration. Faults in `sqlite_test.go`: query error, scan type error, callback error (`errors.Is`), ctx cancelled mid-iteration
- [ ] Step 12: new `internal/quicken/v9/v9fixture/builder.go` — `Builder` with row structs and methods returning assigned `Z_PK`; `Seed(tb, *sql.DB)`, `WriteBundle(tb, dir) Bundle` (closed, non-WAL, ReferenceDDL). Rows per `internal/quicken/v9/reference.sql`: `Z_PRIMARYKEY` :91 (defaults CategoryTag 75, UserTag 76, CashFlowTransaction 79, SmartCashFlowTransaction 80, InvestmentTransaction 81; overridable), `ZACCOUNT` :79 (ZNAME, ZTYPENAME, ZCURRENCY, ZCLOSED, ZACTIVE, ZDELETIONCOUNT), `ZTRANSACTION` :83 (Z_ENT, ZACCOUNT, ZAMOUNT, ZPOSTEDDATE/ZENTEREDDATE nullable, ZRECONCILESTATUS nullable, ZUSERPAYEE, ZNOTE, ZCHECKNUMBER, ZDELETIONCOUNT), `ZCASHFLOWTRANSACTIONENTRY` :64 (ZPARENT, ZAMOUNT, ZCATEGORYTAG, ZTRANSFER text, ZQUICKENID, ZNOTE, ZDELETIONCOUNT), `ZRECONCILERECORD` :52 (ZACCOUNT, ZENDDATE, ZENDINGBALANCE, ZDELETIONCOUNT), `ZTAG` :84 (Z_ENT, ZNAME, ZTYPE, ZHIDDEN, ZPARENTCATEGORY, ZDELETIONCOUNT), `Z_15USERTAGS` :37, `ZUSERPAYEE` :57 (ZNAME, ZDELETIONCOUNT). Money is a decimal string bound as text; dates take `time.Time` → Core Data seconds since 2001-01-01 UTC (exported helper). Acceptance test reads every kind back via `sqlite.QueryRows`, asserts overridden entity numbers land, `typeof(ZAMOUNT)` is `integer` for `"12.00"` and `real` for `"12.34"`, and a `"12.345"` value is storable. `OpenBundle` untouched

### Sweep
- [ ] Step 13: fix what `go build ./... && golangci-lint run ./...` reports; doc comments per `clean-architecture` (`doc.go` for `platform/duckdb`); `go doc ./internal/platform/duckdb` reads as the contract

### Verify
- [ ] Step 14: full verification per `.claude/rules/agent-briefs.md` → *Verification* (`-race` on the three packages); under `set -o pipefail`, `go list -deps -test ./internal/snapshot/ ./internal/cli/ ./cmd/quarry/ ./internal/quicken/v9/... ./internal/platform/sqlite/ | grep -c duckdb-go` prints 0 (grep then exits 1 — expected; a `go list` failure prints an error instead), control: same on `./internal/platform/duckdb/` is >0; no spec tick, set `status: done`

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Bulk insert = driver Appender behind `(*DB).AppendRows`; money enters DuckDB only as `duckdb.Decimal(unscaled cents, 18, 2)` — P1-7 forbids float64 in the path.
- Money leaves DuckDB only as `CAST(... AS VARCHAR)` or the driver Decimal type — scanning DECIMAL into float64 silently breaks P1-7.
- `CheckpointClose` is the only close before a rename; it returns `ErrWALRemains` — 01a's swap (P1-2) depends on "no `.partial.wal` before rename".
- `duckdb-go` is imported only by `internal/platform/duckdb` (and 01a's store adapter) — other test binaries must not link it (link time, Linux CI OOM risk in `devenv.nix`).
- `IsDiskFull` → S2, `IsPermission` → S1; classification lives here, not string-matched by callers.
- v9 money fixtures are strings; SQLite NUMERIC affinity picks the storage class (whole → INTEGER, fractional → REAL).

**Left unbuilt** — named so nobody assumes it exists:
- `internal/store`, the importer, its `Source` port, snapshot→manifest resolution — SCENARIO-01a.
- Store file mode 0600 after rename — 01a asserts it (Create only sets it on the partial).
- `ZFINANCIALINSTITUTION` / `accounts.institution` seeding in `v9fixture.Builder` — 01a if needed.
- Upgrading `OpenBundle` / Phase 0 fixtures to typed accounts — 01a (P1-9 trap).

**Traps** — things that look right and are not:
- `Z_15USERTAGS` columns are `Z_15CASHFLOWTRANSACTIONENTRIES` / `Z_76USERTAGS` whatever the UserTag entity number — the importer names them literally.
- Payees are `ZTRANSACTION.ZUSERPAYEE` → `ZUSERPAYEE.ZNAME`, not `ZBPFIPAYEE`.
- Appender errors surface at `Close`, and it ignores ctx; holding its `sql.Conn` with a 1-conn pool deadlocks any other query.
- If the Appender rejects the Decimal value, fall back to a prepared insert with `CAST(? AS DECIMAL(18,2))` from a string — not float64.
- `go mod tidy` before any `.go` file imports duckdb-go removes it. GOMODCACHE is per-worktree (`.devenv/`), so module fetches need network.
