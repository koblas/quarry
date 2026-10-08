---
id: SCENARIO-16
status: done
---

# SCENARIO-16: Help and docs describe both targets

Cadence: code-first — no mandatory item touched (help copy, README, PRD; no file-writing code)
Acceptance test: `internal/cli/claude_help_test.go` `Test_help_names_both_claude_targets` (new; one row each for `claude`, `claude install`, `claude uninstall`, `mcp`, ruled Long verbatim at wrap width through `cli.Execute`/`runClaude` `--help`)
Narrow loop: `go test ./internal/cli/ -run 'Help|group_help|root_help'` and `go test ./cmd/quarry/ -run 'readme|notices_and_prd|claude_commands_agree|every_quarry_name|run_help_prints_quarrys|monthly_summary_job'`
Mutation checks: none (code-first; copy and docs only)
Runs: A (1-2) | B1 (3) | B2 (4-5) | V (6-7)
Size: OWNS A RUN — 2 Build batches, 1 feature package (`internal/cli` only; `cmd/quarry` holds tests and docs pins)

User contract: every string is `specification.md` `## Surface & Copy` section A and `### Changes to existing surfaces` 1-6, verbatim; none invented. No behaviour, flag, stdout/stderr or exit-code change. Port survey: n/a. Edge/flag cross: n/a, help only; `--help` of the group, both children and `mcp` are the four cells.
Existing state read: `claude.go:35-37`, `claude_install.go:20-31`, `claude_uninstall.go:20-29`, `mcp.go:28-32` still carry the Code-only copy; README L5/L14/L20/L28/L30/L44/L46 match the spec's line numbers today; PRD `:333` matches, `:266` does not (see Handoff).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `internal/cli/claude_help_test.go` (new) `Test_help_names_both_claude_targets` — table over the four commands, each asserting its ruled Long (group row reuses `claudeGroupHelp`); new package-level consts `claudeInstallLong`, `claudeUninstallLong` (moved from the bodies of `claude_install_test.go:745-765`, `claude_uninstall_test.go:540-560`) and `mcpHelpLong` (hoisted from `mcp_test.go:29-53`) hold the NEW text, each defined once.
- [x] Step 2: pins to the new copy — delete `Test_claude_install_help_prints_the_ruled_text` and `Test_claude_uninstall_help_prints_the_ruled_text` (now table rows); `claude_test.go:11-21` `claudeGroupHelp` (group Long, child rows `install     Install quarry in Claude Code and Claude Desktop` and `uninstall   Uninstall quarry from Claude Code and Claude Desktop`); `mcp_test.go:28-58` `Test_mcp_help_prints_the_ruled_long_text` keeps its name and uses `mcpHelpLong`; `cmd/quarry/run_status_test.go:150` root row (`"  claude      Install quarry in Claude Code and Claude Desktop, or remove it\n"+`, name column stays 12 wide). Run narrow loop: acceptance fails at its Long assertion, pins red with it.

