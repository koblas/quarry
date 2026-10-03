---
id: SCENARIO-01
status: open
---

# SCENARIO-01: The marketplace lists the quarry plugin, which starts quarry's MCP server (absorbs SCENARIO-07)

Cadence: code-first (nothing on the mandatory test-first set: no production Go, no write-safety guard, no atomic adapter)
Acceptance test: `cmd/quarry/run_plugin_manifest_test.go` `Test_plugin_manifests_list_quarry_and_start_its_mcp_server`
Acceptance test (SCENARIO-07, folded): `cmd/quarry/run_skill_text_test.go` `Test_skill_text_carries_the_ruled_frontmatter_and_rules`
Narrow loop: `go test ./cmd/quarry/ -run 'plugin|skill|readme|status_json_carries'`
Mutation checks (follow `proof.md` → Mutation verification): add `plugin/.mcp.json` → `Test_plugin_manifests_list_quarry_and_start_its_mcp_server` | change one word in SKILL.md section 6 → `Test_skill_text_carries_the_ruled_frontmatter_and_rules` | change a README section word → `Test_readme_section_for_claude_code_is_verbatim_and_precedes_credits` | rename json tag `fetch_error` at `internal/report/document/status.go:82`, then restore → `Test_status_json_carries_each_path_the_skill_reads`
Runs: A (1-2) | B1 (3-5) | V (6-8)
Size: OWNS A RUN — 3 batches, 0 feature packages (static files plus `cmd/quarry` package-main tests); SCENARIO-07 folded in

## Where copy lives and how it is pinned (binding for this plan)

