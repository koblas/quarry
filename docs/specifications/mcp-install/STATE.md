# mcp-install — current state

Scenarios complete: SCENARIO-01 (folds 02, 03, 09, 09b, 15, 16, 19, 21, 22). Last updated by SCENARIO-01.

## Binding decisions
- claudeplugin returns values and typed errors (`*ExitError`, `*ListUnreadableError`, `ErrForeignMarketplace`, the Runner's own error); all copy lives in `internal/cli/render_claude.go`, which builds the exit and unreadable lines from the errors' `Error()` — SCENARIO-10 renders the same errors under the uninstall prefix (SCENARIO-01)
- Runtime failures: cli writes replay + line to stderr, returns `ReportedError`; a bare error would become an exit-2 UsageError (`run.go:85`), so a Runner error returns as `&runtimeError{err}` (SCENARIO-01)
- `readState`: marketplace list, then plugin list, each once; a marketplace-list failure skips the plugin list; foreign check runs after both lists parse; SCENARIO-10 reuses it unchanged (SCENARIO-01)
- `Install` returns the steps already done alongside its error; SCENARIO-06's partial line depends on it. On failure the done steps print on stdout first, skipped steps print nothing (SCENARIO-01)
- Identity is exact and case-sensitive (`name`, `source`, `repo`). Only a JSON `false` in `enabled` turns the copy off; missing/null/non-bool counts as on, never R4 (SCENARIO-01)
- BR-8: `root.go:43` `newClaudeCommand(env.RunTool, jsonOut)` passes no store, snapshots or config factory (SCENARIO-01)
- With no LookPath the Server calls the Runner with name `"claude"`; tests assert argv only (SCENARIO-01)

## Left unbuilt
- `defaultEnv.RunTool` (`cmd/quarry/run.go:186-200`) is nil; `internal/platform/toolrun`, `LookPath`/`WithLookPath`/`Env.LookPath`, quarry-PATH warning, cannot-run — SCENARIO-04
- Partial form, empty-buffer variant (drop `; see its message above`), signal, R5 — SCENARIO-06. Today a second-step failure prints the first-step form naming the second command
- `WithHome`, the `Env` home field, R7 — SCENARIO-12
- Group unknown-subcommand `Args` + RunE, group Long pin, 27/27b — SCENARIO-17
- `uninstall`, and root.go doc text `claude (with its install and uninstall children)` — SCENARIO-10

## Traps
- `noArgs` (`errors.go:15`) lacks the `; Run '...' for usage.` suffix the ruled refusal needs
- Until SCENARIO-04 the binary's `quarry claude install` hits a nil Runner; no cmd/quarry test may run past the usage and `--json` refusals
- Until SCENARIO-17, `quarry claude bogus` prints group help and exits 0; do not pin it
- Green on arrival: SCENARIO-06's first-step row (its architect picks 07's partial line as acceptance test) and SCENARIO-28's root-help row (17 only names `Test_run_help_prints_quarrys_description`)
- Pins other scenarios must re-assert: root-help claude row `cmd/quarry/run_status_test.go:145-162` (SCENARIO-17 rewrites the `claude` Short; SCENARIO-10 may touch it); `run_usage_test.go` rows `claude install extra` / `claude install --bogus` / subtest `claude install help`; `run_read_usage_test.go` rows `claude install extra|--json|--json extra` (SCENARIO-10 adds the uninstall twins)

## Open debts
- Test fakes ignore `name` (`internal/cli/claude_install_test.go` `toolCalls.run`, `internal/claudeplugin/fake_test.go:49`) — SCENARIO-04 pins `name`
- Test fakes ignore `ctx` — SCENARIO-06 pins `ctx` (cancellation/signal)
- `internal/claudeplugin/doc.go` says the package "installs and removes"; removal arrives with SCENARIO-10
