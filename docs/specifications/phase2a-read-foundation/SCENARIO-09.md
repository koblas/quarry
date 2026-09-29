---
id: SCENARIO-09
status: done
---

# SCENARIO-09: sql --json returns typed values (absorbs SCENARIO-10, the row cap)

Size verdict: OWNS A RUN (renderer + one flag plumb + one stderr note over existing cells; FOLD of SCENARIO-10 per the sizing pass - its `truncated`/`limit`/warning are only observable through this document).
Cadence: code-first (no mandatory test-first item: no write-safety guard, no atomic adapter, no bug fix)
Acceptance test: `cmd/quarry/run_sql_json_test.go` `Test_run_sql_json_returns_typed_values`
Acceptance test (SCENARIO-10, folded): `cmd/quarry/run_sql_json_test.go` `Test_run_sql_json_caps_the_rows_and_says_so`
Narrow loop: `go test ./internal/cli/ -run 'SQL|JSON' && go test ./cmd/quarry/ -run 'Test_run_sql'`
Mutation checks: NaN/Inf guard in `jsonSQLCell` -> `Test_render_sql_json_encodes_each_native_kind`; DATE-vs-TIMESTAMP dispatch on `QueryColumn.Type` -> same test (DATE and TIMESTAMP rows); float32 path (`float64(f)` widening) -> same test (`0.1` FLOAT row); warning written only after stdout succeeds -> `Test_sql_writes_no_warning_when_stdout_fails`; `truncated`/`limit` echo -> `Test_run_sql_json_caps_the_rows_and_says_so`

Existence facts: `sql` has no `jsonOut` (`sql.go:16`, sole caller `root.go:30`, checked by grep of `cmd internal`); `--json` is already the root persistent flag (`root.go:25`). `report.QueryResult.Truncated` and the limit rule exist (`report/query.go:21-34`) - no query-path change. `marshalDocument` (`json.go:130-141`) is the encoder. Nothing for `sqlDocument`/`renderSQLJSON`/`truncationNote` exists. Survey of `store.QueryValue.Native` producers: `duckdb/text.go:64,184` (time.Time for DATE/TIMESTAMP*/TIMESTAMPTZ; `Text` string for the rest, incl. +-infinity dates, DECIMAL, HUGEINT).

Contract: `sql --json <q>` stdout = `marshalDocument` of `{columns:[{name,type}], rows:[[...]], row_count, limit, truncated, warnings}` in that key order (Surface & Copy -> `sql`); `row_count` = rows in the document (after the cap); `limit` = the flag value as given. Truncated (both modes, exit 0): stderr `quarry: warning: showing the first <N> rows; the query returned more; pass --limit 0 to print every row` written after stdout succeeded; `warnings[]` carries the note without its stderr prefix - default `showing the first <N> rows; ...` (prefix to strip, `quarry: warning: ` vs `quarry: `, pending ruling - see Handoff) (`[]` otherwise). A refusal under `--json` leaves stdout empty (errors return before render).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_sql_json_test.go` `Test_run_sql_json_returns_typed_values` - `run(... "sql","--json", <literal query>)` on `runSQLOnBuiltStore`-style setup (`run_sql_test.go:202-210` builds via `syncAccountsFixture`; add a variant taking args): one row of DECIMAL, HUGEINT, INTEGER, DOUBLE, BOOLEAN, DATE, TIMESTAMP, NULL; assert the document (bytes are the contract, `//nolint:testifylint` with reason per STATE trap) and empty stderr
- [x] Step 2: same file `Test_run_sql_json_caps_the_rows_and_says_so` - table of the SCENARIO-10 outline rows 3/2, 2/2, 3/0 (rows from `SELECT ... FROM range(3)`-style literal query): assert `len(rows)` (the printed rows) as well as `row_count`, `limit`, `truncated`, `warnings`, and stderr equal to the warning (or empty). No stubs needed - both compile today and fail at their assertion (`--json` currently prints the table); paste the red output.

