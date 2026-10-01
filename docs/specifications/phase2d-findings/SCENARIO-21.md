---
id: SCENARIO-21
status: done
---

# SCENARIO-21: sql --csv prints every row with a header

Cadence: code-first (read-only rendering: no write guard, no atomic adapter, no bug fix)
Acceptance test: `internal/cli/sql_csv_test.go` `Test_sql_csv_prints_every_row_with_a_header`
Narrow loop: `go test ./internal/cli/` (whole package: `-run` misses the `_internal` tests)
Mutation checks: NULL is decided by `QueryValue.Null`, not by `Text == "NULL"` → `Test_sql_csv_tells_null_from_the_empty_string_and_the_word_null`; explicit `--limit` found by `Flags().Changed("limit")`, not by `limit == default` → `Test_sql_csv_honours_an_explicit_limit_equal_to_the_default`; quote on `"`/CR/LF/`,`/empty → `Test_csvField_quoting_matrix` rows
Runs: A (1-2) | B1 (3-5) | V (6-7)
Size: OWNS A RUN — 3 batches (writer + matrix; `--csv` flag, limit, conflict; Long/help pins), 1 package (`internal/cli`). No port, no `cmd/quarry` edit: `--csv` is a local flag of `newSQLCommand`; `Query`/`report.Server` already take `limit 0 = all`.

Inventory (nothing new on any port): `newSQLCommand` (`sql.go:21`) gains `var csvOut bool`; `Args` (`sql.go:50-62`) and `RunE` (`sql.go:63-95`) are the only callers of the path; output goes through `emit` (`output.go:45`, stdout fault = `cannot write the result to stdout: <err>`, warnings only after a good write); `store.QueryValue{Null, Text}` (`Text` is `"NULL"` for NULL); `truncationNote`, `queryFailure` (Q5 unprintable refusal) stay untouched and apply to `--csv` unchanged. `findings.go:55-60` already carries the `--csv` Long/Example text but no flag; that is SCENARIO-22's.

Contract (Surface & Copy → `quarry sql --csv`, P2d-12/13), pinned verbatim:
- Stdout: header of column names, then one line per row; fields joined by `,`; every line ends `\n` (also when a field holds CR); no BOM; zero rows = header line only, exit 0; stderr empty unless a truncation note applies.
- A field is quoted iff it contains `,` `"` CR or LF, or is the empty string (`""`); `"` doubled. A lone tab is NOT a trigger (unquoted, verbatim). `\n \t \r` are written verbatim, never as `\n` text (table escaping `sqlCellEscaper` is NOT used). NULL = unquoted empty field; a non-NULL `Text` of `NULL` prints `NULL`. No formula escaping (`=1+1`, `+x`, `-x`, `@x` verbatim).
- DECIDED (unruled in spec): header names use the same quoter (`a,b` → `"a,b"`, `say "hi"` → `"say ""hi"""`, empty name → `""`). A one-column NULL row is a blank line (the rule applied literally).
- Limit: unset `--limit` under `--csv` = every row (`Query` gets maxRows 0); explicit `--limit n` (including `--limit 500`) honoured with the existing `quarry: warning: showing the first ...` stderr note; without `--csv` the default stays 500. `--limit` validation (`< 0` → U7) unchanged.
- `--csv --json` (either order) → `UsageError` `--csv and --json cannot be used together; choose one output format` (no `Run ... --help` suffix; `cmd` prints `quarry: ` and exits 2), stdout empty, query not run, stdin not read. DECIDED (unruled): checked last in `Args`, so u5/u6/u7 win when also present.
- Unprintable value and every other query fault: existing refusals, stdout empty.
- Help (verbatim from spec): `--csv` help `print the rows as CSV, with a header line`; `--limit` help ``print at most `n` rows (500 unless set, every row with --csv; 0 prints every row)``; Example line `  quarry sql --csv "SELECT * FROM transactions" > transactions.csv`; Long = Changes item 2 (replaces `sql.go:37-47` from "Amounts are DECIMAL" to the end).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `internal/cli/sql_csv_test.go` (new) `Test_sql_csv_prints_every_row_with_a_header` — `Execute` `sql --csv` over `fakeReportStore` (`fakes_test.go:18`) holding 600 rows of `id, payee, memo` incl. payee `Smith, "Jo"` (comma+quote), one NULL memo beside one `""` memo; asserts whole stdout, `gotMaxRows == 0`, empty stderr
- [x] Step 2: `internal/cli/sql.go:21-99`, `internal/cli/csv.go` (new), `render_sql.go` — signature-only stubs so the test compiles and fails at its stdout assertion (`--csv` flag registered, `renderSQLCSV` returning empty)

