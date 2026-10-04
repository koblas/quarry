---
id: SCENARIO-11
status: open
---

# SCENARIO-11: Existing surfaces stop saying investments are not imported

Cadence: code-first (no mandatory test-first item: copy and comments only, no guard, no adapter)
Acceptance test: `cmd/quarry/run_accounts_not_valued_test.go` `Test_run_accounts_shows_not_valued_for_brokerage_and_retirement_accounts`
Narrow loop: `go test ./cmd/quarry/ -run 'Accounts|Currency|Usage|SyncHelp|Skill|Notices|SharedDocuments|(?i)sql' && go test ./internal/cli/ ./internal/report/... ./internal/mcp/`
Mutation checks: `notValuedBalance` cell text (`render_accounts.go:13`) reverted to `not imported` → `Test_run_accounts_shows_not_valued_for_brokerage_and_retirement_accounts`
Runs: A (1-2) | B1 (3) | B2 (4-5) | V (6-7)
Size: OWNS A RUN — 3 batches, cli + plugin + report (one feature package touched for behaviour: `internal/cli` cell/help; the rest is copy). Phase-1 spec superseded note verified present (`phase1-import-store/specification.md:229`, `:527`) — do not redo.

Survey (grep, whole tree minus `.git/.devenv/specifications/.claude`): remaining production `not imported` = `render_accounts.go:12,13,79`, `accounts.go:17-18`, `json_accounts.go:17`, `store.go:50-52` (comment). Left alone on purpose: `importer/investments.go:209` (skipped position, different meaning), `docs/initial-prd.md:349` (category tax line), `docs/adr/001-shared-store-package.md:22` (historical, unowned), `run_status_shares_test.go:28` (`NotContains`, stays).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_accounts_not_valued_test.go` `Test_run_accounts_shows_not_valued_for_brokerage_and_retirement_accounts` — `run(...["accounts","--currency","native"])` after a real `sync` of a `v9fixture` store holding a CHECKING, a `BROKERAGENORMAL` and a `RETIREMENTIRA` account, each investment account with one cash-only investment row (model `run_investments_test.go:99-113`: `b.InvestmentTransaction`, no Position, no shares, so the share gate has nothing to check). Assert whole stdout (both investment rows read `not valued`, aligned to the new 10-char cell) and `NotContains "not imported"`
- [x] Step 2: `render_accounts.go:12-13` `notImportedBalance` — rename `notValuedBalance` = `"not valued"` only as far as needed so the test compiles; must fail at its assertion first (cell still `not imported`)

### Build
- [ ] Step 3 (batch 1, accounts + sync copy): `render_accounts.go:12-13,79` const + `accountBalance` doc; `json_accounts.go:17` doc comment (`absent, not valued, or unconverted`); `accounts.go:17-18` Long = S.7 sentence verbatim, hard-wrapped at the file's ~72-col style, rest of Long unchanged; `sync.go:46-50` Long = S.7 sentence verbatim wrapped the same way. Re-pin, then fix whatever the narrow loop reds (column width shrinks by 2 in every row of each table):
  - `run_accounts_test.go:32,51` cells + `:96-97` Long inside `Test_run_accounts_help_describes_the_command_without_needing_home` (Long asserted at wrap width) — this is the accounts-Long pin
  - `run_usage_test.go:49-54` inside `Test_run_prints_the_sync_help` — the sync-Long pin, byte-exact wrap
  - `run_accounts_fx_test.go:33,57,62`, `run_accounts_fx_edges_test.go:100,104,109,185`, `run_currency_native_test.go:91`
  - `internal/cli/render_accounts_internal_test.go:38,48,83,84,114,120,160,168,176,192-200` (also rename `Test_renderAccounts_leaves_a_not_imported_cell_blank_…` and `notImported` locals), `fx_warning_internal_test.go:59,67,68`, `internal/report/accounts_test.go:155,166` (names only)
  - Cross-cell crossing: closed and not-in-reports status columns (`render_accounts_internal_test.go:160-176`), `--currency CAD` blank-converted cell (`:114-120`), `--all` (`run_accounts_test.go:51`), `--json` carries `null` balance for an investment account — acceptance test also asserts `accounts --json` has `"balance":null` for the brokerage row (n/a for text-only re-pins elsewhere)
- [ ] Step 4 (batch 2, SKILL.md): `plugin/skills/quarry/SKILL.md:3` description clause, `:72`, `:73` = S.7 copy verbatim. Update the byte-pinned copies in `cmd/quarry/run_skill_text_test.go:165` (`skillFrontmatter`) and `:218-219` (`skillSection7`) in `Test_skill_text_carries_the_ruled_frontmatter_and_rules`; keep description ≤ `skillDescriptionMaxRunes` (1536). Drift tests `run_skill_drift*_test.go`, `run_skill_use_cases_test.go`, `run_skill_recipes_test.go` do not pin these strings (grep) — run them anyway via `Skill` filter
- [ ] Step 5 (batch 3, reports + docs): `internal/report/sql_conventions.go:5-17` append S.7 paragraph (hard-wrapped like the rest; no `mask`/`redact`); pins: new `Test_sql_conventions_explain_investment_data` in `internal/report/sql_conventions_test.go` (phrases `investment_transactions`, `not in transactions`, `split_new_shares`, `quarry does not convert prices yet`), plus the two hand-copied help blocks that embed the conventions — `internal/cli/sql_test.go:206-220` (`Test_sql_help_describes_the_command_and_its_flags`) and `cmd/quarry/run_shared_documents_test.go:~349-361` (`Test_run_prints_the_sql_status_and_findings_documents_byte_for_byte`); `internal/mcp/describe_schema_test.go:53` and `report/document` schema tests compare against the const (no edit). Regenerate `plugin/skills/quarry/references/schema.md`: `go test ./cmd/quarry/ -run Test_skill_schema_reference_matches_the_committed_file -update` (STATE trap: other patterns match nothing). `docs/initial-prd.md:123` add "type from Phase 4b; currency as recorded, NULL when Quicken has none"; `:125` rename `investment_transactions` and "Cash side also appears in `transactions` (from Phase 4c)"; add both as rows in `cmd/quarry/run_plugin_notices_test.go` `Test_notices_and_prd_carry_the_ruled_plugin_edits` table (collapsed-whitespace match). `internal/store/store.go:50-52` `IsInvestmentAccount` comment: "accounts shows it as not valued". Fold STATE debt: `internal/store/duckstore/history_test.go:16` comment → a fixture fact ("Phase 1's import_runs: 19 columns, one run (id 4), no store_info beside it; the extra column is investment_transactions_not_imported"), no narrative

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; `golangci-lint fmt ./...` first (copy tests carry `//nolint:lll` where lines are pinned byte-equal)

