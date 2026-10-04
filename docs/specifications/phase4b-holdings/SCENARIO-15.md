---
id: SCENARIO-15
status: done
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
- [x] Step 3 (B1, batch 1 conventions): `store.go:196-210` `Actions() []string` (alphabetical, fresh slice) + `store_test.go` `Test_actions_are_the_thirteen_investment_transaction_actions` (literal list, plus every `Action*` const present); `internal/report/sql_conventions.go:1-26` const → `var`, last sentence `prices holds … (securities.currency, NULL when Quicken records none).` loses `quarry does not convert prices yet.`, append the S.7 holding_shares/v_holdings text then the action sentence built from `store.Actions()` and hard-wrapped to the file's 76-column width (unexported wrap helper in `report`; no `go doc` change); `sql_conventions_test.go:23-30` re-pin (phrases `holding_shares`, `v_holdings`, `Neither includes cash in investment accounts`; drop the retired phrase) + `Test_sql_conventions_list_the_action_values` (whitespace-collapsed `action is one of ` + `strings.Join(store.Actions(), ", ")`); `internal/importer/investments.go:21-36` + `investments_test.go` `Test_investment_action_map_covers_every_store_action` (values of `investmentActions` == `store.Actions()` as sets; production map unchanged)
- [x] Step 4 (B1, batch 1 hand copies + regen): `internal/cli/sql_test.go:206-234` and `cmd/quarry/run_shared_documents_test.go:~340-375` hand copies re-pinned to the new wrapped text; regenerate `plugin/skills/quarry/references/schema.md` with `go test ./cmd/quarry -run Test_skill_schema_reference_matches_the_committed_file -update` (no new table/view, so no `Len` bump; `holding_shares` and `v_holdings` sections already present at :100, :293); `internal/mcp/describe_schema_test.go:53` and `document/schema_test.go` compare against the var, no edit
- [x] Step 5 (B2, batch 2 accounts + SKILL): `internal/cli/accounts.go:17-25` Long — replace the `Brokerage and retirement accounts show "not valued": …` paragraph with S.7's sentence re-wrapped at the same ~72-column width as its neighbours; `cmd/quarry/run_accounts_test.go:90-105` pin re-wrapped identically (this is the wrap-width pin; assert lines, not just words). `plugin/skills/quarry/SKILL.md` :3 frontmatter (add use `what they hold in investment accounts and its value on a day;` before `or what to clean up`; exclusion → `for net worth, gains, dividend totals or ACB`), §4 table row `Holdings and their value on a day` after `Account balances` (:45), §5 sentence `For holdings over time query v_holdings; never sum investment_transactions.shares.` at the end of :54, :72, :73, :94 (`search_transactions, holdings, data_quality`); pins `cmd/quarry/run_skill_text_test.go:165` (frontmatter), `:189-203` (§4 row), `:204` (§5), `:218-219` (§7), `:236` (§9) re-pinned byte-for-byte (`¤` for backtick)
- [x] Step 6 (B2, batch 2 PRD): `docs/initial-prd.md` :123 `(type from Phase 4b;` → `(type deferred: Quicken's type codes are unlabelled;` keeping the rest, :129 `v_holdings (shares and value by day, from holding_shares; Phase 4b)`, CLI table row `quarry holdings` after `quarry accounts` (:165), MCP row after `search_transactions` (:202) with the S.7 text; `cmd/quarry/run_plugin_notices_test.go:15-60` update the `(type from Phase 4b;` pin and add one collapsed-whitespace row each for the view line, CLI row and MCP row. Confirm `grep -n investment internal/mcp/*.go` finds nothing to change (MCP `instructions` const unchanged)
- [x] Step 7 (B3, batch 3 rune widths): `internal/cli/render.go:494-501` `widestLen` → `utf8.RuneCountInString` (doc: rune count); callers :371-372, 398-399, 425-427, 467-469 need no edit; `render_internal_test.go` (849 lines; new `render_width_internal_test.go`) `Test_widestLen_counts_runes_not_bytes` (ASCII control, `é`, CJK, emoji; empty slice 0) and `Test_shareMismatchRows_align_non_ASCII_names` (two rows, one account `Épargne`, one security `日本株`; every `  quarry` column starts at the same rune offset; ASCII-only row is the control). Balance rows share the helper: add one non-ASCII balance row to `balanceMismatchRows` coverage in the same file

### Sweep
- [x] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues` (`lll` on the long pinned lines needs the existing `//nolint:lll // ruled copy …` form); doc comments on `store.Actions`, `SQLConventions`, `widestLen`

### Verify
- [x] Step 9: full verification + `.claude/scripts/spec-check.py phase4b-holdings` → tick SCENARIO-15 with its acceptance test; rewrite STATE.md (remove SCENARIO-15 from `## Left unbuilt`, close the two 4a debts it owns, keep the `render.go:222` wording debt as 4c)

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

All runs done. V: `go build` ok, `golangci-lint run ./...` 0 issues, covered full suite rc=0 (all packages ok), `uncovered-diff.py` 0 uncovered added lines since d35dedd, `go test -race` on cli/store/report/importer ok, `test-stats.py --base d35dedd --changed` TOTAL 1841 (+8) (cmd/quarry +1, cli +3, importer +1, report +1, store +2), `spec-check.py phase4b-holdings` OK. Spec ticked, STATE.md rewritten. Doc comments on `store.Actions`, `SQLConventions`, `widestLen` present.