- **No production Go.** Existing for reuse: `syncBundle` `run_helpers_test.go:23`, `writeStatusFixtureBundle` `run_status_test.go:161`, `run` (package main). No code survey of ports needed: nothing here is a port.
- **Manifests: whole file byte-equal** against a Go string constant (S.2 is the whole file; JSON has no backticks; constant ends in `\n`).
- **SKILL.md and the README section: section by section, byte-equal per section.** A whole-file constant is impossible: the section 10 one-liners are developer-authored (S.6 gives jobs, not copy), and S02/S03 will touch them. The test holds each ruled block once, as a constant; SKILL.md is the deliverable; the spec is not read at test time.
  - Go raw strings cannot hold backticks. Constants use `¤` for a backtick, through one helper (`ticks(s)`) defined in `run_skill_text_test.go`. `¤` is not in the spec text (checked); the helper's test control: a constant containing `¤` resolves to a string with none left.
  - Split SKILL.md once into: frontmatter, intro (between the `# ` title and `## 1.`), and a map from `## N. …` heading line to body. Table rows: heading → expected body, for sections 1-9. Assert the heading set and order are exactly `## 1.` … `## 10.`, once each, with the ruled titles (section 3 `## 3. Conventions`, 4 `## 4. Pick the command`, 5 `## 5. When to use quarry sql`, 6 `## 6. quarry cannot change data`, 7 `## 7. Not covered yet`, 8 `## 8. When a command fails`, 9 `## 9. Without a shell: MCP tools`, 10 `## 10. References`, 1 `## 1. Check freshness first`, 2 `## 2. Every number comes from quarry`).
  - **De-quote when writing SKILL.md: yes.** Every `> ` blockquote prefix in S.4 is the spec's own display device; strip it (the S.4 section 5 fence inside the quote becomes a plain fenced block). Each former `> ` line becomes its own paragraph (blank line between), except lines inside a fence and list items, which stay contiguous. The test constants then fix this reading for every later scenario. S.4 section 4 (table, then `Dates are …` paragraph) and section 8 (the S.5 table, heading then blank then table; no extra prose) are ruled copy too; keep the `\|` escapes in the section 4 first row.
  - **Frontmatter:** exact block `---\nname: quarry\ndescription: <S.3 text>\n---` equal to the constant; description rune count ≤ 1536 (asserted as a number, not the spec's 1,197). Do not parse YAML (`go.yaml.in/yaml/v3` is indirect only; no `go.mod` change).
  - **Section 10:** pin only (a) the six link targets, in S.1 order (`references/schema.md`, `spending.md`, `cash-flow.md`, `recurring-and-anomalies.md`, `search.md`, `findings.md`), each on its own line, and (b) the credit sentence as the last non-empty line of the file. The descriptive text after each link is unpinned. **The test must not stat the six files: they do not exist until S02/S03.** Link resolution is deferred to S03 batch 3 (see Handoff).
- **`status --json` path pins** are a test over a synced fixture store (`syncBundle(t, writeStatusFixtureBundle(t, home))`, as `run_status_json_test.go:18`): decode stdout into `any`, walk each dotted path (`snapshot.taken_at`, `dates.last`, `rates.fetch_error`, `rates.last`, `findings.open`) and require the key present (null allowed for `fetch_error`); `taken_at` and `dates.last` non-null strings, `findings.open` a number. Each path must also appear verbatim in SKILL.md section 1 so the list and the text cannot diverge. This extends Rule P3 beyond its literal list (orchestrator ruling).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_plugin_manifest_test.go` (new) `Test_plugin_manifests_list_quarry_and_start_its_mcp_server` — reads `../../.claude-plugin/marketplace.json` and `../../plugin/.claude-plugin/plugin.json`, byte-equal to the S.2 constants; `plugin/.mcp.json` absent; helper `repoFile(t, rel)` (reads `../../`+rel) lives here for later scenarios. Red: the first read fails (files absent)
- [x] Step 2: `cmd/quarry/run_skill_text_test.go` (new) `Test_skill_text_carries_the_ruled_frontmatter_and_rules` + `ticks` helper + section splitter — frontmatter, intro, sections 1-9 table rows (S.4, S.5), heading order, section 10 links, credit line. Red: `../../plugin/skills/quarry/SKILL.md` absent

### Build
- [ ] Step 3: `.claude-plugin/marketplace.json`, `plugin/.claude-plugin/plugin.json` (new, verbatim S.2) + in the Step 1 test a decode pin: marketplace plugin `source` starts `./` and `<source>/.claude-plugin/plugin.json` exists; plugin `name` equals the marketplace plugin's `name`; `mcpServers.quarry` has `command` `quarry` and `args` `["mcp"]` — green on acceptance 1
- [ ] Step 4: `plugin/skills/quarry/SKILL.md` (new; S.3 frontmatter, `# Answer questions from Quicken data with quarry`, intro, sections 1-10, credit line) + `Test_status_json_carries_each_path_the_skill_reads` in `cmd/quarry/run_status_json_test.go` after `:18-96` (or the skill text file; one test, one place) — green on acceptance 2. Section 10 one-liners: write from S.6 jobs
- [ ] Step 5: `README.md:5` insert `## Use quarry with Claude Code` (S.8 verbatim, incl. its two code fences) before `## Credits`; `cmd/quarry/run_plugin_readme_test.go` (new) `Test_readme_section_for_claude_code_is_verbatim_and_precedes_credits` — section from its heading to the next `## ` equals the constant (via `ticks`), and its index is below `## Credits`'s

### Sweep
- [ ] Step 6: `THIRD_PARTY_NOTICES:7-8` dweekly "Used in:" — keep the existing parenthetical and append `; plugin/ (skill layout and the untrusted-data and reporting rules in SKILL.md)`; `docs/initial-prd.md:215` (references list and "generated from the store" clause, S.9 rows 1-2), `:217` (row 3), and one sentence under `**Open questions**` (~`:330`, row 4: "Quicken's category tax line is not imported; tax totals are by user-named category until it is."); leave `:129`, `:117`. These doc edits are not pinned by tests (prose, not contract). Then fix what `go build ./... && golangci-lint run ./...` reports, to `0 issues`

### Verify
- [ ] Step 7: full verification per `.claude/rules/agent-briefs.md` → Verification (`uncovered-diff.py` reports nothing: no production line added); if `claude` is on PATH run `claude plugin validate --strict plugin` and `claude plugin validate --strict .` once each, foreground, and report both exit codes, else say unverified (the orchestrator's exit evidence covers it)
- [ ] Step 8: `.claude/scripts/spec-check.py phase3d-skill`; tick SCENARIO-01 with its acceptance test and SCENARIO-07 as `delivered by SCENARIO-01` before the test reference (reference last on the line); write `STATE.md`; set `status: done`

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- SKILL.md has no `>` blockquotes; each ruled block is a section pinned byte-equal by `ticks`-constants in `run_skill_text_test.go` — later edits to ruled sections go through that table, never around it.
- The test pins section 10 by link target and order only; descriptive text after each link is free — so S02/S03 may refine it without touching this test.
- `repoFile(t, rel)` (`run_plugin_manifest_test.go`) is the one reader of repo files from `cmd/quarry`; S02-S06 reuse it.
- No `plugin/.mcp.json`; MCP config only inline in `plugin.json` (Rule P1) — pinned absent.
- `¤` is the backtick stand-in for Go raw-string copy constants; any new copy constant uses `ticks`.

**Left unbuilt** — named so nobody assumes it exists:
- `plugin/skills/quarry/references/*` (schema.md, five `.md` files, `sql/*.sql`) — S02, S03 batch 1, S04.
- Link and path resolution (§10 links, plus backticked `references/…` paths in sections 4 and 5, relative to `plugin/skills/quarry/`) — S03 batch 3 stats them; S01 deliberately does not.
- Command, flag, MCP tool and table/view resolution for SKILL.md and the README — S03.

**Traps** — things that look right and are not:
- S.4 section 4's first table row contains `\|` inside code spans: the drift check in S03 must split `--by category\|payee\|tag\|month` as one flag `--by`, and not treat `\|` as a cell boundary.
- Description is plain-scalar YAML with no `: ` inside; do not "fix" quoting or line-wrap it — `claude plugin validate` and the pin both want the single line.
- `THIRD_PARTY_NOTICES` "Used in:" has an existing parenthetical; S.9's "Now" column abbreviates it. Keep it and append (flagged reading, not a ruled deletion).
- `populatedAnalysisStore` must not be edited (exact-bytes goldens); this scenario does not use it.

## Phase report

Run A (steps 1-2) done. Red, both at the first `repoFile` read (`require.NoError`): `open ../../.claude-plugin/marketplace.json: no such file or directory` and `open ../../plugin/skills/quarry/SKILL.md: no such file or directory`. No production Go.

Files:
- `cmd/quarry/run_plugin_manifest_test.go` (new): `wantMarketplaceJSON`, `wantPluginJSON` (byte-equal, end in `\n`), `repoFile(t, rel) string`, the acceptance test (also asserts `plugin/.mcp.json` absent; that arm passes today). Step 3 adds the decode pin here.
- `cmd/quarry/run_skill_text_test.go` (new): `ticks`, `splitSkill`/`skillText`, `referenceLinkTargets`, `lastNonEmptyLine`, the acceptance test, and constants `skillFrontmatter`, `skillIntro`, `skillSection1..9`, `skillCredit` at file bottom, generated once from the spec's S.3-S.5 with the de-quote rule. `//nolint:lll` sits above the const block (lines over 200 chars).
- Lint on `./cmd/quarry/...` is `0 issues` (fmt run).

For B1 (step 4) when writing SKILL.md, the constants fix the shape:
- Frontmatter is `---\nname: quarry\ndescription: <one line>\n---`; then a blank line, `# Answer questions from Quicken data with quarry`, blank, intro paragraph, blank, `## 1. …`.
- Each section: heading, blank line, body, blank line. Bodies are trimmed of newlines before comparing. Blockquotes de-quoted: each non-list, non-fence line is its own paragraph; list items contiguous; blank line between a paragraph and a following list or fence.
- Section 4 keeps `\|` in the first row; section 8 is the S.5 table only.
- Section 10: one `](references/…)` link per line in S.1 order; credit sentence is the last non-empty line of the file. Anything else in section 10 is free.
- The `status --json` path test (step 4) is not written yet; `Test_status_json_carries_each_path_the_skill_reads` still to add.

Do not redo: the constants; do not hand-edit them (they match the spec text byte for byte).
