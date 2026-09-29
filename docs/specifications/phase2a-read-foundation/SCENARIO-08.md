---
id: SCENARIO-08
status: open
---

# SCENARIO-08: sql prints a query result as a table

Size verdict: OWNS A RUN (one behaviour, `quarry sql <query>` table output; L per sizing: platform text formatter + driver-free result shape + limit fetch + renderer + command).
Cadence: code-first (no write-safety guard, atomic adapter or bug fix touched; S12 owns the lockdown guard)
Acceptance test: `cmd/quarry/run_sql_test.go` `Test_run_sql_prints_the_query_result_as_a_table`
Narrow loop: `go test ./internal/platform/duckdb/ ./internal/store/... ./internal/report/ ./internal/cli/ -run 'QueryTable|ValueText|Query|Numeric|SQL' && go test ./cmd/quarry/ -run 'sql'`
Mutation checks: fetch cap `limit+1` in `(*report.Server).Query` → `Test_query_fetches_one_row_more_than_the_limit`; truncation `len(rows) > limit` flipped to `>=` → `Test_query_truncates_only_past_the_limit` (row `rows = limit → false` reddens; `limit+1` cannot); anchored numeric predicate → `Test_query_column_numeric` (row `INTEGER[]`)

## Implementation Plan

Contract. `quarry sql [--limit n] <query>`: query passed to DuckDB verbatim (no rewrite, no wrapper). Stdout: header of column names, then rows; columns joined by two spaces; numeric columns (header included) right-aligned, others left; no trailing spaces; cells and header names escaped (`\n` `\t` `\r`) BEFORE width is measured (rune count); NULL → `NULL`, aligned with its column; zero rows → header only, exit 0. Stderr empty on success. Interim until S12/S15/S18: any open or query fault → exit 1, `quarry: run query: <driver error>` (DuckDB's multi-line `LINE 1:`/caret text included; empty or whitespace query → `quarry: run query: empty query`); 0 or 2+ args → exit 2 with cobra's `accepts 1 arg(s)…; Run 'quarry sync --help' for usage.`. Use/Short/Long/Example and `--limit` help verbatim from spec *Surface & Copy → sql*.

"Raw DuckDB text" = the Go rendering equals `CAST(expr AS VARCHAR)` on the same connection, proved per type by the oracle test (Step 4). Cells carry BOTH `Text` and a typed driver-free `Native` (nil | bool | int64 | uint64 | float64 | time.Time | string=Text), filled in one pass, so S09 encodes JSON with no re-query.

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_sql_test.go` (new) `Test_run_sql_prints_the_query_result_as_a_table` — `syncAccountsFixture` (`run_accounts_test.go:119`); query over `v_account_balances ORDER BY source_id` selecting `name`, `balance` (pins `0.00` trailing zeros, `12345.67` ungrouped, RRSP NULL right-aligned), one integer column, one literal built with `chr(10)`, `chr(9)`, `chr(13)`; exact stdout, stderr empty, exit 0
- [ ] Step 2: `internal/cli/sql.go` (new) `newSQLCommand(newReport)` stub — ruled Use, `cobra.ExactArgs(1)`, RunE returns nil; registered in `internal/cli/root.go:27-29`. Acceptance must then fail at the stdout equality, not the exit code

### Build
- [ ] Step 3: `internal/platform/duckdb/table.go` (new) `(*DB).QueryTable(ctx, query, maxRows)` + `Table{Columns, Rows}`, `Column{Name, Type}` (Type = `DatabaseTypeName`), `Value{Null, Text, Native}` — scans into `any`, stops after `maxRows` rows (0 = all), returns the driver error unwrapped (like `Exec`, `duckdb.go:90-92`; never `QueryRows`' query-embedding wrap at `duckdb.go:144`). `table_test.go`: `Test_query_table_returns_columns_and_rows`, `Test_query_table_stops_at_max_rows` (available rows = maxRows and maxRows+1; maxRows 0), fault `Test_query_table_returns_the_driver_error` (`errors.As` to `*duckdb.Error` holds), rows.Err fault via a query failing past the first vector (e.g. `error()` beyond row 2048 of `range`) — if the probe shows it surfaces at Query, fold into one test and declare `rows.Err` per proof.md. `ColumnTypes` and `Scan` into `*any` → `// unreachable:` (rows open after a successful Query; `*any` accepts every driver value)
- [ ] Step 4: `internal/platform/duckdb/text.go` (new) value → `(Text, Native)` — `text_test.go` `Test_value_text_matches_duckdb_cast` table: expression → Text equals the same connection's `CAST(... AS VARCHAR)`, Native asserted per row. Rows: DECIMAL `0.00`, `-0.05`, `12.50`, `DECIMAL(38,2)` (`SUM` result), `DECIMAL(4,0)`; HUGEINT (`SUM(BIGINT)`), UHUGEINT; each int width, UBIGINT; DOUBLE `1.0`, `0.1`, `123456789.0`, `1e15`, `1e16`, `1e-05`, `nan`, `-inf`, `-0.0`, `AVG` result; FLOAT `1.0`; BOOLEAN; VARCHAR, ENUM, BIT; DATE; TIME `03:04:05.5`; TIMESTAMP `.12`, `.000001`, `_S`/`_MS`/`_NS`; DATE/TIMESTAMP `±infinity` (driver returns int-max/min sentinel times — map back exactly); TIMESTAMPTZ; UUID vs BLOB (`\x00ab`); INTERVAL; LIST/ARRAY/MAP/UNION holding only ints and plain ASCII strings; NULL. May stay behind: a row that will not match after a fair attempt moves to the declared-divergence set (Handoff) with a row asserting its fallback text, and is reported — do not chase it
- [ ] Step 5: `internal/store/store.go:300-304` (after `AccountList`) `QueryResult{Columns []QueryColumn; Rows [][]QueryValue}`, `QueryColumn{Name, Type}` + `Numeric()`, `QueryValue{Null, Text, Native}` — `store_test.go` `Test_query_column_numeric`: TINYINT…BIGINT, HUGEINT, U-variants, FLOAT, DOUBLE, `DECIMAL(18,2)` true; controls `INTEGER[]`, `INTEGER[2]`, `DECIMAL(4,1)[]`, `VARCHAR`, `STRUCT("a" INTEGER)` false
- [ ] Step 6: `internal/store/duckstore/duckstore.go:62-68` `ReadDB` += `QueryTable`; `query.go` (new) `(*Store).Query(ctx, query, maxRows)` via `openRead` (`duckstore.go:147-151`), deferred Close, maps `duckdb.Table` → `store.QueryResult`, wraps once `run query: %w` on open and on query fault — `query_test.go`: reads a built store (reuse the accounts/status test builders), fault tests for open (`WithOpenReadOnly`) and query, Close on success and on query fault (`spyReadDB`, `status_test.go:150-160`, embeds `ReadDB` so it needs no new method)
- [ ] Step 7: `internal/report/store.go:11-16` `Store.Query`; `fakeStore` (`status_test.go:15-25`) records `maxRows`; `internal/report/query.go` (new) `(*Server).Query(ctx, query, limit)` → `QueryResult{store.QueryResult; Truncated bool}`: fetch `limit+1` when `limit > 0` else 0, trim to `limit`, store error unchanged — `query_test.go`: `Test_query_fetches_one_row_more_than_the_limit` (recorded maxRows `limit+1`; `0` for limit 0), `Test_query_truncates_only_past_the_limit` (rows = limit → false; limit+1 → true, `limit` rows kept; limit 0 → all, false), negative limit behaves as 0, `math.MaxInt` limit does not overflow (fetches all), `Test_query_returns_the_store_fault`
- [ ] Step 8: `internal/cli/sql.go` `newSQLCommand` body — Short/Long/Example verbatim, `--limit` default 500 with the ruled help; factory error, store error and stdout write error → `runtimeError`. `render_sql.go` (new) `renderSQLTable(store.QueryResult)` reusing `padRight`/`padLeft` (`render_accounts.go:68-76`) — `render_sql_internal_test.go`: escaping widens the column, header escaped, NULL in numeric column right-aligned, left-aligned last column has no trailing spaces, zero rows header only, multibyte width; `sql_test.go` (fake: `fakeReportStore`, `internal/cli/accounts_test.go:24-35`, embeds `report.Store` — add `Query`): help shows `--limit n` … `(default 500)`, `--limit` value reaches `Server.Query`, factory/store/stdout faults
- [ ] Step 9: `cmd/quarry/run_sql_test.go` — `Test_run_sql_prints_only_the_header_for_zero_rows`, `Test_run_sql_prints_at_most_limit_rows` (stdout only; do NOT assert stderr — S09 adds the warning here), `Test_run_sql_reports_a_query_error` (interim text, exit 1, stdout empty), `Test_run_sql_needs_exactly_one_argument` (0 and 2 args: exit 2, stdout empty; text NOT asserted — S18 rules it). Keep cmd tests few (DuckDB-linked binary). No wiring change: `NewReport` and the `report.Store` guard (`cmd/quarry/run.go:25-27`) already cover it

### Sweep
- [ ] Step 10: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on every new exported symbol; `duckdb` package doc (`doc.go`) gains QueryTable's text rendering (it still promises money never passes through float64 — keep it true)

### Verify
- [ ] Step 11: full verification + `spec-check.py phase2a-read-foundation` → tick SCENARIO-08 with its acceptance test

## Handoff

**Binding decisions:**
- `store.QueryValue{Null, Text, Native}` — S09's JSON reads `Native` (ints/DOUBLE/BOOLEAN native, time.Time for DATE/TIMESTAMP) and `Text` (DECIMAL/HUGEINT/else); it must not re-query or re-format.
- Limit rule lives in `(*report.Server).Query` (fetch `limit+1`, `Truncated`); S09 renders the warning and `truncated`/`limit` from it and never touches the query path. `limit <= 0` = every row until U7 (S18) rejects negatives in cli.
- Query is never rewritten or wrapped: Q1/Q3 depend on DuckDB's own errors for `INSERT`/`SET`. `(*duckdb.DB).QueryTable` returns the driver error unwrapped; duckstore wraps once (`run query: %w`) so S12's classifier reaches `*duckdb.Error` via `errors.As`.
- Numeric = `QueryColumn.Numeric()`, anchored on the whole type name.

**Left unbuilt:** `sql --json` (S09 — `--json` is a root persistent flag, so `sql --json` prints the table until then); truncation warning (S09/S10); lockdown + Q1–Q4 (S12); R1–R3 (S15); U5/U6 text, `-` stdin (`sql -` runs the query `-` → parser error, exit 1), U7 (S18); O2 (S18).

**Traps:**
- `duckdb.Decimal.String()` drops trailing zeros (`12.50` → `12.5`); `Float64()` breaks the no-float money rule. Format from `Value`/`Scale`.
- DuckDB DOUBLE text ≠ Go `'g'`: fixed with `.0` below 1e16 (`1.0`, `123456789.0`, `1000000000000000.0`), exponent at ≥1e16 and <1e-4 (`1e+16`, `1e-05`), `nan`/`inf`/`-inf`/`-0.0`.
- UUID and BLOB both scan as `[]byte`; dispatch on the column type. STRUCT scans as `map[string]any`: declared field order is lost.
- Declared divergences from DuckDB text (ruled): STRUCT prints DuckDB-shaped `{'a': 1}` in the field order declared by the column type name (parse it; quoted identifiers, `""` escapes, nesting), falling back to keys sorted only when the type name cannot be parsed — one test row pins the fallback. An unknown Go type fails the query with Q5 (spec), never `%v` (plus any oracle row Step 4 moved here). Nothing ever prints Go struct syntax (`{18 2 125}`, `{3 0 0}`).
- Until S12 the READ_ONLY-only DSN lets `sql` run `COPY … TO '<file>'` (writes outside the store, probed) and `read_csv('<file>')`; `ATTACH` of a new file fails. No S08 test may rely on external access — S12 turns it off.
- S09: `encoding/json` refuses NaN/Inf — `marshalDocument`'s `// unreachable:` line becomes reachable; JSON form of TIMESTAMPTZ and `TIMESTAMP_*` has no ruling. The Long already describes `-` and the warning; do not edit it.
