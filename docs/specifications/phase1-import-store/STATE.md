# phase1-import-store — current state

Scenarios complete: SCENARIO-01a (+ folded 04, 05), SCENARIO-01d, SCENARIO-01b (+ folded 06). Last updated by SCENARIO-01b.

## Binding decisions
- Store ADR: `docs/adr/001-shared-store-package.md`. `internal/store` = row types only, no driver; `internal/store/duckstore` = DDL + builder + atomic swap. (SCENARIO-01a)
- `importer.Store` is one method: `Replace(ctx, store.Rows) (string, error)`. `importer.Server.Import(ctx, snapshotPath) (store.Result, error)` maps a v9 snapshot end to end, validates it, and writes it only once every check passes. (SCENARIO-01a, validated SCENARIO-01b)
- Sequencing ADR: `docs/adr/002-sync-sequences-import-in-snapshot.md`. `(*Server).SyncAndImport` branches on `errors.Is(err, store.ErrValidationFailed)`: it sets `Outcome.Store` to the unbuilt `store.Result` and builds the V1 refusal via `(*Server).validationFailedRefusal` (own frame — distinct from `storeBuildRefusal`'s S3 "cannot build the store" text); every other import error keeps `Store` nil and goes through the S3 frame. `(Outcome).StdoutWriteRefusal` is O1 while `Store == nil`, O1b once set — V1 already gets O1b. (SCENARIO-01d, wired SCENARIO-01b)
- Balance/split validation: `store.Result{Built, Counts, Validation}`; `Validation{Balances BalanceCheck, Splits SplitCheck}`, `Validation.Failed()`. `store.ErrValidationFailed` is returned unwrapped (`causeText` walks to it). `BalanceMismatch`/`SplitMismatch`/never-reconciled (`[]store.Account`) entries already carry every spec :216,227-228 display field (statement date, quarry/quicken/difference cents, payee name, account name, closed/active) — 02 and 09 render from them, never recompute or re-read `Rows`. (SCENARIO-01b)
- Newest-statement selection runs in Go (`internal/importer/statements.go` `newestStatements`/`newerReconcile`), not SQL `ORDER BY`: a NULL `ZENDDATE` ranks newest (refuses reason 10 rather than falling back), else latest `ZENDDATE`, ties → higher `Z_PK`. Only that one record is parsed; records on investment, deleted or missing accounts are skipped before ranking. A record's missing-date and bad-balance faults are independent offenders — both may be added to `off` — so `s4ClassOrder` (5/6/11 outrank 10) decides which is reported; a fixed balance surfaces the date fault on a later run. (SCENARIO-01b)
- `accountRef` gained `Type` (the balance gate's investment-account exclusion, `investmentTypes` in `statements.go`, reused by `validate.go`). `v9fixture.ReconcileRow.EndDate` is `*time.Time` (nil writes NULL); `EndingBalance` "" already writes NULL. (SCENARIO-01b)
- `cli.renderStore` renders Store, Rows, Balances, Splits (`balancesPhrase`/`splitsPhrase` in `internal/cli/render.go`) — every success-path `cmd/quarry` stdout fixture now needs these two lines; a 0-transaction fixture renders `Splits    no transactions to check` and any unreconciled account renders a never-reconciled clause. (SCENARIO-01b)
- Money is int64 cents in `store.Rows`, parsed exactly via `typeof(col)` + `CAST(col AS TEXT)` — `parseMoney` is reused by statement-balance parsing too, never reimplemented. (SCENARIO-01a)
- IDs are `<prefix>-<Z_PK>` VARCHAR (acct/cat/payee/tag/txn/split), derived from each row's own Z_PK. 01c's transfer ids derive from split ids. (SCENARIO-01a)
- `*importer.UnmappableError{Reason}` carries the ruled `<reason>` text verbatim incl. ` (and N more)`. S4 offenders accumulate across every mapping step (`s4ClassOrder`: 7, then 1-6, then 11, then 8-10; within a class, undated offenders sort before dated ones) in `internal/importer/offenders.go`. (SCENARIO-01a)
- P1-5d: a reference to a **deleted** row is dropped silently with whatever depends on it; a reference to a row that **does not exist at all** is treated as NULL (reason 10 on required, stored NULL on nullable). (SCENARIO-01a)
- `v9fixture.Builder` writes real SQL NULL for every zero ref and every empty required string. `Builder.WithoutEntity(name)` omits a `Z_PRIMARYKEY` row. `OpenBundle`/`ExtraSchemaBundle` seed typed accounts; `MissingSchemaBundle` stays untyped on purpose. (SCENARIO-01a/01d)
- Every account/category boolean is read via `COALESCE(col, 0)` — a NULL imports as `false`. (SCENARIO-01a)
- `(*duckdb.DB).Create`/`CheckpointClose`/`AppendRows`/`Decimal`; `QueryRows(ctx, query, args, row)` is the shared multi-row read shape on both `platform/sqlite` and `platform/duckdb`. (PREP-c)

## Left unbuilt
- Pre-swap ctx check (I2), S1/S2 classification, EDQUOT routing — SCENARIO-14. `duckstore.Replace` does not check ctx before its rename.
- Stale `quarry.duckdb.wal` removal, `.partial` leftover sweep — SCENARIO-20.
- `transfers`, `import_runs` tables; `not_imported` count — 01c, 08, 01c/S21. `store.Counts.Transfers` is always 0 until 01c.
- **V1 stdout block** (`NOT REBUILT`/`NOT BUILT`, `!` rows) and re-inserting `each difference is listed on stdout; ` into the stderr clause — SCENARIO-09, which runs next and only needs to render the block and restore that clause; the failing-clause text itself (singular/plural, denominators) already lives in `internal/snapshot/import.go`'s `validationFailedRefusal`/`balanceMismatchClause`/`splitMismatchClause` and does not change. `sync.go` still returns before writing stdout on any non-mismatch error.
- `--json` `store` key, `store: null` on mismatch, and `balances`/`splits`/`never_reconciled`/`mismatched` — SCENARIO-02 (`store.Validation` already carries every field the json shape needs). Transfers line, `; N investment transactions not imported` — 01c.
- `--from` flag + Example paragraph — 03. `snapshot.Server` method for `--from` — SCENARIO-03.

## Traps
- `WithEntity`/`WithoutEntity` overrides are the only proof entity numbers are resolved from `Z_PRIMARYKEY`, never hard-coded.
- SQLite NUMERIC affinity stores a whole-valued decimal string as INTEGER, not TEXT/REAL — branch on `typeof()`, not the source column's declared type. `ORDER BY … DESC` puts NULLs **last**; the newest-statement rule needed Go-side ranking, not SQL, to make NULL rank first.
- `mattn/go-sqlite3` auto-converts a TIMESTAMP-declared column to a Unix-epoch `time.Time` unless the query casts it — `ZPOSTEDDATE`/`ZENTEREDDATE`/`ZENDDATE` need `CAST(... AS REAL)`.
- `ZDELETIONCOUNT` may be NULL — every row filter treats it as 0 (not deleted).
- `CAST(col AS TEXT)` of a NULL money column scans as NULL — scan into `sql.NullString`, not `string`.
- DuckDB's `InstanceCache` refuses a second connection to the same path with a different config while the first is open.
- `causeText` (`snapshot/destination_refusal.go:15`) prints the innermost error — wrapping with extra text as `%w: …` loses it; `validationFailedRefusal` builds its own message directly rather than routing through `causeText`.
- `uncovered-diff.py` is blind to untracked files (`git diff <start>` doesn't see them) — `git add` before trusting its output.
- O1b and the S3 refusal both tell the user to run `--from`, which does not exist until SCENARIO-03 — ruled copy; do not "fix" it early.
- `cmd/quarry`'s test binary links DuckDB; linux-small CI has OOMed linking it (`devenv.nix`).
- `go list -deps` without `-test` misses test-only imports.

## Open debts
- `--json` stdout gains top-level `store` key (P1-11); `Test_run_prints_the_manifest_as_json_with_the_json_flag` needs to change to "stdout minus `store` equals the manifest" — owned by SCENARIO-02.
- `duckstore.Replace`'s `CheckpointClose`-failure branch, and `build`'s schema-exec failure via a real fault, have no black-box trigger through the current API — declared unreachable with a stated reason each; SCENARIO-14's fault work may want a seam to exercise them for real.
- The "1 of 1 accounts does not match" balance-clause singular is unruled (spec only shows denominators ≥ 2); no test asserts it. Whichever scenario first hits a single-account file should raise it to product-vision if it matters — unowned, dies unless re-opened.
