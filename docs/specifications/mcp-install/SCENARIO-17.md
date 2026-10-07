---
id: SCENARIO-17
status: done
---

# SCENARIO-17: mcp help points at quarry claude install (folds 18, 27, 27b, 28)

Cadence: code-first (no mandatory test-first item: no file write, no atomic adapter; only production logic is the group's unknown-subcommand refusal)
Acceptance test: `internal/cli/mcp_test.go` `Test_mcp_help_prints_the_ruled_long_text` (existing test, `const long` rewritten to the ruled paragraph)
Acceptance test (SCENARIO-18, folded): `cmd/quarry/run_plugin_readme_test.go` `Test_readme_section_for_installing_and_removing_the_plugin_is_verbatim` (new)
Acceptance test (SCENARIO-27, folded): `internal/cli/claude_test.go` `Test_claude_refuses_an_unknown_subcommand` (new file)
Acceptance test (SCENARIO-27b, folded): `internal/cli/claude_test.go` `Test_claude_prints_its_group_help_on_stdout` (bare and `--json`)
Acceptance test (SCENARIO-28, folded): `cmd/quarry/run_status_test.go` `Test_run_help_prints_quarrys_description` — green on arrival (row already at :150; `claude.go:16` Short already ruled)
Narrow loop: `go test ./internal/cli/ -run 'Mcp|Claude' && go test ./cmd/quarry/ -run 'readme|plugin|claude|help|usage|skill'`
Mutation checks: group refusal in `newClaudeCommand` (drop `Args`/the arg check) → `Test_claude_refuses_an_unknown_subcommand` and the `claude bogus` row in `run_usage_test.go`; group `RunE` returning nil instead of `cmd.Help()` → `Test_claude_prints_its_group_help_on_stdout`; edit one README command line → README pin and `Test_claude_commands_agree_with_the_manifests_and_readme`; change marketplace.json `name` → the drift test; change install Long's `koblas/quarry` → drift test (go.mod tie) and `Test_claude_install_help_prints_the_ruled_text`
Runs: A (1) | B1 (2-4) | V (5-6)
Size: OWNS A RUN — 3 Build batches, 0 feature packages (cli + cmd/quarry tests + README/PRD text)

Surveyed (existing, no new port): `claude.go:11-24` group has Short/Long already matching spec; no `Args`/`RunE`. `run.go:89` Execute appends `; Run '<CommandPath> --help' for usage.` to any non-Usage error, with the matched command (the group). Caller table: LSP not needed; every row grep (strings): `claude`-group pins live in `run_status_test.go:150`, `run_usage_test.go:223-230,335-340,416-433`, `run_read_usage_test.go:16-18,60-65`, `claude_install_test.go:709-731`, `claude_uninstall_test.go:416-436`.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `internal/cli/mcp_test.go:28-53` `Test_mcp_help_prints_the_ruled_long_text` — replace the first paragraph of `const long` with the spec's `### Changes to existing surfaces` paragraph (rest unchanged); red at `HasPrefix`. `mcp_test.go:55-62` root-row test needs no change

### Build
- [x] Step 2: `internal/cli/mcp.go:25-28` `newMCPCommand` Long — first paragraph per spec (names `quarry claude install`, other clients, full-path sentence); Step 1 goes green
- [x] Step 3: `internal/cli/claude.go:12-24` `newClaudeCommand` + new `internal/cli/claude_test.go` — group gets `Args` (cobra.NoArgs yields `unknown command "bogus" for "quarry claude"` and Execute adds the ruled suffix; or a hand-built `UsageError`) and a `RunE` printing `cmd.Help()` to stdout. RunE is mandatory: cobra returns help for a non-runnable command before `Args` runs, so `Args` alone changes nothing. Tests: `Test_claude_refuses_an_unknown_subcommand` (exact `err.Error()` via `errors.AsType[UsageError]`/Execute, `claude --json bogus` row, `tool.argv` empty, child `claude install extra` unchanged); `Test_claude_prints_its_group_help_on_stdout` (`claude` and `claude --json`: err nil, stderr empty, stdout holds group Long + both child rows, `tool.argv` empty). Add `claude bogus` row to `cmd/quarry/run_usage_test.go:233` table (exit 2, ruled stderr). Check `claude --help` output: a runnable group may gain a `quarry claude [flags]` Usage line; if it does, STOP and report (not ruled)
- [x] Step 4: README + PRD + drift pins — (a) `README.md:14-19` block becomes fenced `quarry claude install` + sentence `This runs claude plugin marketplace add koblas/quarry and claude plugin install quarry@quarry for you; you can run those two yourself instead.`; `README.md:23` first sentence per spec; (b) new `## Install or remove the plugin` between that section and `## Run a monthly summary`: fenced install pair and uninstall pair copied from the two Longs (`claude plugin ... --scope user ...`), what each does in 1-2 sentences (re-run safe, restart Claude Code, project copies stay and are named, store untouched), and MCP-only alternative `claude mcp add --scope user quarry -- quarry mcp`; README voice (second person, short, `quarry` in backticks); (c) `cmd/quarry/run_plugin_readme_test.go:41-62` `readmeClaudeCodeSection` const edited to the new block + sentence; new const `readmeInstallSection` + `Test_readme_section_for_installing_and_removing_the_plugin_is_verbatim` (extract by heading with the `readmeClaudeCodeText` method, generalise it to `readmeSection(t, heading)`; assert order Claude Code < Install < `## Run a monthly summary` < Credits); (d) new `cmd/quarry/run_claude_drift_test.go` `Test_claude_commands_agree_with_the_manifests_and_readme`: read `claude install`/`claude uninstall` help through `run`, take the indented `claude plugin ...` lines; assert each appears verbatim in the README install section (one control: count 4); assert `quarry@quarry` = `plugin.json` name + `@` + `marketplace.json` name, uninstall's `marketplace remove` target = marketplace name, and `koblas/quarry` = `go.mod` module path minus `github.com/` (the only in-repo statement of the repo; reuse `repoFile`); (e) `docs/initial-prd.md` Decisions: new bullet after `Interfaces:` (`:332`; spec's ":326" is the risk table, stale) with the spec's sentence; PRD :270 untouched (spec does not rule it)

