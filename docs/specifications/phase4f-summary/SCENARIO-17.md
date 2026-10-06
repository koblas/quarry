---
id: SCENARIO-17
status: open
---

# SCENARIO-17: Docs describe the monthly job

**Unruled copy — rule before dispatch (each has a stated default the developer uses if silent):**
1. `declaredFields` row for `summary --json` (STATE Left unbuilt) needs every field name written between backticks in a reference file's prose, and U15/recipe copy names none. Default: add a short `## Reading the document` section to `monthly-summary.md`: "Run `quarry summary --json` and read `snapshot` (`covers_month` false or null: the store may lack the end of the month; say so), `findings`, `anomalies`, `recurring`, `net_worth` (with `changes`) and `warnings`; relay the warnings." Row names: `month, since, until, currency, snapshot, covers_month, dates, findings, anomalies, recurring, net_worth, changes, warnings`. Alternative: drop the row and the section.
2. Reference file line 1 title and recipe heading: default `# Monthly summary job` (= the ruled SKILL §10 link text), blank line 2, U15 on line 3, recipe under `## Run quarry summary every month`, plist in an ```xml fence, steps numbered 1-5 as in spec :260-297.
3. SKILL §4 row: spec :308 shows no backticks; default follows the recurring row: `` | What happened last month; a monthly summary | `quarry summary --json` (`--month YYYY-MM` for an earlier month) | ``, placed right after the `Unusually large charges` row (SKILL.md:44). The paragraph under the table (`spend, cashflow, recurring and anomalies cover this year`) stays unchanged.
4. SKILL §10 bullet goes last (after Findings, SKILL.md:110); README body backticks `quarry summary`; PRD Decisions bullet appended after `Command names` (docs/initial-prd.md:340).

Cadence: code-first (docs and doc-pinning tests; no mandatory test-first item)
Acceptance test: `cmd/quarry/run_skill_monthly_summary_test.go` `Test_monthly_summary_job_is_documented_where_a_reader_looks`
Narrow loop: `go test ./cmd/quarry/ -run 'skill|reference|readme|declared|use_case|monthly_summary_job'`
Mutation checks: `umask 077` / `only when they ask you to` deleted from the reference → `Test_reference_files_state_their_job`; §10 bullet deleted → `Test_skill_text_carries_the_ruled_frontmatter_and_rules`; README section moved after `## Credits` → `Test_readme_monthly_summary_section_is_verbatim_between_claude_code_and_credits`
Runs: A (1-2) | B1 (3-4) | B2 (5-8) | V (9-10)
Size: OWNS A RUN — 2 batches, 0 feature packages (docs + cmd/quarry tests only)

Existing surfaces surveyed (grep, no LSP — markdown): SKILL §4 is pinned byte for byte (`run_skill_text_test.go:189-207`, `skillSection4`); §10 link list pinned in order (`:34-41`); `Test_every_quarry_name_the_skill_uses_exists`, `Test_references_name_no_mcp_tool`, `Test_references_name_no_quicken_table`, `Test_skill_links_and_reference_paths_resolve` derive from `references/` on disk (`run_skill_drift_test.go:51-72`): the new file is scanned with no edit, so it must pass them. The README drift source is the Claude Code section only (`run_plugin_readme_test.go:27-37`, ends at the next `## `).

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: new `cmd/quarry/run_skill_monthly_summary_test.go` `Test_monthly_summary_job_is_documented_where_a_reader_looks` — one test: `skillRunCell` (run_skill_use_cases_test.go:~188) finds the §4 row and its cell opens `quarry summary --json`; §10 links `references/monthly-summary.md` and the file exists; reference line 3 equals U15; README has `## Run a monthly summary` after the Claude Code section and before `## Credits`; PRD Decisions carries the ruled bullet (spec :305)
- [ ] Step 2: no stubs (no Go symbols); run it, quote the failing assertion (missing §4 row)