B3:
- `internal/cli/render.go` `widestLen` counts `utf8.RuneCountInString` (import `unicode/utf8` added); callers unchanged. New `internal/cli/render_width_internal_test.go`: `Test_widestLen_counts_runes_not_bytes`, `Test_shareMismatchRows_pad_non_ASCII_names_by_runes`, `Test_balanceMismatchRows_pad_non_ASCII_labels_by_runes` (exact rows).
- Finding: fmt `%-*s` already pads by runes, so byte widths never misaligned columns; they over-padded the gap (3 spaces instead of 2 around the widest non-ASCII label). Red was that extra padding, asserted on exact rows.
- Test data uses katakana `ニホン`, not Han: `gosmopolitan` rejects Han literals.
- Red before fix: all three tests failed at assertions (widestLen 8/9/8 bytes vs 7/3/3 runes; rows over-padded). Mutation (`len(s)` with `utf8` still referenced) reddens all three; restored, green.
- Narrow loop `go test -count=1 ./internal/cli/ -run 'widest|Mismatch|sql|accounts'` ok; `golangci-lint run ./internal/cli/...` 0 issues. Full suite not run (V).

B2:
- `internal/cli/accounts.go` Long: not-valued paragraph is S.7's sentence at the 72-column wrap; pinned in `cmd/quarry/run_accounts_test.go:96-98` and the acceptance test. Acceptance `Test_run_accounts_and_sql_help_carry_the_holdings_copy` is green.
- `plugin/skills/quarry/SKILL.md`: frontmatter (use added, exclusion drops `holdings`), §4 row, §5 sentence appended to the schema.md paragraph, §7 net-worth and `Dividends, realized gains, ACB` bullets (the `Investments, holdings, ...` heading is gone), §9 tool list. `run_skill_text_test.go` constants re-pinned byte for byte. Code spans: `quarry holdings`, `v_holdings`, `investment_transactions.shares`, `holdings` carry backticks like their neighbours (S.7's table cells show them without).
- `docs/initial-prd.md`: type deferred (:123), `v_holdings` line, CLI row (purpose text derived from the S.1 Short, S.7 gave none: `Securities held in each investment account on one day (--as-of, default today) with share count, latest price and its date, and value; --account to narrow, cash in investment accounts not included`), MCP row verbatim from S.7. `run_plugin_notices_test.go`: type pin updated, three collapsed-whitespace rows added.
- `grep -n investment internal/mcp/server.go describe_schema.go tools.go`: only `holdingsDescription` (tools.go:130, already ruled); MCP `instructions` const has no match, unchanged.
- No remaining copy of the old accounts or net-worth wording outside `docs/specifications/` (grep over *.go, *.md, excluding .git/.devenv).
- Narrow loops green: `go test ./cmd/quarry/ -run 'surfaces|accounts_help|sql_help|skill|notices|shared_documents'`, `./internal/cli/ -run 'widest|Mismatch|sql|accounts'`; lint on `./cmd/quarry/... ./internal/cli/...`: 0 issues. Full suite not run (V).
- Note for V: the old `phase4ViewPattern` / `v_holdings` pin retirement in S.7's Pins row was already done in S02 (see the plan's survey).

B1 notes (kept):

- `internal/store/store.go:212-228`: `Actions()` filled (alphabetical literal of the 13 consts, fresh slice each call). `store_test.go`: `Test_actions_are_the_thirteen_investment_transaction_actions`, `Test_actions_returns_a_fresh_slice`.
- `internal/report/sql_conventions.go`: still a `const`; tail now ends `...none).` + holding_shares/v_holdings sentence + literal action sentence, wrapped at 76 (matches the acceptance test's blocks byte for byte). `sql_conventions_test.go`: phrases re-pinned (retired `quarry does not convert prices yet` now NotContains), new `Test_sql_conventions_list_the_action_values` (collapsed text ends with `action is one of ` + `strings.Join(store.Actions(), ", ")` + `.`).
- `internal/importer/investment_actions_internal_test.go` (new, internal package: the map is unexported): `Test_investment_action_map_covers_every_store_action` (sorted map values == `store.Actions()`). Not in `investments_test.go` as planned (external package).
- Hand copies re-pinned: `internal/cli/sql_test.go:~230`, `cmd/quarry/run_shared_documents_test.go:~373`; `plugin/skills/quarry/references/schema.md` regenerated with `-update` (diff only the conventions tail).
- Mutations (run with -count=1): drop `ActionSplit` from `Actions()` -> reddens the store test (both assertions), `Test_sql_conventions_list_the_action_values`, the importer map test; swap first two -> same three plus `Test_actions_returns_a_fresh_slice`. Restored.
- Narrow loops green: store, report/..., importer, cli. `cmd/quarry` skill/schema/shared_documents green; the acceptance test is red only on its accounts-Long assertion (B2, step 5); sql blocks now pass. Lint on touched packages: 0 issues. Full suite not run (V).
- Trap: `-run 'Action'` is case-sensitive and matches none of the new tests; use `actions|action_map|Test_sql_conventions`.
