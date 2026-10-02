# phase3a-mcp-core — current state

Scenarios complete: SCENARIO-01..02. Last updated by SCENARIO-02.

## Binding decisions
- `internal/report/document` owns the sql, status and findings `--json` documents plus shared primitives (`DateLayout`, `Money`, `NullString`, `Rows`, `FindingCounts`, `NotImported`); MCP tools render from these same builders (`NewSQL`, `NewStatus`, `NewFindingsList`) — a second copy is the drift PRD:108 forbids (SCENARIO-01)
- document imports report, store, finding, platform (`tomlstr`); never cli, mcp or config. report never imports document (cycle) (SCENARIO-01)
- Builders return values; encoding is the caller's: cli indents via `marshalDocument`, mcp encodes compact (Rule 10). Warnings are always passed in, `[]` never null (SCENARIO-01)
- `document.StatusIgnore(ignore, problem)` is the status ignore policy: non-empty problem -> nil ignore, ignored unknown, one `CannotTellIgnored(problem)` warning, never refuses. S09/S10 call it with `config.ProblemAbsolute` (SCENARIO-01)
- `report.ClassifyQueryFailure` is reason-only (`QueryFailureKind` + `Detail` + `Err`); each surface owns its copy. cli `queryFailure` returns the same error types/unwrap chain as before; S05's MCP refusal lines map from it (SCENARIO-01)
- `report.SQLConventions` is the single conventions text; `sql` Long concatenates it, S07 `describe_schema` embeds it, neither may mention masking (SCENARIO-01)
- `BasicString` lives in `internal/platform/tomlstr`; config (`keyPartText`) and document (`UnmatchedIgnoreWarnings`) both call it (SCENARIO-01)
- `internal/mcp` never imports `internal/cli`, and cli never imports mcp: cli reaches the server only through `Env.ServeMCP` (`MCPServeFunc`), wired in `cmd/quarry/run.go` `newMCPServe`. Otherwise mcp -> report becomes a feature-imports-feature violation (SCENARIO-02)
- Transport is `sdk.IOTransport` over the Env streams in no-op closers; `StdioTransport` and `slog.Default()` are banned (Rule 9), closing real stdout breaks S15's EPIPE/EOF exits. `Serve` returns `Run`'s error unwrapped: S15 maps nil (EOF), `context.Canceled`, `syscall.EPIPE` to exit 0 by `errors.Is`. SDK import is aliased `sdk` (package names collide) (SCENARIO-02)
- Every tool input schema is hand-written with `additionalProperties:false`; `tools/list` pins it. S03/S07/S09/S11 keep that when replacing handlers. The `type` enum comes from `finding.Types()`, never hand-typed (SCENARIO-02)
- Instructions and descriptions keep the spec code blocks' hard line breaks, no trailing newline; the query description's last line ("Send one statement; ...") was copy-ruled. Tool results use `Out = any`, `OutputSchema {"type":"object"}`, handlers set `StructuredContent: json.RawMessage` (SCENARIO-02)
- `absentNullArguments` (`internal/mcp/arguments.go`) is registered via `AddReceivingMiddleware` and must stay: SDK v1.8.0 panics the whole server (nil map in jsonschema-go `applyDefaults`) on `tools/call` with `"arguments": null` when the schema has a `default` (query, data_quality). S03 and S11 inherit it and must keep their `default`s covered by `Test_a_tool_call_with_null_arguments...` (SCENARIO-02)
- Root help pin `cmd/quarry/run_status_test.go` `Test_run_help_prints_quarrys_description` lists every subcommand; a new command adds its row there (SCENARIO-02)

## Left unbuilt
- spend/cashflow/recurring/anomalies/accounts/snapshots/sync documents stay in `internal/cli/json_*.go` — phase 3b moves the first four (SCENARIO-01)
- `marshalDocument` stays in cli; no compact encoder in document — mcp owns it (SCENARIO-03+)
- Multi-statement `sql` pin is CLI-only so far; MCP side is SCENARIO-03 (SCENARIO-01)
- `notBuilt` (`internal/mcp/tools.go`): shared stub handler for all four tools. S03 (query), S07 (describe_schema), S09 (sync_status), S11 (data_quality) replace it; S11 deletes it and `Test_an_unbuilt_tool_answers_isError` (SCENARIO-02)
- `mcp.WithReport` / `mcp.WithConfig` options and cmd/quarry wiring via `newReportFactory`/`newConfigLoader`: S03, S09, S11 (SCENARIO-02)
- Schema-validation pins (limit 0/501 refused, default applied, `sql:" "` reaching the handler): S03 query, S11 data_quality (SCENARIO-02)
- `quarry mcp extra` / `quarry mcp --json` still serve (no `Args: noArgs`). S15 adds refusal rows (`run_read_usage_test.go:16` table, `run_usage_test.go:137`, `:220`, `internal/cli/errors.go:14 noArgs`), the `$HOME` check and exit mapping; TTY hint S16/S15 (SCENARIO-02)
- Per-call timeout option and stderr error line: S06 and S03 (SCENARIO-02)

## Traps
- `queryFailure` empty-query arm must return `errSQLNeedsQuery` (a `cli.UsageError`): `exitCode` in `cmd/quarry/run.go` maps it to exit 2; another type changes the exit code (SCENARIO-01)
- findings `--json` pins in `run_findings_json_test.go` use `JSONEq` — green after a field reorder; only `cmd/quarry/run_shared_documents_test.go` sees key order (SCENARIO-01)
- `document.DateLayout` is used by text renderers (`render.go`, `render_status.go`) as well as JSON (SCENARIO-01)
- `marshalDocument`'s `// unreachable:` encode-failure claim depends on `document.NewSQL` being the only producer of sql cell values; a new cell kind extends it there (SCENARIO-01)
- `cli.Execute` rewrites any RunE error that is not `UsageError`, `ReportedError` or `*runtimeError` into a usage error with exit 2; the mcp RunE wraps in `runtimeError` (SCENARIO-02)
- A nil `Env.ServeMCP` panics in `cli/mcp.go` RunE like other nil factories; tests of `mcp` must supply it (SCENARIO-02)
- An MCP acceptance test without a deadline hangs 10 minutes on a wiring regression (SCENARIO-02)

## Open debts
- PRD `docs/initial-prd.md` §Security "Redaction on import" bullet and Risks table "masking on import" mitigation are now false: redaction deferred by user 2026-10-02. Do not edit silently; PRD §Decisions entry added when user confirms wording — unowned until then
- S01 comment MINORs (comments name MCP consumers not built yet; S02 makes none true): `internal/report/document/doc.go:1-2` -> S03, `internal/report/sql_conventions.go:3-4` -> S07, `internal/report/document/status.go:12` -> S09; each re-checks its comment when its consumer lands
- No direct unit test on `report.SQLConventions`; pinned only through the `sql --help` byte pin — unowned, MINOR
- Checkpoint S01 MINOR: `internal/report/query_failure_test.go:12` pins precedence only QueryError over Interrupted; unprintable vs QueryError and read-only vs external-access order unpinned (add rows if constructible) — unowned
- Checkpoint S01 MINOR: `internal/report/document/findings_test.go:42` two behaviours under one "and" name; `assert.Empty(fixedEntry.Items)` on a finding given no items proves nothing — split, give items — unowned
- Checkpoint S01 MINOR: `internal/report/document/status_test.go:41` derefs `*read.Findings.Ignored` without `require.NotNil` — unowned
