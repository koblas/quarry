# phase1-import-store — current state

Infrastructure complete: PREP-c (checkpoint BLOCKER + MAJORs fixed). No scenario implemented yet — SCENARIO-01a is next.
Last updated by PREP-c's fix pass.

## Binding decisions
- `internal/platform/duckdb` wraps `github.com/duckdb/duckdb-go/v2` (driver name `"duckdb"`, `sql.Open("duckdb", path[+"?access_mode=READ_ONLY"])`). `DB` pool is pinned to 1 conn. This package (and 01a's store adapter) is the **only** importer of the driver — verified via `go list -deps -test`: 0 hits on snapshot/cli/cmd/v9/sqlite, 5 on `platform/duckdb` itself. Re-check this after adding the store adapter. (PREP-c)
- Money enters DuckDB only via `duckdb.Decimal(unscaled cents int64, width, scale uint8) (any, error)`, which self-validates `|unscaled| < 10^width` — the Appender silently accepts an overflow otherwise, there is no driver backstop. Money leaves only via `CAST(... AS VARCHAR)` or the driver's `Decimal` type. Never float64 in the path (P1-7). (PREP-c)
- `(*duckdb.DB).Create(ctx, path)` refuses an existing path (`ErrExists`) via `os.Stat` then `sql.Open`+`Ping`+`chmod 0600` — not `O_EXCL`, because DuckDB refuses a pre-created 0-byte file. `(*duckdb.DB).CheckpointClose(ctx)` is the only close before any rename; it returns `ErrWALRemains` if `path+".wal"` still exists after `CHECKPOINT`+`Close` (01a's swap, P1-2, depends on this). `IsDiskFull`/`IsPermission` classify both `errors.Is` os shapes and the driver's own `*duckdbdriver.Error{Type: ErrorTypeIO}` (case-insensitive `syscall.Errno.Error()` substring match) — classification lives in this package, callers never string-match. (PREP-c)
- `(*DB).QueryRows(ctx, query, args []any, row func(scan func(dest ...any) error) error) error` is the shared multi-row read shape on **both** `internal/platform/sqlite` and `internal/platform/duckdb` — same signature, same "callback error stops iteration, `errors.Is` holds" contract. Give any future read port this shape rather than inventing another. (PREP-c)
- `internal/quicken/v9/v9fixture.Builder` (`builder.go`) is the row-level v9 fixture: `Account`/`Transaction`/`Entry`/`Reconcile`/`Category`/`UserTag`/`Payee`/`LinkUserTag` methods return the assigned `Z_PK`; `WithEntity(name, ent)` overrides one of the 5 entity numbers `Seed`/`WriteBundle` write into `Z_PRIMARYKEY` (CategoryTag 75, UserTag 76, CashFlowTransaction 79, SmartCashFlowTransaction 80, InvestmentTransaction 81 by default). `WriteBundle(tb, dir) Bundle` is closed/non-WAL. `Z_15USERTAGS`'s own columns are literal (`Z_15CASHFLOWTRANSACTIONENTRIES`/`Z_76USERTAGS`), not entity-derived. `OpenBundle`/`ClosedWALBundle`/etc. (Phase 0 fixtures) are untouched. (PREP-c)
- SQLite's NUMERIC affinity silently collapses a whole-valued decimal *string* ("12.00") to INTEGER storage on any `DECIMAL`-declared v9 column (confirmed on both `ZAMOUNT` and `ZRECONCILERECORD.ZENDINGBALANCE`); 01a's importer must format money with 2 decimals itself rather than trust the source storage class's shape.

## Left unbuilt
- `internal/store` (quarry's own DuckDB schema DDL, row types, builder, atomic swap) — unowned until SCENARIO-01a. Importer declares its own narrow `Store` port; store package implements it (per specification.md's architecture rulings).
- The importer package and its `Source` port over `platform/sqlite`'s new `QueryRows` — SCENARIO-01a.
- `snapshot.Server` method for `--from` (Manifest decode + snapshot→manifest resolution reusing `buildManifest`'s steps) — SCENARIO-01a.
- `cli.ServerFactory` reshape (snapshot → import sequencing) and its callers (`cmd/quarry/run.go` `newServer`, cli/cmd tests) — SCENARIO-01a, caller table required at plan time (LSP-tagged).
- `ZFINANCIALINSTITUTION` / `accounts.institution` seeding in `v9fixture.Builder` — 01a, only if the importer needs it.
- `Z_PRIMARYKEY` rows for `Account`/`CashFlowTransactionEntry`/`ReconcileRecord`/`UserPayee` entities — `v9fixture.Builder` does not write them (only the 5 overridable kinds); add if 01a's importer starts resolving those entity numbers too.
- Upgrading `OpenBundle` / Phase 0 command-test fixtures to typed accounts (ZTYPENAME/ZCURRENCY set) — 01a, P1-9 trap: once import is wired, untyped Phase-0 fixture accounts become S4 refusals.

## Traps
- `Z_15USERTAGS` columns are literally `Z_15CASHFLOWTRANSACTIONENTRIES`/`Z_76USERTAGS` regardless of any `WithEntity("UserTag", ...)` override — the importer must name them literally, not derive from the entity number. (PREP-c)
- DuckDB's `InstanceCache` refuses a second connection to the same path with a different config while the first is open ("Connection Error: ... different configuration than existing connections") — close a writer `*duckdb.DB` before opening `OpenReadOnly` on the same path. (PREP-c)
- Appender faults (constraint violations, etc.) surface at `Close`/`Flush`, not at `AppendRow`; `AppendRows` ignores ctx inside the Appender itself and checks `ctx.Err()` per row instead. (PREP-c)
- `go mod tidy` run before any `.go` file actually imports `duckdb-go` removes it from `go.mod` again — always add the import (even a blank one) before tidying. (PREP-c)
- `sql.Open("duckdb", ...)` eagerly opens an *existing* file (permission/validity faults surface from `sql.Open` itself, verified via `go tool cover`), but *creating* a new file in an unwritable directory is deferred to `PingContext`/`Connect` — a cancelled-ctx test is the reliable way to reach the Ping-only branch without racing real cancellation against connection setup. `database/sql.DB.Close()` is idempotent at the database/sql layer (a second call never reaches the driver), so the driver's own double-close error is unreachable through this package's API. (PREP-c fix pass)
- DuckDB batches query results into internal chunks; with only 1-2 rows a whole result is fetched by the first `Next()`, so a ctx cancelled from inside a `QueryRows` callback needs enough rows (~20k was reliable) to force a later chunk fetch that actually observes it. (PREP-c fix pass)
- **`uncovered-diff.py` is blind to untracked files** (`git diff <start>` doesn't see them) and reports "0 runs" — indistinguishable from a clean gate — rather than an error. `git add` new files before trusting its output; this bit the PREP-c developer once (self-reported 0 uncovered lines that were really 17). (PREP-c fix pass)
- Payees are `ZTRANSACTION.ZUSERPAYEE` → `ZUSERPAYEE.ZNAME`, not `ZBPFIPAYEE`. (carried from triage)
- `buildManifest` (snapshot package) still reads with concrete `sqlite`/`os` calls, no read port — 01a must not grow a second ad hoc snapshot reader; extract/reuse rather than duplicate.
- EDQUOT during the DuckDB build should route to S2 like `IsDiskFull` on the snapshot side does — decide explicitly in 01a rather than leaving disk-full classification split between packages.

## Open debts
- `cmd/quarry/run_test.go` / `internal/snapshot/sync_faults_test.go` split (behaviour-neutral) before 01a adds more command tests to either — unowned, dies unless SCENARIO-01a re-opens it.
- `--json` stdout gains top-level `store` key (P1-11); `Test_run_prints_the_manifest_as_json_with_the_json_flag` needs to change to "stdout minus `store` equals the manifest" — owned by SCENARIO-01a.
- Architecture rulings (cli/store sequencing point, ADRs for the sequencing choice and for `internal/store`'s placement) are still open per specification.md — owned by SCENARIO-01a's architect pass, not by this infra prep.