### Build
- [x] Step 3: `internal/cli/json_sql.go` (new) `jsonSQLCell(col store.QueryColumn, v store.QueryValue) any` + `json_sql_internal_test.go` `Test_render_sql_json_encodes_each_native_kind` (table through `renderSQLJSON`, precedent `json_accounts_internal_test.go`) - Null->nil; bool; int64/uint64 native (uint64 max); float64/float32 native, float32 encoded as float32 (`0.1` not `0.10000000149...`), `-0.0` -> `-0`, NaN/+Inf/-Inf (DOUBLE and FLOAT) -> `"nan"`,`"inf"`,`"-inf"` never null; `time.Time`: `Type=="DATE"` -> `2006-01-02` (`jsonDateLayout`), any other time column -> `.UTC().Format(time.RFC3339Nano)` (add years <1000 and >9999: `Format` never errors, `json.Marshal(time.Time)` would); everything else (`string`, unknown Native type) -> `v.Text`. Control arm per guard: finite float beside NaN.
- [x] Step 4: `internal/cli/json_sql.go` `sqlDocument`, `sqlColumnDocument`, `renderSQLJSON(result report.QueryResult, limit int, warnings []string) ([]byte, error)` - `columns`/`rows`/each row built with `make` so zero rows and zero columns are `[]` not `null`; goes through `marshalDocument`. Tests in the same internal test file: document shape and key order (`Test_render_sql_json_lists_columns_rows_and_the_cap` - type string verbatim e.g. `DECIMAL(18,2)`, `row_count` = len(rows) with `Truncated` and `limit`), `Test_render_sql_json_keeps_empty_lists_as_arrays` (0 rows; `warnings` `[]`).
- [x] Step 5: `internal/cli/sql.go:14-55` `newSQLCommand(newReport, jsonOut *bool)` + `truncationNote(limit int) string` (unexported, the one place a copy ruling lands, with the test expectations; `humanize.Count(limit, "row", "rows")`) + `root.go:30` wiring - mirror `accounts.go:34-51`: build `warnings`, render table or JSON once, write stdout, then `"quarry: warning: "+note` to `cmd.ErrOrStderr()` only if the stdout write succeeded (ignore the stderr write result like accounts). Tests in `sql_test.go` (add a stderr-capturing variant of `executeSQL:17-26`): `Test_sql_says_when_it_cuts_the_rows` (table: human and `--json` x truncated / exactly-limit / `--limit 0`; asserts stderr and `warnings[]` carry the same text; wording rows: `--limit 2`, `--limit 1` -> `first 1 row`, default limit with 501 fake rows -> `first 500 rows`); `Test_sql_writes_no_warning_when_stdout_fails` (both modes, `failingWriter`, empty stderr; JSON arm is the fault test for the new Write); `Test_sql_json_writes_nothing_for_a_query_fault` (`fakeReportStore{err: ...}`, empty stdout AND stderr).
- [x] Step 6: `cmd/quarry/run_sql_test.go:52-62` `Test_run_sql_prints_at_most_limit_rows` - also assert the human-mode stderr warning (`first 2 rows`); this is the table-mode end-to-end proof.

### Sweep
- [x] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`. Re-word `json.go:137` `// unreachable:` (single line): documents hold only strings, bools, ints, pointers, slices and the finite floats `jsonSQLCell` lets through; NaN/Inf become strings and any unknown Native falls to `Text`, so no value can fail JSON encoding. Add the matching one-line `// unreachable:` at `newSQLCommand`'s `renderSQLJSON` error return (precedent `accounts.go:37-38`). Doc comments on new symbols; update `newSQLCommand`'s doc for `--json`.

### Verify
- [x] Step 8: full verification per `agent-briefs.md` + `.claude/scripts/spec-check.py phase2a-read-foundation`; tick SCENARIO-09 with its acceptance test and SCENARIO-10 as `delivered by SCENARIO-09` with `Test_run_sql_json_caps_the_rows_and_says_so` last on the line; set both plan statuses done; rewrite STATE.md (drop the `sql`: `--json` and truncation-warning items from *Left unbuilt*; fix the S09 trap about `marshalDocument`).

## Handoff

**Binding decisions:**
- `renderSQLJSON` is the only `sql --json` encoder and goes through `marshalDocument`; `jsonSQLCell` is the only producer of row `any` values (nil, bool, int64, uint64, finite float32/float64, string) - `marshalDocument`'s unreachable claim depends on that; a new cell kind must extend `jsonSQLCell`, not `renderSQLJSON`.
- DATE vs TIMESTAMP is decided by `QueryColumn.Type == "DATE"` (Native is `time.Time` for both); nested/other types never reach the time branch because their Native is `Text`.
- The truncation note is `truncationNote(limit)` in cli, the same string on stderr (`quarry: ` prefixed) and in `warnings[]`, both modes.
- `row_count` counts rows in the document, not rows the query returned.

**Copy gaps for the orchestrator (inferred defaults; block only the developer's wording tests; a ruling changes `truncationNote` and its test expectations):**
- `warnings[]` prefix: spec says "same text without prefix"; plan strips all of `quarry: warning: `. The other reading (`warning: showing ...`) differs from accounts, whose note has no `warning:`. Rule it.
- The spec fixes the copy only for 500. Plan uses `humanize.Count(limit, "row", "rows")`: `--limit 1` -> `first 1 row`, `--limit 10000` -> `first 10,000 rows` (grouping matches status/sync copy; table cells stay ungrouped). Confirm or rule.
- `limit` for negative `--limit` echoes the value until U7 (S18) refuses it; `report.Server` treats `<=0` as every row.

**Left unbuilt:** U5-U9, `-` stdin, O2 for `sql` (SCENARIO-18); nested JSON for LIST/STRUCT/MAP (out of 2a, text strings).

**Traps:**
- `json.Marshal(time.Time)` errors outside years 0-9999 and `encoding/json` errors on NaN/Inf - format times and floats yourself in `jsonSQLCell`.
- `marshalDocument` HTML-escapes `<`, `>`, `&` (`<`); valid and decoding equal, leave it (shared encoder) and compare decoded values or use full expected bytes without those characters.
- `fakeReportStore.Query` ignores `maxRows` and returns its whole canned result; `report.Server` does the cut - truncation cli tests need `limit+1` fake rows.
- JSON integers above 2^53 lose precision in JS consumers; ruled native in the spec, do not stringify.
