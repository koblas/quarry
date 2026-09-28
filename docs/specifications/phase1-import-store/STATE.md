# phase1-import-store — current state

Scenarios complete: SCENARIO-01a (+ folded 04, 05), SCENARIO-01d, SCENARIO-01b (+ folded 06),
SCENARIO-09 (+ folded 11). Last updated by SCENARIO-09.

## Binding decisions
- Store ADR: `docs/adr/001-shared-store-package.md`. `internal/store` = row types only, no driver; `internal/store/duckstore` = DDL + builder + atomic swap. (SCENARIO-01a)
- `importer.Store` is one method: `Replace(ctx, store.Rows) (string, error)`. `importer.Server.Import(ctx, snapshotPath) (store.Result, error)` maps a v9 snapshot end to end, validates it, and writes it only once every check passes. (SCENARIO-01a, validated SCENARIO-01b)
- Sequencing ADR: `docs/adr/002-sync-sequences-import-in-snapshot.md`. `(*Server).SyncAndImport` branches on `errors.Is(err, store.ErrValidationFailed)`: it sets `Outcome.Store` to the unbuilt `store.Result` (with `Path` populated from `s.storePath` on this returned copy only — `importer.Server.Import`'s own contract still leaves `Path` empty on a failed build) and `Outcome.StoreExisted` from `previousStoreExists()`, and builds the V1 refusal via `validationFailedRefusal` (own frame, restored `each difference is listed on stdout; ` clause). Every other import error keeps `Store` nil and goes through the S3 frame. `(Outcome).StdoutWriteRefusal` is O1 while `Store == nil`, O1b once set. (SCENARIO-01d, wired SCENARIO-01b, `StoreExisted`/`Path` added SCENARIO-09)
- Balance/split validation: `store.Result{Built, Counts, Validation}`; `Validation{Balances BalanceCheck, Splits SplitCheck}`, `Validation.Failed()`. `store.ErrValidationFailed` is returned unwrapped. `BalanceMismatch`/`SplitMismatch` both carry a numeric `SourceID` (the account's, resp. the transaction's own source id) used for display order only — never emitted once `--json` V1 lands. Sorting happens **once, in the core** (`internal/importer/validate.go`'s `checkBalances`/`checkSplits`), never re-sorted by a renderer or by `--json`: Balances `Mismatched`/`NeverReconciled` by account name (byte order) then account source id; Splits `Mismatched` by date, account name, account source id, transaction source id — all source-id ties numeric (`cmp.Compare`), never a string comparison (a "9" vs "10" tie inverts under string order). (SCENARIO-01b, sort order SCENARIO-09)
- V1 count form is always `X of Y <noun>` (`balanceMismatchClause`/`xOfYPhrase`): the noun agrees with `Y`, the verb (stderr only) with `X`. The V1 stdout block's Balances/Splits DIFFER lines use `xOfYPhrase` too (`internal/cli/render.go`). (SCENARIO-01b checkpoint fix; DIFFER form SCENARIO-09)
- **V1 stdout block** (SCENARIO-09): `cli.renderStoreFailure(result, storeExisted, home)` renders Store (`NOT REBUILT (<path> unchanged)` when `storeExisted`, else `NOT BUILT (no store at <path> yet)`), Rows, and each of Balances/Splits independently: the DIFFER form + `!` rows (`balanceMismatchRows`/`splitMismatchRows`) when that check itself has mismatches, else its ordinary success phrase. `cli.accountLabel(name, currency, closed, active)` renders `Name (CUR[, closed][, inactive])` — `inactive` only when open (closed suppresses it, not merely absence) — and is reused for Splits rows via `closed=false, active=true`. `cli.formatMoney(cents)` is the one thousands-grouped 2-decimal renderer for every amount column. `cli.sync.go`'s `RunE`: `validationFailed := outcome.Store != nil && !outcome.Store.Built` renders on the human path (`renderSuccess` + `renderStoreFailure`) but `--json` still returns before writing (V1 `--json` stays interim until SCENARIO-02).
- `cli.renderStore` (success path) and `cli.renderStoreFailure` (V1 path) share `rowsPhrase`/`balancesPhrase`/`splitsPhrase`/`balancesExtrasPhrase` for whichever check did *not* fail — only the failed check's line switches to its DIFFER form.
- Money is int64 cents in `store.Rows`, parsed exactly via `typeof(col)` + `CAST(col AS TEXT)`. (SCENARIO-01a)
- IDs are `<prefix>-<Z_PK>` VARCHAR (acct/cat/payee/tag/txn/split), derived from each row's own Z_PK. 01c's transfer ids derive from split ids. (SCENARIO-01a)
- `*importer.UnmappableError{Reason}` carries the ruled `<reason>` text verbatim. S4 offenders accumulate across every mapping step (`s4ClassOrder` in `internal/importer/offenders.go`). (SCENARIO-01a)
- P1-5d: a reference to a **deleted** row is dropped silently with whatever depends on it; a reference to a row that **does not exist at all** is treated as NULL. (SCENARIO-01a)
- `v9fixture.Builder` writes real SQL NULL for every zero ref and every empty required string; assigns `Z_PK` strictly by call order per table (so "account #9" = the 9th `b.Account(...)` call). `Builder.WithoutEntity(name)` omits a `Z_PRIMARYKEY` row. (SCENARIO-01a/01d)
- Every account/category boolean is read via `COALESCE(col, 0)`. (SCENARIO-01a)
- `(*duckdb.DB).Create`/`CheckpointClose`/`AppendRows`/`Decimal`; `QueryRows(ctx, query, args, row)` is the shared multi-row read shape. (PREP-c)

## Left unbuilt
- Pre-swap ctx check (I2), S1/S2 classification, EDQUOT routing — SCENARIO-14. `duckstore.Replace` does not check ctx before its rename.
- Stale `quarry.duckdb.wal` removal, `.partial` leftover sweep — SCENARIO-20.
- `transfers`, `import_runs` tables; `not_imported` count — 01c, 08, 01c/S21. `store.Counts.Transfers` is always 0 until 01c.
- Transfers line in the V1 block — SCENARIO-01c (same rule as the success block).
- `--json` V1 (`"built": false`, `mismatched[]` sorted by the already-sorted core lists, `SourceID` omitted from the JSON shape) — SCENARIO-02; `store.Validation` already carries every field and the display order the json shape needs, nothing to re-derive.
- `--from` flag + Example paragraph — 03. `snapshot.Server` method for `--from` — SCENARIO-03.

## Traps
- `WithEntity`/`WithoutEntity` overrides are the only proof entity numbers are resolved from `Z_PRIMARYKEY`, never hard-coded.
- SQLite NUMERIC affinity stores a whole-valued decimal string as INTEGER, not TEXT/REAL — branch on `typeof()`, not the source column's declared type.
- `mattn/go-sqlite3` auto-converts a TIMESTAMP-declared column to a Unix-epoch `time.Time` unless the query casts it.
- `ZDELETIONCOUNT` may be NULL — every row filter treats it as 0 (not deleted).
- `CAST(col AS TEXT)` of a NULL money column scans as NULL — scan into `sql.NullString`, not `string`.
- DuckDB's `InstanceCache` refuses a second connection to the same path with a different config while the first is open.
- `causeText` prints the innermost error — wrapping with extra text as `%w: …` loses it; `validationFailedRefusal` builds its own message directly.
- `uncovered-diff.py` is blind to untracked files (`git diff <start>` doesn't see them) — `git add` before trusting its output.
- O1b and the S3 refusal both tell the user to run `--from`, which does not exist until SCENARIO-03 — ruled copy; do not "fix" it early.
- `cmd/quarry`'s test binary links DuckDB; linux-small CI has OOMed linking it (`devenv.nix`).
- `v9fixture.Builder` assigns `Z_PK` strictly by call order, so "higher `Z_PK`" and "inserted later" are the same fact for any row kind — a tie-break-by-source-id test needs a second, differently-ordered key (name, date) to discriminate a "wrong sort direction" bug from a "no sort at all" bug; a same-name/same-date pair (two accounts both literally named the same, or two mismatches on the exact same day) isolates the source-id tie-break specifically.
- A string comparison of numeric source ids inverts two-digit vs one-digit values ("10" < "9"); every source-id tie-break in `internal/importer/validate.go` uses `cmp.Compare` on the int64, never `strings.Compare` on a formatted id.
- `internal/platform/duckdb`'s `Test_query_rows_fails_when_the_context_is_cancelled_mid_iteration` is flaky under full-suite load (timing-dependent ctx cancellation) — passes standalone and on rerun; unrelated to this package, pre-existing.

## Open debts
- `--json` stdout gains top-level `store` key (P1-11); `Test_run_prints_the_manifest_as_json_with_the_json_flag` needs to change to "stdout minus `store` equals the manifest" — owned by SCENARIO-02.
- `duckstore.Replace`'s `CheckpointClose`-failure branch, and `build`'s schema-exec failure via a real fault, have no black-box trigger through the current API — declared unreachable with a stated reason each; SCENARIO-14's fault work may want a seam to exercise them for real.