### Sweep
- [x] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues` (`//nolint:lll` on the new ruled README const as the existing one has); doc comment on `newClaudeCommand` says it prints help bare and refuses an unknown name; `newRootCommand` doc already names claude (`root.go:10`), no edit

### Verify
- [x] Step 6: `.claude/scripts/verify.sh <start> ./internal/cli/... ./cmd/quarry/...`; `spec-check.py mcp-install`; tick SCENARIO-17 with its acceptance test and a line per fold (18, 27, 27b, 28 each naming its test, 28 "green on arrival"); update STATE.md (feature complete, `## Left unbuilt` empty)

## Phase report

Runs A, B1 and V done, uncommitted. Acceptance `Test_mcp_help_prints_the_ruled_long_text` green; all six ticked tests pass under `spec-check.py --run mcp-install`.
- A/B1: `mcp.go` Long; `claude.go` group `Args: cobra.NoArgs` + `RunE` printing help; new `claude_test.go`; `claude bogus` row in `run_usage_test.go`; README install block, network sentence, new `## Install or remove the plugin`; `readmeSection(t, heading)`; new `run_claude_drift_test.go`; PRD Decisions bullet. Mutations on those guards recorded in earlier runs (each restored).
- V: checkpoint pin added in the drift test: README Claude Code section must contain each install line without ` --scope user`, in backticks. Mutation: marketplace-add repo changed to `other/repo` in README and `readmeClaudeCodeSection` together -> `Test_claude_commands_agree_with_the_manifests_and_readme` red, README verbatim pin stays green (const edited in step); restored, byte-identical.
- Spec line `quarry claude` amended to "group; runnable only to print help / refuse unknown names" (orchestrator-approved). Progress line ticked with folds named; 28 green on arrival.
- Sweep: `go build ./...` ok, `golangci-lint run ./...` 0 issues.
- verify.sh 3acb48d7: `go build rc=0`, `go test rc=0`, `uncovered-diff: 0 uncovered added line(s) in 0 run(s)`, `go test -race rc=0`, `golangci-lint rc=0`. test-stats rows: cmd/quarry 1022 (+2) tempdir 690 (+0) disk 609 (+0); internal/cli 672 (+2) 2 (+0) 1 (+0); TOTAL 1694 (+4) 692 (+0) 610 (+0).

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- The group is runnable only to refuse an unknown name and print help — cobra checks `Runnable()` before `Args`, so `Args` on a non-runnable group is dead code. Reversing it brings back exit 0 for `quarry claude bogus`
- The `install` Long's `koblas/quarry` is tied to `go.mod`, not to `marketplace.json`: the manifest's `owner.name` is "David Koblas" (a display name), so the spec's "name/owner" tie cannot be literal; the owner stays pinned by the byte pin in `run_plugin_manifest_test.go:31-37`
- README new section holds the four Long command lines verbatim (with `--scope user`); the install block keeps the scope-less pair the spec rules; the drift test checks only the new section
- No `SKILL.md`/references edit: grep found no MCP-setup or `claude plugin` text there (only the PATH row at `SKILL.md:86`); the skill drift test reads README's Claude Code section, whose `quarry claude install` resolves in the help tree

**Left unbuilt** — named so nobody assumes it exists:
- `--json` for install/uninstall, `--desktop`, `--scope`, project-scope removal — out of scope per spec, no owner
- Adding the new README section to `skillDriftSources` (`run_skill_drift_test.go:42-48`) — not planned; `claude mcp add ... -- quarry mcp` may confuse its scanner

**Traps** — things that look right and are not:
- `readmeClaudeCodeText` ends at the next `\n## `: once the new section exists the old const must end at the install block's replacement, not include the new heading
- Root-help claude row and group Short are already ruled and present; any diff there is a bug
- `quarry claude --json` alone must still print help (ruled); `--json` is the root's persistent flag, `jsonOut` is not read by the group
- `Test_claude_*_help_prints_the_ruled_text` and `Test_claude_uninstall_reports_each_outcome` must stay byte-identical; group-help `Contains` pins (`  install     ...`, `  uninstall   ...`) break if the child column width changes

## Orchestrator ruling (2026-10-07)

Step 3: if making the group runnable adds a `quarry claude [flags]` usage line to its help, accept it and pin what cobra prints; do not stop. Only Short/Long are ruled copy.
