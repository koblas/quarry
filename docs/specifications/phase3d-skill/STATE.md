# phase3d-skill — current state

Scenarios complete: SCENARIO-01 (with SCENARIO-07 folded), SCENARIO-02. Last updated by SCENARIO-02.

## Binding decisions
- Plugin files are static; no production Go. All pins are package-main tests in `cmd/quarry` reading repo files via `repoFile(t, rel)` (`run_plugin_manifest_test.go`), the one reader S02-S06 reuse (SCENARIO-01)
- Manifests (`.claude-plugin/marketplace.json`, `plugin/.claude-plugin/plugin.json`) are byte-equal to Go constants; MCP config only inline in `plugin.json`, `plugin/.mcp.json` pinned absent (SCENARIO-01)
- Test constants are the copy source: SKILL.md sections 1-9, frontmatter, intro, credit and the README section are each byte-equal to a constant in `run_skill_text_test.go` / `run_plugin_readme_test.go`. Edits to ruled copy go through those constants, never around them (SCENARIO-01)
- `¤` is the backtick stand-in in raw-string constants, resolved through `ticks(s)`; new copy constants use it (SCENARIO-01)
- De-quote rule: SKILL.md has no `>` blockquotes; each former quoted line is its own paragraph except fenced blocks and list items (SCENARIO-01)
- Section 10 pinned only by six link targets in order (`references/schema.md`, `spending.md`, `cash-flow.md`, `recurring-and-anomalies.md`, `search.md`, `findings.md`) and the credit as last non-empty line; descriptive text after each link is free (SCENARIO-01)
- `status --json` paths the skill reads (`snapshot.taken_at`, `dates.last`, `rates.fetch_error`, `rates.last`, `findings.open`) are pinned by `Test_status_json_carries_each_path_the_skill_reads` and must appear as plain substrings in SKILL.md section 1 (SCENARIO-01)
- Frontmatter description is a single plain-scalar line, no `: `, ≤ 1536 runes; no YAML parsing, no `go.mod` change (SCENARIO-01)
- The notices clause and the four PRD replacement sentences are pinned by `Test_notices_and_prd_carry_the_ruled_plugin_edits` (whitespace-collapsed, so wrapping is free) (SCENARIO-01)
- `claude plugin validate --strict plugin` and `--strict .` both pass (SCENARIO-01)
- `plugin/skills/quarry/references/schema.md` is generated, never hand-edited: `generateSchemaReference(t, home)` in `run_skill_schema_reference_test.go` is the one generator (relations from `duckstore.Schema`, view comments from `duckdb_views()`, `report.SQLConventions`, `findings holds` paragraph from `run sql --help`). Regenerate with `go test ./cmd/quarry -run Test_skill_schema_reference_matches_the_committed_file -update`; the package-level `-update` flag is the only `flag.` use in `cmd/quarry`. S03 reads `schema.md` as a plain file and must not re-derive it (SCENARIO-02)
- `schema.md` carries no user data: empty store equals populated store, no account/category/payee names, no `v_balances_daily`/`v_net_worth`/`v_holdings`; first line is the generated-by header (SCENARIO-02)

## Left unbuilt
- `plugin/skills/quarry/references/*` (five `.md` files, `sql/*.sql`; `schema.md` is built) — S03 batch 1, S04
- Link and path resolution (section 10 links; backticked `references/…` paths in sections 4 and 5, relative to `plugin/skills/quarry/`) — S03 batch 3
- Command, flag, MCP tool and table/view resolution for SKILL.md and README — S03

## Traps
- S.4 section 4 first table row has `\|` inside code spans; S03's drift check must read `--by category\|payee\|tag\|month` as one flag `--by`, not split on `\|` (SCENARIO-01)
- Do not reflow or re-quote the description line: validator and pin both want the single plain line (SCENARIO-01)
- `populatedAnalysisStore` must not be edited (exact-bytes goldens; `schema.md` golden reads it read-only) (SCENARIO-01, SCENARIO-02)
- A change to any `COMMENT ON VIEW` text in `internal/store/duckstore/schema.go` or to `report.SQLConventions` / `run sql --help` reddens the schema golden until regenerated with `-update` (SCENARIO-02)
- SKILL.md section 1 writes `<rates.last>` without backticks, so path pins match plain substrings, not backticked ones (SCENARIO-01)

## Open debts
- Checkpoint S02 MINOR: `cmd/quarry/run_skill_schema_reference_test.go:113` `if *updateSchemaReference` branch in the test body; move it into a `refreshSchemaReference(t, got)` helper; unowned
- Checkpoint S02 MINOR: `cmd/quarry/run_skill_schema_reference_test.go:114,118,197` hand-build `"../../"+path`, and `readSchemaReference` duplicates `repoFile` (`run_plugin_manifest_test.go:71`); reuse `repoFile`; unowned
- Checkpoint S02 MINOR: `cmd/quarry/run_skill_schema_reference_test.go:173` `require.Len(comments, 2)` breaks when a third commented view is added; use `NotEmpty`; unowned
- Checkpoint S02 MINOR: `cmd/quarry/run_skill_schema_reference_test.go:120` regenerate hint hard-codes the test name that `:70` derives; unowned
- `THIRD_PARTY_NOTICES` "Used in:" keeps its existing parenthetical and appends `; plugin/ (…)`; S.9's "Now" column abbreviated it, so this is a flagged reading, not a ruled deletion — product-vision final pass to confirm
- Proposed `.claude/CLAUDE.md` sentence about `plugin/` (spec "flagged only") not applied; outside this pipeline — unowned
