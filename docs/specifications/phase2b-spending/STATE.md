# phase2b-spending — current state

Scenarios complete: SCENARIO-01..04 (02, 03, 04 folded into 01). Last updated by SCENARIO-01.

## Binding decisions
- Store columns (all appended LAST in their table; the build Appender is positional): `accounts.in_reports BOOLEAN NOT NULL` (`store.Account.NotInReports`, writer inverts, zero value = in reports), `transactions.excluded_from_reports BOOLEAN NOT NULL` (`store.Transaction.ExcludedFromReports`), `transactions.posted_date DATE` nullable (`store.Transaction.PostedDate *time.Time`, set whenever Quicken has a posted date, even equal to `date`). (SCENARIO-01)
- `transactions.date` = UTC day of `ZENTEREDDATE`, else `ZPOSTEDDATE` (Quicken's register date); importer ORDER BY uses the same COALESCE. Balance validation stays date-free. Refusal subjects and the split-mismatch listing now show register dates (copy unchanged). (SCENARIO-01)
- Quicken flags read as `COALESCE(col, default) <> 0` (NULL `ZUSEDINREPORTS` -> in reports, NULL `ZEXCLUDEFROMREPORTS` -> not excluded; any non-zero is on). (SCENARIO-01)
- Uncategorized = category with kind `system` AND full_path exactly `Uncategorized` (`importer.uncategorizedPath`); its splits store `category_id NULL`, its category row stays in `categories`. Views key on `category_id IS NULL`, never on the name. (SCENARIO-01)
- `duckstore.FormatVersion = 3` is the ONLY 2b bump (format 3 unshipped): later 2b scenarios add views/columns without bumping. S05's refusal compares against the symbol, never a literal; an old store must get R2 (with `sync --from <id>` fix), not an R3 Catalog Error. (SCENARIO-01, 2a S15)
- Rules live in duckstore-owned store-DDL views (transfer exclusion, sign, split allocation); front ends add parameters only. Every new relation joins `storeRelations()` (`internal/store/duckstore/query_test.go`, literal list) and `minimalRows` (`duckstore_test.go`). (2a REVIEW-01)
- Read-command shape: `openReport` -> store call -> `renderResult` -> `emit` (`internal/cli/output.go`); `report.Store` port gets one method per read, implemented by `*duckstore.Store`, `fakeStore` (`internal/report/fakes_test.go`), `fakeReportStore` (`internal/cli/fakes_test.go`); `(*Server).readRefusal` for I1/R1-R3; `noArgs` for no-positional commands; U9 via `ExecuteC`; H1 via `resolveHome(command)`; `warnings` always `[]`; absolute paths in `--json`; `marshalDocument` is the one encoder. Reads open through `openRead`, format check via `duckdb_columns()`; a session setting goes in the read DSN, never a post-open `SET`. (2a S02, S07, S15, S18)
- Native currency only; group per currency, never sum CAD+USD. Cents: SUM(DECIMAL(18,2)) is DECIMAL(38,2), cast to BIGINT cents in SQL. (2a S04)
- Investment accounts have NULL balance via the one predicate `store.IsInvestmentAccount`; `v_account_balances` filters `date <= current_date` in the JOIN. (2a S04)
- Warning prefix (P2b-14): every `warnings[]`-bearing stderr line uses `quarry: warning: `; 2a's accounts all-closed note changes accordingly (its `warnings[]` text unchanged). (spec)

## Left unbuilt
- `in_reports` in `(*duckstore.Store).Accounts` / `v_account_balances`, accounts Status + `--json` `in_reports` — run 2 (SCENARIO-26).
- `v_cash_flow`, `v_spending`, their `storeRelations()` entries and `minimalRows` rows — run 3 (SCENARIO-06).

## Traps
- A column added mid-`CREATE TABLE` while the writer appends at the end silently writes values into the neighbouring column. (SCENARIO-01)
- A dev store synced before run 3 lacks the 2b views; re-sync after run 3. (SCENARIO-01)
- Existing importer tests set only `PostedDate`; with entered NULL they still date by posted, so only exact-struct asserts on `Transactions` need a `PostedDate` bump. `store.Account`/`AccountBalance` test literals rely on `NotInReports` zero = in reports. (SCENARIO-01)
- Plan `-run` patterns are case-sensitive: importer tests are `Test_import_*`, cmd ones `Test_run_*`; use lowercase words. (SCENARIO-01)
- Importer dates are UTC calendar days but `current_date` is process-local: date-boundary tests belong at duckstore level with `store.Transaction.Date` at UTC midnight; cmd fixtures use dates a year ahead. (2a S04)
- `uncovered-diff.py` is blind to untracked files: `git add` first. Q-sentinels wrap `%w: %w`, so `errors.Unwrap` returns nil: use `errors.As`/`Is`. `v9fixture` writes `ZQUICKENID` raw: distinct non-zero per transfer leg. (Phase 1, 2a)
- `fakeReportStore.Query` ignores `maxRows`; a fixture writing at the store path must `CheckpointClose` before any read (DuckDB `InstanceCache`); a held reader shares the old inode. (2a S09, S02, REVIEW-01)

## Open debts
- MINOR (01 checkpoint): `internal/store/duckstore/duckstore.go:24` — nothing pins `FormatVersion` to literal 3 (all tests compare to the constant). Owner: SCENARIO-09's plan (delivers folded SCENARIO-05) builds a store with literal `format_version = 2` for the R2 test, or asserts `FormatVersion == 3` once.
- Gate "matches Quicken reports over 2 years" is carried by 2b (spec) — owned by the spend scenarios.
- 2a debts still open and unowned - die unless re-opened: `sql.go` Long blank lines/wrap NIT (fold into the next sql.go edit), HOME with trailing slash prints absolute paths, `run_status_json_test.go:1` header, `balancesPhrase` three bare ints, duckstore fault-test copy-paste (`docs/specifications/phase2a-read-foundation/STATE.md`).
- Snapshots accumulate (~200 MB each) until 2c.
