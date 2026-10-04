---
id: SCENARIO-06
status: open
---

# SCENARIO-06: each in-scope use-case question is answered by the command the skill names

Size: LIGHT — 3 steps, cmd/quarry
Cadence: code-first (test code only; no mandatory test-first item touched)
Runs: L | V
Acceptance test: `cmd/quarry/run_skill_use_cases_test.go` `Test_each_use_case_question_is_answered_by_the_command_the_skill_names`
Narrow loop: `go test ./cmd/quarry -run 'use_case|Test_skill_run_cell|Test_argv_'`
Mutation checks: per row, change the expected value or the argv (`skillUseCases`) -> exactly that subtest reddens; change a §4 Run cell in SKILL.md -> the row's pin reddens; restore with `git checkout`.
Surface & Copy delivered: none (the commands and SKILL.md already exist; nothing new is ruled).

## Implementation Plan

### Acceptance (green on arrival: the seven commands already exist, so no red is manufactured)
- [x] `run_skill_use_cases_test.go`: `skillUseCases()` rows {name, §4 question, argv, answer} and the acceptance table, run through `runWith` under `recipeScenario` + `spendEnv`'s clock

### Build
- [x] Step 1: seven answers from the fixture: recurring new = Netflix only; spend by payee has `Netflix|CAD` 87.92, no `Savings Sweep`, CAD total 595.79; Netflix first charge and 9.99 to 11.99; anomalies = Hardware 250.00 vs 40.00; findings ids `duplicate:txn-gas-1+txn-gas-2` and `uncategorized:no-payee`; cashflow 2026 CAD totals; search "Corner Deli"
- [x] Step 2: `Test_use_case_argv_matches_the_command_the_skill_names` reads SKILL.md via `repoFile`, looks the row's question up in §4 (`skillRunCell`), and checks the argv against that cell's first code span (`argvMismatches`: literal tokens up to the first `<placeholder>`, `a|b` alternatives, every `--flag` named in the cell)
- [x] Step 3: crafted-text controls for `skillRunCell` (unknown question, escaped pipes) and `argvMismatches` (wrong subcommand, wrong flag value, flag not in the cell); mutation proof per row

### Sweep / Verify (run V)
- [ ] lint, full suite, spec tick, `spec-check.py`, STATE.md

## Handoff
`skillUseCases()` rows are S06's eval table; S03 does not touch them. Rows read `skillEvalStore` as it stands (R1-R14).

## Phase report
Run L done (acceptance + 3 Build steps ticked; V remains).
- Files: `cmd/quarry/run_skill_use_cases_test.go` (new; `skillUseCases()` rows, acceptance test, `Test_use_case_argv_matches_the_command_the_skill_names`, `skillRunCell`, `argvMismatches` and their controls). No production Go, no fixture change.
- Acceptance was green on arrival: the seven commands already exist. No red was manufactured; each row was proven able to go red by mutation instead (below).
- Row 3 argv is `recurring --json --since 2000` (flag order free) so it prefix-matches the §4 cell `quarry recurring --json`; its `doc.Since == "2000-01-01"` assertion is what makes dropping `--since 2000` red, because the fixture's price change is in the default window.
- Row 6 asserts the 2026 CAD period row, not only totals: `--by month` leaves totals unchanged.
- Mutations run, each reddened exactly the named subtest: expected values rows 1-7 (acceptance subtest of that row); argv rows 1, 2, 3, 5, 6, 7 reddened the acceptance subtest (row 5 also its pin); row 4 (extra `--since 2000`) reddened only its pin, since the answer is unchanged; SKILL.md §4 cell edits (anomalies command renamed, search renamed, cashflow `\|year` alternative dropped, findings `--json` dropped) each reddened only that row's pin subtest. Restored; tree clean of mutations.
- Narrow loop green; `golangci-lint run ./cmd/quarry/...` 0 issues. Full suite, `spec-check.py`, spec tick, STATE.md are run V's.
- Left: spec tick line should name `cmd/quarry/run_skill_use_cases_test.go` `Test_each_use_case_question_is_answered_by_the_command_the_skill_names`.
