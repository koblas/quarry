---
id: SCENARIO-03
status: open
---

# SCENARIO-03: query returns rows as the sql --json document (folds SCENARIO-04, SCENARIO-05)

Cadence: code-first — no mandatory test-first item (read-only lockdown and `duckstore` are unchanged; this only maps their errors to copy)
Acceptance test: `cmd/quarry/run_mcp_query_test.go` `Test_run_mcp_query_returns_the_sql_json_document`
Acceptance test (SCENARIO-04, folded): `cmd/quarry/run_mcp_query_test.go` `Test_run_mcp_query_over_its_limit_returns_the_first_rows_and_says_so`
Acceptance test (SCENARIO-05, folded): `cmd/quarry/run_mcp_query_test.go` `Test_run_mcp_query_refuses_what_it_cannot_run`
Narrow loop: `go test ./internal/mcp/ && go test ./cmd/quarry/ -run 'MCP|Mcp'`
Mutation checks: stderr log only on isError → `Test_a_successful_call_writes_nothing_to_stderr`; blank-sql guard in `query` handler → `Test_query_refuses_blank_sql_without_touching_the_store`; `WithReport` wiring line in `newMCPServe` → the three acceptance tests; hard cap guard (limit outside 1..500 is 500) → `Test_query_caps_a_limit_the_schema_did_not_check`; limit passthrough → `Test_query_asks_the_store_for_the_effective_limit`
Runs: A (1-3) | B1 (4-6) | B2 (7) | V (8-9)
Size: OWNS A RUN — 4 Build batches (envelope; handler + wiring + blank + multi-statement; limit + truncation; refusal arms), 1 package (`internal/mcp`) + `cmd/quarry` wiring; folds S04, S05

