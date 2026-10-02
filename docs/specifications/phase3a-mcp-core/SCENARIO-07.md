---
id: SCENARIO-07
status: open
---

# SCENARIO-07: describe_schema describes the store

Cadence: code-first (no write-safety guard, no atomic adapter touched)
Acceptance test: `cmd/quarry/run_mcp_describe_test.go` `Test_run_mcp_describe_schema_describes_the_store`
Acceptance test (SCENARIO-08, folded): `cmd/quarry/run_mcp_describe_test.go` `Test_run_mcp_describe_schema_lists_the_first_500_categories_and_says_so`
Narrow loop: `go test ./internal/store/duckstore/ -run 'Schema|reads' && go test ./internal/report/... ./internal/mcp/ -run 'Schema|Describe|unbuilt|Conventions' && go test ./cmd/quarry/ -run 'MCP|mcp|SQL'`
Mutation checks: cap comparison in `(*report.Server).DescribeSchema` (`>` → `>=`) → `Test_describe_schema_keeps_500_and_cuts_501`; sort before the cut (cut first) → same test (asserts *which* entry is dropped)
Runs: A (1-2) | B1 (3-5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (report; duckstore adapter, mcp delivery)

Surface delivered (verbatim, spec §2.6): document keys `conventions, relations, accounts, categories, dates, warnings` in that order; account `{id,name,type,currency,closed}`; category `{id,full_path,kind,hidden}`; relation `{name,kind,columns:[{name,type}]}`, kind `table`|`view`; warning `describe_schema lists the first 500 categories of <n>; query the categories table for the rest` (and `accounts`/`accounts table`), `<n>` via `humanize.Thousands`. Refusals: store refusal lines verbatim (STATE). Nothing mentions masking.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_mcp_describe_test.go` (new) `Test_run_mcp_describe_schema_describes_the_store` + `Test_run_mcp_describe_schema_lists_the_first_500_categories_and_says_so` — `replaceStore` (`run_helpers_test.go:76`) fixture: closed account, USD account, two names differing only in case, hidden category, grandchild category, transactions with distinct first/last days; peer via `startMCP` (`run_mcp_test.go:144`) under `mcpTestDeadline`. Oracles, none circular: relation names/kinds vs `quarry sql --json` over `information_schema.tables`, each relation's columns vs `sql --json "SELECT * FROM <name> LIMIT 0"` columns; `conventions` is a substring of `quarry sql --help` stdout; accounts/categories/dates vs the fixture's own values; key order on the raw frame (`structuredFrame`), never the SDK-decoded map; stderr empty. S08 test: 501 categories, asserts 500 kept, the dropped `full_path`, the ruled warning
- [x] Step 2: no stubs — the tool answers `this tool is not available yet` today, so both tests go red at `require.False(result.IsError)`; quote that failure

### Build
- [x] Step 3: `internal/store/store.go` `Schema`, `Relation`, `Column` (+ `table`/`view` kind consts) — driver-free, reuses `store.Account`/`store.Category`/`TransactionRange`; `internal/report/store.go:11-27` `Store.Schema(ctx)`; `internal/store/duckstore/schema_read.go` (new) `(*Store).Schema` — ONE `openRead`, introspects `duckdb_tables()`/`duckdb_views()`/`duckdb_columns()` filtered `database_name = current_database() AND schema_name = 'main' AND NOT internal` (precedent `duckstore.go:189`), columns by `column_index`, plus accounts, categories, min/max transaction date; faults wrap with `openFault` as `status.go:70-92`. `internal/report/fakes_test.go:11` `fakeStore` gains `schema` field + `Schema` method. Tests (`schema_read_test.go`): every relation vs an `information_schema` oracle via `openReadOnly` (`fakes_test.go:123`), DDL column order, closed/hidden values, no-transactions → zero dates, empty store → no accounts/categories; add a `Schema` row to `read_faults_test.go:21-40 rowReads` (open/query/scan/close faults); one query-fault row per `QueryRows` call using `spyReadDB.passQueries` (`fakes_test.go:40`)
- [x] Step 4: `internal/report/describe_schema.go` (new) `Schema` value (`store.Schema` + `AccountsTotal`, `CategoriesTotal`) + `(*Server).DescribeSchema(ctx, maxListed int)` — one `s.store.Schema` call; relations sorted tables-then-views then name; accounts in `quarry accounts` order — `lower(name)`, then `name`, then `id` (byte order on the last two; orchestrator ruling); categories by `full_path`; sort, THEN cut to `maxListed` (≤0 keeps all); `readRefusal(ctx, "describe_schema", err)` (`refusal.go:29`). Tests (`describe_schema_test.go`): one row per sort arm (view named before a table; `lower(name)` tier: "alpha" before "Zeta" where byte order says the reverse; case-only pair "Chequing" before "chequing" via the `name` tier; same-name id tiebreak; category depth-2 path), `Test_describe_schema_keeps_500_and_cuts_501` per list with the dropped entry named, maxListed 0 row, refusal rows: `OpenFaultMissing` → `~` line, cancelled ctx → `describe_schema interrupted`, other error unchanged
- [x] Step 5: `internal/report/document/schema.go` (new) `Schema` + `NewSchema(report.Schema, warnings)` — `Conventions: report.SQLConventions`, dates via `nullDate` (`status.go:172`), empty lists `[]`; tests: exact compact-JSON key order, empty store `accounts:[]`/`categories:[]`/both dates null, `Test_sql_conventions_never_mention_masking` (in `internal/report`). `internal/mcp/describe_schema.go` (new) `(*Server).describeSchema(ctx, noInput)` — `s.newReport(ctx, commandName)`, `DescribeSchema(ctx, maxRows)`, warnings accounts-then-categories when total > len; `tools.go:91` `notBuilt[noInput]` → `handler(s.describeSchema)`; drop the describe_schema row at `server_test.go:160`. mcp fake `query_helpers_test.go:33` gains `Schema` + read counter. Tests (`describe_schema_test.go`): one store read per call, a fresh report per call, both warnings and their order, `1,001` grouping, 500 → no warning, factory error passes through, a store `RefusalError` is the one text line + one stderr line

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on new symbols; fold S03 doc-budget MINORs: `internal/mcp/query.go:15-17`, `internal/mcp/result.go:16-18` (handler), `:37-39` (errorLog) to ≤2 lines; `internal/mcp/server.go:44` `WithReport` doc says "tools", not "the query tool"; re-check `internal/report/sql_conventions.go:3-4` now its MCP consumer exists

### Verify
- [ ] Step 7: full verification + `spec-check.py phase3a-mcp-core` → tick SCENARIO-07 and SCENARIO-08 ("delivered by SCENARIO-07") with their acceptance tests; STATE.md rewrite (close the SQLConventions direct-test debt and the S07 comment MINOR)

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `report.Store.Schema` is one open returning `store.Schema`; ordering and the cap live in `report.Server.DescribeSchema`, not the store — a fake store with unsorted rows must still yield the ruled order, and a future CLI `schema` command reuses the same method
- The cap value is passed in (`maxListed`), like `Query`'s limit; mcp passes `maxRows` (500). Overflow copy lives in mcp, naming the tool; `document.NewSchema` takes warnings as given (Rule 7)
- Warning order when both lists overflow: accounts, then categories (document field order)
- Account sort is `quarry accounts` order — `lower(name)`, then `name`, then `id` — so CLI and MCP agree (orchestrator ruling, spec §2.6); overflow `<n>` uses `humanize.Thousands`
- Relations come from `duckdb_tables/views/columns` at call time, filtered to current database, `main`, `NOT internal` — no hand-written list anywhere, tests included

**Left unbuilt** — named so nobody assumes it exists:
- `describe_schema stopped after 30 seconds; try again` and the deadline mapping — S06
- The cross-tool no-store refusal table (`describe_schema` row) — S13; S07 pins one refusal through the handler for coverage only
- `notBuilt` remains for sync_status and data_quality — S09, S11

**Traps** — things that look right and are not:
- `duckdb_views()` without `NOT internal` lists DuckDB's own system views; `duckdb_tables()` without `current_database()` can list attached/temp catalogs
- `cli` fakes (`fakes_test.go:18`, `status_test.go:19`) and mcp `fakeStore` embed `report.Store`: they compile without `Schema` and panic if a test calls it
- `status --json` refuses a store with no import run; a `replaceStore` fixture without `ImportRuns` cannot use it as the dates oracle
- `read_faults_test.go`'s query-fault row fails only the first query; the other `QueryRows` calls need `passQueries` rows or the coverage gate lists them

## Phase report

Run B1 (steps 3-5) done; both acceptance tests green, `golangci-lint run ./...` 0 issues.

Files:
- `internal/store/store.go` (end of file): `RelationTable`/`RelationView`, `Column`, `Relation`, `Schema` (store fills only ID/Name/Type/Currency/Closed on accounts, ID/Name/FullPath/Kind/Hidden on categories).
- `internal/store/duckstore/schema_read.go` (new): `(*Store).Schema`, four queries (relations join, accounts, categories, dates) on one `openRead`. Tests `schema_read_test.go` (oracle = `information_schema`, DDL order, closed/hidden, dates, empty store, query + scan fault per query via `passQueries`); `read_faults_test.go` `rowReads` gained a `Schema` row.
- `internal/report/store.go` `Store.Schema`; `internal/report/describe_schema.go` (new) `Schema`, `(*Server).DescribeSchema`; tests `describe_schema_test.go`; `fakes_test.go` `fakeStore.schema`/`schemaReads`.
- `internal/report/document/schema.go` (new) `Schema`, `NewSchema`; `schema_test.go` (key order, empty store).
- `internal/mcp/describe_schema.go` (new) `(*Server).describeSchema`, `listCutWarning`; `tools.go:91` now `handler(s.describeSchema)`; `server_test.go` describe_schema row dropped from the unbuilt test; `query_helpers_test.go` fakeStore `Schema` + `schemaReads`, `harness.describeSchema`; tests `describe_schema_test.go`.
- `internal/report/sql_conventions_test.go` (new): `Test_sql_conventions_never_mention_masking` plus a phrase pin (closes the SQLConventions direct-test debt).

Mutations (all reverted, diff clean): `maxListed > 0` -> `>= 0` reddened `Test_describe_schema_keeps_every_account_and_category_when_the_cap_is_zero` ("[]" should have 501 item(s), but has 0); cut before sort reddened `Test_describe_schema_keeps_500_and_cuts_501/501_accounts_and_categories_lose_the_last_in_order`; accounts cut `maxListed` -> `maxListed+1` reddened the same 501 row ("should have 500 item(s), but has 501").

Run V must not redo: tests above; remaining plan step 6 (doc-budget MINOR folds: `internal/mcp/query.go:15-17`, `result.go:16-18`/`:37-39`, `server.go:44` WithReport doc, `sql_conventions.go:3-4` comment re-check), step 7 (full verify, spec tick S07 + S08 folded, STATE.md, `status: done`). Lint is already at 0 issues; coverage gate (`uncovered-diff.py`) and `test-stats.py` not yet run.