### Build
- [ ] Step 3 (batch 1, reference + SKILL): new `plugin/skills/quarry/references/monthly-summary.md` (U15 line 3 + launchd recipe verbatim from spec :260-297, `&` as `&amp;` in XML; no code span equal to `monthly_summary`; never tells Claude to run sync unasked, SKILL §6); `SKILL.md:44` §4 row, `:110` §10 bullet (spec :310). Pins: `run_skill_text_test.go:196` `skillSection4` row, `:34-41` `skillReferenceLinks` (append), `run_skill_references_test.go:102` new case in `Test_reference_files_state_their_job` with U15's phrase list (spec :339) exactly. Fault/edge: none (static text); `Test_every_quarry_name_the_skill_uses_exists` is the check that `summary`, `sync` and `--month` resolve
- [ ] Step 4 (batch 1, JSON-field pin, per ruling 1): `run_skill_json_fields_test.go:49-52` new `declaredFields` row `{file: "monthly-summary.md", argv: {"summary","--json"}, names: …}` (runs under `spendEnv`, clock 2026-09-29, default month August; if that store refuses, use `spendEnvAt` with `summaryClock`, run_helpers_test.go:88); needs the `## Reading the document` section of ruling 1 in the reference
- [ ] Step 5 (batch 2, README): `README.md:26` insert `## Run a monthly summary` between the Claude Code section and `## Credits` (spec :311, link per U16). Pin: new `Test_readme_monthly_summary_section_is_verbatim_between_claude_code_and_credits` + `readmeMonthlySummarySection` const (`¤` for backtick, `ticks()`), modelled on `run_plugin_readme_test.go:15-26`, in `run_skill_monthly_summary_test.go`; plus the link target resolves (`repoFileExists`, run_skill_drift_names_test.go:175)
- [ ] Step 6 (batch 2, PRD): `docs/initial-prd.md:340` Decisions bullet verbatim (spec :305), pinned by Step 1's PRD assertion; PRD :69 and the CLI/MCP tables are already done (S01b, S16): do not touch
- [ ] Step 7 (batch 2, use case): `run_skill_use_cases_test.go:93-104` new `skillUseCases` entry, question = the §4 row's Question cell, argv `summary --json`, answer decodes `document.Summary` and asserts its `Month` — the existing `Test_use_case_argv_matches_the_command_the_skill_names` then ties the row's cell to the argv
- [ ] Step 8 (batch 2, sibling check): `grep -rn "recurring-and-anomalies.md" cmd internal plugin` for any other list that mirrors the reference files; add the new file to each, or say in the phase report that none exists

### Sweep
- [ ] Step 9: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues` (long ruled-copy consts need `//nolint:lll // ruled copy is pinned word for word`, as run_plugin_readme_test.go:38)

### Verify
- [ ] Step 10: full verification per `.claude/rules/agent-briefs.md` + `.claude/scripts/spec-check.py phase4f-summary` → tick SCENARIO-17 with its acceptance test, rewrite STATE.md (drop the SCENARIO-17 Left-unbuilt line), `status: done`

## Handoff

**Binding decisions:**
- `monthly-summary.md` is the single source of the launchd recipe; README links to it, SKILL §10 lists it — spec :257. The log-privacy line and `umask 077` stay (U15 test row pins them).
- SKILL §9 and `mcp --help` Tools line were delivered by S16; this scenario does not edit them.

**Left unbuilt:**
- Hand verification of `$HOME` in a LaunchAgent's `sh -c`, TCC on `~/Documents/*.quicken`, `bootstrap`/`kickstart` on macOS 26 — SCENARIO-18 (orchestrator, `REFERENCE-CHECK.md`); the reference must not assert them beyond the ruled copy.

**Traps:**
- `readmeClaudeCodeText` ends at the next `## `: the new README section goes after it, never inside; the section's own test must not reuse that helper.
- `Test_references_name_no_mcp_tool` fails on a code span that is exactly a tool name (`monthly_summary`); write the tool in prose or inside a longer span.
- SKILL §4 and §10 are pinned byte for byte and link order is pinned: edit SKILL.md and the consts in one batch or the batch is red.
- `unmentionedNames` requires each declared field in backticks in the reference prose; a `declaredFields` row without the prose section fails.
- plist fence lines carry the absolute `/Users/you/go/bin/quarry`, a token that is not `quarry`, so the drift scan skips them; the `quarry sync; quarry summary` span is scanned and must resolve.

## Orchestrator rulings (2026-10-06)

Unruled copy 1-4: all defaults accepted as written above. 1: add `## Reading the document` to monthly-summary.md naming every summary --json top-level field in backticks; keep the declaredFields row. 2-4: defaults as listed.