### Verify
- [ ] Step 7: full verification block + `.claude/scripts/spec-check.py phase4a-investments`; tick SCENARIO-11 in `specification.md` with its acceptance test; rewrite `STATE.md` (drop SCENARIO-11 from Left unbuilt, add `not valued` decision, remove the `history_test.go:16` debt if folded); `status: done`

Ruled copy → asserting test: accounts Long → `Test_run_accounts_help_describes_the_command_without_needing_home`; `not valued` cell → acceptance test + `Test_renderAccounts*`; sync Long → `Test_run_prints_the_sync_help`; SKILL description/72/73 → `Test_skill_text_carries_the_ruled_frontmatter_and_rules`; SQL paragraph → `Test_sql_conventions_explain_investment_data`, `Test_sql_help_describes_the_command_and_its_flags`, schema.md → `Test_skill_schema_reference_matches_the_committed_file`; PRD L123/L125 → `Test_notices_and_prd_carry_the_ruled_plugin_edits`.

## Handoff

**Binding decisions**:
- `not valued` is the only investment-account balance cell; `--json` stays `balance: null` (no new field) — 4b may replace the cell with a value, the `nil` cents path in `accountBalance` is the seam.
- SQL conventions const stays the single source for `quarry sql` help, `describe_schema` and `schema.md`; two test files hand-copy it byte-wise (`internal/cli/sql_test.go`, `cmd/quarry/run_shared_documents_test.go`) — any later edit to the const must update both.

**Left unbuilt**: no `net-worth.md`/`investments.md` references, no MCP tool (4b/4c); `docs/adr/001-shared-store-package.md:22` still names `NotImported` (historical, unowned).

**Traps**:
- Ruled SQL copy says prices are in `securities.currency` "NULL when Quicken records none", but the importer stores `""` as `""` (STATE open debt): implement verbatim, leave to the final product-vision pass.
- Every accounts table re-pin changes column widths, not just the cell text; re-run, do not hand-edit padding.
- Narrow `-run` is case-sensitive: `(?i)sql` needed to reach `Test_sql_*`/`run_shared_documents`.

## Phase report

Run A (steps 1-2) done.
- Added `cmd/quarry/run_accounts_not_valued_test.go:19` `Test_run_accounts_shows_not_valued_for_brokerage_and_retirement_accounts`: text `--currency native` whole stdout, `NotContains "not imported"`, and `accounts --json` first row `Balance == nil`. Reuses `addTransaction` (`run_accounts_test.go:132`) and `accountRowJSON` (`run_accounts_json_test.go`).
- Step 2 needed no stub: the test references no new symbol, so there is no production edit. Const `notImportedBalance` (`render_accounts.go:12-13`) is still `"not imported"`; B1 renames it.
- RED at the assertion: actual `"Account    Type        Currency       Balance  Status\nBrokerage  brokerage   CAD       not imported\nChequing   chequing    CAD             100.00\nIRA        retirement  CAD       not imported\n"` against expected `not valued` rows (Balance column 2 narrower); `NotContains "not imported"` also fails. The fixture syncs cleanly (cash-only dividend rows, exit 0, no share-gate failure). The JSON assertions pass on arrival (balance is already null) and are a regression pin only.
- Next (B1, step 3): do not rewrite this test; it goes green once the cell text is `not valued`.
