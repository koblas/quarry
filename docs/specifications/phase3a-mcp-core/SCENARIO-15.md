---
id: SCENARIO-15
status: done
---

# SCENARIO-15: quarry mcp ends with the ruled exit code (SCENARIO-16 folded: TTY hint)

Cadence: code-first (no mandatory test-first item: no write guard, no atomic adapter)
Acceptance test: `cmd/quarry/run_mcp_exit_test.go` `Test_run_mcp_ends_with_the_ruled_exit_code`
Acceptance test (SCENARIO-16, folded): `cmd/quarry/run_mcp_terminal_test.go` `Test_run_mcp_at_a_terminal_prints_the_hint_and_keeps_serving`
Narrow loop: `go test ./internal/cli/ -run 'MCP|Mcp' && go test ./cmd/quarry/ -run 'Mcp|MCP|Usage|Help|Terminal'`
Mutation checks: EPIPE arm in `mcpStopped` → the "client stops reading" row; `context.Canceled` arm → the "SIGTERM" row; `resolveHome("mcp")` call in `newMCPServe` → the "$HOME unset" row; `signal.Ignore(syscall.SIGPIPE)` line in `newMCPServe` → `Test_quarry_mcp_exits_0_when_the_real_stdout_pipe_breaks`; `IsTerminal: isTerminal` line in `defaultEnv` → `Test_defaultEnv_probes_stdin_for_a_terminal`; termios probe swapped for `ModeCharDevice` → `Test_isTerminal_reports_dev_null_as_no_terminal`
Runs: A (1-2) | B1 (3-5) | V (6-7)
Size: OWNS A RUN — 3 batches, 0 feature packages (cli + cmd/quarry wiring); absorbs SCENARIO-16

## Implementation Plan

