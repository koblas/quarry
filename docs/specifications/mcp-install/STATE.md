# mcp-install — current state

Scenarios complete: SCENARIO-01 (folds 02, 03, 09, 09b, 15, 16, 19, 21, 22), SCENARIO-04 (folds 05, 25). Last updated by SCENARIO-04.

## Binding decisions
- claudeplugin returns values and typed errors (`*ExitError`, `*ListUnreadableError`, `ErrForeignMarketplace`, `ErrClaudeNotFound`, the Runner's own); all copy lives in `internal/cli/render_claude.go`, which builds the exit and unreadable lines from the errors' `Error()` — SCENARIO-10 renders the same errors under the uninstall prefix (SCENARIO-01, 04)
- Runtime failures: cli writes replay + line to stderr, returns `ReportedError`; a bare error would become an exit-2 UsageError (`run.go:85`), so an unclassified Runner error returns as `&runtimeError{err}` — the `default:` arm of `reportClaudeFailure` (SCENARIO-01, 04)
- `readState`: marketplace list, then plugin list, each once; a marketplace-list failure skips the plugin list; foreign check runs after both lists parse; SCENARIO-10 reuses it unchanged (SCENARIO-01)
- `Install` returns the steps already done alongside its error; SCENARIO-06's partial line depends on it. On failure the done steps print on stdout first, skipped steps print nothing (SCENARIO-01)
- Identity is exact and case-sensitive (`name`, `source`, `repo`). Only a JSON `false` in `enabled` turns the copy off; missing/null/non-bool counts as on, never R4 (SCENARIO-01)
- BR-8: `root.go` `newClaudeCommand` passes no store, snapshots or config factory (SCENARIO-01)
- `toolrun.Run` (matches `claudeplugin.Runner`) owns the Runner error types: `*StartError{Path, Err}` (could not start), `*SignalError{Signal}` (signal ended it, ctx live), `ctx.Err()` whenever ctx is done; status -1 on any err, a non-zero exit is a nil err. Amends the spec Seams line. SCENARIO-06 classifies signal / R5 from exactly these (SCENARIO-04)
- The Runner's `name` is the path `LookPath("claude")` resolved; `ExitError.Argv` stays `claude …`. Nil LookPath means name `"claude"`, no quarry check; the wiring test (`cmd/quarry/run_claude_wiring_test.go`), not a nil check, proves production sets it. `Install` decides on `err != nil`, never `path == ""` (SCENARIO-04)
- `Env.Home` (cli value, "" when unknown) rendered through `claudePath(home, p)` (raw when home is ""); claudeplugin has no home. SCENARIO-12/26 reuse both for R6 (SCENARIO-04)
- `installDoneLead(res)` is the one place the `added the quarry marketplace, but ` prefix is built; `reportClaudeFailure(cmd, verb, home, lead, done, err)` takes it; SCENARIO-06's partial "exited" line reuses it (SCENARIO-04)

## Left unbuilt
- Render of `*toolrun.SignalError` and ctx errors: today they reach the `default:` arm as `quarry: <err>`; partial form, empty-buffer variant (drop `; see its message above`), R5 — SCENARIO-06. A second-step failure still prints the first-step form naming the second command
- `ErrClaudeNotFound` copy is install-only (`installClaudeNotFoundRefusal`); uninstall's not-found and cannot-run lines — SCENARIO-10
- R7 beyond the cannot-run path, `WithHome`-free home handling in other lines — SCENARIO-12/26
- Group unknown-subcommand `Args` + RunE, group Long pin, 27/27b — SCENARIO-17
- `uninstall`, and root.go doc text `claude (with its install and uninstall children)` — SCENARIO-10

## Traps
- `homepath.Abbreviate("", "/opt/claude")` returns `"~/opt/claude"`; guard stays in `claudePath`, do not change the shared helper (SCENARIO-04)
- `exec.LookPath` can return a path together with `ErrDot`, and wraps every failure in `*exec.Error`; fakes must too (SCENARIO-04)
- After cancel, `CommandContext`'s SIGKILL reads as "killed": `toolrun` checks ctx before classifying a signal. `ErrWaitDelay` after a clean exit is success (SCENARIO-04)
- `cmd/quarry` `testEnv` has a RunTool fake that errors and a LookPath fake returning `*exec.Error{ErrNotFound}`, so no cmd/quarry test starts a child; only the wiring test runs a real (non-shebang) file via `runProcess`, no `t.Parallel` (SCENARIO-04)
- cli test fakes: `toolCalls` records `name`; helpers `runClaudeAt` (home), `findsAt`, `cannotStart`; `errNoStart` is a bare string error, wrong shape for a cannot-run fake (SCENARIO-04)
- `noArgs` (`errors.go:15`) lacks the `; Run '...' for usage.` suffix the ruled refusal needs
- Until SCENARIO-17, `quarry claude bogus` prints group help and exits 0; do not pin it
- Green on arrival: SCENARIO-06's first-step row (its architect picks 07's partial line as acceptance test) and SCENARIO-28's root-help row (17 only names `Test_run_help_prints_quarrys_description`)
- Pins other scenarios must re-assert: root-help claude row `cmd/quarry/run_status_test.go:145-162` (SCENARIO-17 rewrites the `claude` Short; SCENARIO-10 may touch it); `run_usage_test.go` rows `claude install extra` / `claude install --bogus` / subtest `claude install help`; `run_read_usage_test.go` rows `claude install extra|--json|--json extra` (SCENARIO-10 adds the uninstall twins)

## Open debts
- Test fakes ignore `ctx` — SCENARIO-06 pins `ctx` (cancellation/signal)
- `internal/claudeplugin/doc.go` says the package "installs and removes"; removal arrives with SCENARIO-10
- `toolrun.run` nil `cmd.ProcessState` (wait4 ECHILD) returns the raw err via the `// unreachable:`-marked final return; guarded but no test provokes it — unowned, dies unless re-opened
- `toolrun` grandchild test through real `Run` takes the full 2s `waitDelay` — unowned, accepted cost
