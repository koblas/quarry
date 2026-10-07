---
id: SCENARIO-10
status: done
---

# SCENARIO-10: Uninstall runs both steps

Cadence: test-first — identity guard on deletion: `Uninstall` must run no `uninstall`/`remove` child while a marketplace named `quarry` is foreign (BR-3/R1)
Acceptance test: `internal/cli/claude_uninstall_test.go` `Test_claude_uninstall_runs_both_steps_when_both_are_present`
Acceptance test (SCENARIO-11, folded): `internal/cli/claude_uninstall_test.go` `Test_claude_uninstall_skips_both_steps_when_nothing_is_installed`
Acceptance test (SCENARIO-13, folded): `internal/cli/claude_uninstall_test.go` `Test_claude_uninstall_reports_a_partial_uninstall_when_the_marketplace_step_fails`
Acceptance test (SCENARIO-05b, folded): `internal/cli/claude_uninstall_test.go` `Test_claude_uninstall_refuses_when_claude_is_not_on_the_path`
Acceptance test (SCENARIO-15b, folded): `internal/cli/claude_uninstall_test.go` `Test_claude_uninstall_refuses_json`
Acceptance test (SCENARIO-20, folded): `internal/cli/claude_uninstall_test.go` `Test_claude_uninstall_refuses_a_foreign_quarry_marketplace`
Narrow loop: `go test ./internal/claudeplugin/ ./internal/cli/ -run 'install|read_state'` (pins: `go test ./cmd/quarry/ -run 'usage|help'`)
Mutation checks: drop the `return` on `readState`'s error in `(*Server).Uninstall` → `Test_uninstall_refuses_a_foreign_marketplace_before_any_step`; swap the two steps (marketplace remove before plugin uninstall) → `Test_uninstall_runs_only_the_steps_that_are_present`; plugin step gated on `st.marketplaceOurs` instead of `st.userCopy` → same test, row "marketplace only"; marketplace step gated on `st.userCopy` → row "plugin only"; plugin step gated on `st.userCopy && !st.userCopyOff` → row "turned-off user copy"
Runs: A (1-2) | B1 (3-5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (`internal/claudeplugin`)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `internal/cli/claude_uninstall_test.go` (new) `Test_claude_uninstall_runs_both_steps_when_both_are_present` — `cli.Execute` `claude uninstall` over `toolCalls.lists(ourMarketplace, userPluginOn)`; asserts the four argv in order, stdout = both "did" lines + `Restart Claude Code to unload it.`, stderr empty, nil err. Reuses helpers in `claude_install_test.go:22-135`; add argv consts `uninstallPluginArgv`, `removeMarketplaceArgv` beside `:22-26`
- [x] Step 2: `internal/claudeplugin/claudeplugin.go:50-93` `UninstallResult` + `(*Server).Uninstall(ctx) (UninstallResult, error)` signature-only; `internal/cli/claude_uninstall.go` (new) `newClaudeUninstallCommand(runTool, lookPath, home, jsonOut)` stub registered at `claude.go:20` — red at the argv assertion

### Build
- [x] Step 3: `claudeplugin.go:16-19` args consts `uninstallPluginArgs`, `removeMarketplaceArgs`; `:50-93` `UninstallResult{PluginUninstalled, MarketplaceRemoved}` + `Ran()`, `(*Server).Uninstall` = `findClaude` → `readState` (`state.go:87-120`, unchanged) → plugin step iff `st.userCopy` → marketplace step iff `st.marketplaceOurs`, each through `run` (`state.go:157-177`); never looks up quarry. Tests first, `internal/claudeplugin/uninstall_test.go` (new): `Test_uninstall_refuses_a_foreign_marketplace_before_any_step` (calls = the two lists only, zero `UninstallResult`); `Test_uninstall_runs_only_the_steps_that_are_present` table — both present, neither, plugin only (marketplace absent), marketplace only (plugin absent), turned-off user copy (`enabled:false` → still uninstalled); every row user-scope only; asserts calls order and `UninstallResult`; `Test_uninstall_reports_that_a_step_ran_only_when_one_did`; `Test_uninstall_refuses_before_any_child_when_claude_is_not_found` (`*exec.Error` shape, `ErrClaudeNotFound`); `Test_uninstall_runs_each_child_at_the_path_lookpath_found` (lookPath asked only for `claude`); `Test_uninstall_stops_when_the_plugin_step_exits_non_zero`; `Test_uninstall_reports_the_uninstalled_plugin_when_the_marketplace_step_exits_non_zero`; `Test_uninstall_returns_the_runner_error_from_a_step_unchanged` (`assert.Same`); `Test_uninstall_reports_an_interrupt_naming_the_next_command` (cancel on the plugin-uninstall reply → `*InterruptedError{Argv: "claude plugin marketplace remove --scope user quarry"}`, result shows plugin uninstalled)
- [x] Step 4: `internal/cli/claude_uninstall.go` — ruled Short/Long verbatim, `Args` refusal then `--json` refusal (same shape as `claude_install.go:28-36`), RunE renders success; `render_claude.go:16-65` add uninstall step/skip lines, `uninstallRestartLine`, `uninstallRemovedLead` (`uninstalled the quarry plugin, but `), `renderUninstalled`, `renderUninstallDone`, `uninstallDoneLead`; `render_claude.go:108-115` replace install-only `installForeignRefusal` / `installClaudeNotFoundRefusal` with per-verb copy (a table keyed by verb, no `default:` arm), `reportClaudeFailure` signature unchanged. Tests in `claude_uninstall_test.go`: the five folded acceptance tests above (05b asserts no child ran, stdout empty; 15b asserts `UsageError` text, no child; 20 asserts R1 uninstall copy, argv = two lists); `Test_claude_uninstall_reports_each_outcome` R8 table — plugin absent + marketplace ours (`not installed`/`Removed`/restart), plugin present + marketplace absent (`Uninstalled`/`not in`/restart), turned-off user copy (`Uninstalled`, no R2 hint); `Test_claude_uninstall_reports_each_failure` table through `runClaudeAt`, one row per arm under the uninstall prefix: first step exit+output, partial empty output, partial signal, second step with the first skipped (first-step form), marketplace list exit, unreadable plugin list (`quarry claude uninstall --help yourself`), R5 between steps (stdout `Uninstalled`), R5 before the command runs, cannot-run first step and partial (`findsAt` under home → `~` path, `cannotStart`); `Test_claude_uninstall_returns_the_runners_own_error_as_a_runtime_failure`; `Test_claude_uninstall_returns_the_stdout_write_error_when_the_result_cannot_be_printed` (`failingWriter`); `Test_claude_uninstall_refuses_arguments`; `Test_claude_uninstall_help_prints_the_ruled_text` (Long at wrap width + group help row `  uninstall   Uninstall quarry's plugin from Claude Code\n`, no child)
- [x] Step 5: pins — `cmd/quarry/run_usage_test.go:222-226` row `claude uninstall extra`, `:329-332` row `claude uninstall --bogus`, `:407-415` subtest `claude uninstall help`; `cmd/quarry/run_read_usage_test.go:14-17` const `claudeUninstallArgs`, `:58-60` rows `claude uninstall extra|--json|--json extra`; re-assert root-help claude row `run_status_test.go:145-162` and existing `claude install` rows unchanged

### Sweep
- [x] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments: `Server` (`claudeplugin.go:21`) and `WithLookPath` (`:35-39`) name uninstall; `Uninstall` lists its errors; `root.go:8-10` → `claude (with its install and uninstall children)`; `claude.go:11-12` group comment; `internal/claudeplugin/doc.go` already says "installs and removes" — now true, close the STATE debt

### Verify
- [x] Step 7: full verification + `spec-check.py mcp-install` → tick SCENARIO-10, and 11, 13, 05b, 15b, 20 each with "delivered by SCENARIO-10" and its test; rewrite STATE.md

## Handoff

**Binding decisions**:
- `UninstallResult{PluginUninstalled, MarketplaceRemoved}` is its own type with `Ran()`; `Result` stays install-only. SCENARIO-12 adds its keep/remaining-copies fields here, not on `Result`
- `Uninstall` order: `findClaude` → `readState` → plugin step → marketplace step; R1 comes from the shared `readState`, so no uninstall child can run on a foreign `quarry` — both targets (`quarry@quarry`, marketplace `quarry`) are named, never identified, so that check is the whole identity guard
- Plugin step decided on `st.userCopy` only (enabled ignored); marketplace step on `st.marketplaceOurs` only. `Uninstall` never calls `lookPath("quarry")`
- Per-verb refusal copy (foreign, not-found) is chosen inside `reportClaudeFailure` by verb; its signature stays `(cmd, verb, home, lead, done, err)`
- Lead `uninstalled the quarry plugin, but ` only via `uninstallDoneLead(res)`; R5 keeps no lead

**Left unbuilt**:
- BR-6 keep branch (no marketplace remove while a non-user `quarry@quarry` remains), `Kept` line, R6 hints, any non-user copy in `state` — SCENARIO-12; the seam is between the plugin and marketplace steps in `(*Server).Uninstall`
- Group unknown-subcommand refusal, 27/27b — SCENARIO-17

**Traps**:
- Until SCENARIO-12, a project-scope `quarry@quarry` plus our marketplace makes uninstall run `marketplace remove` — do not pin that row anywhere; every uninstall fixture here is user-scope only
- `run_status_test.go` root row is unchanged (group Short unchanged); a diff there is a bug
- Group help padding: cobra pads names to 11, so `uninstall` gets three spaces before its Short, `install` five

## Phase report

Runs A, B1 and V done (steps 1-7). Not committed.

Built: see steps 3-5. `UninstallResult` + `Ran()`, `(*Server).Uninstall`, `claude_uninstall.go`, `render_claude.go` uninstall lines plus `claudeRefusalCopy` (keyed by verb), cmd/quarry pins.

V pass (checkpoint findings, pin-only, production unchanged except the `Uninstall` doc, now 4 lines):
- foreign guard table: `[foreign]`, `[ours, foreign]`, `[foreign, ours]`. Mutation `break` after `st.marketplaceOurs = true` in `readState`: red row "ours listed before a foreign one" only; restored (diff identical)
- interrupt table `Test_uninstall_reports_an_interrupt_naming_the_command_it_stopped`: 8 cells (marketplace list, plugin list, after plugin list present/absent, during uninstall, after uninstall, during remove, signal with ctx done)
- cli failure table +3 rows: marketplace list not JSON, plugin list exit 2, interrupted during a step
- New tests were green on arrival (they pin existing behaviour); the foreign row red is the mutation above

Sweep: `go build ./...` ok; `golangci-lint run ./...` `0 issues.`; `go doc ./internal/claudeplugin` shows UninstallResult and "installs and removes".
Verify (`verify.sh 7ac411b0 ./internal/claudeplugin/... ./internal/cli/... ./cmd/quarry/...`): go build rc=0, go test rc=0, uncovered-diff rc=0 (0 uncovered added lines), go test -race rc=0, golangci-lint rc=0. test-stats: claudeplugin 31 (+10), cli 667 (+12), cmd/quarry 1020 (+0), TOTAL 1718 (+22).
spec-check mcp-install OK.
