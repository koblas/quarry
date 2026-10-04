---
id: SCENARIO-03
status: open
---

# SCENARIO-03: Investment income counts in cash flow (absorbs SCENARIO-04)

Cadence: code-first (no bug fix, write-safety guard or atomic adapter; the data side is already built, only help and skill copy changes)
Acceptance test: `cmd/quarry/run_investment_cashflow_test.go` `Test_run_cashflow_counts_investment_dividends_interest_and_capital_gains_as_income_and_buys_and_sells_as_neither`
Acceptance test (SCENARIO-04, folded): `cmd/quarry/run_investment_cashflow_test.go` `Test_run_spend_counts_investment_margin_interest_as_spending`
Narrow loop: `go test ./internal/cli/ -run 'Test_cashflow_help|Test_spend_help'`; `go test ./cmd/quarry/ -run 'Investment|Skill'`
Mutation checks: system-kind filter in the `v_cash_flow` definition (`internal/store/duckstore/schema.go:239`, drop `AND c.kind IS DISTINCT FROM 'system'`) → acceptance test (done: buy and sell rows then count, reddening its `v_cash_flow` map)
Runs: L | V
Size: LIGHT — 3 steps, `internal/cli` plus `cmd/quarry` tests, `plugin/skills/quarry/SKILL.md`

Premise: cash rows already flow through `v_cash_flow` by category kind (SCENARIO-01a/01b), so the acceptance tests and the uncategorized-by-sign test are green on arrival; the scenario's new behaviour is the three ruled copy lines. The SKILL.md description edit (dropping "dividend totals") and SKILL.md:73 deletion belong to SCENARIO-10.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_investment_cashflow_test.go` (committed 7aa9c72): the two acceptance tests plus `Test_run_cashflow_counts_an_uncategorized_investment_transaction_by_its_sign`. Green on arrival, mutation above reddened the first.
- [x] Step 2: copy pins red first. `internal/cli/report_help_test.go` `Test_cashflow_help_says_what_cashflow_counts` and `Test_spend_help_says_what_spend_counts` (full Long at wrap width); `cmd/quarry/run_skill_text_test.go:220` `skillSection7` (subtest `## 7. Not covered yet`). Each failed at its assertion.

### Build
- [x] Step 3: copy, verbatim from `specification.md` → *Changes to existing surfaces*: `internal/cli/cashflow.go:55-59` (sentence between "here too." and "Uncategorized splits", paragraph rewrapped); `internal/cli/spend.go:36-38` (own paragraph after the one ending "Closed accounts are included."); `plugin/skills/quarry/SKILL.md:74` replaced.

### Sweep
- [ ] Step 4 (run V): `go build ./... && golangci-lint run ./...` to `0 issues`; full verification; tick SCENARIO-03 and SCENARIO-04 (04 line: `delivered by SCENARIO-03`, its acceptance test last); `spec-check.py phase4c-networth`; STATE.md rewrite closing the 01a cash-flow re-cover and buy-row debts; `status: done`.

## Handoff

- Ruled copy delivered here: cashflow Long sentence, spend Long sentence (own paragraph), SKILL.md:74. Remaining copy edits from the *Changes to existing surfaces* table stay with later scenarios (holdings Long, SKILL description, §4 row, :73, §9, PRD).
- SKILL.md:73 ("Net worth: quarry does not compute net worth yet") is still true and stays until SCENARIO-10 ships `quarry networth`.

## Phase report

Run L done (red/green for copy only; data tests green on arrival, committed in 7aa9c72).

Files changed: `internal/cli/cashflow.go:50-60`, `internal/cli/spend.go:36-40`, `plugin/skills/quarry/SKILL.md:74`, `internal/cli/report_help_test.go` (cashflow and spend Long pins), `cmd/quarry/run_skill_text_test.go:220`.

Red seen: `Test_spend_help_says_what_spend_counts` and `Test_cashflow_help_says_what_cashflow_counts` (help output lacked the new sentence), `Test_skill_text_carries_the_ruled_frontmatter_and_rules/##_7._Not_covered_yet` (expected text differed). Green now: all three, narrow loop above.

Not run yet: lint, full covered suite, `uncovered-diff.py`, `test-stats.py`, spec tick, STATE.md. Run V does them. No production Go logic changed (string literals only), so the coverage gate has nothing new to execute beyond the Long constants.
