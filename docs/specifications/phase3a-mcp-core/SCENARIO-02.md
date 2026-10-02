---
id: SCENARIO-02
status: done
---

# SCENARIO-02: An MCP client connects and sees quarry's four tools

Cadence: code-first (no write guard, no atomic adapter; the server is read-only and its handlers are stubs)
Acceptance test: `cmd/quarry/run_mcp_test.go` `Test_run_mcp_lists_quarrys_four_tools_over_json_rpc`
Narrow loop: `go test ./internal/mcp/ ./internal/cli/ -run 'MCP|Mcp|Serve|Tool|Root' && go test ./cmd/quarry/ -run 'Mcp|MCP|help_and_usage'`
Mutation checks: `ServeMCP` line in `defaultEnv` (`cmd/quarry/run.go:160-171`) deleted → `Test_run_mcp_lists_quarrys_four_tools_over_json_rpc`; `IOTransport` swapped for `StdioTransport` in `(*Server).Serve` → same test (fails at its deadline, never hangs); `(devel)` fallback removed → `Test_serve_identifies_an_unversioned_build_as_devel`
Runs: A (1-3) | B1 (4-6) | V (7-8)
Size: OWNS A RUN — 3 batches, 0 feature packages (`internal/mcp` is a delivery peer of cli) + cli + cmd/quarry wiring

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `go.mod:5-11`, `go.sum` — `go get github.com/modelcontextprotocol/go-sdk@v1.8.0`. Try `GOPROXY=off` first, because `go env GOMODCACHE` already holds v1.8.0 and jsonschema-go v0.4.3. Fall back to the network: proxy.golang.org, sum.golang.org. `go mod tidy` runs only after code imports it (Sweep).
- [x] Step 2: `cmd/quarry/run_mcp_test.go` (new) `Test_run_mcp_lists_quarrys_four_tools_over_json_rpc`:
  - `runWith(ctx, {"mcp"}, testEnv(...))` runs in a goroutine with `Stdin`/`Stdout` on `io.Pipe` pairs. Stdout is teed into a buffer. The SDK client uses `IOTransport` over the other ends.
  - The client connects (initialize) and calls ListTools, then closes the session and waits for `runWith` to return.
  - Asserts:
    - `ServerInfo` is `{Name:"quarry", Version:<what the wiring produces under go test>}`.
    - `Instructions` is verbatim from §2.1.
    - Exactly 4 tools. Each has its description verbatim (§2.4–2.7), its `InputSchema` JSONEq to a literal, and `OutputSchema` JSONEq to `{"type":"object"}`.
    - Every captured stdout line unmarshals with `"jsonrpc":"2.0"`, and stderr is empty.
  - One timeout ctx bounds connect, list and the wait on `runWith`. Exit code is not asserted (S15 owns it).
- [x] Step 3: signature-only stubs so the test compiles:
  - `internal/mcp/doc.go` (new), plus `internal/mcp/server.go` (new) with `Server`, `Option`, `NewServer`, `WithVersion`, and `(*Server).Serve(ctx, stdin io.Reader, stdout, stderr io.Writer) error` returning nil.
  - `internal/cli/run.go:14-45`: type `MCPServeFunc` (same signature) and field `Env.ServeMCP`.
  - Red: the client's initialize fails because there is no `mcp` command yet. Quote it.

### Build
- [x] Step 4: `internal/mcp/server.go`, `internal/mcp/tools.go` (new), `internal/mcp/server_test.go` (new):
  - SDK imported under an alias (`sdk`). `Serve` runs `sdk.NewServer(&Implementation{Name:"quarry", Version}, &ServerOptions{Instructions})` over `sdk.IOTransport`. The Env streams are wrapped in no-op closers. Pass no Logger.
  - `WithVersion("")` keeps the `(devel)` default.
  - `instructions` and the 4 description consts are verbatim.
  - The query description is the §2.4 block plus one new last line: `Send one statement; if you send several, only the last one's rows come back.` S01's last-statement pin is at `cmd/quarry/run_shared_documents_test.go:32-35`. Where this line goes is unruled, so get a copy ruling before run A.
  - The 4 tools are registered via `sdk.AddTool[In, any]`, each with an explicit `*jsonschema.Schema` and `OutputSchema {"type":"object"}`:
    - query: `sql` string minLength 1 (required), `limit` integer 1..500 default 500.
    - describe_schema and sync_status: empty object.
    - data_quality: `status` enum open|ignored|fixed|all default open; `type` enum built from `finding.Types()`; `limit` 1..500 default 50.
    - Every schema sets `additionalProperties:false`.
  - One shared `notBuilt` handler for all 4 returns an error.
  - Tests:
    - `Test_serve_identifies_an_unversioned_build_as_devel`: table of `""` → `(devel)` and `v1.2.3` → itself.
    - `Test_serve_returns_nil_when_the_client_closes_stdin`.
    - `Test_serve_returns_the_write_error_when_stdout_fails` (fault): a failing writer; assert `errors.Is` its sentinel, and that the error is not wrapped.
    - `Test_an_unbuilt_tool_answers_isError` (stub).
