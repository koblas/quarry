---
id: SCENARIO-03
status: open
---

# SCENARIO-03: every quarry name the skill uses exists

Cadence: code-first (no production Go and nothing on the mandatory test-first set: only test files and static `plugin/` prose)
Acceptance test: `cmd/quarry/run_skill_drift_test.go` `Test_every_quarry_name_the_skill_uses_exists`
Narrow loop: `go test ./cmd/quarry/ -run 'Test_every_quarry_name|Test_drift_|Test_reference_files_|Test_references_|Test_skill_'`
Mutation checks: flags resolved against the union of all commands, not the bound command's set → `Test_drift_check_flags_crafted_command_text/flag_on_another_command`; any word accepted after a parent → `.../unknown_subcommand`; whole help text scanned instead of the `Flags:`/`Global Flags:` sections → `.../flag_only_in_the_commands_own_prose`
Runs: A (1-2) | B1 (3) | B2 (4-5) | V (6-7)
Size: OWNS A RUN, 3 batches in 1 package (`cmd/quarry` tests) plus 5 static `plugin/` files

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_skill_drift_test.go` (new) `Test_every_quarry_name_the_skill_uses_exists`. It has three subtests, one per outline kind: "quarry commands and flags", "MCP tool names", "tables and v_* views".
  - Sources: SKILL.md (`repoFile`, `run_plugin_manifest_test.go:71-76`); the README section cut from `README.md` between `## Use quarry with Claude Code` and `## Credits`; and `references/*.md` plus `references/sql/*.sql`, both listed from disk.
  - Each subtest first requires one known positive: `snapshots prune` from SKILL §6, `sync_status` from §9, `v_spending` from the recipes. It then asserts zero mismatches.
- [x] Step 2: same file. Signature-only stubs `skillDriftSources`, `helpTree`, `commandMismatches`, `toolMismatches` and `relationMismatches`, returning empty. The test must go red at the positive `Contains`.

