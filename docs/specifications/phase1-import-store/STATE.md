# phase1-import-store — current state

Scenarios complete: SCENARIO-01a (+ folded 04, 05). Last updated by SCENARIO-01a.

## Binding decisions
- Store ADR: `docs/adr/001-shared-store-package.md`. `internal/store` = row types only, no driver; `internal/store/duckstore` = DDL + builder + atomic swap — keeps DuckDB out of snapshot/cli/importer test binaries (verified via `go list -deps -test`). (SCENARIO-01a)
- `importer.Store` is one method: `Replace(ctx, store.Rows) (string, error)`. `importer.Server.Import(ctx, snapshotPath) (store.Result, error)` maps a v9 snapshot end to end; validation runs on `store.Rows` in memory before `Replace`, so a failed check never creates a partial. 01d's `snapshot.Importer` port is shaped to `Import`'s signature; the importer never sees `Manifest`. (SCENARIO-01a)
- Money is int64 cents in `store.Rows`, parsed exactly from `typeof(col)` + `CAST(col AS TEXT)` — never floats. A REAL with an exponent or whose integer part is `≥ 1e13`, or an integer-stored value `≥ 1e16` dollars, refuses (S4 reason 6); >2 decimal places refuses with reason 3/4/5 by subject. 01b's statement-balance parsing must reuse `internal/importer`'s `parseMoney`, not reimplement it. (SCENARIO-01a)
- IDs are `<prefix>-<Z_PK>` VARCHAR (acct/cat/payee/tag/txn/split) — permanent SQL surface, derived from each row's own Z_PK, never from read/output order (`ORDER BY` clauses exist only for deterministic output and S4 reporting). 01c's transfer ids derive from split ids. (SCENARIO-01a)
- `*importer.UnmappableError{Reason}` carries the ruled `<reason>` text verbatim incl. ` (and N more)`; `errors.As` is the contract. S4 offenders accumulate across every mapping step before Import picks the first-ordered class's first offender (`s4ClassOrder`: 7, then 1-6, then 8-10). Within a class, undated offenders (accounts/categories, by name) sort before dated ones (transactions/splits, by date/account/source id) — a binding ordering choice the spec left open, now in `internal/importer/offenders.go`. 01d renders `Reason` in the S3 frame; 14 swaps the frame. (SCENARIO-01a)
- `v9fixture.Builder` writes real SQL NULL for every zero ref (`TransactionRow.Account/Payee`, `EntryRow.Parent/CategoryTag`, `TagRow.ParentCategory`) and every empty required string (`Name`, `Type`, `Currency`, `Amount`); `TagRow.Type` is `*int64` (0 is a valid ZTYPE, so nil is the only "no type"). `Builder.Institution(row)` seeds `ZFINANCIALINSTITUTION`; `AccountRow.Institution` is a zero ref to it. `Builder.WithoutEntity(name)` omits a `Z_PRIMARYKEY` row, for missing-entity (S4 reason 7) tests. (SCENARIO-01a)

## Left unbuilt
- Pre-swap ctx check (I2), S1/S2 classification, EDQUOT routing — SCENARIO-14. `duckstore.Replace` does not check ctx before its rename.
- Stale `quarry.duckdb.wal` removal, `.partial` leftover sweep — SCENARIO-20.
- `transfers`, `import_runs` tables; `not_imported` count — 01c, 08, 01c/S21. `store.Counts.Transfers` is always 0 until 01c; `splits.transfer_account_id` is never set in 01a.
- Reconcile-record reads, S4 reason 5, statement required-NULL reasons — 01b.
- `OpenBundle`/`ExtraSchemaBundle` typed accounts + `Z_PRIMARYKEY` rows, `snapshot.Importer` port, `cmd`/`cli` wiring, sequencing ADR — 01d.
- Until 01b lands, whatever wires the importer in swaps in an unchecked store (no balance/split-sum gate yet).

## Traps
- `WithEntity`/`WithoutEntity` overrides are the only proof entity numbers are resolved from `Z_PRIMARYKEY`, never hard-coded.
- SQLite NUMERIC affinity stores a whole-valued decimal string as INTEGER, not TEXT/REAL — `parseMoney` branches on `typeof()`, not on whether the source column declaration has a decimal point.
- `ZDELETIONCOUNT` may be NULL — every row filter treats it as 0 (not deleted), never as a required field.
- `mattn/go-sqlite3` auto-converts a TIMESTAMP-declared column to a Unix-epoch `time.Time` unless the query itself casts it — `ZPOSTEDDATE`/`ZENTEREDDATE` must be read via `CAST(... AS REAL)` to get the raw Core Data epoch-seconds value.
- `CAST(col AS TEXT)` of a NULL money column scans as NULL — scan into `sql.NullString`, not `string`, or Scan errors before the code reaches the "no amount" check.
- `Z_15USERTAGS`' own columns (`Z_15CASHFLOWTRANSACTIONENTRIES`/`Z_76USERTAGS`) are literal, not entity-derived.
- Payees are `ZTRANSACTION.ZUSERPAYEE` → `ZUSERPAYEE.ZNAME`, not `ZBPFIPAYEE`.
- `uncovered-diff.py` is blind to untracked files (`git diff <start>` doesn't see them) — `git add` before trusting its output.

## Open debts
- `--json` stdout gains top-level `store` key (P1-11); `Test_run_prints_the_manifest_as_json_with_the_json_flag` needs to change to "stdout minus `store` equals the manifest" — owned by SCENARIO-02.
- `duckstore.Replace`'s `CheckpointClose`-failure branch, and `build`'s schema-exec failure via a real fault (only a white-box schema-collision test exercises it here), have no black-box trigger through the current API — declared unreachable with a stated reason each; SCENARIO-14's S1/S2/S3 fault work may want a seam to exercise them for real.
