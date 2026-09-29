# Review Report — REVIEW-01

### Target
Feature branch `854069b..HEAD` (phase2a-read-foundation), all changed Go files.

Coverage gate: `uncovered-diff: 0 uncovered added line(s) in 0 run(s) since 854069b; 7 declared unreachable` (all 7 judged to hold by correctness-reviewer; `duckdb/table.go:58,75` reachable only on a ctx-cancel race, classified Q4 correctly). test-stats TOTAL 572 (+174).

### Triggered reviewers
- arch-reviewer, correctness-reviewer, refactor-advisor: `cmd/**/*.go`, `internal/**/*.go`
- test-reviewer: `**/*_test.go`

### Skipped reviewers
- api-reviewer: no HTTP surface
- pipeline-reviewer: no `.claude/**` change

### BLOCKER
none

### MAJOR
1. correctness-reviewer — `internal/platform/duckdb/text.go:100,297,366` — fixed-size ARRAY scalar elements are quoted/escaped like LIST elements; DuckDB prints them bare.
   Failure: `array_value('', 'a b ', 'NULL', 'x''y')` → got `['', 'a b ', 'NULL', 'x\'y']`, DuckDB `[, a b , NULL, x'y]`; `[array_value('b,c')]` → got `[['b,c']]`, want `[[b,c]]`; `array_value(TIMESTAMP '2026-01-01 01:02:03')` → got `['2026-01-01 01:02:03']`, want `[2026-01-01 01:02:03]`. LIST inside ARRAY and STRUCT/MAP members inside ARRAY keep their own quoting. Same fault in `--json` (ARRAY is DuckDB text).
   Fix: pass the container kind into `listText`; for `kindArray` render each direct scalar element with `valueText`, no `needsQuotes`/`escapeNested`. Add VARCHAR ARRAY rows (empty, `'NULL'`, quote, comma) and a TIMESTAMP ARRAY row to `Test_query_table_text_matches_duckdb_cast`.
2. correctness-reviewer — `internal/platform/duckdb/text.go:248` (`offsetText`, used :191 TIMESTAMPTZ, :199 TIMETZ) — offsets with seconds differ from DuckDB.
   Failure: TZ=America/New_York `TIMESTAMPTZ '1800-06-01 12:00:00+00'` → got `…-04:56:02`, want `…-04:56` (Europe/Amsterdam `+00:17:30` vs `+00:17`); `TIMETZ '12:00:00-00:00:30'` → got `-00:00:30`, want `-00:30` (DuckDB drops a zero minutes field when seconds ≠ 0; `-01:02:03`, `±00:01:30`, `±05:30` match).
   Fix: TIMESTAMPTZ offset at minute precision (`±HH` / `±HH:MM`; add a negative sub-minute oracle row to settle truncate vs round); TIMETZ: `minutes == 0 && seconds != 0` → `±HH:SS`. Oracle rows with `t.Setenv("TZ", …)` or a zone-fixed instant.
3. test-reviewer — `internal/store/duckstore/duckstore_test.go` (no test) — spec "A sync running concurrently: readers see the old store, no message" untested.
   Failure: `Replace` could open/truncate/unlink the final path before rename, or break a live reader, with all tests green.
   Fix: open `duckdb.OpenReadOnly` on the built path and hold it; `Replace` with different rows; assert success, held reader still sees old rows, fresh `Status`/`Query` sees new rows.
4. test-reviewer — `internal/store/duckstore/query_test.go` (no test) — "quarry's own tables and views … `SELECT *` never gets Q5" and the help's `quarry sql "SHOW TABLES"` untested.
   Fix: on `newBuiltStore` (every table filled), `SHOW TABLES` → NoError, lists every table + `v_account_balances`; iterate and `SELECT * FROM <name>` → NoError (no `*store.UnprintableValueError`).
5. test-reviewer — `internal/cli/render_sql_internal_test.go:20-58` `Test_renderSQLTable` — "a value's own spaces, leading or trailing, print as they are" unpinned; `strings.TrimRight(line, " ")` passes every case.
   Fix: rows `sqlRow("a ", "1")`, `sqlRow("x", " b ")`, and an all-spaces middle cell; exact bytes incl. trailing space on the last line.

