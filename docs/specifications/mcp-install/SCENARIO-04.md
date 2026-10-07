---
id: SCENARIO-04
status: open
---

# SCENARIO-04: Install warns when quarry is not on PATH

Cadence: code-first — the `toolrun` exec adapter is not on the mandatory set: it writes no file, guards nothing and makes no atomic or exclusive-create claim. Nothing else in the scope is on the set either.
Acceptance test: `internal/cli/claude_install_test.go` `Test_claude_install_warns_when_quarry_is_not_on_the_path`
Acceptance test (SCENARIO-05, folded): `internal/cli/claude_install_test.go` `Test_claude_install_refuses_when_claude_is_not_on_the_path`
Acceptance test (SCENARIO-25, folded): `internal/cli/claude_install_test.go` `Test_claude_install_reports_a_claude_it_cannot_run`
Narrow loop: `go test ./internal/platform/toolrun/ ./internal/claudeplugin/ ./internal/cli/ -run 'Run|Install|Claude|claude'` plus `go test ./cmd/quarry/ -run 'claude|Claude'`
Mutation checks: delete `LookPath: exec.LookPath` in `defaultEnv` → `Test_the_shipped_claude_install_reports_a_claude_it_cannot_run`; delete `RunTool: toolrun.Run` in `defaultEnv` → same test; drop the `LookPath("claude")` early return in `Install` → `Test_claude_install_refuses_when_claude_is_not_on_the_path` (zero runner calls); swap the ctx-before-signal check in `toolrun` → `Test_run_returns_the_context_error_when_cancelled_mid_run`
Runs: A (1-2) | B1 (3-5) | B2 (6-7) | V (8-9)
Size: OWNS A RUN — 5 batches, 1 feature package (`internal/claudeplugin`) + 1 new platform package (`internal/platform/toolrun`)

User-visible contract (`## Surface & Copy`, verbatim):
- Warning: stderr after the stdout lines, after the R2 hint if one prints. Exit 0.
- Not-found refusal (install form): stderr only, stdout empty, exit 1. Fires on any LookPath error.
- Cannot-run line: exit 1. The `%q` path follows R7 (`~` form). The reason in parentheses is `osreason.Reason` of the start error. When the marketplace add already ran, the line takes the `added the quarry marketplace, but ` prefix.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `internal/cli/claude_install_test.go` `Test_claude_install_warns_when_quarry_is_not_on_the_path` — fake LookPath finds `claude` and fails `quarry`; nothing installed. Assert the stdout lines, the warning as the only stderr line, nil err. Must fail at the stderr assertion.
- [ ] Step 2: signature-only stubs:
  - `claudeplugin.LookPath` type and `WithLookPath` (`claudeplugin.go:15-35`).
  - `Result.QuarryNotOnPath` (`:37-42`).
  - `cli.Env` fields `LookPath` and `Home` (`internal/cli/run.go:46-57`).
  - `newClaudeCommand` (`claude.go:13`) and `newClaudeInstallCommand` (`claude_install.go:13`) take both values. Call site `root.go:44` still passes no store, snapshots or config factory (BR-8).

### Build
- [ ] Step 3: new `internal/platform/toolrun` — `doc.go`, `toolrun.go` `Run` + `*StartError{Path, Err}`, and `toolrun_test.go`.
  - `Run` matches `claudeplugin.Runner`. It uses `exec.CommandContext` with nil Stdin (the null device) and one shared buffer for Stdout and Stderr.
  - Exit status N comes back with a nil err.
  - When `Start` fails and ctx is live, `Run` returns `*StartError` wrapping the `*fs.PathError` unchanged.
  - Helper process: a `TestMain` dispatches on its first argument, so no env var is needed. It re-runs the test binary and never runs `claude`.
  - Rows:
    - `Test_run_keeps_stdout_and_stderr_in_arrival_order`
    - `Test_run_returns_the_exit_status_with_no_error` (0 and 3)
    - `Test_run_gives_the_child_an_empty_stdin`: the helper checks `os.SameFile(stdin, os.DevNull)`.
    - `Test_run_reports_a_file_it_cannot_start`: rows for a non-executable file (EACCES) and a missing path (ENOENT). Assert `errors.As` `*fs.PathError` and `StartError.Path`.
