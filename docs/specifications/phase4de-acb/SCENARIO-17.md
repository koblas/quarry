---
id: SCENARIO-17
status: open
---

# SCENARIO-17: ACB refuses what it cannot answer

Cadence: code-first (no write-safety guard or atomic adapter touched; quarry never writes config.toml)
Acceptance test: `cmd/quarry/run_acb_refusals_test.go` `Test_run_acb_refuses_an_unclassified_account_and_a_currency_other_than_cad`
Acceptance test (SCENARIO-20, folded): `cmd/quarry/run_skill_references_test.go` `Test_skill_has_claude_classify_accounts_before_the_first_acb`
Narrow loop: `go test ./internal/report/ -run 'acb' && go test ./internal/cli/ -run 'acb|currency|config_always|read_commands' && go test ./cmd/quarry/ -run 'acb|read_commands|skill|reference'`
Mutation checks: R-6 guard in `(*Server).ACB` → `Test_acb_refuses_while_an_investment_account_is_unclassified`; CAD-only check in acb's Args → acceptance `--currency USD` row
Runs: A (1-2) | B1 (3-4) | B2 (5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (report; cli + docs + cmd pins don't count); absorbs SCENARIO-20

Scope: R-6 and `--currency` only. The S17 Given's other arms are already pinned and are NOT re-planned: bad/future `--year` by S15 (`ParseACBYear`, `report.ACBYearError` rows), unknown `--security` by S16 (`cmd/quarry/run_acb_security_test.go`, `RefusalUnknownSecurity`). Unreadable/malformed config already refuses acb before any read (`cmd/quarry/run_config_test.go:363` `Test_run_accounts_and_acb_refuse_a_malformed_config_even_when_given_a_currency`), so it never reaches R-6.

Precedence (default; one pin per adjacent pair): positional arg → `--currency` (Args, exit 2) → `--year` (exit 2) → config refusal → store open/read refusal → **R-6** → unknown `--security` (exit 1). R-6 precedes unknown security because a security held only in an unclassified account would otherwise be refused as "covers no security named X", sending the user to the wrong fix.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_acb_refusals_test.go` `Test_run_acb_refuses_an_unclassified_account_and_a_currency_other_than_cad` — `runWith` + `replaceStoreWithRates`, per-test config; rows: one open unclassified brokerage (N=1), an open one plus a closed retirement one (N=2), an unclassified account listed in `findings.ignore` (still refused), `--json` (stdout empty), `--security <pooled ticker>` (R-6 wins), `--currency USD`, `--currency native`, `--currency usd` (exit 2, before the store: unclassified fixture), `--currency USD --year 24` (currency line), `--year 24` (year line, exit 2, over the unclassified store); exact stderr + exit + empty stdout on each. Add one cross-surface row: N equals the row count of `quarry findings --type unclassified-account --status all --json` on the same store (closed + ignored in the mix)
- [ ] Step 2: `cmd/quarry/run_skill_references_test.go` `Test_skill_has_claude_classify_accounts_before_the_first_acb` — SKILL §4 carries the trigger sentence verbatim and the acb row; findings.md has a `## Classifying accounts` section carrying the classification question and `quarry findings --type unclassified-account --json`. No stubs needed (both red at their assertions)

### Build
- [ ] Step 3: report R-6 — `internal/report/acb.go:213-231` `(*Server).ACB` (check after the read error, before `walkACB`/`withSelection`; count `Classification.Unclassified` over every `history.Accounts`, closed included) + `internal/report/refusal.go:103-109` new `unclassifiedAccountsRefusal(n)` beside `unknownSecurityRefusal` (`RefusalGeneric`, literal `~` path like `internal/finding/finding.go:325`). Re-type fixture account `acct-7` to `Type: "chequing"` (as `cmd/quarry/run_helpers_test.go:96`) at `internal/report/acb_walk_test.go:40` and `acb_select_test.go:28`, comments at `acb_walk_test.go:16-17`, `acb_select_test.go:20` ("in neither list", not "unclassified"). Tests: `Test_acb_refuses_while_an_investment_account_is_unclassified` (rows: 0 = control passes, 1, 2; closed counts; unlisted non-investment account does not count; registered-listed does not count; bound: N=1 vs N=2 wording); `Test_acb_refuses_an_unclassified_account_before_an_unknown_security`; existing `Test_acb_returns_a_failed_store_read` covers read-before-R-6. Fixtures R-6 breaks, repaired HERE so B1 ends green: `cmd/quarry/run_config_test.go:270-276` `readCommandFixture` before-loop (`require.Equal(t, 0, …)` runs acb config-less over unclassified `acct-cad`) — leave `acb` out of `before` (no reader: `:347` skips it), keep acb in `readCommandArgs` (its config-refusal rows still apply); any other acb fixture that now refuses gets its account classified, never a weakened guard
- [ ] Step 4: cli `--currency` + tables — `internal/cli/currency.go:41-54` (or acb-local Args in `internal/cli/acb.go:55`) CAD-only check after `currencyFlag.args`, line echoes `money.Currency.String()`; `internal/cli/acb_test.go:194-208` rewrite `Test_acb_leaves_an_unlisted_account_out_of_the_pool` → `Test_acb_leaves_a_registered_account_out_of_the_pool` (config `Registered: acct-1`) + new `Test_acb_refuses_while_an_account_is_unclassified` (cadConfig, RefusalError not UsageError); drop `:230-240` once the table row covers it. Tables: `internal/cli/currency_test.go:22-27` — acb joins `currencyCommands`, but `:68-80` (accepts usd/CAD/Native) iterates a list WITHOUT acb, plus new `Test_acb_currency_flag_accepts_only_cad` (cad, CAD accepted; usd, native refused with the ruled line; EUR keeps `badCurrencyFlag`); `:217,:231,:249,:262` `--currency native` → `CAD` (`:231` would otherwise fail on the UsageError). `internal/cli/report_help_test.go:182-205` acb row with a per-row placeholder (`currency`, not `code`). `cmd/quarry/run_read_usage_test.go:81`, `cmd/quarry/run_usage_test.go:212` add `acb`
- [ ] Step 5: docs (after the copy ruling below) — `plugin/skills/quarry/SKILL.md:36-50` §4 acb row after the holdings row + trigger paragraph right after the table; `:70` §6 4d sentence verbatim + acb.adjustment sentence; `plugin/skills/quarry/references/findings.md:31-35` mirror bullet; new `## Classifying accounts` section after "Ignoring a finding", before "A spreadsheet" (content per spec Part A last bullet + 401(k)/IRA example). Pins: `cmd/quarry/run_skill_text_test.go:189` `skillSection4`, `:218` `skillSection6`; `cmd/quarry/run_skill_references_test.go:79-91` findings.md phrases (mirror sentence, question, never-a-second-`[accounts]`-header rule)

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comment on `(*Server).ACB` (names the unclassified refusal)

### Verify
- [ ] Step 7: full verification + `spec-check.py phase4de-acb` → tick SCENARIO-17 with its acceptance test; tick SCENARIO-20 `— delivered by SCENARIO-17 — ` + its acceptance test (test reference last on the line)

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- R-6 lives in `(*report.Server).ACB` on the ONE `InvestmentHistory` read, counting every account `Classification.Unclassified` accepts (closed included); `ACBRequest` carries no ignore list, so an ignored finding cannot unblock — S19's MCP `acb` inherits it by calling `Server.ACB`
- R-6 is `RefusalGeneric` with a literal `~/Library/...` path (fixed copy, like the finding's GroupClause); S19 adds a `RefusalKind` only if MCP must word it differently
- Order: usage (positional, `--currency`, `--year`) → config → store → R-6 → unknown security
- acb's `--currency`: CAD in any case accepted and ignored; USD/native refused exit 2 before config/store; unreadable value keeps `--currency must be CAD, USD or native`
- S20 tick covers SKILL/findings.md classify-first copy only; conventions sentence 2, schema.md and SKILL §9 stay with S19

**Defaults pending ruling** — scoped product-vision copy ruling. First three gate run A (Steps 1, 3, 4 assert them verbatim); the rest gate run B2 (Step 5):
- R-6 count: `1 is in neither …` / `N are in neither …`
- Echo canonical: `--currency usd` → `drop --currency USD`; `native` → `drop --currency native`
- `acb --currency EUR` keeps the generic line, which advertises USD that acb then refuses
- §6 + findings.md mirror: ruled 4d sentence verbatim, then `The same goes for ¤acb.adjustment¤ lines the user reads you from a T3 slip: show the lines, and write them only after the user says yes.`
- Trigger paragraph placement: right after the §4 table
- Question: `(RRSP, RRIF, TFSA, RESP, FHSA, LIRA, a US 401(k) or IRA, or similar)`
- "Classifying accounts" body text (spec gives content, not text)

**Left unbuilt**:
- MCP `acb` and its R-6 wording — S19

**Traps**:
- `acbWalkRequest`/`acb_select_test` fixtures `require.NoError`: a later fixture adding an unlisted brokerage/retirement account now fails on R-6, not on the walk
- `report_help_test` regex hard-codes the `code` placeholder; acb's ruled help uses `currency`

**Orchestrator: product-vision copy ruling 2026-10-05 is in `RULING-S17.md` (same dir) — supersedes this plan and spec :159/:295/:299/:361/:363. Implement its copy verbatim.**
