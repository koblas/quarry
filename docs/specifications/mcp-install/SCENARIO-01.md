---
id: SCENARIO-01
status: open
---

# SCENARIO-01: Fresh install runs both steps

Cadence: code-first — no mandatory item (quarry writes no file; every change is a `claude` child)
Acceptance test: `internal/cli/claude_install_test.go` `Test_claude_install_runs_both_steps_when_nothing_is_installed`
Acceptance test (SCENARIO-02, folded): `internal/cli/claude_install_test.go` `Test_claude_install_skips_both_steps_when_both_are_present`
Acceptance test (SCENARIO-03, folded): `internal/cli/claude_install_test.go` `Test_claude_install_installs_only_the_plugin_when_the_marketplace_is_present`
Acceptance test (SCENARIO-09, folded): `internal/cli/claude_install_test.go` `Test_claude_install_refuses_a_plugin_list_that_is_not_json`
Acceptance test (SCENARIO-09b, folded): `internal/cli/claude_install_test.go` `Test_claude_install_replays_a_failed_marketplace_list`
Acceptance test (SCENARIO-15, folded): `internal/cli/claude_install_test.go` `Test_claude_install_refuses_json`
Acceptance test (SCENARIO-16, folded): `internal/cli/claude_install_test.go` `Test_claude_install_refuses_arguments`
Acceptance test (SCENARIO-19, folded): `internal/cli/claude_install_test.go` `Test_claude_install_refuses_a_foreign_quarry_marketplace`
Acceptance test (SCENARIO-21, folded): `internal/cli/claude_install_test.go` `Test_claude_install_hints_when_the_user_copy_is_turned_off`
Acceptance test (SCENARIO-22, folded): `internal/cli/claude_install_test.go` `Test_claude_install_refuses_a_plugin_list_entry_without_an_id`
Narrow loop: `go test ./internal/claudeplugin/ ./internal/cli/ -run 'claude|read_state|install' && go test ./cmd/quarry/ -run 'Test_run_help_prints|Test_run_rejects_usage|Test_run_usage_hint|Test_run_help_and_usage|Test_run_read_commands_reject'`
Mutation checks: foreign-marketplace refusal return in `readState` → `Test_claude_install_refuses_a_foreign_quarry_marketplace`; `repo == "koblas/quarry"` term of the ours-predicate → `Test_read_state_classifies_quarry_marketplaces` (row: github, other repo)
Runs: A (1-2) | B1 (3-4) | B2 (5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (`internal/claudeplugin`)

## Observed

- Throwaway home: `HOME=` reassignment refused by the worktree hook; used `CLAUDE_CONFIG_DIR=$TMPDIR/h` instead. `claude plugin list --json` → `[]`, rc 0; `claude plugin marketplace list --json` → `[]`, rc 0 (empty-home fixture).
- Same command inside the sandbox: output (streams combined) `Error: An unknown error occurred (Unexpected)`, rc 1 — real list-failure shape, use as 09b's fixture.
- `claude plugin enable --help`: not attempted (hook blocks the word); R2 line kept as ruled. Local-scope entry shape: not observed.
- Port survey: no `os/exec` in production today; the Runner func stands in for nothing concrete. This scenario's calls: the two list argv and the two install argv (spec `### Child argv`).

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `internal/cli/claude_install_test.go` `Test_claude_install_runs_both_steps_when_nothing_is_installed` — `cli.Execute` `claude install`, recording fake `RunTool` keyed by argv (both lists `[]`); asserts argv sequence (4 calls), the two "did" lines + restart line on stdout, empty stderr, nil error
- [ ] Step 2: `internal/claudeplugin/doc.go`, `claudeplugin.go` `Runner` type; `internal/cli/run.go:46-56` `Env.RunTool claudeplugin.Runner` — signature-only so the test compiles; red at `require.NoError` (unknown command "claude")

### Build
- [ ] Step 3: `internal/claudeplugin/state.go` `readState`, shared child runner, `ExitError`, list-unreadable error, `ErrForeignMarketplace` + `state_test.go` — `Test_read_state_classifies_quarry_marketplaces` arms: `[]`, only caveman (absent), ours, name quarry + source `directory`, + source missing, + github other repo, + repo missing, repo case variant `Koblas/quarry`, name `Quarry` (not ours, not foreign). `Test_read_state_finds_the_user_copy` arms: user `quarry@quarry`, project-only, `local` scope, other id at user, `enabled:false`, `enabled` missing, `enabled` null/string (counts as on). foreign → `ErrForeignMarketplace` after both lists. R4 rows per list: not JSON, object, `null`, non-object entry, `name` / `id` / `scope` missing, each non-string; marketplace R4 and marketplace non-zero exit run no plugin list; plugin list R4 beats a foreign marketplace. Fault rows: non-zero exit on each list (`ExitError` carries argv string, status, output), runner `err` on each list (`errors.Is` on the injected error, no copy)
- [ ] Step 4: `internal/claudeplugin/claudeplugin.go` `Server`, `NewServer`, `WithRunner`, `(*Server).Install`, `Result` (steps done + ran flag, user copy turned off) + `install_test.go` — arms: nothing present (add then install), marketplace only (install), both (none), plugin user + marketplace absent (add only), project-only + ours (install, no hint), `enabled:false` (no step, turned-off flag), `enabled:false` + marketplace absent (add, flag); foreign from `readState` → returned, zero step calls; add exits 1 → install not run, no step done; install exits 1 → `ExitError` naming install, Result holds add done; runner `err` on each step
- [ ] Step 5: `internal/cli/claude.go` `newClaudeCommand` group (ruled Short/Long, no Args/RunE), `claude_install.go` (ruled Short/Long; prune-style `Args` per `snapshots_prune.go:41-45`, then `--json` refusal; builds the Server from `RunTool` only), `render_claude.go` (step lines, restart line, R2 hint, R1, R4 per list, replay + `\n` if missing + exited-with-status line; writes stderr itself, returns `ReportedError`); `root.go:43` `root.AddCommand(newClaudeCommand(env.RunTool, jsonOut))`; `root.go:7-10` doc comment adds `claude (with its install child)`. Fold acceptance tests in `claude_install_test.go`, each top-level: 02 `Test_claude_install_skips_both_steps_when_both_are_present`, 03 `Test_claude_install_installs_only_the_plugin_when_the_marketplace_is_present`, 09 `Test_claude_install_refuses_a_plugin_list_that_is_not_json`, 09b `Test_claude_install_replays_a_failed_marketplace_list`, 15 `Test_claude_install_refuses_json`, 16 `Test_claude_install_refuses_arguments`, 19 `Test_claude_install_refuses_a_foreign_quarry_marketplace`, 21 `Test_claude_install_hints_when_the_user_copy_is_turned_off`, 22 `Test_claude_install_refuses_a_plugin_list_entry_without_an_id`; plus `Test_claude_install_reports_each_list_failure` (marketplace R4, plugin-list exit 1 with output lacking `\n`, install-step exit 1 → first-step form, runner `err` → `&runtimeError{err}` per `mcp.go:52`: `errors.Is` the injected error and not a `UsageError`) and `Test_claude_install_help_prints_the_ruled_text` (Short + Long at wrap width). 15/16 assert zero runner calls (control: Step 1 fake). Pins: `cmd/quarry/run_status_test.go:149-150` claude row between cashflow and findings; `run_usage_test.go:217-220` row `claude install extra`; `:323` row `claude install --bogus`; `:388-396` subtest `claude install --help` with HOME unset; `run_read_usage_test.go:53-54` rows `claude install extra`, `claude install --json`, `claude install --json extra` (argument refusal wins)

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on every new exported symbol; `go doc ./internal/claudeplugin` reads as the contract

### Verify
- [ ] Step 7: full verification + `spec-check.py mcp-install` → replace the SCENARIO-01 Progress line with exactly this one unwrapped line:

- [x] SCENARIO-01: Fresh install runs both steps (folds 02 `Test_claude_install_skips_both_steps_when_both_are_present`, 03 `Test_claude_install_installs_only_the_plugin_when_the_marketplace_is_present`, 09 `Test_claude_install_refuses_a_plugin_list_that_is_not_json`, 09b `Test_claude_install_replays_a_failed_marketplace_list`, 15 `Test_claude_install_refuses_json`, 16 `Test_claude_install_refuses_arguments`, 19 `Test_claude_install_refuses_a_foreign_quarry_marketplace`, 21 `Test_claude_install_hints_when_the_user_copy_is_turned_off`, 22 `Test_claude_install_refuses_a_plugin_list_entry_without_an_id`) — `internal/cli/claude_install_test.go` `Test_claude_install_runs_both_steps_when_nothing_is_installed`

## Handoff

**Binding decisions:**
- claudeplugin returns values and typed errors (argv string, status, output, which list, foreign); all copy is in `internal/cli/render_claude.go`. SCENARIO-10 renders the same errors under the uninstall prefix.
- Runtime failures: cli writes the replay and line to stderr, returns `ReportedError`. `Execute` turns a bare error into an exit-2 UsageError (`run.go:85`).
- `readState`: marketplace list, then plugin list, each once. A failure or R4 on the marketplace list stops before the plugin list runs. The foreign check runs after both lists. SCENARIO-10 reuses it unchanged.
- `Install` returns the steps already done alongside its error; SCENARIO-06's partial line depends on it.
- One `ExitError` for lists and steps; the rendered line is `<argv> exited with status N; see its message above`.
- Identity is exact and case-sensitive (`name`, `source`, `repo`). Only a JSON `false` in `enabled` means turned off; a missing, null or non-bool value counts as on, never R4 (out-of-domain row ruled here).
- BR-8: `root.go:43`'s `newClaudeCommand(env.RunTool, jsonOut)` passes no store, snapshots or config factory.
- With no LookPath the Server calls the Runner with name `"claude"`. Tests assert argv only, never `name`.

**Left unbuilt:**
- `defaultEnv.RunTool` (`cmd/quarry/run.go:186-200`) is nil; also `internal/platform/toolrun`, `LookPath`/`WithLookPath`/`Env.LookPath`, the quarry-PATH warning and cannot-run — all SCENARIO-04.
- `WithHome`, the `Env` home field and R7 — SCENARIO-12.
- Partial form, empty-buffer variant, signal, R5 — SCENARIO-06. Today a second-step failure prints the first-step form naming the second command, and a runner `err` returns as `&runtimeError{err}` (generic `quarry: <err>`, exit 1; copy owned by 04/06).
- Group unknown-subcommand `Args` + RunE, the group Long pin, and 27/27b — SCENARIO-17.
- `uninstall`, plus root.go doc text `claude (with its install and uninstall children)` — SCENARIO-10.

**Traps:**
- `noArgs` (`errors.go:15`) lacks the `; Run '...' for usage.` suffix the ruled refusal needs.
- Until SCENARIO-04 the binary's `quarry claude install` hits a nil Runner; no cmd/quarry test may run past the usage and `--json` refusals.
- Until SCENARIO-17, `quarry claude bogus` prints group help and exits 0; do not pin it.
- Green on arrival: SCENARIO-06's first-step row (its architect picks 07's partial line as the acceptance test) and SCENARIO-28's root-help row (17 only names `Test_run_help_prints_quarrys_description`).