- [ ] Step 4: `toolrun.go` — cancellation, signal, WaitDelay.
  - Return `ctx.Err()` whenever ctx is done when `Start` or `Wait` returns an error. Check ctx before classifying a signal, because `CommandContext` kills with SIGKILL itself.
  - `*SignalError{Signal os.Signal}` when the child was signalled and ctx is live.
  - `cmd.WaitDelay` is a named const; tests pass it through an unexported `run(ctx, waitDelay, …)`, never a package var.
  - `exec.ErrWaitDelay` after a clean exit is a success.
  - On any err the exit code is -1.
  - Rows:
    - `Test_run_returns_the_context_error_when_cancelled_mid_run` (not `*SignalError`)
    - `Test_run_returns_the_context_error_when_cancelled_before_start` (not `*StartError`)
    - `Test_run_reports_a_signal_it_did_not_send`: the helper SIGKILLs itself; assert `Signal.String() == "killed"`.
    - `Test_run_returns_when_a_grandchild_holds_the_output_open`: the child exits 0 and a grandchild holds stdout. Assert status 0, nil err, and return within the bound.
- [ ] Step 5: `internal/claudeplugin` — the LookPath port.
  - Code sites:
    - `claudeplugin.go:5-8`: `Runner` doc gains the signal and ctx error cases.
    - `claudeplugin.go:49-73`: `Install` checks `LookPath("claude")` before `readState`. On any error it returns new `ErrClaudeNotFound` wrapping the lookup error, decided on `err != nil` and never on `path == ""`.
    - The resolved path is the `name` passed to every Runner call: thread it through `readState`/`list`/`run` (`state.go:60-70,97,130-139`).
    - `LookPath("quarry")` runs only after both steps succeed or were skipped. Any error sets `QuarryNotOnPath`.
    - Nil LookPath means name `"claude"` and no checks; the STATE decision holds.
  - Fake: `fake_test.go:49-59` records `name` and gains a fake LookPath. Pin `name ==` the LookPath result, and `"claude"` when LookPath is nil (closes the Open debt).
  - Rows in `install_test.go`:
    - `Test_install_refuses_before_any_child_when_claude_is_not_found`: rows for `&exec.Error{Name:"claude", Err: exec.ErrNotFound}`, a path returned together with `exec.ErrDot`, and `fs.ErrPermission`.
    - `Test_install_runs_each_child_at_the_path_lookpath_found`
    - `Test_install_flags_quarry_missing_only_after_success`: control row where quarry is found; a failure row with no quarry lookup.
    - `Test_install_returns_the_start_error_from_a_step_unchanged`
- [ ] Step 6: `internal/cli` — copy and wiring.
  - `claude_install.go:34-50` passes `WithLookPath(lookPath)`. It writes the warning after the R2 hint, using a new `const` beside `render_claude.go:12-22`.
  - `reportClaudeFailure` (`render_claude.go:57-78`) adds two arms:
    - `ErrClaudeNotFound` gives the install not-found refusal.
    - `*toolrun.StartError` gives the cannot-run line. The path comes from a new `claudePath(home, p)`, which returns `p` raw when home is `""` and otherwise uses `homepath.Abbreviate`. The reason comes from `osreason.Reason(start.Err)`.
  - New `installDoneLead(res)` builds `added the quarry marketplace, but ` from `res.MarketplaceAdded`. SCENARIO-06 reuses it.
  - Update `runClaude` (`claude_install_test.go:68-75`) and add a test LookPath helper. Existing literals at `:80,:252,:263,:274` keep nil LookPath.
  - `errNoStart` (`:28`) stays only for `:282`, which is now the `default:` arm (unclassified Runner error leads to `runtimeError`). The cannot-run fakes inject `&toolrun.StartError{Path: p, Err: &fs.PathError{Op: "fork/exec", Path: p, Err: syscall.EACCES}}`.
  - Rows:
    - Step-1 test, plus `Test_claude_install_warns_after_the_turned_off_hint` (R2 then the warning, both after stdout).
    - `Test_claude_install_warns_when_quarry_is_not_on_the_path_and_nothing_ran` (both skipped).
    - SCENARIO-05 test: table over the three LookPath error shapes; zero runner calls, empty stdout, `ReportedError`.
    - SCENARIO-25 test: path under `$HOME` gives `"~/…"`.
    - `Test_claude_install_reports_a_claude_it_cannot_run_after_adding_the_marketplace`: partial prefix, with `Added…` on stdout.
    - `Test_claude_install_reports_a_claude_it_cannot_run_at_a_path_outside_home`: home `""` and a path outside home, both printed raw.