### Build
- [x] Step 3: `internal/cli/csv.go` (new) `csvField`/record writer + `errCSVAndJSON`; `render_sql.go` `renderSQLCSV(store.QueryResult) string` — the one shared writer (SCENARIO-22's `findings --csv` reuses it; its fields must be able to be NULL without `store.QueryValue`). Tests in `csv_internal_test.go` `Test_csvField_quoting_matrix` (table: plain, comma, quote doubled, CR, LF, CRLF kept verbatim, lone tab unquoted, tab+comma quoted with tab verbatim, empty string → `""`, NULL, text `NULL`, space-padded unquoted, `=1+1`/`+`/`-`/`@` unescaped, non-ASCII) and `Test_renderSQLCSV` (header quoting incl. `a,b`, `"`, empty name; zero rows = header only; single-column NULL row = blank line, `""` row = `""`; no BOM; `\n` endings with CR inside) and `Test_sql_csv_tells_null_from_the_empty_string_and_the_word_null` (via command)
- [x] Step 4: `sql.go:50-62,63-95,97` `--csv` flag, `Args` conflict case (last), `RunE` csv branch: limit forced to 0 when `!cmd.Flags().Changed("limit")`, render via `renderSQLCSV` then `emit(cmd, []byte(...), "quarry: warning: ", warnings)`. Tests in `sql_csv_test.go`: default = every row (maxRows 0); `--limit 2` cuts to 2 rows + note on stderr, stdout has 2 rows; `Test_sql_csv_honours_an_explicit_limit_equal_to_the_default` (`--limit 500` → maxRows 501, control arm: no `--csv` still 501); `--limit 0`; `--csv --json` and `--json --csv` (UsageError text, stdout empty, `gotQuery` empty, stdin `-` unread); conflict ordering (no query → u5; `--limit -1` → u7); fault tests: stdout write fault (`failingWriter`, no stderr warning even when truncated), query fault, unprintable value (`store.UnprintableValueError` → existing cast-hint refusal, stdout empty); zero rows header only; existing table/JSON tests still green (`--csv` off)
- [x] Step 5: `sql.go:26-49,97` Long (Changes item 2, verbatim), Example third line, `--limit` help; update `sql_test.go:191-222` `Test_sql_help_describes_the_command_and_the_limit_flag` to pin the new Long whole, all three Example lines, the `--csv` and `--limit` flag lines as pflag prints them (see Handoff: `(default 500)`)

### Sweep
- [x] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `csvField`, `renderSQLCSV`, `errCSVAndJSON`; update `newSQLCommand` doc (`sql.go:19-20`) to mention `--csv`

### Verify
- [x] Step 7: full verification + `spec-check.py phase2d-findings`; tick SCENARIO-21 with its acceptance test; strike the `sql.go` Long re-wrap NIT under `## Open debts` in `docs/specifications/phase2a-read-foundation/STATE.md` (line 69); rewrite `STATE.md`

## Handoff

**Binding decisions:**
- One CSV writer in `internal/cli/csv.go` (field type that can be NULL, quoter, record writer) plus `errCSVAndJSON` — `findings --csv` (SCENARIO-22) and its `--csv --json` line (SCENARIO-23 folded) reuse both; no second quoter, no `encoding/csv` (cannot tell NULL from `""`)
- Header names go through the same quoter as data fields — one rule, so a column alias with a comma stays one column
- Under `--csv` an unset `--limit` means every row, detected with `Flags().Changed("limit")` — the flag default stays 500 so `sql --json` `limit` and the non-CSV path are untouched
- `--csv --json` is the last `Args` case — query/limit usage errors keep their 2a precedence

**Left unbuilt:**
- `--csv` on `findings` (flag, rows, fixed-finding row with NULL item fields) and its `--csv --json` check — SCENARIO-22 (the `Long`/`Example` text already exists at `findings.go:55-60`)
- PRD and sync-Long changes (Changes items 1, 8) — other scenarios

**Traps:**
- The ruled `--limit` help ends `(500 unless set, ...)` and pflag still appends `(default 500)` (default stays 500): the help line reads redundantly. Not ruled; orchestrator may send it to product-vision before B1 — the plan pins what pflag prints
- `QueryValue.Text` is `"NULL"` for a NULL: decide by `.Null`, never the text
- A tab alone does not trigger quoting; a one-column NULL row is a blank line that `encoding/csv` readers skip — both follow the rule literally
- `fakeReportStore.Query` ignores `maxRows` (records it in `gotMaxRows`): assert the recorded value, not truncation, except where the test feeds `limit+1` rows itself

## Phase report

Run V done: sweep clean (`go build ./...` ok, `golangci-lint run ./...` 0 issues); full suite `go test rc=0`, `uncovered-diff.py` 0 uncovered added lines, `go test -race ./internal/cli/...` ok; `test-stats.py --base 4df838e`: internal/cli 208 (+12), tempdir 0 (+0), disk 0 (+0). spec tick + `spec-check.py phase2d-findings` OK; 2a STATE `sql.go` Long re-wrap debt struck; STATE.md rewritten; status: done. Nothing left for later runs.
