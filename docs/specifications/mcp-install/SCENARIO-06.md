---
id: SCENARIO-06
status: open
---

# SCENARIO-06: First install step fails

Cadence: code-first — no file write, guard on user files, or atomic/exclusive-create adapter in scope; mutation checks below are load-bearing classification guards, not mandatory items.
Acceptance test: `internal/cli/claude_install_test.go` `Test_claude_install_reports_a_partial_install_when_the_plugin_step_fails`
Acceptance test (SCENARIO-06 first-step row, green on arrival per STATE): `internal/cli/claude_install_test.go` `Test_claude_install_reports_a_failed_first_step`
Acceptance test (SCENARIO-08, folded): `internal/cli/claude_install_test.go` `Test_claude_install_finishes_on_a_rerun_after_a_partial_install`
Acceptance test (SCENARIO-14, folded): `internal/cli/claude_install_test.go` `Test_claude_install_reports_an_interrupt_while_a_step_runs`
Acceptance test (SCENARIO-23, folded): `internal/cli/claude_install_test.go` `Test_claude_install_drops_see_above_when_the_failed_step_printed_nothing`
Acceptance test (SCENARIO-24, folded): `internal/cli/claude_install_test.go` `Test_claude_install_reports_a_step_stopped_by_a_signal`
Narrow loop: `go test ./internal/claudeplugin/ ./internal/cli/ -run 'Install|install|State|state|Claude|claude'`
Mutation checks: check `*toolrun.SignalError` before the ctx-done check in `(*Server).run` → `Test_install_reports_an_interrupt_naming_the_command_it_stopped` row "signal while ctx is done"; drop the empty-output guard on `; see its message above` → `Test_claude_install_drops_see_above_when_the_failed_step_printed_nothing`; delete the pre-child ctx check in `run` (next-due then names nothing / the previous command) → R5 grid row "cancelled after the plugin list, marketplace absent" (names the add, add never called); `cmd.Context()` → `context.Background()` at `claude_install.go:39` → `Test_claude_install_reports_an_interrupt_while_a_step_runs`
Runs: A (1) | B1 (2-3) | V (4-5)
Size: OWNS A RUN — 2 batches, 1 feature package (`internal/claudeplugin`) + `internal/cli` renderer

Why 07 owns the acceptance test: STATE records the first-step row green on arrival; the partial line is the first red assertion.

User-visible contract (verbatim from `## Surface & Copy` failure table, exit 1, `ReportedError`):
- Step/list failure line: `quarry: claude install: <lead><argv> <exited with status N | was stopped by signal S>[; see its message above][, then run quarry claude install again]` — see-above iff captured output non-empty; the `then run` suffix iff lead non-empty; captured buffer replayed on stderr first; done lines on stdout first.
- R5: `quarry: claude install: stopped before <argv> finished; run quarry claude install again` — no lead, no replay; done lines still on stdout.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `internal/cli/claude_install_test.go` `Test_claude_install_reports_a_partial_install_when_the_plugin_step_fails` — nothing installed, plugin install exits 1 with output; assert stdout `Added…` line, stderr replay then the partial line, `ReportedError`. Needs no stubs; red today at the stderr assertion (first-step form, no lead). Delete the now-duplicate row "an install step that exits 1 after the marketplace was added" in `Test_claude_install_reports_each_list_failure` (`:354-396`).

