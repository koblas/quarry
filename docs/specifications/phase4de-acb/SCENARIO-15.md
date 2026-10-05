---
id: SCENARIO-15
status: open
---

# SCENARIO-15: Sales of one tax year (folds SCENARIO-18, nothing to show)

Cadence: code-first
Acceptance test: `cmd/quarry/run_acb_year_test.go` `Test_run_acb_year_lists_that_years_sales_one_by_one_with_a_total`
Acceptance test (SCENARIO-18, folded): `cmd/quarry/run_acb_year_test.go` `Test_run_acb_warns_when_no_non_registered_account_has_traded`
Runs: A (1-2) | Build, Sweep, Verify steps NOT RECONSTRUCTED (see Phase report)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `--year 2025` over `acbRows` prints the sales table alone (caption `Sales in 2025, in CAD`, ruled columns, one sale row, `Total` row), no position table, stderr empty.
- [x] Step 2: default `acb` over a store with a non-registered account and no trades prints the ruled headers-only tables (existing pin kept) and warning form 1 on stderr, exit 0.

**Orchestrator: product-vision ruling 2026-10-05 recorded in spec after the `--year` text paragraph — supersedes this plan: (e) ROC-only-year security IS listed in `securities` (step 3 test flips); (d) stdout never blank — `render_acb_internal_test.go:209-217` headers-only pin is KEPT, strike the trap saying it flips; `--year` always prints a Total row (0.00 when empty); warning 2 has three forms (form 1 / `…, nor in any other year` / span, single-year bare); `--year 0000` refused exit 2 with the not-a-year line; decisions 1, 3 confirmed; S15 owns both `--year` refusals (S17 drops them).**


## Phase report

Run A. The committed plan body was missing: the file held only the orchestrator ruling block (e5af2b5), so steps 1-2 above are reconstructed from the spec's S15/S18 scenarios and `--year` ruling. Build, Sweep and Verify steps do not exist; re-plan before run B.
- `cmd/quarry/run_acb_year_test.go` (new): helper `acbSaleLine`, const `acbNothingToShowWarning`, both acceptance tests. No production code touched; `--year` is still unregistered.
- Red 1: `run_acb_year_test.go:33` exit code expected 0, actual 2 (`quarry: unknown flag: --year`). Red 2: `run_acb_year_test.go:60` stderr expected the form-1 warning line, actual empty (stdout assertions above it already pass: headers-only pin).
- The `--year 2025` expected table (widths, `Total` row, blank Security/Shares) is unverified until green: the red stops at the exit code. Re-check column widths when `--year` lands.
- Do not retype the headers-only stdout in test 2 with `acbYearLine`: its widths are the filled fixture's, not the empty table's.
