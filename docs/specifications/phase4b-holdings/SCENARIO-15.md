---
id: SCENARIO-15
status: open
---

# SCENARIO-15: Existing surfaces describe holdings

Cadence: code-first (no mandatory test-first item: no write guard, no atomic adapter, no bug fix)
Acceptance test: `cmd/quarry/run_holdings_surfaces_test.go` `Test_run_accounts_and_sql_help_carry_the_holdings_copy` — `run` over `accounts --help` and `sql --help` (HOME unset, as `Test_run_accounts_help_describes_the_command_without_needing_home`), asserting S.7's accounts sentence, the holding_shares / v_holdings sentence and the action sentence verbatim at wrap width. The Then's other clauses have own pins: schema.md → `Test_skill_schema_reference_matches_the_committed_file`; alignment → step 7; SKILL/PRD → steps 5-6.
Narrow loop: `go test ./internal/store/ ./internal/report/... ./internal/importer/ -run 'Action|conventions|Conventions'` then `go test ./internal/cli/ -run 'widest|Mismatch|sql|accounts'` and `go test ./cmd/quarry/ -run 'surfaces|accounts_help|sql_help|skill|notices|shared_documents'`
Mutation checks: `widestLen` rune count (`utf8.RuneCountInString`) → `Test_widestLen_counts_runes_not_bytes` and the non-ASCII `shareMismatchRows` row; generated action list (drop/reorder one value in `store.Actions`) → `Test_actions_are_the_thirteen_investment_transaction_actions` and `Test_sql_conventions_list_the_action_values`. (Not mandatory test-first; developer proves each fails once.)
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | B3 (7) | V (8-9)
Size: OWNS A RUN — 3 batches (B1 conventions, B2 prose surfaces, B3 widestLen), 0 new feature packages (touches store, report, importer, cli, plugin/docs)