### Build
- [ ] Step 2: `internal/claudeplugin` — step-failure classification in the shared runner.
  - `state.go:35-44` `ExitError` grows `Signal os.Signal` (nil = exited with `Status`); doc says "did not exit zero: a non-zero status, or a signal quarry did not send". Keeps `Argv`, `Output`; no steps-done field (that stays `Result`).
  - New `*InterruptedError{Argv}` in `state.go`, `Unwrap` → the ctx's own error.
  - `state.go:134-145` `(*Server).run`: ctx done before the child → `InterruptedError` for this argv, Runner not called (this is "next-due"; the Server's own call order makes it right through skips). Runner err while ctx done → `InterruptedError` (checked **before** signal). `*toolrun.SignalError` → `ExitError{Signal, Output}` keeping the buffer the Runner returned with the error. Other Runner errors (incl. `*StartError`) returned bare — `install_test.go:139-160` and `state_test.go:206-227` `assert.Same` stay green.
  - `claudeplugin.go:63-66` Install doc gains `*InterruptedError`.
  - `fake_test.go:25-56` `fakeClaude`: answers from its **parameter** ctx (`ctx.Err()` reply) and can cancel the test's ctx on a scripted argv. No marker assertion here (~20 `Install(t.Context())` sites): the R5 "during X" rows already redden if Install hands the Runner another ctx.
  - `ExitError.Error()` branches on `Signal` so it never says `status -1`.
  - Tests in `install_test.go`: `Test_install_reports_an_interrupt_naming_the_command_it_stopped` (rows: cancelled before Install → marketplace list, zero calls; during marketplace list; during plugin list; after plugin list with marketplace absent → add, add not called; after plugin list with marketplace ours → install; during add; after add succeeds → install, `MarketplaceAdded`; during install → install, `MarketplaceAdded`; signal while ctx is done → interrupted, not `ExitError`; `errors.Is(err, context.Canceled)`); `Test_install_reports_a_signal_that_stopped_a_child` (rows: a list, the add, the install with `MarketplaceAdded`; `Signal` and `Output` carried).
- [ ] Step 3: `internal/cli/render_claude.go:85-124` — one failure renderer for both verbs.
  - `reportClaudeFailure` `ExitError` arm (`:93-97`) composes from fields, not `Error()`: lead × `exited with status N` / `was stopped by signal <Signal.String()>` × see-above iff `len(Output) > 0` × `, then run quarry claude <verb> again` iff lead != "". New `InterruptedError` arm → R5 line with verb, no replay, no lead. Lead stays `installDoneLead(res)` (`:58-64`); signature unchanged so SCENARIO-10 passes its own verb and lead.
  - `claude_install_test.go:37-95`: `toolCalls` records each ctx it receives; `runClaudeAt` (owns the marker and the cancel func) puts a marker on the command ctx and, after `Execute`, asserts every recorded ctx carries it — closes the "fakes ignore ctx" debt for every helper caller. A reply can cancel that ctx and answer with the parameter ctx's `Err()` or a `*toolrun.SignalError`. Direct `cli.Execute` callers stay unpinned (`Test_claude_install_runs_both_steps_when_nothing_is_installed` `:249` is the acceptance test — leave it; the `failingWriter` tests `:423`, `:432`, `:444` need their own Env).
  - Stateful fake (marketplace/plugin flags flipped by add/install, install fails N times) for `Test_claude_install_finishes_on_a_rerun_after_a_partial_install` — two `cli.Execute` calls: first partial (exit 1), second `already in` / `Installed` / restart, exit 0, argv shows no second add.
  - Fold tests: `Test_claude_install_reports_a_failed_first_step` (green on arrival — say so), `Test_claude_install_drops_see_above_when_the_failed_step_printed_nothing`, `Test_claude_install_reports_a_step_stopped_by_a_signal` (marketplace ours, install SIGKILL with output, first-step form), `Test_claude_install_reports_an_interrupt_while_a_step_runs` (add runs, install cancelled with output scripted: stdout `Added…`, stderr R5 naming install only, no replay).
  - `Test_claude_install_reports_each_step_failure` table, each row one variable off a neighbour: partial × status × empty; partial × signal × output; partial × signal × empty; first-step × signal × empty; second step fails with first skipped (first-step form naming install, status 2); plugin list × signal × output; marketplace list × status × empty (rewrite of the `:354-396` row "a marketplace list that exits 1 with no output", which pins old copy).
  - `Test_claude_install_reports_an_interrupt_before_or_between_children` rows: cancelled before Execute → names marketplace list, zero calls; cancelled after plugin list → names add.

### Sweep
- [ ] Step 4: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `ExitError`, `InterruptedError`, `reportClaudeFailure`, the new render helper.

### Verify
- [ ] Step 5: `verify.sh <start> ./internal/claudeplugin/... ./internal/cli/...` + `spec-check.py mcp-install`; replace the SCENARIO-06 Progress line with: `- [x] SCENARIO-06: First install step fails (first-step row green on arrival `Test_claude_install_reports_a_failed_first_step`; folds 07 (acceptance test below), 08 `Test_claude_install_finishes_on_a_rerun_after_a_partial_install`, 14 `Test_claude_install_reports_an_interrupt_while_a_step_runs`, 23 `Test_claude_install_drops_see_above_when_the_failed_step_printed_nothing`, 24 `Test_claude_install_reports_a_step_stopped_by_a_signal`) — `internal/cli/claude_install_test.go` `Test_claude_install_reports_a_partial_install_when_the_plugin_step_fails`; rewrite STATE.md.

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `*ExitError` grows `Signal os.Signal`; exit and signal share one renderer arm — SCENARIO-10's uninstall partial/signal/empty variants reuse it unchanged.
- `*InterruptedError{Argv}` unwraps the ctx's error; `(*Server).run` checks ctx before each child (next-due) and classifies any Runner error while ctx is done as interrupted before looking at `*SignalError` — SCENARIO-10's R5 depends on both checks living in the shared `run`.
- Unclassified Runner errors and `*StartError` come back bare — `assert.Same` pins `install_test.go:139`, `state_test.go:206`, and the cli `runtimeError` default arm rely on it.
- Steps done stay in `Result`; the lead is built only by `installDoneLead` (SCENARIO-10 adds its uninstall twin).
- Failure copy is composed in `render_claude.go` from `Argv`/`Status`/`Signal.String()`/`len(Output)`; claudeplugin `Error()` strings are no longer user copy (amends STATE's first binding for the exit line).
- cli fakes record every ctx and `runClaudeAt` asserts the command ctx's marker on each; both fakes answer from their parameter ctx — SCENARIO-10/12 tests inherit the pin through `runClaudeAt`.
- `internal/claudeplugin` now imports `internal/platform/toolrun` (for `*SignalError` only) — feature → platform edge; toolrun never imports claudeplugin.

**Left unbuilt** — named so nobody assumes it exists:
- uninstall lead `uninstalled the quarry plugin, but ` and uninstall R5 call site — SCENARIO-10.
- No cmd/quarry R5 test: `signal.NotifyContext` → `cli.Execute` ctx is existing wiring; toolrun already proves kill-on-cancel.

**Traps** — things that look right and are not:
- `toolrun.SignalError.Error()` is `stopped by signal: killed` (colon) — render `Signal.String()`, never `Error()`.
- `state.go:138-139` drops the output on any Runner err; toolrun returns the buffer with `*SignalError` and `ctx.Err()` (`toolrun.go:64,72`) — keep it for the signal case.
- A fake returning the test's captured ctx's `Err()` passes with propagation broken; use the parameter.
- R5 has no lead per the ruled row even when the add ran; if that reads wrong, it is a copy-ruling question, not a developer choice.
