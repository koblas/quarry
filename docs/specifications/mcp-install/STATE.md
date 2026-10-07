# mcp-install — current state

Scenarios complete: SCENARIO-01 (folds 02, 03, 09, 09b, 15, 16, 19, 21, 22), SCENARIO-04 (folds 05, 25), SCENARIO-06 (folds 07, 08, 14, 23, 24), SCENARIO-10 (folds 11, 13, 05b, 15b, 20), SCENARIO-12 (fold 26). Last updated by SCENARIO-12. Only SCENARIO-17 remains.

## Binding decisions
- claudeplugin returns values and typed errors (`*ExitError`, `*InterruptedError`, `*ListUnreadableError`, `ErrForeignMarketplace`, `ErrClaudeNotFound`, the Runner's own); all copy lives in `internal/cli/render_claude.go`. The exit/signal line is composed from fields (`Argv`, `Status`, `Signal.String()`, `len(Output)`) by `claudeStepFailureLine(verb, lead, exit)`, never from `Error()`; the unreadable line still uses `Error()`. Install and uninstall render the same errors under their own verb prefix (SCENARIO-01, 04, 06, 10)
- `*ExitError.Signal os.Signal` (nil = exited with `Status`); exit and signal share one renderer arm (SCENARIO-06)
- `*InterruptedError{Argv}` unwraps the ctx's error. `(*Server).run` checks ctx before each child and classifies any Runner error while ctx is done as interrupted before looking at `*toolrun.SignalError`; the interrupt line has no lead and no replay, for install and uninstall alike (SCENARIO-06, 10)
- Runtime failures: cli writes replay + line to stderr, returns `ReportedError`; a bare error would become an exit-2 UsageError (`run.go:85`), so an unclassified Runner error (and `*StartError` outside the cannot-run arm) returns as `&runtimeError{err}` — the `default:` arm of `reportClaudeFailure`; `assert.Same` pins rely on bare return (SCENARIO-01, 04, 06, 10)
- `readState`: marketplace list, then plugin list, each once; a marketplace-list failure skips the plugin list; foreign check runs after both lists parse and scans every entry. It is the whole identity guard for `Uninstall`: both targets (`quarry@quarry`, marketplace `quarry`) are named, never looked up. It also collects every non-user `quarry@quarry` entry (`state.others`, list order, nil when none) (SCENARIO-01, 10, 12)
- `Install`/`Uninstall` return the steps already done (`Result` / `UninstallResult`) alongside the error; on failure done steps print on stdout first, skipped steps print nothing. Leads only via `installDoneLead(res)` / `uninstallDoneLead(res)`. `Result` stays install-only (SCENARIO-01, 04, 06, 10)
- `Uninstall` order: `findClaude` -> `readState` -> plugin step iff `st.userCopy` (enabled ignored) -> `Remaining = st.others` -> marketplace step iff ours and `others` empty, else `MarketplaceKept` when ours. Scope is compared to `user` only, so an unknown scope keeps (fails safe). Never calls `lookPath("quarry")` (SCENARIO-10, 12)
- `UninstallResult{PluginUninstalled, MarketplaceRemoved, MarketplaceKept, Remaining []Copy}`; `Ran()` excludes Kept (no restart line for a keep alone). cli composes the Kept line and R6 hints (`uninstallRemainingHints`, one stderr line per copy in list order, only on success, after stdout). Marketplace absent + project copy prints the "not in Claude Code" line plus hints, no Kept line (SCENARIO-12)
- Refusal copy per verb lives in `claudeRefusalCopy` (`render_claude.go:52`), read by `reportClaudeFailure`; signature `(cmd, verb, home, lead, done, err)` unchanged (SCENARIO-10)
- Identity is exact and case-sensitive (`name`, `source`, `repo`). Only a JSON `false` in `enabled` turns the copy off; missing/null/non-bool counts as on (SCENARIO-01)
- BR-8: `root.go` `newClaudeCommand` passes no store, snapshots or config factory (SCENARIO-01)
- `toolrun.Run` owns the Runner error types (`*StartError`, `*SignalError`, `ctx.Err()`); status -1 on any err. `claudeplugin` imports `internal/platform/toolrun` for `*SignalError` only (SCENARIO-04, 06)
- The Runner's `name` is the path `LookPath("claude")` resolved; `ExitError.Argv` stays `claude …`; the wiring test (`cmd/quarry/run_claude_wiring_test.go`) proves production sets it (SCENARIO-04)
- `Env.Home` rendered through `claudePath(home, p)` (raw when home is ""); hint paths abbreviate first, then `%q` (SCENARIO-04, 12)
- cli fakes record every ctx; `runClaudeAt` owns a marker on the command ctx and asserts it on each recorded ctx (SCENARIO-06)

## Left unbuilt
- SCENARIO-17 (folds 18, 27, 27b, 28) must do: (1) `mcp` Long names `quarry claude install` + `internal/cli/mcp_test.go:28,55` pins; (2) README: install block, network sentence, new `## Install or remove the plugin` section, README const pin `cmd/quarry/run_plugin_readme_test.go:41-62` (command lines equal the Long's); (3) drift test tying `install` Long's `koblas/quarry` / `quarry@quarry` to `.claude-plugin/marketplace.json`, `plugin/.claude-plugin/plugin.json`, README; PRD Decisions line; (4) `claude` group unknown-subcommand `Args` + RunE refusal (spec Surface & Copy; `noArgs` `errors.go:15` lacks the `; Run '...' for usage.` suffix), group Long pin, bare group prints help (27b); (5) `claude` Short rewrite + root-help row (28)

## Traps
- `toolrun.SignalError.Error()` is `stopped by signal: killed` (colon) — render `Signal.String()`, never `Error()` (SCENARIO-06)
- `run` keeps the output the Runner returned with `*SignalError`; dropping it loses the replay (SCENARIO-06)
- A fake returning the test's captured ctx's `Err()` passes with propagation broken; answer from the parameter ctx (SCENARIO-06)
- `homepath.Abbreviate("", "/opt/claude")` returns `"~/opt/claude"`; guard stays in `claudePath` (SCENARIO-04, 12)
- `exec.LookPath` can return a path together with `ErrDot`, and wraps every failure in `*exec.Error`; fakes must too (SCENARIO-04)
- After cancel, `CommandContext`'s SIGKILL reads as "killed": `toolrun` checks ctx before classifying a signal. `ErrWaitDelay` after a clean exit is success (SCENARIO-04)
- `cmd/quarry` `testEnv` has a RunTool fake that errors and a LookPath fake returning `*exec.Error{ErrNotFound}`, so no cmd/quarry test starts a child; only the wiring test runs a real file, no `t.Parallel` (SCENARIO-04)
- cli test fakes: `toolCalls` (`script`, `cancelBefore`, `cancel`, `lists`, `reply`), helpers `runClaude`, `runClaudeAt` (home, marker), `findsAt`, `cannotStart`; `errNoStart` is a bare string error. claudeplugin fakes: `fakeClaude`, `fakePath`. Interrupt tables: `cancel` alone = step succeeded then ctx ended; `cancel`+`ctxErr` = ended during (SCENARIO-04, 06, 10)
- `UninstallResult.Remaining` must be nil, not empty, when no copy exists: `assert.Equal(…UninstallResult{…})` rows break otherwise; do not add a non-user entry to the `reports_each_outcome` table (stderr asserted empty) (SCENARIO-12)
- Until SCENARIO-17, `quarry claude bogus` prints group help and exits 0; do not pin it (SCENARIO-10)
- Group help padding: cobra pads names to 11, so `uninstall` gets three spaces before its Short, `install` five. The root-help claude row is unchanged by SCENARIO-10/12; a diff there is a bug until SCENARIO-17 rewrites the `claude` Short (SCENARIO-10)
- Uninstall Long's "names each one" is now true; ruled copy, do not reword (SCENARIO-10, 12)
- Pins SCENARIO-17 must re-assert: root-help claude row `cmd/quarry/run_status_test.go:145-162`; `claude install` and `claude uninstall` rows in `run_usage_test.go` and `run_read_usage_test.go`; `Test_claude_uninstall_help_prints_the_ruled_text` and `Test_claude_uninstall_reports_each_outcome` (byte-identical)
- Green on arrival: SCENARIO-28's root-help row (17 only names `Test_run_help_prints_quarrys_description`)

## Open debts
- Final product-vision pass: the Kept line says "a single project" even for a `managed`/`local` scope copy or several remaining copies (SCENARIO-12) — ruled copy, not re-ruled here
- `claudeRefusalCopy[verb]` returns empty strings for an unknown verb (only install/uninstall call it) — NIT, unowned, dies unless re-opened (SCENARIO-10)
- `toolrun.run` nil `cmd.ProcessState` (wait4 ECHILD) returns the raw err via the `// unreachable:`-marked final return; no test provokes it — unowned, dies unless re-opened
- `toolrun` grandchild test through real `Run` takes the full 2s `waitDelay` — unowned, accepted cost
- No cmd/quarry interrupt test: `signal.NotifyContext` -> `cli.Execute` ctx is existing wiring and toolrun proves kill-on-cancel — unowned, accepted
