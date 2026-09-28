# phase1-import-store — current state

Scenarios complete: SCENARIO-01a (+ folded 04, 05), SCENARIO-01d, SCENARIO-01b (+ folded 06),
SCENARIO-09 (+ folded 11), SCENARIO-01c (+ folded 07, 12, 21), SCENARIO-08 (+ folded 19). Last updated by SCENARIO-08.

## Binding decisions
- Store ADR: `docs/adr/001-shared-store-package.md`. `internal/store` = row types only, no driver; `internal/store/duckstore` = DDL + builder + atomic swap. (SCENARIO-01a)
- `importer.Store` is one method: `Replace(ctx, store.Rows) (string, error)`. `Import(ctx, store.SnapshotRef) (store.Result, error)` / `snapshot.Importer` take `SnapshotRef{Path, SHA256, SchemaFingerprint}`; `SyncAndImport` fills it from `manifest.Snapshot.Path/SHA256` + `manifest.Schema.Fingerprint`. 03's `--from` must fill SHA256 with the F5-verified hash and the **current-reference** fingerprint. (SCENARIO-01a, ref SCENARIO-08)
- Sequencing ADR: `docs/adr/002-sync-sequences-import-in-snapshot.md`. `SyncAndImport` branches on `errors.Is(err, store.ErrValidationFailed)`: `Outcome.Store` = unbuilt result (`Path` set from `s.storePath` on this copy only), `Outcome.StoreExisted`, V1 refusal via `validationFailedRefusal`. Other import errors keep `Store` nil → S3 frame. `StdoutWriteRefusal` is O1 while `Store == nil`, O1b once set. (SCENARIO-01d/01b/09)
- **Warnings**: `(snapshot.Outcome).Warnings()` = `Manifest.Warnings` then W2 (no `quarry: warning: ` prefix, `%d` count, singular at 1) only when `Store != nil && Store.Built` and one-sided legs exist — no W2 on V1 (R2). `cli/sync.go` prints `Warnings()` on every path, so W2 also prints on stderr under `--json`. W2 is never appended to `Manifest.Warnings` (P1-11: manifest never rewritten); 02 serializes `Warnings()` into `warnings[]`. Mutation-proven: W2-before-W1 reddens `Test_outcome_warnings_put_w1_before_w2`; dropping `!Built` reddens `Test_outcome_warnings_omit_w2_when_the_store_was_not_built`. (SCENARIO-08)
- Validation: `store.Result{Built, Counts, Validation, NotImported}`; `Validation.Failed()` = balances or splits only — transfers never fail a build. `Counts`/`Validation`/`NotImported` are populated on the V1 return too. `store.ErrValidationFailed` returned unwrapped. (SCENARIO-01b)
- **Core sort order, sorted once** in `internal/importer/validate.go`, never re-sorted by renderers or `--json`: Balances by account name then account source id (`byNameThenSourceID`); Splits `Mismatched` and one-sided legs through ONE comparator `rowIndex.compareTransactions` (date, account name, account source id, txn source id); one-sided adds split source id last (`describeOneSided`). All source-id ties `cmp.Compare` on int64 — never `strings.Compare` on ids. Mutation-proven: `strings.Compare(a.ID, b.ID)` on the split key reddens `Test_import_orders_one_sided_transfers_by_date_account_and_transaction`; dropping the key reddens `Test_describeOneSided_orders_legs_of_one_transaction_by_split_source_id`. (SCENARIO-01b/09/08)
- `OneSidedTransfer{ID, SourceID(split), Date, Account, Currency, Closed, Active, Payee, Amount(split's own), OtherAccount, OtherAccountID}`; `SplitMismatch` gained `Closed, Active`. `SourceID`/`Closed`/`Active` are display-only — **02 must not emit them in JSON** (spec Edge row "account label"). `OtherAccount == nil` ↔ numeric link ↔ `other account: unknown`; json `other_account: null`. (SCENARIO-08)
- **V1 / success stdout** (`internal/cli/render.go`): `renderStore` and `renderStoreFailure` share `rowsPhrase`/`balancesPhrase`/`splitsPhrase`/`balancesExtrasPhrase` and end with `writeTransfers` (Transfers line + `?` rows via `oneSidedRows`) on both paths (R1). `accountLabel(name, cur, closed, active)` = `Name (CUR[, closed][, inactive])` on every `!`/`?` row, real flags (R4). `payeeLabel` = `(no payee)` fallback; `formatMoney` for every amount; columns padded among one block's rows only, amounts right-aligned. V1 count form `X of Y <noun>` (`xOfYPhrase`). `--json` on V1 still returns before writing (interim until 02). (SCENARIO-09/01c/08)
- **Transfers** (`internal/importer/transfers.go` `pairTransfers`): numeric `ZTRANSFER` links to counterpart `ZQUICKENID` among imported splits (dup quicken ids: lowest source id); anything else is an account name. Walk + from/to pick via `bySourceID`; each split in ≤1 row; `xfer-<from source id>`; one-sided rows `to_split_id` NULL, `cross_currency` false; `transfer_account_id` per pair / name match (lowest same-named source id). No amount check on pairs. Mutation-proven: `Failed()` counting one-sided reddens `Test_import_builds_the_store_with_a_one_sided_transfer`; `bySourceID` via `strings.Compare` reddens `Test_import_stores_each_split_in_at_most_one_transfer`. (SCENARIO-01c)
- **import_runs**: one row per build, `id` = `importRunID` (1; Phase 2 appends max+1), `started_at` = `Import` entry, `finished_at` stamped before `Replace` (excludes write/checkpoint/swap), both UTC `TIMESTAMP`; `snapshot_path`/`snapshot_sha256`/`schema_fingerprint`; `<table>_rows` BIGINT for each of the 8 `Counts` tables; `balances_checked`, `balances_mismatched`, `splits_mismatched` (always 0 in Phase 1), `transfers_one_sided`, `investment_transactions_not_imported`. `Counts` does not count `import_runs`. Appended last in `duckstore.build`. V1 never calls `Replace`, so records no run. (SCENARIO-08)
- `InvestmentTransaction` is an **optional** entity (`investmentEntity`, absent → count 0). `surveyTransactions`' `hasInvestment` guard is mutation-proven by `Test_import_counts_no_investment_transactions_when_an_imported_one_shares_the_absent_entitys_zero`. (SCENARIO-01c)
- Money is int64 cents in `store.Rows`, parsed via `typeof(col)` + `CAST(col AS TEXT)`. IDs `<prefix>-<Z_PK>`. `*importer.UnmappableError{Reason}` carries ruled text; S4 offenders accumulate (`s4ClassOrder`). P1-5d: deleted ref → dropped silently; nonexistent ref → NULL. Account/category booleans via `COALESCE(col, 0)`. (SCENARIO-01a)
- `v9fixture.Builder`: real SQL NULL for zero refs/empty strings; `Z_PK` by call order per table; `WithoutEntity(name)`. `(*duckdb.DB).Create`/`CheckpointClose`/`AppendRows`/`Decimal`/`QueryRows`. (SCENARIO-01a/01d, PREP-c)