### Build
- [x] Step 3: `claude.go:35-37`, `claude_install.go:20-31`, `claude_uninstall.go:20-29`, `mcp.go:28-32` `Short`/`Long`/paragraph — spec §A and change 3, line breaks exactly as ruled, raw-string Longs with no trailing newline (as today). Acceptance, group-help, mcp and root-help pins green; `Test_claude_commands_agree_with_the_manifests_and_readme` still finds the two `  claude plugin ...` lines per child (check: the new Longs keep them two-space indented, one per line).
- [x] Step 4: `README.md` L5, L14, L20, L28, L30, L44, new paragraph after L44, L46 (spec change 4, text verbatim; keep L20's two commands in backticks) + `cmd/quarry/run_plugin_readme_test.go:14-15` heading consts (`## Use quarry with Claude Code or Claude Desktop`, `## Install or remove quarry`), `:56-77` `readmeClaudeCodeSection`, `:79-101` `readmeInstallSection` (¤ for backticks, byte-equal). `Test_readme_section_*` (names kept: mcp-install cites one), `Test_claude_commands_agree_with_the_manifests_and_readme` and `Test_every_quarry_name_the_skill_uses_exists` green. Order pins (Claude Code < Install < Monthly < Credits) unchanged.
- [x] Step 5: `docs/initial-prd.md:333` -> spec change 5 (replaces the `Plugin install:` bullet); new Security bullet from change 6 inserted after `:266`; `cmd/quarry/run_plugin_notices_test.go:77` two new rows in `Test_notices_and_prd_carry_the_ruled_plugin_edits` (one per PRD line, whole sentence, `collapseWhitespace` match) — departure from sibling precedent: the mcp-install PRD bullet was never pinned; the Gherkin names the PRD, so these two are.

### Sweep
- [x] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; grep `internal/cli` doc comments for "Claude Code only" wording (`newClaudeCommand`, `newClaudeInstallCommand`, `newClaudeUninstallCommand`) and `docs/`/`plugin/` for the retired Shorts (positive control: the old Short in `docs/specifications/mcp-install`).

### Verify
- [x] Step 7: `.claude/scripts/verify.sh <start> ./internal/cli/... ./cmd/quarry/...` + `.claude/scripts/spec-check.py desktop-install` -> tick SCENARIO-16 with its acceptance test; rewrite STATE.md (drop the "Left unbuilt" Help/README/PRD line; note the new README headings).

## Handoff

**Binding decisions:**
- The group, install, uninstall and mcp Longs are asserted once each, in `claude_help_test.go` and `mcp_test.go`; no second copy of the text in any test — drift between copies is what the consts prevent.
- README headings are `## Use quarry with Claude Code or Claude Desktop` and `## Install or remove quarry`; `readmeClaudeCodeHeading`/`readmeInstallHeading` carry them, and `run_skill_drift_test.go:46`, `run_skill_monthly_summary_test.go:69`, `run_claude_drift_test.go:21-22` read the README only through those consts.
- Install Long does not mention D9b (renamed binary). Recommended: leave it. D9b's own stderr line carries the remedy, and the Long lists no other refusal (D4, D9 are not in it either). Not ruled; no copy added.
- Skill drift: the Claude Code section gains no code span (L14, L20 are plain text plus the two existing backticked commands), so `commandMismatches` sees nothing new. `claude_desktop_config.json` appears only in the install section, which skill drift does not scan. Developer confirms by running the narrow loop, not by reasoning.

**Left unbuilt:** nothing in this feature after V. `Test_notices_and_prd_carry_the_ruled_plugin_edits` is the only PRD pin.

**Traps:**
- `readmeSection` matches `heading+"\n"`: the old const is a prefix of the new L5 heading, so forgetting the const edit fails `require.GreaterOrEqual`, not an assert.
- Spec change 6 says "after `:266`"; today `:266` is `Encrypted at rest`, `:268` is `MCP boundary`. Insert literally after `:266`; the spec's line is the order of record. If the orchestrator meant beside the MCP bullet, rule it before B2.
- Two tests deleted at Step 2 (install/uninstall help) were asked to be "re-asserted": they are, as table rows. `Test_mcp_help_prints_the_ruled_long_text` and `Test_claude_prints_its_group_help_on_stdout` keep their names (ticked acceptance tests of mcp-install SCENARIO-17).
- Cobra prints `Long` then a blank line then `Usage:`; Long pins use `HasPrefix` with a trailing newline, the group pin uses `Contains` of `claudeGroupHelp`.

## Phase report

All runs done (A, B1, B2, V). Start commit 7efc8c5c.

V: sweep clean. `docs/`, `plugin/`, `README.md`, `internal/`, `cmd/` carry no retired README heading (`## Install or remove the plugin`, `## Use quarry with Claude Code` bare) nor the old Shorts outside archived mcp-install/phase3d specs (positive control: both found in mcp-install). Doc comments of `newClaudeCommand`/`newClaudeInstallCommand`/`newClaudeUninstallCommand` carry no Code-only wording. Spec change 6 wording updated to "after the `MCP boundary` bullet (orchestrator ruling)".

Verify: `verify.sh` rc=0 for build, test, uncovered-diff (0 added lines uncovered), race, lint (0 issues). test-stats: cmd/quarry 1025 (+0), internal/cli 730 (-1: two help tests deleted, one table test added), TOTAL 1755 (-1).