Surface survey: no new port or adapter; one new exported `store.Actions()` (alphabetical copy of the 13 `Action*` consts). `report.SQLConventions` is consumed as a value only (`cli/sql.go:39` inside a field literal, `document/schema.go:72`, `mcp` and tests) so const → `var` breaks nothing (greps above). `phase4ViewPattern` / `v_holdings` pins already retired in S02: no step. "N investment accounts not checked" (`render.go:222`) stays: spec :18 keeps it until 4c.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_holdings_surfaces_test.go` (new) `Test_run_accounts_and_sql_help_carry_the_holdings_copy` — copy blocks verbatim from S.7; the conventions block is the hand copy, not `report.SQLConventions`
- [x] Step 2: `internal/store/store.go:196-210` `Actions` — signature-only stub (returns nil); fails at the action sentence assertion

### Build
- [ ] Step 3 (B1, batch 1 conventions): `store.go:196-210` `Actions() []string` (alphabetical, fresh slice) + `store_test.go` `Test_actions_are_the_thirteen_investment_transaction_actions` (literal list, plus every `Action*` const present); `internal/report/sql_conventions.go:1-26` const → `var`, last sentence `prices holds … (securities.currency, NULL when Quicken records none).` loses `quarry does not convert prices yet.`, append the S.7 holding_shares/v_holdings text then the action sentence built from `store.Actions()` and hard-wrapped to the file's 76-column width (unexported wrap helper in `report`; no `go doc` change); `sql_conventions_test.go:23-30` re-pin (phrases `holding_shares`, `v_holdings`, `Neither includes cash in investment accounts`; drop the retired phrase) + `Test_sql_conventions_list_the_action_values` (whitespace-collapsed `action is one of ` + `strings.Join(store.Actions(), ", ")`); `internal/importer/investments.go:21-36` + `investments_test.go` `Test_investment_action_map_covers_every_store_action` (values of `investmentActions` == `store.Actions()` as sets; production map unchanged)
- [ ] Step 4 (B1, batch 1 hand copies + regen): `internal/cli/sql_test.go:206-234` and `cmd/quarry/run_shared_documents_test.go:~340-375` hand copies re-pinned to the new wrapped text; regenerate `plugin/skills/quarry/references/schema.md` with `go test ./cmd/quarry -run Test_skill_schema_reference_matches_the_committed_file -update` (no new table/view, so no `Len` bump; `holding_shares` and `v_holdings` sections already present at :100, :293); `internal/mcp/describe_schema_test.go:53` and `document/schema_test.go` compare against the var, no edit
- [ ] Step 5 (B2, batch 2 accounts + SKILL): `internal/cli/accounts.go:17-25` Long — replace the `Brokerage and retirement accounts show "not valued": …` paragraph with S.7's sentence re-wrapped at the same ~72-column width as its neighbours; `cmd/quarry/run_accounts_test.go:90-105` pin re-wrapped identically (this is the wrap-width pin; assert lines, not just words). `plugin/skills/quarry/SKILL.md` :3 frontmatter (add use `what they hold in investment accounts and its value on a day;` before `or what to clean up`; exclusion → `for net worth, gains, dividend totals or ACB`), §4 table row `Holdings and their value on a day` after `Account balances` (:45), §5 sentence `For holdings over time query v_holdings; never sum investment_transactions.shares.` at the end of :54, :72, :73, :94 (`search_transactions, holdings, data_quality`); pins `cmd/quarry/run_skill_text_test.go:165` (frontmatter), `:189-203` (§4 row), `:204` (§5), `:218-219` (§7), `:236` (§9) re-pinned byte-for-byte (`¤` for backtick)
- [ ] Step 6 (B2, batch 2 PRD): `docs/initial-prd.md` :123 `(type from Phase 4b;` → `(type deferred: Quicken's type codes are unlabelled;` keeping the rest, :129 `v_holdings (shares and value by day, from holding_shares; Phase 4b)`, CLI table row `quarry holdings` after `quarry accounts` (:165), MCP row after `search_transactions` (:202) with the S.7 text; `cmd/quarry/run_plugin_notices_test.go:15-60` update the `(type from Phase 4b;` pin and add one collapsed-whitespace row each for the view line, CLI row and MCP row. Confirm `grep -n investment internal/mcp/*.go` finds nothing to change (MCP `instructions` const unchanged)
- [ ] Step 7 (B3, batch 3 rune widths): `internal/cli/render.go:494-501` `widestLen` → `utf8.RuneCountInString` (doc: rune count); callers :371-372, 398-399, 425-427, 467-469 need no edit; `render_internal_test.go` (849 lines; new `render_width_internal_test.go`) `Test_widestLen_counts_runes_not_bytes` (ASCII control, `é`, CJK, emoji; empty slice 0) and `Test_shareMismatchRows_align_non_ASCII_names` (two rows, one account `Épargne`, one security `日本株`; every `  quarry` column starts at the same rune offset; ASCII-only row is the control). Balance rows share the helper: add one non-ASCII balance row to `balanceMismatchRows` coverage in the same file

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues` (`lll` on the long pinned lines needs the existing `//nolint:lll // ruled copy …` form); doc comments on `store.Actions`, `SQLConventions`, `widestLen`

### Verify
- [ ] Step 9: full verification + `.claude/scripts/spec-check.py phase4b-holdings` → tick SCENARIO-15 with its acceptance test; rewrite STATE.md (remove SCENARIO-15 from `## Left unbuilt`, close the two 4a debts it owns, keep the `render.go:222` wording debt as 4c)

## Handoff

**Binding decisions:**
- `store.Actions()` is the one action vocabulary list (alphabetical copy of the 13 consts): conventions text generates from it, the importer map is pinned against it, a new action is added to the consts and the list together — the phase4a rule "added there only" now has a test.
- OVERRIDE (orchestrator, supersedes the next bullet): `report.SQLConventions` stays a `const` (no package-level `var`): the action sentence is written literally in the const, and a drift test asserts it equals the sentence built from `store.Actions()` (and the importer map). The hand copies in `sql_test.go` and `run_shared_documents_test.go` pin the wrapped literal.
- (superseded by the override) `report.SQLConventions` is a `var` built at init from a const body plus the generated, hard-wrapped action sentence — all four surfaces (sql help, describe_schema, schema.md, MCP) read the one value.
- `widestLen` counts runes (like `renderTable`), not display columns: wide CJK glyphs misalign by terminal width, accepted.

**Left unbuilt:**
- Balances clause `N investment accounts not checked` (`render.go:222`) — spec :18, reopens in 4c.
- Cut-note advice (`WHERE date = …`) and `Holdings` vs `accounts` sort order — final product-vision pass (STATE `## Open debts`).
- No STATE debts folded: none touches these files.

**Traps:**
- Conventions are hard-wrapped: an unwrapped sentence passes `report` tests and fails both hand copies and `schema.md`. Regenerate with `-update` only after the three pins agree.
- `SQLConventions` as a `var` initialised from `store.Actions()` must not depend on package init order across `report`; keep the helper pure.
- Pins in `run_skill_text_test.go` use `¤` for backtick; SKILL.md edits must match byte for byte, including frontmatter on one line.
- Verify the `-run` filters: `-run 'Holdings'` misses lowercase `Test_holdings_*` (STATE trap).

## Phase report

Run A done (steps 1-2).

- `cmd/quarry/run_holdings_surfaces_test.go` (new): `Test_run_accounts_and_sql_help_carry_the_holdings_copy`. Red at all three `assert.Contains` (accounts Long at :23, sql conventions holding_shares/v_holdings block at :26, action sentence at :35); the accounts Long is step 5 (B2), so it fails first, not the action sentence as the plan guessed.
- `internal/store/store.go:212-215`: `Actions()` signature-only stub returning nil; B1 fills it (alphabetical, fresh slice). The acceptance test does not call it (hand copy), so it compiles without it.
- Wrap widths measured: sql conventions paragraph is greedy wrap at 76 columns (python `textwrap.wrap(..., 76)` reproduces the current paragraph exactly; 75 does not). The new sentence runs on in the same paragraph: `...none).` then `holding_shares holds ...`, then `Neither includes cash in investment accounts. action is one of ...` (action sentence continues on the same line as `Neither ...`). The test's literal blocks are that wrap; B1's const must match them byte for byte.
- Accounts Long wrap: the existing paragraph's lines run 72-73 columns; the new sentence is wrapped at 72: `...quarry values their` / `holdings (quarry holdings) but not yet the cash in them, so it cannot` / `compute their balance.` B2 edits `internal/cli/accounts.go:17-19` and `cmd/quarry/run_accounts_test.go:96-98` to this wrap.
- Orchestrator override recorded in `## Handoff`: `SQLConventions` stays a `const`; drift test against `store.Actions()` and the importer map replaces the var/generator.
- Red now: the acceptance test. Not run: any other suite.
