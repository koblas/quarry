# mcp-install — current state

Scenarios complete: SCENARIO-01 (folds 02, 03, 09, 09b, 15, 16, 19, 21, 22), SCENARIO-04 (folds 05, 25), SCENARIO-06 (folds 07, 08, 14, 23, 24). Last updated by SCENARIO-06.

## Binding decisions
- claudeplugin returns values and typed errors (`*ExitError`, `*InterruptedError`, `*ListUnreadableError`, `ErrForeignMarketplace`, `ErrClaudeNotFound`, the Runner's own); all copy lives in `internal/cli/render_claude.go`. The exit/signal line is composed from fields (`Argv`, `Status`, `Signal.String()`, `len(Output)`) by `claudeStepFailureLine(verb, lead, exit)`, never from `Error()`; the unreadable line still uses `Error()`. SCENARIO-10 renders the same errors under the uninstall prefix (SCENARIO-01, 04, 06)
- `*ExitError.Signal os.Signal` (nil = exited with `Status`); exit and signal share one renderer arm, so uninstall's partial/signal/empty variants reuse it unchanged (SCENARIO-06)
- `*InterruptedError{Argv}` unwraps the ctx's error. `(*Server).run` checks ctx before each child (the next-due command) and classifies any Runner error while ctx is done as interrupted before looking at `*toolrun.SignalError`; SCENARIO-10's R5 depends on both checks living in the shared `run`. The R5 line has no lead and no replay, with or without a step done (SCENARIO-06)
- Runtime failures: cli writes replay + line to stderr, returns `ReportedError`; a bare error would become an exit-2 UsageError (`run.go:85`), so an unclassified Runner error (and `*StartError` outside the cannot-run arm) returns as `&runtimeError{err}` — the `default:` arm of `reportClaudeFailure`; `assert.Same` pins in `install_test.go` / `state_test.go` rely on bare return (SCENARIO-01, 04, 06)
- `readState`: marketplace list, then plugin list, each once; a marketplace-list failure skips the plugin list; foreign check runs after both lists parse; SCENARIO-10 reuses it unchanged (SCENARIO-01)
- `Install` returns the steps already done (`Result`) alongside its error; on failure done steps print on stdout first, skipped steps print nothing. The lead is built only by `installDoneLead(res)`; SCENARIO-10 adds its uninstall twin (SCENARIO-01, 04, 06)
- Identity is exact and case-sensitive (`name`, `source`, `repo`). Only a JSON `false` in `enabled` turns the copy off; missing/null/non-bool counts as on, never R4 (SCENARIO-01)
- BR-8: `root.go` `newClaudeCommand` passes no store, snapshots or config factory (SCENARIO-01)
- `toolrun.Run` (matches `claudeplugin.Runner`) owns the Runner error types: `*StartError{Path, Err}`, `*SignalError{Signal}` (ctx live), `ctx.Err()` whenever ctx is done; status -1 on any err, a non-zero exit is a nil err. `claudeplugin` imports `internal/platform/toolrun` for `*SignalError` only (feature -> platform edge) (SCENARIO-04, 06)
- The Runner's `name` is the path `LookPath("claude")` resolved; `ExitError.Argv` stays `claude …`. Nil LookPath means name `"claude"`, no quarry check; the wiring test (`cmd/quarry/run_claude_wiring_test.go`) proves production sets it. `Install` decides on `err != nil`, never `path == ""` (SCENARIO-04)
- `Env.Home` rendered through `claudePath(home, p)` (raw when home is ""); claudeplugin has no home. SCENARIO-12/26 reuse both for R6 (SCENARIO-04)
- `reportClaudeFailure(cmd, verb, home, lead, done, err)` takes the verb and lead as arguments: SCENARIO-10 passes `"uninstall"` and its own lead (`uninstalled the quarry plugin, but `) and gets partial/first-step/signal/empty/R5 lines unchanged (SCENARIO-04, 06)
- cli fakes record every ctx; `runClaudeAt` owns a marker on the command ctx and asserts it on each recorded ctx; both fakes answer from their parameter ctx. SCENARIO-10/12 tests that go through `runClaudeAt` inherit the pin (SCENARIO-06)

## Left unbuilt
- `ErrClaudeNotFound` copy is install-only (`installClaudeNotFoundRefusal`); uninstall's not-found and cannot-run lines — SCENARIO-10
- Uninstall lead `uninstalled the quarry plugin, but ` and the uninstall R5 call site — SCENARIO-10
- R7 beyond the cannot-run path, `WithHome`-free home handling in other lines — SCENARIO-12/26
- Group unknown-subcommand `Args` + RunE, group Long pin, 27/27b — SCENARIO-17
- `uninstall`, and root.go doc text `claude (with its install and uninstall children)` — SCENARIO-10
- No cmd/quarry R5 test: `signal.NotifyContext` -> `cli.Execute` ctx is existing wiring and toolrun proves kill-on-cancel — unowned, accepted

## Traps
- `toolrun.SignalError.Error()` is `stopped by signal: killed` (colon) — render `Signal.String()`, never `Error()` (SCENARIO-06)
- `run` keeps the output the Runner returned with `*SignalError` (toolrun returns the buffer with the error); dropping it loses the replay (SCENARIO-06)
- A fake returning the test's captured ctx's `Err()` passes with propagation broken; answer from the parameter ctx (SCENARIO-06)
- `homepath.Abbreviate("", "/opt/claude")` returns `"~/opt/claude"`; guard stays in `claudePath` (SCENARIO-04)
- `exec.LookPath` can return a path together with `ErrDot`, and wraps every failure in `*exec.Error`; fakes must too (SCENARIO-04)
- After cancel, `CommandContext`'s SIGKILL reads as "killed": `toolrun` checks ctx before classifying a signal. `ErrWaitDelay` after a clean exit is success (SCENARIO-04)
- `cmd/quarry` `testEnv` has a RunTool fake that errors and a LookPath fake returning `*exec.Error{ErrNotFound}`, so no cmd/quarry test starts a child; only the wiring test runs a real file via `runProcess`, no `t.Parallel` (SCENARIO-04)
- cli test fakes: `toolCalls` (records name and ctx, `script`, `cancelBefore`, `cancel`), helpers `runClaudeAt` (home, marker), `findsAt`, `cannotStart`; `errNoStart` is a bare string error, wrong shape for a cannot-run fake. Direct `cli.Execute` callers (the acceptance test, `failingWriter` tests) are not marker-pinned (SCENARIO-04, 06)
- `noArgs` (`errors.go:15`) lacks the `; Run '...' for usage.` suffix the ruled refusal needs
- Until SCENARIO-17, `quarry claude bogus` prints group help and exits 0; do not pin it
- Green on arrival: SCENARIO-28's root-help row (17 only names `Test_run_help_prints_quarrys_description`)
- R5 has no lead even when the add ran; if that reads wrong it is a copy-ruling question, not a developer choice (SCENARIO-06)
- Pins other scenarios must re-assert: root-help claude row `cmd/quarry/run_status_test.go:145-162` (SCENARIO-17 rewrites the `claude` Short; SCENARIO-10 may touch it); `run_usage_test.go` rows `claude install extra` / `claude install --bogus` / subtest `claude install help`; `run_read_usage_test.go` rows `claude install extra|--json|--json extra` (SCENARIO-10 adds the uninstall twins)

## Open debts
- `internal/claudeplugin/doc.go` says the package "installs and removes"; removal arrives with SCENARIO-10
- `toolrun.run` nil `cmd.ProcessState` (wait4 ECHILD) returns the raw err via the `// unreachable:`-marked final return; guarded but no test provokes it — unowned, dies unless re-opened
- `toolrun` grandchild test through real `Run` takes the full 2s `waitDelay` — unowned, accepted cost