Existing surface (go doc / LSP, no Glob needed): `internal/mcp` has `Server`, `NewServer`, `WithVersion`, `Serve(ctx, stdin, stdout, _ io.Writer)` (stderr ignored today), `notBuilt[In]`, `queryInput{SQL, Limit}`, `absentNullArguments`. `report.Server.Query(ctx, sql, limit)` already asks limit+1 and sets `Truncated`; `report.ClassifyQueryFailure`, `document.NewSQL(result, limit, warnings)` exist. Surface mcp consumes from report: `Server.Query`, `ClassifyQueryFailure`; errors from the store arrive as `report.RefusalError` (message already the ruled `~` line, no prefix) and are used via `err.Error()`. Everything else stays on the concrete types.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_mcp_query_test.go` (new) `Test_run_mcp_query_returns_the_sql_json_document` — `syncAccountsFixture` under a fresh HOME (`run_accounts_test.go:109`), `startMCP` (`run_mcp_test.go:~131`), call `query`; `TextContent` equals `json.Compact` of `quarry sql --json` for the same SQL (`run` over `testEnv`, `run_sql_json_test.go:128`) minus the trailing newline; `StructuredContent` bytes taken from the raw frame in `peer.stdout` (client decoding loses key order) equal the same bytes; `IsError` false; stderr empty. Include one `<`/`&` cell (CLI's encoder HTML-escapes; MCP must match) and the multi-statement pin: `SELECT 1 AS a; SELECT 2 AS b` returns columns `[b]`
- [x] Step 2: same file `Test_run_mcp_query_over_its_limit_returns_the_first_rows_and_says_so` — cases: explicit `limit` 3 over `range(10)`; `limit` 1 over `range(2)` (singular wording `the first 1 row`, via humanize.Count); omitted limit over `range(501)` (500 rows, warning names 500); rows == limit is not truncated (control); `SELECT 1 WHERE false` as its own case (`rows` and `warnings` are `[]` not null, `row_count` 0, `truncated` false). Asserts `row_count`, `truncated`, `limit`, verbatim §2.4 warning
- [x] Step 3: same file `Test_run_mcp_query_refuses_what_it_cannot_run` — table over the seven scenario rows (write, `read_csv` of a temp file, whitespace-only, `;`, `-- note`, invalid SQL, JSON-typed column); `;` and `-- note` reach the store and map from `QueryFailureEmpty` to the blank-sql line (no handler pre-parse); each asserts `IsError`, exactly one `TextContent` equal to the §4 line (no `quarry: ` prefix, no structured content) and stderr growth for that call equal to exactly `quarry: mcp: query: <line>\n` (one shared peer and synced store; do not re-sync per row). Red now at the assertion (`notBuilt` text). No production stubs needed: only existing symbols are used

### Build
- [x] Step 4: `internal/mcp/result.go` (new), `server.go:44-59` `Serve`, `tools.go:148-166` `notBuilt`/`addTools` — shared envelope S07/S09/S11 reuse: a generic handler wrapper taking the tool name and a `func(ctx, In) (document any, err error)`; success = `json.Marshal(doc)` → `StructuredContent: json.RawMessage` + one `TextContent` of the same bytes; failure = `IsError` with one `TextContent` of the error text, no document; the log is NOT in the wrapper: `errorLog` is a receiving middleware on `tools/call` registered beside `absentNullArguments` (`server.go:52`), because the SDK answers schema/type refusals itself as `isError` results (`SetError`, SDK `server.go:~403`) that never reach a handler. After `next`, a `*CallToolResult` with `IsError` writes `quarry: mcp: <Params.Name>: <first TextContent text>\n` as ONE `Write` under a mutex (tests share one `bytes.Buffer`); `Serve` passes its `stderr` to it; a result whose request ctx is done writes nothing (S06/S17 hook). Fault test: encode failure is `// unreachable:` as in `cli/json.go:140`. Tests `result_test.go`: success writes nothing to stderr; failure writes exactly one line, SDK-side refusal (missing `sql`) included; text == structured bytes; key order survives (read a raw frame, not the SDK client's decoded map); an error that is not a `RefusalError` still surfaces its text unchanged (the generic path S07/S09/S11 inherit)
- [x] Step 5: `internal/mcp/query.go` (new), `server.go:16-42` `WithReport`, `tools.go:156-159`, `cmd/quarry/run.go:133-148` `newMCPServe` — `WithReport(factory)` where the factory type is declared in mcp (`func(ctx, command string) (*report.Server, error)`; cmd converts `cli.ReportFactory`; mcp must not import cli); `query` handler: blank (`strings.TrimSpace`) → `query needs SQL in the sql parameter` before any factory call; factory called per call (never cached); `Server.Query`; `document.NewSQL`. `newMCPServe` builds `mcp.NewServer(WithVersion, WithReport(newReportFactory()))` inside the closure after `resolveHome("mcp")`, keeping `signal.Ignore`/`ready()`/`Serve` order. Tests `query_test.go` with a fake `report.Store` embedding the interface (override `Query` only): blank and tab/newline-only sql never reach the store (call count 0); factory error is a refusal + stderr line; one fault test for `Server.Query`'s non-classified store error. Remove the `query` row of `server_test.go:147` `Test_an_unbuilt_tool_answers_isError`
- [x] Step 6: `internal/mcp/query.go` limit + warning; `server_test.go:129` null-args test — truncation warning `returned the first <n> rows; the query has more; aggregate or filter in SQL to see the rest` (n via `humanize.Count`, effective limit). Handler guard: a limit outside 1..500 is treated as 500 (Rule 5's hard cap must not depend on the SDK applying the schema); unit test calls the handler directly with `Limit: 0` and `Limit: 501` and asserts the store was asked 501. Tests: omitted limit → store asked 501 (`limit+1`, default observed in handler); 1 and 500 accepted; 0 and 501 refused by schema with the store untouched and exactly one `quarry: mcp: query: ` stderr line (SDK text after the prefix unasserted); non-integer `"5"` refused; `limit` 1 with 2 rows gives `truncated` true and the singular wording; add `query` rows to the null/omitted/empty-arguments test (null arguments must refuse for missing `sql`, never panic: `query` carries a `default`)
- [x] Step 7: `internal/mcp/query_refusal.go` (new) — map `report.ClassifyQueryFailure` to the ruled MCP copy: Unprintable `<Err text>; cast it in the query, e.g. CAST(<col> AS VARCHAR)`, Rejected `query failed: <Detail>`, Empty → the blank-sql line (reached by `;` / `-- note`), ReadOnly and ExternalAccess lines from §4; `QueryFailureInterrupted` and `Other` share one explicit case returning the error text unchanged (the `exhaustive` linter lists every kind; S06 splits Interrupted out there). Unit tests through a real `report.Server` over a fake store, one row per kind, plus the store refusals the generic path must carry verbatim in `~` form (`report.WithHome`): missing, other-format with `SnapshotPath` (`--from` rebuild form), not-DuckDB/permission/other (`cannot read the store at …`), locked (`close that program…`); and a plain store error with no classification. Wrapped errors (`fmt.Errorf("%w")`) also classify

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; fold S15 doc-budget MINORs: `cmd/quarry/run.go:135-137` `newMCPServe` doc to ≤2 lines, `internal/cli/mcp.go:17-19` `newMCPCommand` doc ≤2 lines, `cmd/quarry/run_mcp_exit_test.go:135-137` `mcpPipeSubprocessEnv` comment ≤1 line without a test name; reword `internal/report/document/doc.go:1-2` so it states only what is true once S03 lands (documents shared by CLI and MCP, no claim that every one is returned over MCP yet); doc comments on `WithReport`, the wrapper and `queryRefusal`

### Verify
- [ ] Step 9: full verification + `spec-check.py` → tick SCENARIO-03, then SCENARIO-04 and SCENARIO-05 with "delivered by SCENARIO-03" and their own acceptance tests (test reference last on the line); STATE.md rewrite

## Handoff

**Binding decisions:**
- The envelope in `result.go` is the only place a tool result and the stderr log line are built; S07/S09/S11 pass a `func(ctx, In) (doc, error)` and never touch `CallToolResult` — one chokepoint is what keeps "stderr only on isError" true for all four tools.
- `mcp.WithReport(factory)`: factory type declared in mcp, called per tool call with command `"mcp"`; `newMCPServe` builds the mcp Server inside the closure after the `$HOME` check. S09/S11 add `WithConfig` the same way.
- Refusal text is the error text: `report.RefusalError` lines pass through verbatim, so a new tool needs no store-refusal mapping. Query-specific copy lives only in `query_refusal.go`.
- Warning count uses `humanize.Count` (CLI parity): limit 1 reads "the first 1 row". Spec shows only 500.
- Stderr logging is a `tools/call` receiving middleware (`errorLog`), not part of the handler wrapper: SDK-side schema refusals are `isError` results too and must log. Handlers cannot write stderr.
- The handler clamps a limit outside 1..500 to 500; the schema default is also pinned (omitted → store asked 501) so neither layer silently carries the cap.

**Left unbuilt:**
- Deadline/cancel mapping, `WithTimeout` seam and ctx classification: S06 (`QueryFailureInterrupted` shares the generic arm until then).
- `describe_schema`, `sync_status`, `data_quality` handlers: still `notBuilt`; `WithConfig` option: S09/S11.
- Direct unit test of `report.SQLConventions`, PRD debts: unchanged, see STATE.

**Traps:**
- The SDK client decodes `StructuredContent` into a map, losing key order: assert order on a raw frame (`peer.stdout`) or the `TextContent`.
- CLI `marshalDocument` HTML-escapes (`<` becomes `<`); `json.Marshal` matches, `Encoder.SetEscapeHTML(false)` would not.
- A nil report factory panics in the handler (precedent: nil `Env.ServeMCP`); tests of `query` must pass `WithReport`.
- The SDK still writes an `isError` "context canceled" result for a cancelled id: `errorLog` stays silent when the call ctx is done (S06 pins it, S17 relies on it).
- The log writer is shared across concurrent calls: one `Write` per line under a mutex.

## Phase report

Run B1 (steps 4-6) done. Narrow loop green (`go test ./internal/mcp/`, `go test ./cmd/quarry/ -run 'MCP|Mcp|mcp'`, `-race` on mcp), `golangci-lint run ./internal/mcp/... ./cmd/...` 0 issues. All three acceptance tests green. Step 7 (`query_refusal.go`) was built here too, so it is ticked; B2 has only its tests left to review.

- New: `internal/mcp/result.go` (`toolFunc`, `handler`, `errorLog`, `errorText`; handler takes no tool name, the log middleware reads it from the request), `query.go` (`Server.query`, `effectiveLimit`, `truncationWarning`, `errBlankSQL`), `query_refusal.go` (`queryRefusal`, `errWriteRefused`, `errExternalRefused`; Interrupted and Other share one arm).
- Edited: `server.go` (`ReportFactory`, `WithReport`, `commandName`, `Serve` passes stderr to `errorLog`), `tools.go:90` (`addTools` is a `Server` method; `query` uses `handler(s.query)`), `cmd/quarry/run.go` `newMCPServe` (server built inside the closure after `resolveHome`; its doc is already 2 lines).
- Tests: `internal/mcp/{result,query,query_refusal}_test.go`, `query_helpers_test.go` (`fakeStore`, `newHarness`, `rowsOf`), `log_internal_test.go` (white-box: errorLog ctx-done / nil result / no-text arms, `effectiveLimit` clamp via `Server.query`); `server_test.go` has `startServerLogging` and no `query` row in the unbuilt test. Null/omitted/empty `query` arguments are pinned in `query_test.go`, not by adding rows to the data_quality test.
- `cmd/quarry/run_mcp_query_test.go:58` (Run A): the HTML `Contains` asserted the unescaped `"<a&b>"`, contradicting the equality above it; now `"\u003ca\u0026b\u003e"`. No other edit to Run A tests.
- Mutations (all red, restored byte-identical): drop `|| !result.IsError` -> `Test_a_successful_call_writes_nothing_to_stderr`; blank guard to `== "\x00"` -> `Test_query_refuses_blank_sql_without_touching_the_store`; cap guard to `limit < 1` -> `Test_query_caps_a_limit_the_schema_did_not_check` (501 vs 502); `Query(.., maxRows)` -> `Test_query_asks_the_store_for_the_effective_limit` (2 vs 501); delete `WithReport` line -> nil-pointer panic in `Test_run_mcp_query_*`.
- Not done (V): Step 8 other doc-budget folds (`internal/cli/mcp.go:17-19`, `run_mcp_exit_test.go:~135`, `document/doc.go:1-2`, doc comments check), full verify, spec tick, STATE.md, `status: done`. No `go doc`/count figures yet.
- Trap: tests that read `peer.stderr` between calls rely on `errorLog` writing before the SDK sends the response (true: the middleware wraps the handler).