### MINOR
- arch-reviewer — `internal/cli/sql.go:167` `queryFailure` maps store errors to copy in cli while status/accounts do it in `report.readRefusal`. Optional: `report.Server.Query` returns `RefusalError`.
- arch-reviewer — `internal/cli/sql.go:126` `readStdinQuery` concurrency/I-O policy in cli. Optional move.
- refactor-advisor — `internal/cli/{sql,accounts,status}.go` RunE bodies repeat open→fetch→render→`writeResult`→warnings; extract `emit(...)` + `openReport(...)` (also unifies the three identical `// unreachable:` comments and the stderr warning prefix).
- refactor-advisor — interrupt wrap `fmt.Errorf("%w: %w", store.ErrQueryInterrupted, err)` built at `duckstore/query.go:22,39` and `cli/sql.go:136`; one `store.Interrupted(err)` helper.
- refactor-advisor — `internal/platform/duckdb/text.go:61,180-197` time-kind names as repeated string literals; one table.
- refactor-advisor — `internal/cli/render_status.go:17` `dateLayout` duplicates `json.go:13` `jsonDateLayout`.
- refactor-advisor — `internal/cli/render.go:99-160` `balancesPhrase(checked, neverReconciled, investmentAccounts)` three bare ints.
- test-reviewer — `internal/store/duckstore/accounts_test.go:20-23,63,74-83` `localToday()` vs DuckDB `current_date` fails across local midnight; accept before/after days.
- test-reviewer — H1 for `sql` untested; `sql` and `accounts` Short strings untested.
- test-reviewer — `accounts --json` checked with `JSONEq`/`Contains` (`json_accounts_internal_test.go:32`, `run_accounts_json_test.go:65`); key order is the contract — pin exact bytes.
- test-reviewer — `cmd/quarry/run_accounts_test.go:156-167` `--all` test asserts only a `Contains`; assert full stdout.
- test-reviewer — `cmd/quarry/run_sql_test.go:170-189` SIGINT test's fixed 500ms sleep doesn't prove query stage reached; say its job is signal wiring (query-stage cancel proven in `duckstore/query_test.go:118`).
- test-reviewer — duplicated cmd fixtures/helpers (sync-under-HOME snippet ×7, one-account bundle ×2, `writeStatusFixtureBundle` vs `run_store_info_test.go:25-35`, multi-line-dash test syncs a store it never reads, sql tests assert values defined in `syncAccountsFixture`).
- test-reviewer — shared fakes live in first-written test files (`report/status_test.go:16-36`, `cli/accounts_test.go:22-60`, `duckstore/status_test.go:148-215`, `duckstore/query_test.go:15`); no cli-level status command fault/`--json`-refusal tests.
- test-reviewer — duckstore Status/Accounts/Query fault tests copy-pasted (~12); `OpenReadOnly`+cleanup repeated ×6 in `duckstore_test.go`.
- test-reviewer — six new `package main` test files lack the white-box justification header.
- test-reviewer — chmod-based tests assume non-root (`duckstore/open_test.go:88-106`, `duckdb/faults_test.go:98-109`, `run_store_faults_test.go:73-78`).

### NIT
- arch-reviewer — `internal/report/report.go:34` `NewServer` without a Store panics on first read (matches `snapshot.NewServer`).
- refactor-advisor — warning prefix `quarry: ` (accounts, copy-ruled) vs `quarry: warning: ` (sql, copy-ruled) — ruled, no change.
- refactor-advisor — `duckdb/text.go:229` `timestampText` lacks doc; `cli/sql.go:39-46` `--limit` check inside `Args`; `cli/json_status.go:36` splits-checked derivation uncommented.
- correctness-reviewer — `cli/sql.go` Q5 hint puts the column name unquoted (`CAST(my col AS VARCHAR)` is a parse error) — copy question for product-vision.
- test-reviewer — `report/status_test.go:26-30` `fakeStore.Query` derefs nil `gotMaxRows`; `duckdb/faults_test.go:136-172` three predicates in one table; `duckstore/accounts_test.go:42-70` many facts in one test; `report/status_test.go:47` trivial getter; `run_store_info_test.go:74` weak NotEmpty (explained); `cli/sql_test.go` 390 lines; `report/query_test.go` no `MaxInt-1`/zero-row case.

### Could not check (correctness-reviewer)
- Whether a query spilling past DuckDB's memory limit writes temp files beside the store (contradicting "cannot … write other files").
- SIGPIPE on a real closed pipe; Ctrl-C on a real terminal during `sql -`.

### Strengths
- 95-row oracle comparing Go rendering to DuckDB's own CAST on the same connection.
- Deterministic interrupt seams (`onQuery` cancel, blocked `io.Pipe` stdin); cmd tests assert no store/dir created.
- Dependency rule clean: `internal/store` stdlib-only, `platform/duckdb` imports no quarry package, no feature→feature import, ports declared at consumers with compile-time guards.
- Lockdown probe: `SET`/`RESET` locked, `EXPORT DATABASE`/`glob` Permission, no file created beside the store.

### Verdict: FAIL
5 MAJOR (correctness 2, test 3), 18 MINOR, 5 NIT.
