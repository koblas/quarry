# phase2a-read-foundation — current state

Scenarios complete: SCENARIO-01. Last updated by SCENARIO-01.

## Binding decisions
- `store_info(format_version INTEGER, quarry_version VARCHAR, built_at TIMESTAMP)`, one row, appended LAST in `duckstore.build` (after `import_runs`): a store carrying it is complete. `Counts` does not count it. `duckstore.FormatVersion = 2` is the only definition; SCENARIO-15/16's format check compares against that symbol, never a literal. (SCENARIO-01)
- `built_at` = `Replace`'s own `time.Now().UTC()` just before `build`; always `>= import_runs.finished_at` (Phase 1 stamps that before `Replace`). `status` shows `built_at`, never `finished_at`. DuckDB `TIMESTAMP` is microsecond UTC: assert `>=`, never `>`. (SCENARIO-01)
- `quarry_version`: `duckstore.WithQuarryVersion(v)` (empty keeps default `(devel)`, never stored empty), wired in `cmd/quarry/run.go` `newServerFactory` via `buildVersion(debug.ReadBuildInfo())`, prepended ahead of `storeOpts`. The `(devel)` default lives once, in duckstore. (SCENARIO-01)
- `import_runs` gained nullable `snapshot_taken_at TIMESTAMP`, `source_path`, `balances_never_reconciled`, `investment_accounts`, `transfers_paired`, `transfers_cross_currency`; a NULL `snapshot_taken_at`/`source_path` is legal (unparseable manifest `taken_at` -> zero -> NULL; zero `Source` -> NULL), so the read side (`status`) must render NULL, not assume non-NULL. `store.SnapshotRef{TakenAt (UTC), Source}` are filled only in `snapshot.importVerified` from the manifest (recorded values under `--from`). `Source` is stored verbatim. (SCENARIO-01)
- Transfers invariant `transfers_rows = paired + one_sided` is `importer.checkTransferTotals`, returned as an error (not a panic: a Go panic exits 2 = usage code) that flows `Import` -> `importFailureRefusal` default arm (S3); the call site carries `// unreachable:`, the function is unit-tested. (SCENARIO-01)
- Phase 1 seams still binding: one `duckstore.Store` wired as `importer.Store` and `snapshot.StoreProbe`; `duckstore.DB` port + `WithCreate` and `runWith(ctx, args, stdout, stderr, cli.ServerFactory)` / `newServerFactory(storeOpts ...duckstore.Option)` are the fault seams (no package vars); `internal/store` holds driver-free exchanged values only (`docs/adr/001-shared-store-package.md`); `Replace` swaps atomically and never opens the final path, so a concurrent reader sees the old inode. (Phase 1)
- Phase 1 output helpers reused by `status`: `rowsPhrase`/`balancesPhrase`/`splitsPhrase`/`writeTransfers` in `internal/cli/render.go`, `formatMoney`, `internal/platform/humanize` (all human counts), `homepath.Abbreviate`; `--json` money via `jsonMoney` (2 decimals, no grouping). (Phase 1)

## Left unbuilt
- Read side: `duckstore` open/read port, `internal/report`, `v_account_balances` view, read lockdown in `duckdb.OpenReadOnly`, R1-R3/Q1-Q4 refusals, `status`/`accounts`/`sql` commands, usage validators U5-U9 - SCENARIOs 02+.
- `import_runs` history across rebuilds (`importRunID` stays 1 per build) - Phase 2c.

## Traps
- The test binary's `debug.ReadBuildInfo().Main.Version` is `""` or `(devel)` either way, so no `cmd/quarry` test can tell "wired" from "not wired"; only the duckstore option test and `Test_buildVersion_reads_the_main_module_version` carry the proof. Do not claim a wiring mutation. (SCENARIO-01)
- DuckDB's `InstanceCache` refuses a second differently-configured open of one path in-process while the first is live: fixtures writing at the final path must finish (sync's connection closed) before `OpenReadOnly`. (Phase 1, SCENARIO-01)
- `fakeStore` (importer tests) captures `Rows` by value with wall-clock times: assert `ImportRun` field-wise, never whole-`Rows` equality. (Phase 1, SCENARIO-01)
- `v9fixture` writes `ZQUICKENID` raw: give every transfer leg an explicit distinct non-zero `QuickenID`; a paired cross-currency transfer is two legs in accounts of different `Currency` with mutual numeric `Transfer` links (proven by the acceptance fixture). (Phase 1, SCENARIO-01)
- `uncovered-diff.py` is blind to untracked files: `git add` before running it. `cmd/quarry`'s test binary links DuckDB (linux-small CI OOM history). (Phase 1)
- `internal/platform/duckdb`'s mid-iteration ctx-cancel test flakes under full-suite load. (Phase 1)

## Open debts
- Snapshots accumulate (~200 MB each) until 2c - known gap, owned by Phase 2c.
- Phase 1 doc-budget MINORs, TOCTOU on `--from`, Compose-method refactors and the other Phase 1 debts (`docs/specifications/phase1-import-store/STATE.md`) - unowned - die unless re-opened.
- Phase 2 copy candidate: V1 stderr tail `fix the account in Quicken and run quarry sync` reads off when only splits fail - unowned until findings copy (2d).
