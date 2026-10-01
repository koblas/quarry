---
id: SCENARIO-17
status: done
---

# SCENARIO-17: a payee with little history is judged against its category

Cadence: code-first (read-only report; no write-safety guard, atomic adapter or bug fix touched)
Acceptance test: `cmd/quarry/run_anomalies_category_test.go` `Test_run_anomalies_judges_a_first_time_payee_against_its_category`
Acceptance test (SCENARIO-18, folded): `cmd/quarry/run_anomalies_category_test.go` `Test_run_anomalies_counts_an_uncategorized_first_time_charge_as_not_judged`
Narrow loop: `go test ./internal/report/ -run 'Anomal'`; `go test ./internal/cli/ ./cmd/quarry/ -run 'anomal|Anomal'`
Mutation checks: category history `>= AnomalyCategoryMinHistory` → `>` → `..._need_ten_earlier_charges_in_the_category...`; payee-before-category `switch` order swapped → `..._judged_on_the_payee_never_the_category...`; category index keyed without currency → `..._another_currencys_category_charges...`; `Before` → `!After` on the category cut → `..._do_not_count_category_charges_that_do_not_precede...`
Runs: L | V
Size: LIGHT — 2 steps, report + cli

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_anomalies_category_test.go` (new) — S17: ten earlier groceries charges (distinct payees, median 210.40) then a first-time payee at 1,842.10 → row `… 1,842.10  210.40  8.8x  category, 10 earlier`; S18: an uncategorized 150.00 first-time charge plus a 45.00 one → footer `2 charges checked; 1 had too little history to judge`, nothing listed. S18 is green on arrival (the arm shipped in SCENARIO-14); S17 red on the missing row

### Build
- [x] Step 2: `internal/report/anomalies.go:13-143` — `AnomalyCategoryMinHistory = 10`, `AnomalyCategoryMultiplier = 5`, `BaselineCategory`, category index per (category id, currency) of single-category charges, `judge` takes the payee's and the category's strictly earlier charges and picks payee when ≥ 3, else category when ≥ 10; fix `NotJudged` doc and the verdict docs ("of 100.00 or more … no baseline"). Tests `internal/report/anomalies_category_test.go` (new): 9 vs 10 earlier; exactly 5× vs a cent over; Usual/Earlier/Times from all 11 earlier on another account; other currency, other category and multi-category earlier charges not history; same-day and later charges not history; uncategorized and multi-category charge get none; two splits of one category do get it; payee with 3 earlier judged on the payee though the category would flag (2 earlier → category); NULL payee listed via category
- [x] Step 3: `internal/cli/render_anomalies.go:17-19` `anomaliesBaselineWord` gains `BaselineCategory: "category"`; `render_anomalies_internal_test.go` — `category, 212 earlier` cell with a NULL payee `(no payee)` row

### Sweep
- [x] Step 4 (run V): `go build ./... && golangci-lint run ./...` to `0 issues`; doc comments on the new consts and `BaselineCategory`

### Verify
- [x] Step 5 (run V): full verification + `spec-check.py phase2e-recurring-anomalies` → tick SCENARIO-17, and SCENARIO-18 "delivered by SCENARIO-17"; rewrite STATE.md

## Handoff

Binding: category history = earlier (strictly earlier date) charges of any payee, account and window with the same `Category.ID` and currency; a charge with `Category == nil` is neither judged nor history. Payee baseline wins whenever the payee has ≥ 3 earlier charges.

## Phase report

Run L done (`<start>` e660e44c792af7eb16f852251bb129588408bf4d). Steps 1-3 green on the narrow loops; `golangci-lint` clean on report, cli, cmd; Sweep and Verify (steps 4-5) are run V's.
- Red: `Test_run_anomalies_judges_a_first_time_payee_against_its_category` failed at its assertion (row missing, footer `1 charge checked; 1 had too little history to judge`). `Test_run_anomalies_counts_an_uncategorized_first_time_charge_as_not_judged` was green on arrival: that arm shipped in SCENARIO-14.
- `internal/report/anomalies.go`: consts `AnomalyCategoryMinHistory` / `AnomalyCategoryMultiplier`, `BaselineCategory`, `baselineFor`, `judge(c, payee, category)`, `categoryCharges` (`groupByCategory`, `before`: binary search for strictly earlier). `NotJudged` and verdict docs fixed. `internal/cli/render_anomalies.go:17-20` `anomaliesBaselineWord` + category. Tests: `cmd/quarry/run_anomalies_category_test.go`, `internal/report/anomalies_category_test.go` (9 tests), `render_anomalies_internal_test.go` category row.
- Mutations (all reddened, restored byte-identical): `> AnomalyCategoryMinHistory` -> `ten_earlier_charges_are_enough`; category/payee `switch` swapped -> `three_earlier_charges_keep_the_payee_baseline`; currency dropped from `categoryKey` -> `an_earlier_charge_in_another_currency`; `<=` -> `<` on the multiplier -> `exactly_five_times_the_median_is_not_listed`; same-day counted (`Compare(day+1ns)`) -> `a_charge_on_the_same_day`.
- V must not undo: category history is any payee, any account, any date strictly before, same `Category.ID` and currency. A `Category == nil` charge is neither judged by category nor history. `--json` still prints text (SCENARIO-19).