- [x] Step 5: `internal/cli/mcp.go` (new) `newMCPCommand`, `internal/cli/root.go:6-10,25-34`, `internal/cli/mcp_test.go` (new):
  - Command: Use/Short/Long verbatim (§2.1), no Example, no flags. RunE calls `serve(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())` and returns its error as `&runtimeError{err}`.
  - `root.go`: register the command, and name `mcp` in the doc comment (STATE debt).
  - Tests:
    - `Test_mcp_help_prints_the_ruled_long_text`: Long asserted verbatim at wrap width.
    - `Test_root_help_lists_mcp_with_its_short_line`.
    - `Test_mcp_hands_the_env_streams_to_the_server`: a recording `ServeMCP` fake.
    - `Test_mcp_returns_a_server_error_unwrapped_not_as_usage` (fault).
- [x] Step 6: `cmd/quarry/run.go:79-101,158-171`, `cmd/quarry/run_usage_test.go:277-306`:
  - `run.go`: `newMCPServe() cli.MCPServeFunc` does `mcp.NewServer(mcp.WithVersion(buildVersion(info))).Serve` from `debug.ReadBuildInfo`, the same source as `store_info.quarry_version` (`:66`). Wire `ServeMCP` in `defaultEnv`.
  - `run_usage_test.go`: add an "mcp help" subtest to `Test_run_help_and_usage_errors_do_not_need_home`.
  - Acceptance goes green. Run the three mutation checks from the header.

### Sweep
- [x] Step 7: `go mod tidy` (jsonschema-go becomes a direct require). Then fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`. Add doc comments on `MCPServeFunc`, `Env.ServeMCP`, `mcp.Server`/`NewServer`/`WithVersion`/`Serve`, and `newMCPServe`. Check with `go doc ./internal/mcp`.

### Verify
- [x] Step 8: full verification + `spec-check.py phase3a-mcp-core` → tick SCENARIO-02 with its acceptance test. Rewrite STATE.md:
  - Drop the `root.go:6-10` debt.
  - Re-own the S01 comment MINORs: `document/doc.go:1-2` → S03, `report/sql_conventions.go:3-4` → S07, `document/status.go:12` → S09. S02 makes none of them true yet.

## Handoff

**Binding decisions:**
- `internal/mcp` never imports `internal/cli`, and cli never imports mcp. cli reaches the server only through `Env.ServeMCP` (`MCPServeFunc`), and `cmd/quarry` wires it. Breaking this makes mcp → report a feature-imports-feature violation (Rule 2).
- The transport is `sdk.IOTransport` over the streams cli passes in, wrapped in no-op closers. `StdioTransport` and `slog.Default()` are banned (Rule 9). Closing the real stdout would break S15's EPIPE/EOF exits.
- `Serve` returns `Run`'s error unwrapped. S15 maps `nil` (EOF), `context.Canceled` and `syscall.EPIPE` to exit 0 by `errors.Is`.
- Every input schema is hand-written with `additionalProperties:false`, and `tools/list` pins it. Unknown params are refused, as the sizing probe verified with inferred schemas. S03, S07, S09 and S11 keep that field when they replace handlers.
- Instructions and descriptions keep the spec code blocks' hard line breaks, with no trailing newline. Later re-pins compare against those bytes.
- The `type` enum is built from `finding.Types()`, never hand-typed. The acceptance test lists the 8 names literally, so a drift is seen.
- Tool results will use `Out = any` with `OutputSchema {"type":"object"}` (Rule 10). Handlers set `StructuredContent: json.RawMessage`.

**Left unbuilt:**
- `notBuilt` (internal/mcp/tools.go), the shared stub handler. S03 (query), S07 (describe_schema), S09 (sync_status) and S11 (data_quality) replace it. The last one deletes it and `Test_an_unbuilt_tool_answers_isError`.
- `mcp.WithReport` / `mcp.WithConfig` options and their cmd/quarry wiring through `newReportFactory`/`newConfigLoader`: S03, S09 and S11.
- Schema-validation pins (limit 0/501 refused, default applied, `sql:" "` reaching the handler): S03 for query, S11 for data_quality.
- `quarry mcp extra` and `quarry mcp --json` still *serve* in S02 (no `Args: noArgs`, no refusal). S15 adds the rows: the `run_read_usage_test.go:16` table, `run_usage_test.go:137` and `:220`, `internal/cli/errors.go:14 noArgs`, the `$HOME` check, and the exit mapping. The TTY hint is S16/S15.
- The per-call timeout option and the stderr error line: S06 and S03.

**Traps:**
- `cli.Execute` (`internal/cli/run.go:60-74`) rewrites any RunE error that is not `UsageError`, `ReportedError` or `*runtimeError` into a usage error with exit 2. The mcp RunE must wrap its error in `runtimeError`.
- Our package and the SDK package are both named `mcp`. Alias the SDK import.
- Under `go test`, `debug.ReadBuildInfo().Main.Version` may be `""`, `(devel)` or a VCS pseudo-version. Check it before writing the acceptance literal.
- An acceptance test without a deadline hangs for 10 minutes on any wiring regression.

## Phase report

Run V done (steps 7-8). `GOPROXY=off go mod tidy` promoted jsonschema-go to a direct require; lint 0 issues; `go doc ./internal/mcp` shows doc comments on every exported symbol. Full suite rc=0, `uncovered-diff.py` 0 uncovered, `-race` green on mcp/cli/cmd. One missed consumer fixed: root help pin `cmd/quarry/run_status_test.go` `Test_run_help_prints_quarrys_description` gained the `mcp` row. Spec ticked, `spec-check.py` OK, STATE.md rewritten (root.go debt dropped, S01 comment MINORs re-owned to S03/S07/S09, null-arguments trap and `absentNullArguments` guard recorded as binding for S03/S11).