## Left unbuilt
- Pre-swap ctx check (I2), S1/S2 classification, EDQUOT routing — SCENARIO-14. `duckstore.Replace` does not check ctx before its rename.
- Stale `quarry.duckdb.wal` removal, `.partial` leftover sweep — SCENARIO-20.
- `--json` (`store.transfers {paired, cross_currency, one_sided[]}`, `warnings[]` from `Outcome.Warnings()`, `not_imported`, `rows.transfers`, V1 `"built": false` with `mismatched[]` in core order; `SourceID`/`Closed`/`Active` omitted from `one_sided[]`/`splits.mismatched[]`; nil `OneSided` must encode as `[]`) — SCENARIO-02.
- `--from` flag + Example paragraph, `snapshot.Server` method for `--from` (fills `SnapshotRef`) — SCENARIO-03.
- `import_runs` history across rebuilds — Phase 2.

## Traps
- `WithEntity`/`WithoutEntity` overrides are the only proof entity numbers come from `Z_PRIMARYKEY`.
- SQLite NUMERIC affinity stores a whole-valued decimal string as INTEGER — branch on `typeof()`.
- `mattn/go-sqlite3` auto-converts TIMESTAMP-declared columns unless the query casts.
- `ZDELETIONCOUNT` may be NULL — treat as 0. `CAST(col AS TEXT)` of NULL scans as NULL — use `sql.NullString`.
- DuckDB's `InstanceCache` refuses a second connection to one path with a different config while the first is open.
- `causeText` prints the innermost error — `validationFailedRefusal` builds its own message.
- `uncovered-diff.py` is blind to untracked files — `git add` first.
- O1b and S3 tell the user to run `--from` before SCENARIO-03 exists — ruled copy; do not "fix" early.
- `cmd/quarry`'s test binary links DuckDB; linux-small CI has OOMed linking it (`devenv.nix`).
- `v9fixture` writes `ZQUICKENID` raw (unset = 0, shared): give every transfer leg an explicit distinct non-zero `QuickenID`. A transaction with zero entries is a split mismatch (V1) — fillers need entries summing to their amount.
- An account literally named with digits reads as a numeric link — accepted. (SCENARIO-01c)
- `fakeStore` enforces no primary keys — pin the exact `Rows.Transfers` slice. `fake.Rows` now carries `ImportRuns` with wall-clock times: never assert whole-`Rows` equality. (SCENARIO-01c/08)
- `Z_PK` by call order means "higher id" = "inserted later": a source-id tie-break test needs a second, differently-ordered key (name, date) or a same-name/same-date pair.
- The one-sided comparator's split-source-id key is invisible through `Import`: `pairTransfers` emits legs in split order and `slices.SortFunc` is insertion-sort-stable at ≤12 elements, but pdqsort is unstable above that (the real file has 29 legs), so the key is load-bearing. Only the white-box `Test_describeOneSided_orders_legs_of_one_transaction_by_split_source_id` (legs fed out of order) reddens on dropping it. (SCENARIO-08)
- `internal/platform/duckdb`'s ctx-cancel-mid-iteration test flakes under full-suite load — see Open debts.

## Open debts
- **Interim W2 (narrowed)**: stdout `?` rows and the stderr W2 line ship; `warnings[]` and `transfers.one_sided[]` in `--json` do not. **Final-gate BLOCKER if SCENARIO-02 does not land them.** Owned by SCENARIO-02.
- `--json` stdout gains top-level `store` key (P1-11); `Test_run_prints_the_manifest_as_json_with_the_json_flag` must become "stdout minus `store` equals the manifest" — SCENARIO-02.
- `duckstore.Replace`'s `CheckpointClose`-failure branch and `build`'s schema-exec failure are declared unreachable; SCENARIO-14's fault work may want a seam for them.
- `internal/platform/duckdb`'s `Test_query_rows_fails_when_the_context_is_cancelled_mid_iteration` flakes under full-suite CPU load — pre-existing — unowned, for the final gate to rule on.
- `internal/snapshot/sync_faults_helpers_test.go` is not gofmt-clean at `<start>` 5fcce5b (pre-existing; `gofmt -l` lists it) — unowned, cheap fold for any fix pass.
