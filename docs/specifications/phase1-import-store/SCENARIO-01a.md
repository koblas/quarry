---
id: SCENARIO-01a
status: done
---

# SCENARIO-01a: the importer builds a store from a v9 snapshot

Size: OWNS A RUN (fallback seam if it overruns: Step 10's S4 ordering / `(and N more)` moves to 01d). Absorbs FOLDs 04, 05.
Cadence: test-first — `duckstore` partial + rename (atomicity / temp-then-rename adapter).
Acceptance test: `internal/importer/import_test.go` `Test_import_builds_every_table_from_a_v9_snapshot`
Acceptance test (SCENARIO-04, folded): `internal/importer/import_test.go` `Test_import_twice_from_the_same_snapshot_keeps_every_id`
Acceptance test (SCENARIO-05, folded): `internal/importer/import_test.go` `Test_import_keeps_each_categorys_parent_path_kind_and_hidden`
Narrow loop: `go test ./internal/store/... ./internal/importer/ ./internal/quicken/v9/v9fixture/`
Mutation checks: partial removal on a failed build in `(*duckstore.Store).Replace` → `Test_replace_removes_the_partial_when_the_build_fails`; no rename before the build succeeded → `Test_replace_leaves_the_existing_store_byte_identical_when_the_build_fails`; partial removal on a failed rename → `Test_replace_removes_the_partial_when_the_rename_fails`; entity numbers read from `Z_PRIMARYKEY`, never hard-coded → `Test_import_resolves_entities_by_name_from_z_primarykey`

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `internal/importer/import_test.go` — the three acceptance tests: `v9fixture.Builder.WriteBundle`, `bundle.DataPath` used as the snapshot path, a fake `Store` (test file) capturing `store.Rows`; assert exact rows incl. cents, UTC dates, `source_id`s. Fixture has no `ZTRANSFER` legs
- [x] Step 2: new `internal/store/{doc.go,store.go}` (row types, `Rows`, `Counts`, `Result`); new `internal/importer/{doc.go,importer.go,ports.go}` — `Server`, `NewServer`, `WithStore`, `WithSourceOpener`, `Source`/`Store` ports, `(*Server).Import(ctx, snapshotPath) (store.Result, error)`, `UnmappableError` — signature-only stubs; red at assertion

### Build
- [x] Step 3: `internal/quicken/v9/v9fixture/builder.go:35-92` row types + `:241-299` `Seed` — write NULL for a zero ref (`Payee`, `CategoryTag`, `ParentCategory`, entry `Parent`) and for an empty required value (`Name`, `Type`, `Currency`, `Amount`), a way to leave `TagRow.Type` NULL; `AccountRow.Institution` seeding `ZFINANCIALINSTITUTION`; `WithoutEntity(name)` omitting a `Z_PRIMARYKEY` row. Update `builder_test.go` for each
- [x] Step 4: new `internal/store/duckstore/{doc.go,duckstore.go,schema.go}` `New(dir)`, `(*Store).Replace(ctx, store.Rows) (string, error)` — DDL for accounts, categories, payees, transactions, splits, tags, split_tags; `duckdb.Create` at `.quarry-<UTC start>.duckdb.partial`, `AppendRows` (money via `duckdb.Decimal(cents, 18, 2)`), `CheckpointClose`, rename over `quarry.duckdb`; remove partial (+ `.wal`) on every failure. Tests red first: `Test_replace_swaps_in_a_store_that_reads_back_every_row` (via `duckdb.OpenReadOnly`, `CAST(amount AS VARCHAR)`), `..._leaves_no_partial_or_wal`, `..._makes_the_store_owner_only`, the three mutation-check tests (build fault = duplicate primary key, surfaces at Appender close; rename fault = non-empty directory at `quarry.duckdb`), `Test_replace_fails_in_a_read_only_store_directory` (errors, no `.partial`, target untouched; S1 copy is 14's), cancelled-ctx fault. Test-first
- [x] Step 5: `importer.go` `Import` skeleton + `resolveEntities` — open via opener (default `sqlite.OpenReadOnly`), read `Z_PRIMARYKEY` by `Z_NAME`; CashFlowTransaction, CategoryTag, UserTag required → reason 7 (sorted, `, `/` or `). `Test_import_resolves_entities_by_name_from_z_primarykey` (`WithEntity` overrides all three), missing-one and missing-several reason tests; open fault (missing path)
- [x] Step 6: accounts — `ZTYPENAME` map, CAD/USD, closed, active, institution (LEFT JOIN, nullable), `ZDELETIONCOUNT` filter; reasons 1, 2, 10 (name/type/currency). One test per reason; exclusion test uses a deleted account with a bad currency and NULL type — filters run before mapping checks
- [x] Step 7: categories (entity = CategoryTag) with `parent_id`, `full_path` (`:`-joined, parent walk bounded — a cyclic `ZPARENTCATEGORY` must not loop), kind 2/1/0 → income/expense/system, hidden; user tags (entity = UserTag); payees from `ZUSERPAYEE`; reasons 9, 10 (category name/type). Folded S05 test goes green here
- [x] Step 8: money parser (unexported, direct table test) — read `typeof(col)` + `CAST(col AS TEXT)`, exact decimal → int64 cents; a REAL with an exponent or `|v| ≥ 1e13` → reason 6 (15 significant digits cannot carry cents there). Rows: integer `12`, real `12.5`, negative, `12.345` (>2dp), integer bound `±9999999999999999` in / `±10000000000000000` out, REAL `9999999999999.99` in / `10000000000000.5` out (reason 6)
- [x] Step 9: transactions (entity = CashFlowTransaction only; Smart/Investment skipped, uncounted) — date = `ZPOSTEDDATE` else `ZENTEREDDATE`, UTC; status NULL/0/1/2; currency from account; payee/memo/cheque nullable. Splits from `ZCASHFLOWTRANSACTIONENTRY` joined to imported transactions (entries of skipped parents skipped); NULL category → NULL `category_id`. `split_tags` from `Z_15USERTAGS` (literal column names). IDs `acct-/cat-/payee-/tag-/txn-/split-<Z_PK>`. Reasons 3, 4, 6, 8, 10 (transaction account/amount/date, split amount/transaction); a deleted transaction and an InvestmentTransaction row each with a NULL account and amount do not refuse. `Test_import_keeps_every_other_id_when_the_snapshot_gains_a_row` (the extra row sorts first in read order, existing `Z_PK`s unchanged). Folded S04 test green here
- [x] Step 10: S4 selection — collect every offender; class order 7, 1–6, 8–10; first offender by date/account/source id (undated: name/full path/source id); ` (and N more)` counts that class only. `Test_import_reports_the_first_offender_and_how_many_more`, `Test_import_reports_the_earliest_class_when_several_fail`; tests assert reason substring + `errors.As(*UnmappableError)`
- [x] Step 11: `Import` tail — `Counts` (transfers 0), `Store.Replace`, return `store.Result{Path, Counts}`. Faults: `Replace` error propagates (`errors.Is`); table test, one row per source query (query error and scan error, sqlite's wrapped error shape) — `Z_PRIMARYKEY`, `ZACCOUNT`, `ZTAG`, `ZUSERPAYEE`, `ZTRANSACTION`, `ZCASHFLOWTRANSACTIONENTRY`, `Z_15USERTAGS`

### Sweep
- [x] Step 12: fix what `go build ./... && golangci-lint run ./...` reports; `doc.go` for `store`, `duckstore`, `importer`; doc comments on every exported symbol

### Verify
- [x] Step 13: full verification (`.claude/rules/agent-briefs.md`); `git add` new files before `uncovered-diff.py`; `go list -deps -test ./internal/importer ./internal/store ./internal/snapshot ./internal/cli | grep duckdb` → 0 hits, same command on `./internal/store/duckstore` → hits (control); `spec-check.py phase1-import-store` → tick 01a, 04, 05 with their acceptance tests

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Store ADR written: `docs/adr/001-shared-store-package.md`. `internal/store` = types only, no driver; `duckstore` = DDL + builder + swap — keeps DuckDB out of snapshot/cli/importer test binaries.
- `importer.Store` is one method, `Replace(ctx, store.Rows) (string, error)` — 01b validates `store.Rows` in memory before `Replace`; there is no handle to check a partial.
- `(*importer.Server).Import(ctx, snapshotPath) (store.Result, error)` — 01d's `snapshot.Importer` port is shaped to it; importer never sees `Manifest`.
- Money in `store.Rows` is int64 cents, parsed exactly from `CAST(... AS TEXT)` (P1-7). A REAL-stored value with an exponent or `|v| ≥ 1e13` refuses (reason 6): SQLite prints REALs to 15 significant digits (`123456789012345.67` → `123456789012346.0`). Integer-stored values keep the full DECIMAL(18,2) range. 01b's statement balances use the same parser.
- IDs are `<prefix>-<Z_PK>` VARCHAR (acct, cat, payee, tag, txn, split) — permanent SQL surface; 01c's transfer ids derive from split ids.
- `UnmappableError` carries the ruled `<reason>` verbatim incl. ` (and N more)`; 01d renders it in the S3 frame, 14 swaps the frame.

**Left unbuilt** — named so nobody assumes it exists:
- Pre-swap ctx check (P1-14, I2), S1/S2 classification, EDQUOT routing — SCENARIO-14. `Replace` does not check ctx before its rename.
- Stale `quarry.duckdb.wal` removal, `.partial` leftover sweep — SCENARIO-20.
- `transfers`, `import_runs` tables; `not_imported` count — 01c, 08, 01c/S21. `Counts.Transfers` is always 0 in 01a.
- Reconcile-record reads, S4 reason 5, statement required-NULL reasons — 01b.
- `OpenBundle` / `ExtraSchemaBundle` typed accounts + `Z_PRIMARYKEY` rows, `snapshot.Importer`, wiring, sequencing ADR — 01d.
- Until 01b lands, 01d swaps in an unchecked store.

**Traps** — things that look right and are not:
- Builder bound `0` for "no ref"; real v9 has NULL. After Step 3, production code must not special-case 0.
- `WithEntity` overrides are the only proof entity numbers are resolved, not hard-coded.
- SQLite NUMERIC affinity stores `'9999999999999999.99'` as INTEGER `10000000000000000` — the bound fixtures are integers, not 2-dp strings.
- `ZDELETIONCOUNT` may be NULL — treat as 0, not as deleted.

**Inherited notes closed:** the `cmd/quarry/run_test.go` / `internal/snapshot/sync_faults_test.go` split debt is done (`run_*_test.go`, `sync_*_faults_test.go` exist) — drop it from STATE.md `## Open debts`. The `--json` test debt moves to SCENARIO-02.
