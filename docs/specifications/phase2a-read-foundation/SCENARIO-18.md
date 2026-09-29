---
id: SCENARIO-18
status: open
---

# SCENARIO-18: read commands reject bad usage (absorbs SCENARIO-11, 19, 21)

Size verdict: OWNS A RUN (FOLDs per sizing pass: 11 = the `-` arm of U5, 19 = one `Execute` line, 21 = one write helper).
Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_read_usage_test.go` `Test_run_read_commands_reject_bad_usage`
Acceptance test (SCENARIO-11, folded): `cmd/quarry/run_sql_test.go` `Test_run_sql_reads_the_query_from_stdin`
Acceptance test (SCENARIO-19, folded): `cmd/quarry/run_usage_test.go` `Test_run_usage_hint_names_the_matched_command`
Acceptance test (SCENARIO-21, folded): `cmd/quarry/run_accounts_test.go` `Test_run_accounts_reports_a_failed_stdout_write`
Narrow loop: `go test ./internal/cli/ ./internal/store/duckstore/ ./internal/platform/duckdb/ -run 'SQL|Sql|sql|Accounts|Status|Usage|Query|predicate' && go test ./cmd/quarry/ -run 'Usage|usage|Test_run_sql|stdout|read_usage'`
Mutation checks: stdin read + blank check before `newReport` → `Test_run_help_and_usage_errors_do_not_need_home` (`sql -` row); exact-match `duckdb.IsEmptyQuery` (mutate to `Contains`) → `Test_query_error_predicates_classify_driver_errors` (`error('empty query')` row); `cmd.CommandPath()` in `Execute`'s hint (mutate to `"quarry sync"`) → `Test_run_usage_hint_names_the_matched_command`

**Copy ruled (spec Q6/Q4):** stdin read fault → Q6 `quarry: cannot read the query from stdin: <OS reason>` (G1 reason rule), exit 1; Step 5 adds a `*fs.PathError` row (Err-only strip) beside the `iotest.ErrReader` row. Interrupt while reading stdin → Q4: read in a goroutine, `select` on `ctx.Done()`, `ctx.Err()` checked before Q6/U5; add `Test_sql_reports_an_interrupt_while_reading_stdin` (blocking stdin, cancel ctx → Q4, exit 1).

Contract (Surface & Copy → Refusals, verbatim): U5 `quarry: sql needs a query; pass it as one quoted argument, or - to read it from stdin` (2); U6 `quarry: sql takes one query; quote it as one argument` (2); U7 `quarry: --limit must be 0 or more; 0 prints every row` (2); U8 `quarry: <cmd> takes no arguments` (2); U9 `<cobra text>; Run '<cmd.CommandPath()> --help' for usage.` (2); O2 `quarry: cannot write the result to stdout: <raw write error>` (1), stdout empty on every refusal, no note/warning after a failed write. `sql ";"`/`"-- note"`: store present → U5 (2); no store → R1 (1); interrupted → Q4.

Existence facts: `Execute` uses `root.ExecuteContext` and hard-codes the sync hint (`internal/cli/run.go:44-55`); sql `Args: cobra.ExactArgs(1)` (`sql.go:37`); status/accounts have no `Args` (cobra accepts extras); three raw `Write`s at `status.go:42`, `accounts.go:48`, `sql.go:64`; sync's O2 is `snapshot.Outcome.StdoutWriteRefusal` (`snapshot/import.go:190-203`, `%w` of the raw error); `queryRefusal` arms at `duckstore/query.go:35-51`; driver predicates at `duckdb.go:233-243`; store sentinels `store/query.go:51-57`. EPIPE: `signalContext` (`run.go:33-40`) notifies only Interrupt/SIGTERM and `main.go:11` passes `os.Stdout` unwrapped, so Go's default SIGPIPE death holds — nothing to build. cobra v1.10.2 has `ExecuteContextC`.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_read_usage_test.go` (new) `Test_run_read_commands_reject_bad_usage` — sync a v9fixture store once, then the 9 outline rows through `runWith` (`defaultEnv` + `env.Stdin` per row); exit 2, stdout empty, exact stderr. Same file: `Test_run_sql_reports_no_store_for_an_empty_statement` (`sql ";"`, no store → R1, exit 1). No stubs needed — black-box through `runWith`
- [ ] Step 2: `run_sql_test.go` `Test_run_sql_reads_the_query_from_stdin` (runWith, stdin `SELECT 1 AS n`); `run_usage_test.go` `Test_run_usage_hint_names_the_matched_command` (`sql --bogus`, `status --bogus`, `accounts --bogus`, `spend`); `run_accounts_test.go:67-81` rename to `Test_run_accounts_reports_a_failed_stdout_write`, assert exact O2 line. All four red at their assertions