### Build
- [x] Step 3: `plugin/skills/quarry/references/{spending,cash-flow,recurring-and-anomalies,search,findings}.md` (new), written to their §S.6 job lines, plus `cmd/quarry/run_skill_references_test.go` (new).
  - **Facts:** take every fact from `quarry <cmd> --help` and the commands' `--json` output. Invent no behaviour.
  - **Layout:** one paragraph, bullet or table row per physical line, the way SKILL.md is written. The drift check binds flags per line.
  - **Recipe paths:** write them as backticked `references/sql/<name>.sql`.
  - **Forbidden:** no MCP tools, no `v_balances_daily`, `v_net_worth` or `v_holdings`, and no Quicken `Z*` names.
  - `Test_reference_files_state_their_job` asserts required phrases per file, whitespace-collapsed:
    - spending: `quarry spend`, `--by`, `--currency`, `refund`, `references/sql/spending-trend.sql`
    - cash-flow: `quarry cashflow`, `savings rate`, `n/a`, `partial`, `references/sql/income-by-category.sql`
    - recurring-and-anomalies: `quarry recurring --json`, `quarry anomalies --json`, the backticked fields `new`, `state`, `first_charge`, `price_changes`, `per_year`, `usual`, `times`, `not_judged`, and `no SQL form`
    - search: `quarry search`, `transfer`, `excluded`, `native`, `--limit`
    - findings: every type that `quarry findings --help` lists under "quarry looks for:" (parsed from the help, never typed out), `` in Quicken, then `quarry sync` ``, `findings.ignore`, `quarry findings --csv`, and `only when the user asks`
  - `Test_references_name_no_phase_4_view_or_quicken_table` covers references/**, SKILL.md and the README section. It reuses `zTableName` (`run_skill_recipes_test.go:495-501`). Its crafted rows each must be flagged: `v_net_worth` and `ZTRANSACTION`.
- [x] Step 4: `run_skill_drift_test.go`, `helpTree` + `commandMismatches`.
  - **Help tree:** walk `run(<path> --help)` (`main_test.go:34-36`) down through each node's `Available Commands:`. Each node's flags are its `Flags:` ∪ `Global Flags:` long names.
  - `Test_drift_help_parser_reads_only_flag_sections`: crafted help whose Long text mentions `--limit` must not yield `--limit`.
  - `Test_drift_check_flags_crafted_command_text` runs against the real tree. One row per case (flagged unless marked otherwise):
    - `unknown_root_command`: `quarry bogus`
    - `unknown_subcommand`: `quarry snapshots bogus`
    - `sub_subcommand`: `quarry snapshots prune --dry-run` resolves
    - `unknown_flag`: `quarry spend --bogus`
    - `flag_only_in_the_commands_own_prose`: `quarry snapshots --from`
    - `flag_on_another_command`: `quarry search --csv`
    - `parenthesised_flag_binds_to_its_row`: `` `quarry accounts --json` (with `--since`) ``
    - `alternatives_are_one_flag`: `--by category\|payee` yields only `--by`, unflagged
    - `double_dash_and_dash_are_not_flags`: `quarry search -- -x`, `quarry sql --json -` resolve
    - `claude_line_ignored`: `claude plugin validate --strict plugin` passes, while `quarry spend --strict` is flagged
    - `go_env_ignored`: `$(go env GOPATH)/bin` passes
    - `colon_is_not_a_command`: `quarry: no store …; run quarry sync to build it` resolves
    - `fence_line`: a fence line is treated like a span
    - `standalone_unknown_flag`: `--bogus` on a line with no command
- [x] Step 5: new file `cmd/quarry/run_skill_drift_names_test.go` with `toolMismatches`, `relationMismatches` and `linkMismatches`, plus `Test_skill_links_and_reference_paths_resolve`.
  - **Tools:** §9 spans come from `splitSkill` (`run_skill_text_test.go:90-123`). The tool list comes from `startMCP`+`ListTools` (`run_mcp_test.go:26-37,69-90`).
  - **Relations:** checked against `storeRelations` (`run_skill_recipes_test.go:555-567`).
  - **Links:** `skillLinkTarget` (`run_skill_text_test.go:43`).
  - `Test_drift_check_flags_crafted_name_text` rows (flagged unless marked otherwise):
    - `unknown_tool`: `bogus_tool`
    - `sync_named_as_a_tool`: `sync` is flagged when the tools list contains it
    - `unknown_view`: `v_net_worth`
    - `from_list`: `FROM v_bogus b, params p` flags `v_bogus` and not the CTE `params`
    - `join_table`: `JOIN transactions t` resolves
    - `dead_link`: `](references/bogus.md)`
    - `dead_backticked_path`: `` `references/sql/bogus.sql` ``
    - `anchor_and_url`: `#frag` and `http…` are skipped

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`. Add doc comments on the new helpers. No SKILL.md edit is planned; if one is needed, make it through the S01 constants.

### Verify
- [ ] Step 7: full verification.
  - `claude plugin validate --strict plugin` and `--strict .`.
  - `spec-check.py phase3d-skill`.
  - Tick SCENARIO-03 with its acceptance test. Rewrite STATE.md and set `status: done`.

## Handoff

**Binding decisions:**
- **Flags are checked per command.** They resolve against the help tree's `Flags:` ∪ `Global Flags:` sections, never against help text. Help prose names flags of other commands: sql's text names `--limit`/`--csv`, and snapshots' names `--from`.
- **Scanned units are code spans and fence lines only.** A command occurrence is the token `quarry` (exactly, so `quarry:` and `koblas/quarry` don't count) followed by lowercase words. The command path descends while the node has children. At a parent, the next word must be a listed child. At a leaf, the remaining words are arguments.
- **How a flag binds** (`--` followed by a letter; `--` alone, `-` and `-v` are not flags):
  1. to the nearest preceding occurrence in its own unit;
  2. else, for a unit that opens with a flag, to the nearest preceding occurrence on the same physical line;
  3. else to the union of all commands.
- **Foreign units are skipped.** A unit whose first token is a word other than `quarry` (`claude`, `go`, `command`, `$(go`, `SELECT`) contributes no flags before its first `quarry` token.
- **§9 tool spans.** Every §9 code span is a tool name, except the names in `mcpNotTools` = {`sync`}. Each of those must be absent from `ListTools` and present as a root command.
- **Relations.** `v_\w+` anywhere in references/**, plus FROM/JOIN list items in `.sql` files and SQL fences, minus CTE names, must be in `storeRelations`.
- **Paths.** Links resolve relative to their file. Backticked `references/…` spans resolve relative to `plugin/skills/quarry/`.
- **§4 rows are not parsed separately.** The drift check does not reuse `skillRunCell` (a STATE suggestion): per-line binding already covers every §4 row and bullet, while `skillRunCell` reaches only the rows a test names.

**Left unbuilt:**
- **Bare table names in reference prose** (a backticked `transactions` outside SQL) are not resolved. Rule P3 gap, awaiting an orchestrator ruling.
- **JSON field names in prose** (`first_charge`, `not_judged`, …) are not resolved against the `--json` output. Only their presence is pinned.
- **Bare command words** such as `spend` and `sync` are not resolved unless they follow `quarry`. Rule P3 scopes the check to `quarry <cmd>`.

**Traps:**
- **Exit codes prove nothing.** `quarry bogus --help` exits 2, but `quarry snapshots bogus --help` exits 0 and prints the snapshots help. Resolve by descent only.
- Build the tree from `<path> --help`, never `help <path>`: `help snapshots prune` prints an empty `Global Flags:`.
- zsh does not word-split an unquoted `$a`, so `B $a --help` with `a="snapshots prune"` reports "unknown command". When probing from the shell, quote the arguments separately.
- **A leaf's trailing words are legal arguments.** `quarry sync to build it` (§8's ruled stderr text) must resolve, so there is no "takes no arguments" check.
- A `FROM` inside a function call (`EXTRACT(year FROM d)`) reads as a relation. Reference SQL avoids it.

## Phase report

Runs A (steps 1-2), B1 (step 3) and B2 (steps 4-5) done, committed as d6d9549. V (steps 6-7) next.
- **Acceptance is green** (`Test_every_quarry_name_the_skill_uses_exists`, all three subtests). Over the real sources the scan reads 642 code units and resolves commands `accounts anomalies cashflow findings recurring search snapshots prune spend sql status sync` and views `v_account_balances v_cash_flow v_spending`, with 0 mismatches.
- **Lint is `0 issues`**, `go build ./...` clean. The sweep (step 6) is mostly done: doc comments are on every helper, trimmed to the 1-2 line budget. V only re-runs it.
- **Signatures changed from the A stubs:**
  - `toolMismatches(t, sources, tools, root helpNode)`. It needs `t` for `splitSkill` and the root for the `mcpNotTools` command check.
  - `linkMismatches(sources, exists func(rel string) bool) driftCheck` returns what it resolved as well, so the real test asserts two positives.
  - `parseHelp(help) ([]string, []string)` is split out of `helpNodeAt`, so the parser has its own crafted-text test.
- **Where things live:**
  - `run_skill_drift_test.go`: `parseHelp`, `helpTree`/`helpNodeAt`, `codeUnits` (spans and fenced lines), `commandScan`, `commandMismatches`, plus `Test_drift_help_parser_reads_only_flag_sections`, `Test_drift_check_flags_crafted_command_text` (17 rows) and `Test_drift_check_resolves_crafted_command_text` (positive controls).
  - `run_skill_drift_names_test.go`: `toolMismatches`, `relationMismatches` (`relationLists`, `sqlOf`), `linkMismatches`, `Test_skill_links_and_reference_paths_resolve`, `Test_drift_check_flags_crafted_name_text` (17 rows).
  - `run_skill_json_fields_test.go` (new, not in the plan's file list): `declaredFields` (file, argv or recipe, names), `Test_every_field_the_reference_prose_declares_is_in_the_output_and_the_prose`, and `Test_declared_field_check_flags_a_name_the_output_lacks` (the absent-field control, for `absentNames` and `unmentionedNames`).
- **JSON fields:** the declared lists were green on arrival, because B1 wrote the prose from the field comments, and the argv pairing is as the B1 report ruled (`spend --by month` for `partial`, `--by payee` for `payee`, `--by tag` for `tag`; the recipe columns `period`/`currency`/`spent` and `category`/`currency`/`income` are checked against `runShippedRecipe` columns). Search is `search "Savings Sweep" --json`, because `transfer` is only present there.
- **Mutations** (each reverted, `diff` identical; all in `run_skill_drift_test.go`):
  1. Flags checked against any command (`checkFlag` to `checkAnyFlag` for an own occurrence). RED: `Test_drift_check_flags_crafted_command_text/flag_on_another_command` (expected `unknown flag --csv for "quarry search"`, actual nil), and also `flag_only_in_the_commands_own_prose`, `unknown_flag`, `quarry_line_with_the_same_flag`.
  2. Any word accepted after a parent (the unknown-child `report` and `return` replaced by `break`). RED: `.../unknown_subcommand` (expected `unknown command "quarry snapshots bogus"`, actual nil), and `unknown_root_command`, `fence_line`.
  3. Whole help text scanned (`parseHelp` flags re-collected over the full text). RED: `.../flag_only_in_the_commands_own_prose` (expected `unknown flag --from for "quarry snapshots"`, actual nil) and `Test_drift_help_parser_reads_only_flag_sections`.
- **Mechanics worth knowing:**
  - A flag binds to the nearest preceding `quarry` token in its unit; a unit that opens with a flag binds to the nearest `quarry` earlier on the line, else to any command. A flag after an unresolved command is skipped, since the command is already reported.
  - Command words are stripped of trailing `.,;` but the `quarry` token must match exactly.
  - `.sql` sources are skipped by the command scan. A relation FROM/JOIN list ends at the next clause keyword or `)`/`;`. Go regexp has no lookahead, so `relationLists` finds each keyword with `FindAllStringIndex` and cuts the tail itself.
  - SQL fences are those opened with `sql` or holding a `SELECT`.
  - Links are resolved relative to `path.Dir(source.name)`, so the README section (named `README.md Claude Code section`) resolves from the repo root.
- **Left to V:** step 6's re-run, the full verification (covered full suite, `uncovered-diff.py`, `test-stats.py --base e19ff35`, `-race` on `cmd/quarry`), both `claude plugin validate --strict` runs, `spec-check.py phase3d-skill`, ticking SCENARIO-03 with its acceptance test, STATE.md rewrite, `status: done`. STATE.md `## Open debts` must record the ruled gap: bare table names (a backticked `transactions` outside SQL) in reference prose are unchecked, and bare command words not preceded by `quarry` are unresolved.

## Orchestrator rulings (2026-10-03, before run A)
- **Table names in reference prose** (outside SQL fences and `.sql` files) stay unchecked. This is accepted, and recorded in STATE.md `## Open debts` by run V. The `v_*` names and the SQL relations are checked, and `schema.md` is generated.
- **JSON field names are closed in B2.** For each reference file that names `--json` fields (`recurring-and-anomalies.md`, `search.md`, `spending.md`, `cash-flow.md`, `findings.md` where applicable), the test declares the field names that file's prose uses, together with the command whose output carries them. Each declared field must:
  - appear in the prose;
  - be a key, at any depth, in that command's `--json` output on `skillEvalStore`.

  Add one control: a crafted field name that is absent from the output must be flagged. This is test-side data; no prose parsing is required.
- **The `claude plugin validate --strict` runs in V** are fine. `claude` is at /opt/homebrew/bin/claude.