Surface (spec §2.1, verbatim, no new copy): stdout protocol only; usage lines `quarry: mcp takes no arguments` / `quarry: mcp always speaks JSON on stdout; drop --json` (exit 2, `UsageError`, no `Run ... --help` suffix); `$HOME` line = `resolveHome("mcp")` (exit 1); EOF / ctx cancel / EPIPE → exit 0, stderr empty; TTY hint `quarry: mcp: this is an MCP server for Claude and other MCP clients; it reads JSON-RPC on stdin. Press Ctrl-D to stop.` + `\n`, then serving continues.
Design: stop-mapping lives in `cli` `mcp.go` RunE (delivery policy; `mcp.Serve` and its pins `server_test.go` "returns the write error" / "returns the context error" stay untouched); `$HOME` check in `cmd/quarry` `newMCPServe` (only place that owns `resolveHome`). Ordering ruled: `$HOME` check precedes the TTY probe and hint, so TTY + `$HOME` unset prints only the `$HOME` line. Mechanism (B1 owns): `MCPServeFunc` gains a trailing `ready func()` parameter; cli RunE passes a `ready` that probes `env.IsTerminal(stdin)` (nil-safe) and prints the hint to stderr; the `newMCPServe` closure calls `resolveHome` first, returns its error without calling `ready`, then calls `ready()` and `Serve`. Existing cli fakes of `MCPServeFunc` (`mcp_test.go`) take the new parameter. No new port beyond one `Env` func field.

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_mcp_exit_test.go` `Test_run_mcp_ends_with_the_ruled_exit_code` — table over `runWith(testEnv)`, 7 rows (the 7th: `$HOME` unset at a terminal → only the `$HOME` line; "told to stop" uses a pre-cancelled ctx; stdin closed; ctx cancelled while serving [SIGTERM stand-in; signal→ctx is `signalContext`'s own pin]; stdout writer returns `*os.PathError` wrapping `syscall.EPIPE` after a `ping` request on stdin; positional arg; `--json`; `HOME=""`); each asserts exit code, stderr verbatim, stdout empty for refusals. Own deadline ctx (`mcpTestDeadline`, `run_mcp_test.go:17`); extract the pipe/goroutine scaffold from `run_mcp_test.go:84-100` if reuse is cheaper than copying. Rows 1-3 and 6 red now only via exit 1 / wrong stderr; must fail at the assertion, not hang
- [x] Step 2: `cmd/quarry/run_mcp_terminal_test.go` `Test_run_mcp_at_a_terminal_prints_the_hint_and_keeps_serving` — `env.IsTerminal = always true`, real client connects over pipes (session initializes = still serving), stderr == hint line only, closing session → exit 0. Stub so it compiles: `cli.TerminalProbe` type + `Env.IsTerminal` field at `internal/cli/run.go:34-58` (signature only; done). Shared scaffold `startMCP`/`waitForExit` extracted into `run_mcp_test.go`

### Build
- [x] Step 3: `internal/cli/mcp.go:9-32` `newMCPCommand(serve, jsonOut)`, `root.go:38` — `Args: noArgs` (`errors.go:14`); RunE first refuses `*jsonOut` with `UsageError`; positional+`--json` → args error wins. Also add `mcp` to every all-commands usage table: `cmd/quarry/run_read_usage_test.go:16` (`Test_run_read_commands_reject_bad_usage`: rows "mcp with an argument", "mcp with --json"), `run_usage_test.go:137` (`Test_run_rejects_usage_errors`), `run_usage_test.go:220` (`Test_run_usage_hint_names_the_matched_command`: `mcp --bogus` → hint names `quarry mcp --help`). `internal/cli/mcp_test.go`: both refusals + control (`mcp` bare still serves) + serve not called on refusal. Grep tests for `takes no arguments` for any table missed
- [x] Step 4: `internal/cli/mcp.go:25-30` `mcpStopped`, `cmd/quarry/run.go:135-139` `newMCPServe` — RunE returns nil for serve error that is nil, `errors.Is(context.Canceled)` or `errors.Is(syscall.EPIPE)` (wrapped forms too); every other error stays `runtimeError` (existing `mcp_test.go:75` control; add `DeadlineExceeded` row = not swallowed, `net.ErrClosed`-style other = exit 1). `newMCPServe` wrapper calls `resolveHome("mcp")` first and returns its error unchanged without calling `Serve`. Then `signal.Ignore(syscall.SIGPIPE)` (mcp path only; Go otherwise kills the process by SIGPIPE on a broken fd 1 write and the EPIPE arm is never reached; other commands keep the default). Real-process pin `Test_quarry_mcp_exits_0_when_the_real_stdout_pipe_breaks` in `run_mcp_exit_test.go`, re-exec pattern of `signal_test.go:50-100` (env-var child runs `runProcess(ctx, ["mcp"], os.Stdout, os.Stderr)` then `os.Exit`; parent closes its stdout read end, writes a `ping`, asserts exit 0 and empty stderr, with deadline) Update doc on `MCPServeFunc` (`run.go:29-32`) to say which returns are a normal stop. Tests: cli arms above through `cli.Execute` with fake `ServeMCP`; `cmd/quarry` unit for `newMCPServe` with `HOME=""` (returns `errNoHome`-wrapped, text verbatim) and with a cancelled ctx (nil path is the acceptance row)
- [x] Step 5: `internal/cli/mcp.go` RunE, `internal/cli/run.go:42-51`, `cmd/quarry/run.go:167-180` `defaultEnv`, new `cmd/quarry/terminal.go` `isTerminal(io.Reader) bool` — `go get golang.org/x/term` (x/term is NOT in the module cache: needs `allowed_domains` proxy.golang.org, sum.golang.org, storage.googleapis.com; may move `x/sys` off v0.41.0; `go mod tidy`). `Env.IsTerminal TerminalProbe` nil-safe (nil = not a terminal; existing cli Env literals stay valid). RunE passes serve a `ready` callback (see Design) that prints the hint to `cmd.ErrOrStderr()` when `env.IsTerminal(cmd.InOrStdin())`; serve calls it after `resolveHome`. Add cli test: `ready` not called by a fake serve → no hint; `MCPServeFunc` doc names `ready`. `isTerminal` = `*os.File` assertion + `term.IsTerminal(int(f.Fd()))`; never `ModeCharDevice`. Tests: cli (probe true → hint then serve called, stderr exactly hint; probe false/nil → silent; probe receives Env stdin); cmd/quarry `Test_defaultEnv_probes_stdin_for_a_terminal` (wiring pin: `defaultEnv(...).IsTerminal != nil`), `newMCPServe` unit: `HOME=""` → `ready` never called, `Test_isTerminal_reports_dev_null_as_no_terminal` (`/dev/null` file, pipe, `bytes.Buffer` → false). Positive tty case only if a pty is constructible in a few lines on darwin; otherwise say so in the phase report

### Sweep
- [x] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `TerminalProbe`, `isTerminal`, `mcpStopped`; run `go mod tidy` and confirm `x/sys` stays at the version `go.mod` pins or note the bump

### Verify
- [x] Step 7: full verification block (`.claude/rules/agent-briefs.md`) + `.claude/scripts/spec-check.py phase3a-mcp-core`; tick SCENARIO-15 and SCENARIO-16 in `specification.md` (S16 line: "delivered by SCENARIO-15", test last on line); rewrite `STATE.md` (drop the S15 line from Left unbuilt, add decisions below)

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Exit policy for a stopped server lives in `cli` `mcp.go`, not `mcp.Serve` — `Serve` keeps returning `Run`'s error unwrapped (S02's pins); S03+ handlers must never return `context.Canceled`/`EPIPE` as a runtime failure of `Serve`
- `$HOME` check is inside `newMCPServe`'s closure before `Serve` — S03/S09/S11 add `WithReport`/`WithConfig` wiring there and keep the check first
- Stop arms swallow only nil, `context.Canceled`, `syscall.EPIPE`; `DeadlineExceeded` from a ctx deadline is a runtime error (S06's per-call timeout is inside the tool call, not Serve's ctx)
- Terminal probe is `Env.IsTerminal` (nil-safe, termios via x/term); hint printed by cli RunE, so with `$HOME` unset and a terminal both lines appear (hint first) — accepted

**Left unbuilt**:
- Positive pty test for `isTerminal` unless constructible — unowned, MINOR
- Process-level SIGTERM → exit 0 end-to-end — `signalContext` tests plus the cancelled-ctx row cover it; no subprocess test

**Traps**:
- `--json` is a persistent root flag bound to `Execute`'s `jsonOut`; `newMCPCommand` must receive that pointer, not read the flag by name
- `UsageError` from RunE is returned verbatim; do not add the `Run 'quarry mcp --help'` suffix (only cobra's own errors get it)
- An in-process fake-EPIPE writer cannot see SIGPIPE death: only the re-exec test proves the real binary exits 0
- `/dev/null` is a char device: `ModeCharDevice` calls it a TTY; a regression only the `/dev/null` test sees
- A test with the stdin pipe open and no deadline hangs 10 minutes on regression; every new test uses `mcpTestDeadline`

## Phase report

Run V done (Steps 6-7). `go build ./...` ok; `go mod tidy` no diff; full covered suite `go test rc=0`; `uncovered-diff.py` 0 uncovered added lines; `go test -race` on cmd/quarry, internal/cli, internal/mcp ok; `golangci-lint run ./...` 0 issues; `test-stats.py --base a288000 --changed`: cmd/quarry 512 (+7) tempdir 457 (+2) disk 418 (+2); internal/cli 425 (+6); TOTAL 937 (+13). `spec-check.py phase3a-mcp-core` and `--run` both OK. S15 and S16 ticked in specification.md; STATE.md rewritten. x/sys v0.41.0 -> v0.48.0 (via x/term v0.46.0): nothing regressed. Doc comments on `TerminalProbe`, `isTerminal`, `MCPServeFunc` present. Not done: positive pty test for `isTerminal` (unowned MINOR, in STATE.md).