- [ ] Step 7: `cmd/quarry` — real wiring and fakes.
  - `run.go:189-203` `defaultEnv`: `RunTool: toolrun.Run`, `LookPath: exec.LookPath`, `Home` from `os.UserHomeDir` (`""` on error).
  - `main_test.go:27-31` `testEnv`: a RunTool fake that returns an error, and a LookPath fake that returns `&exec.Error{Err: exec.ErrNotFound}`, so no cmd/quarry test can start a child.
  - New `run_claude_wiring_test.go` `Test_the_shipped_claude_install_reports_a_claude_it_cannot_run`, modelled on `run_sync_wiring_test.go:24-43` (no `t.Parallel`):
    - `t.Setenv` HOME to a tempdir and PATH to `HOME/bin`, which holds `claude` with mode 0755 and contents without a shebang.
    - Call `runProcess`.
    - Assert exit 1 and the exact line `…cannot run claude at "~/bin/claude" (exec format error)…`.

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`. A gosec G204 on `toolrun.Run` gets a named `//nolint:gosec // <reason>`. Doc comments on `Run`, `StartError`, `SignalError`, `LookPath`, `WithLookPath`, `ErrClaudeNotFound`, `QuarryNotOnPath` and `Env.Home`.

### Verify
- [ ] Step 9: run `verify.sh <start> ./internal/platform/toolrun/... ./internal/claudeplugin/... ./internal/cli/... ./cmd/quarry/...`, then `spec-check.py mcp-install`.
  - Tick SCENARIO-04 on its Progress line, with folds 05 and 25 named and the acceptance test last.
  - Rewrite STATE.md and close the `name` Open debt.

## Handoff

**Binding decisions:**
- **`toolrun` owns the Runner error types:**
  - `*StartError{Path, Err}` when the child could not start.
  - `*SignalError{Signal}` when a signal ended it and ctx was live.
  - `ctx.Err()` whenever ctx is done when Start or Wait returns an error.
  - On any err the exit code is -1. A non-zero exit has a nil err.

  SCENARIO-06 classifies "stopped by signal" and R5 from exactly these, so it does not re-plan the adapter. This amends the spec Seams line "err only when start failed or ctx cancelled".
- **The Runner's `name` is the path LookPath resolved**, so the cannot-run `%q` is the file exec tried. `ExitError.Argv` stays `claude …` built from `claudeCommand`, because the ruled copy names the command, not the path.
- **Home is a cli value (`Env.Home`) rendered through `claudePath`.** Copy lives in `render_claude.go`, so claudeplugin gets no `WithHome`. SCENARIO-12/26 reuse `Env.Home` and `claudePath` for R6.
- **Nil LookPath means name `"claude"` and no checks.** The wiring pin, not a nil check, proves production sets it.
- **`installDoneLead(res)` is the one place the partial prefix is built.** SCENARIO-06's partial "exited" line must reuse it.

**Left unbuilt:**
- How `*toolrun.SignalError` and ctx errors render. Until then they reach the `default:` arm as `quarry: <err>` (SCENARIO-06).
- Uninstall's not-found and cannot-run lines (SCENARIO-10). `reportClaudeFailure` takes the verb, but the not-found copy differs by verb.

**Traps:**
- `homepath.Abbreviate("", "/opt/claude")` returns `"~/opt/claude"`, because an empty home makes every absolute path match. Guard in `claudePath`; do not change the shared helper.
- `exec.LookPath` can return a non-empty path together with `ErrDot`. Decide on err only.
- `exec.ErrWaitDelay` after a clean exit is not a failure.
- After cancel, `CommandContext`'s SIGKILL makes the process state read "killed". Check ctx first.
- `errNoStart` (`claude_install_test.go:28`) is a bare string error, the wrong shape for a cannot-run fake.

## Phase report
