# mcp-install — current state

Scenarios complete: SCENARIO-01 (folds 02, 03, 09, 09b, 15, 16, 19, 21, 22), SCENARIO-04 (folds 05, 25), SCENARIO-06 (folds 07, 08, 14, 23, 24), SCENARIO-10 (folds 11, 13, 05b, 15b, 20). Last updated by SCENARIO-10.

## Binding decisions
- claudeplugin returns values and typed errors (`*ExitError`, `*InterruptedError`, `*ListUnreadableError`, `ErrForeignMarketplace`, `ErrClaudeNotFound`, the Runner's own); all copy lives in `internal/cli/render_claude.go`. The exit/signal line is composed from fields (`Argv`, `Status`, `Signal.String()`, `len(Output)`) by `claudeStepFailureLine(verb, lead, exit)`, never from `Error()`; the unreadable line still uses `Error()`. Install and uninstall render the same errors under their own verb prefix (SCENARIO-01, 04, 06, 10)
- `*ExitError.Signal os.Signal` (nil = exited with `Status`); exit and signal share one renderer arm (SCENARIO-06)
- `*InterruptedError{Argv}` unwraps the ctx's error. `(*Server).run` checks ctx before each child (the next-due command) and classifies any Runner error while ctx is done as interrupted before looking at `*toolrun.SignalError`; the R5 line has no lead and no replay, with or without a step done, for install and uninstall alike (SCENARIO-06, 10)
- Runtime failures: cli writes replay + line to stderr, returns `ReportedError`; a bare error would become an exit-2 UsageError (`run.go:85`), so an unclassified Runner error (and `*StartError` outside the cannot-run arm) returns as `&runtimeError{err}` — the `default:` arm of `reportClaudeFailure`; `assert.Same` pins rely on bare return (SCENARIO-01, 04, 06, 10)
- `readState`: marketplace list, then plugin list, each once; a marketplace-list failure skips the plugin list; foreign check runs after both lists parse and scans every entry (a foreign entry anywhere refuses, ours before or after). It is the whole identity guard for `Uninstall`: both targets (`quarry@quarry`, marketplace `quarry`) are named, never looked up (SCENARIO-01, 10)
- `Install` and `Uninstall` return the steps already done (`Result` / `UninstallResult{PluginUninstalled, MarketplaceRemoved}` + `Ran()`) alongside the error; on failure done steps print on stdout first, skipped steps print nothing. Leads are built only by `installDoneLead(res)` / `uninstallDoneLead(res)` (`uninstalled the quarry plugin, but `); R5 keeps none. `Result` stays install-only (SCENARIO-01, 04, 06, 10)
- `Uninstall` order: `findClaude` -> `readState` -> plugin step iff `st.userCopy` (enabled ignored; turned-off copy still uninstalled) -> marketplace step iff `st.marketplaceOurs`. Never calls `lookPath("quarry")` (SCENARIO-10)
- Refusal copy per verb lives in `claudeRefusalCopy` (`render_claude.go:52`, map keyed by verb: foreign, notFound), read by `reportClaudeFailure`; its signature `(cmd, verb, home, lead, done, err)` is unchanged (SCENARIO-10)
- Identity is exact and case-sensitive (`name`, `source`, `repo`). Only a JSON `false` in `enabled` turns the copy off; missing/null/non-bool counts as on, never R4 (SCENARIO-01)
- BR-8: `root.go` `newClaudeCommand` passes no store, snapshots or config factory (SCENARIO-01)
- `toolrun.Run` (matches `claudeplugin.Runner`) owns the Runner error types: `*StartError{Path, Err}`, `*SignalError{Signal}` (ctx live), `ctx.Err()` whenever ctx is done; status -1 on any err, a non-zero exit is a nil err. `claudeplugin` imports `internal/platform/toolrun` for `*SignalError` only (SCENARIO-04, 06)
- The Runner's `name` is the path `LookPath("claude")` resolved; `ExitError.Argv` stays `claude …`. Nil LookPath means name `"claude"`; the wiring test (`cmd/quarry/run_claude_wiring_test.go`) proves production sets it. `Install`/`Uninstall` decide on `err != nil`, never `path == ""` (SCENARIO-04)
- `Env.Home` rendered through `claudePath(home, p)` (raw when home is ""); claudeplugin has no home. SCENARIO-12/26 reuse both for R6 (SCENARIO-04)
- cli fakes record every ctx; `runClaudeAt` owns a marker on the command ctx and asserts it on each recorded ctx. SCENARIO-12 tests going through `runClaudeAt` inherit the pin (SCENARIO-06)

## Left unbuilt
- BR-6 keep branch (no marketplace remove while a non-user `quarry@quarry` remains), `Kept` line, R6 hints, any non-user copy in `state`, `UninstallResult` keep/remaining-copies fields — SCENARIO-12. The seam is between the plugin step and the marketplace step in `(*Server).Uninstall` (`claudeplugin.go:124-137`); fields go on `UninstallResult`, not `Result`
- Uninstall Long (`claude_uninstall.go`) says "uninstall names each one" for project copies: true only once SCENARIO-12 lands (SCENARIO-10, ruled copy, do not reword)
- R7 beyond the cannot-run path — SCENARIO-12/26
- Group unknown-subcommand `Args` + RunE, group Long pin, 27/27b, `claude` Short rewrite — SCENARIO-17
- No cmd/quarry R5 test: `signal.NotifyContext` -> `cli.Execute` ctx is existing wiring and toolrun proves kill-on-cancel — unowned, accepted

## Traps
- Until SCENARIO-12, a project-scope `quarry@quarry` plus our marketplace makes uninstall run `marketplace remove`; every uninstall fixture is user-scope only — do not pin that row (SCENARIO-10)
- `toolrun.SignalError.Error()` is `stopped by signal: killed` (colon) — render `Signal.String()`, never `Error()` (SCENARIO-06)
- `run` keeps the output the Runner returned with `*SignalError`; dropping it loses the replay (SCENARIO-06)
- A fake returning the test's captured ctx's `Err()` passes with propagation broken; answer from the parameter ctx (SCENARIO-06)
- `homepath.Abbreviate("", "/opt/claude")` returns `"~/opt/claude"`; guard stays in `claudePath` (SCENARIO-04)
- `exec.LookPath` can return a path together with `ErrDot`, and wraps every failure in `*exec.Error`; fakes must too (SCENARIO-04)
- After cancel, `CommandContext`'s SIGKILL reads as "killed": `toolrun` checks ctx before classifying a signal. `ErrWaitDelay` after a clean exit is success (SCENARIO-04)
- `cmd/quarry` `testEnv` has a RunTool fake that errors and a LookPath fake returning `*exec.Error{ErrNotFound}`, so no cmd/quarry test starts a child; only the wiring test runs a real file, no `t.Parallel` (SCENARIO-04)
- cli test fakes: `toolCalls` (`script`, `cancelBefore`, `cancel`, `lists`, `reply`), helpers `runClaude`, `runClaudeAt` (home, marker), `findsAt`, `cannotStart`; `errNoStart` is a bare string error, wrong shape for a cannot-run fake. claudeplugin fakes: `fakeClaude` (`answer`, `cancellable`, reply `cancel`/`ctxErr`), `fakePath`. Interrupt tables use `cancel` alone to mean "the step succeeded, ctx ended after" and `cancel`+`ctxErr` to mean "ended during" (SCENARIO-04, 06, 10)
- `noArgs` (`errors.go:15`) lacks the `; Run '...' for usage.` suffix the ruled refusal needs
- Until SCENARIO-17, `quarry claude bogus` prints group help and exits 0; do not pin it
- Group help padding: cobra pads names to 11, so `uninstall` gets three spaces before its Short, `install` five. `run_status_test.go:145-162` root-help claude row is unchanged by SCENARIO-10; a diff there is a bug (SCENARIO-10)
- Green on arrival: SCENARIO-28's root-help row (17 only names `Test_run_help_prints_quarrys_description`)
- Pins SCENARIO-17 must re-assert: root-help claude row `cmd/quarry/run_status_test.go:145-162` (17 rewrites the `claude` Short); `claude install` and `claude uninstall` rows in `run_usage_test.go` and `run_read_usage_test.go`

## Open debts
- `claudeRefusalCopy[verb]` returns empty strings for an unknown verb (only install/uninstall call it, from `reportClaudeFailure`) — NIT, unowned, dies unless re-opened (SCENARIO-10)
- `toolrun.run` nil `cmd.ProcessState` (wait4 ECHILD) returns the raw err via the `// unreachable:`-marked final return; guarded but no test provokes it — unowned, dies unless re-opened
- `toolrun` grandchild test through real `Run` takes the full 2s `waitDelay` — unowned, accepted cost