### Build
- [ ] Step 3: `internal/platform/duckdb/duckdb.go:233-243` `IsEmptyQuery` (exact `err.Error() == "empty query"`; the driver's `errEmptyQuery` is unexported) + `faults_test.go:136-167` rows `;`, `-- note` true, `SELECT error('empty query')` false, and `:169-174` errNotDuckDB false
- [ ] Step 4: `store/query.go:51-57` `ErrEmptyQuery`; `duckstore/query.go:35-51` `queryRefusal` arm after `IsAccessDisabled` (`%w: %w`) + `query_test.go:92-113` rows `;`, `-- note` → `store.ErrEmptyQuery`
- [ ] Step 5: `internal/cli/sql.go:37-72` — custom `Args` (count: 0 → U5, >1 → U6; then `limit < 0` → U7; then blank literal → U5); RunE reads `cmd.InOrStdin()` for `-` and blank-checks BEFORE `newReport`; query passed verbatim (trim only for the test); `queryFailure:84-100` maps `store.ErrEmptyQuery` → U5 `UsageError`. Tests in `sql_test.go`: `Test_sql_rejects_bad_usage` (rows incl. precedence `--limit -1` with no arg, whitespace stdin, and U7 bound pair `-1` refuses / `0` runs), `Test_sql_reads_the_query_from_stdin_verbatim` (via new `gotQuery *string` on `fakeReportStore`, `accounts_test.go:26-41`), `Test_sql_reports_a_stdin_read_fault` (`iotest.ErrReader`; copy per ruling), `Test_sql_reports_each_query_refusal:103-138` row ErrEmptyQuery → U5 `UsageError`; delete `run_sql_test.go:175-195` (superseded by Step 1)
- [ ] Step 6: `internal/cli/errors.go` `noArgs` Args func (`cmd.Name() + " takes no arguments"`) before `status.go:21` and `accounts.go:22` RunE + `accounts_test.go` `Test_status_and_accounts_take_no_arguments`
- [ ] Step 7: `internal/cli/run.go:31-55` `Execute` → `ExecuteContextC`; fallback hint uses `cmd.CommandPath()` (never nil — no defensive branch); update `run_usage_test.go:142-150` (`frob`, `synk` → `quarry --help`) and `:212`; extend `Test_run_help_and_usage_errors_do_not_need_home:182-214` with HOME-less U5 (`sql`, `sql -` empty stdin via runWith), U7, U8, U9 `sql --bogus` rows
- [ ] Step 8: `internal/cli/output.go` (new) `writeResult(cmd, out)` → `&runtimeError{fmt.Errorf("cannot write the result to stdout: %w", err)}`; use at `status.go:42`, `accounts.go:48`, `sql.go:64` (note/warning loops stay after it). Tests: `sql_test.go:163-167` → `EqualError` O2 text; `Test_accounts_reports_a_failed_stdout_write` in `accounts_test.go`; status's O2 proven at cmd level — tighten `run_status_test.go:71-85` to the exact line (`fakeReportStore` has no `Status`; do not add one)

### Sweep
- [ ] Step 9: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `IsEmptyQuery`, `ErrEmptyQuery`, `writeResult`, `noArgs`, `Execute` (hint now names the matched command). Fold STATE debts: `sql.go:15-17` `newSQLCommand` doc (→2 lines), `json_sql.go:28-30` `renderSQLJSON` (→2), `json_sql.go:50-54` `jsonSQLCell` (→≤2); `sql_test.go:178-222` `Test_sql_says_when_it_cuts_the_rows` — `wantStderr` table field, inline `append`, no loops

### Verify
- [ ] Step 10: full verification + `spec-check.py phase2a-read-foundation` → tick SCENARIO-18 with its acceptance test; tick 11, 19, 21 each `— delivered by SCENARIO-18 — <file> <Test>` (reference last); rewrite STATE.md

## Handoff

**Binding decisions:**
- Usage precedence: sql `Args` = count (U5/U6) → U7 → blank literal (U5); `-` read + blank check in RunE before `newReport` — H1's "usage never needs `$HOME`" rests on this order.
- The query reaches DuckDB verbatim, stdin included; trimming is only the emptiness test (the S08 "never rewritten" rule).
- U5's DuckDB arm: `duckdb.IsEmptyQuery` exact message match, after `IsAccessDisabled` and after `ctx.Err()` in `queryRefusal`; `store.ErrEmptyQuery` → cli U5 `UsageError` (exit 2 found by `Execute`'s `AsType` through `runtimeError.Unwrap`).
- O2: every read command writes stdout through `writeResult`; the reason is the raw write error (`%w`), matching sync's `StdoutWriteRefusal`. A new read command uses it.
- U9 hint = matched `cmd.CommandPath()`; sync's own `UsageError` constants stay hard-coded, byte-identical.

**Left unbuilt:**
- stdin size cap and TTY detection for `sql -` — ruled not built (spec Q6).
- STATE debts not folded (files untouched): `render_sql.go:121`, `duckdb/text.go`, `duckdb/typename.go:68` doc budgets.

**STATE lines that die:** Left unbuilt 34-36; Open debts 63-64.

**Traps:**
- The O2 lead `cannot write the result to stdout: ` lives in `cli/output.go` and `snapshot/import.go:198,202` — keep them byte-identical.
- A cli test running `sql -` without `Env.Stdin` reads the test process's stdin (cobra falls back to `os.Stdin`).
- Never add SIGPIPE to `signal.Notify`: EPIPE must keep Go's default death.
- `SELECT error('empty query')` yields `Invalid Input Error: empty query` — a `Contains` match would turn a user's own error into U5.
