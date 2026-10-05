---
id: SCENARIO-15
status: open
---

# SCENARIO-15: Sales of one tax year (folds SCENARIO-18, nothing to show)

Cadence: code-first — no bug fix, write-safety guard or atomic adapter touched
Acceptance test: `cmd/quarry/run_acb_year_test.go` `Test_run_acb_year_lists_that_years_sales_one_by_one_with_a_total`
Acceptance test (SCENARIO-18, folded): `cmd/quarry/run_acb_year_test.go` `Test_run_acb_warns_when_no_non_registered_account_has_traded`
Narrow loop: `go test ./internal/report/... ./internal/cli/ ./cmd/quarry/ -run 'ACB|Acb|acb'`
Mutation checks: `acb.go` RunE composes warnings from the uncut report (pass the cut instead) → `Test_run_acb_year_still_warns_of_a_security_it_did_not_sell_that_year`; slot-2 `--year` condition `ReturnOfCapitalGain == 0` (drop it) → `Test_ACBWarnings_is_silent_for_a_year_with_only_a_return_of_capital_gain`
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches, 1 feature package (report + report/document; cli wiring not counted)

Spec: SCENARIO-15 + SCENARIO-18; copy at :237 (flag help), :256-257 (text + S15 ruling), :276 (JSON `years`/`securities`), :278-289 (warnings 1-9), :295-296 (`--year` refusals). Every string below is ruled; implement verbatim.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `--year 2025` over `acbRows` prints the sales table alone (caption `Sales in 2025, in CAD`, ruled columns, one sale row, `Total` row), no position table, stderr empty.
- [x] Step 2: default `acb` over a store with a non-registered account and no trades prints the ruled headers-only tables (existing pin kept) and warning form 1 on stderr, exit 0.

**Orchestrator: product-vision ruling 2026-10-05 recorded in spec after the `--year` text paragraph — supersedes this plan: (e) ROC-only-year security IS listed in `securities` (step 3 test flips); (d) stdout never blank — `render_acb_internal_test.go:209-217` headers-only pin is KEPT, strike the trap saying it flips; `--year` always prints a Total row (0.00 when empty); warning 2 has three forms (form 1 / `…, nor in any other year` / span, single-year bare); `--year 0000` refused exit 2 with the not-a-year line; decisions 1, 3 confirmed; S15 owns both `--year` refusals (S17 drops them).**

### Build
- [ ] Step 3: `internal/report/acb.go:13-19` `ACBRequest.Year`, `:36-45` `ACB.Year` (0 = none; `0000` is refused, so no collision), `:203-210` `Server.ACB` copies it; new `internal/report/acb_year.go` `(ACB).InYear()` — years → that one year, a zero `ACBYear` with `Sales` empty when absent; `Securities` → those with a counted sale OR a counted ROC excess that year (ruling :276), walk order kept, events full; no year → a unchanged. Plus the predicates slot 2 reads: no pool events (`Securities` empty), years with a counted sale (span), year-has-nothing (no sale AND `ReturnOfCapitalGain` 0). Tests `internal/report/acb_year_test.go`: year with sales; absent year; gap year inside first–last; ROC-only year (security listed); year whose only sales are no-rate-excluded (empty, security not listed); security sold in another year dropped; events full history; no year identity; predicates: empty, buy-only, all-pre-rate (Years empty, Securities not), ROC-only years out of the span
- [ ] Step 4: `internal/report/acb_year.go` `ParseACBYear(value, now)` + typed error, on the `ParseAsOf` precedent (`internal/report/asof.go:44-67`; `internal/cli/holdings.go:44-51` wraps as `UsageError`); `--year` registered at `internal/cli/acb.go:73-75` beside `currency.bind`, parsed at the top of RunE `:48`, BEFORE `readConfig`/`openReport`. Lines :295-296, exit 2. Tests: report table — out of domain `0000`, `+2024`, `-202`, `20245`, `abcd`, `24`, `2024-03`, empty (not-a-year line, raw echoed); in domain `0001`, `2024`; bound this year accepted / next year refused, both from the same `report.Today(now())` the walk uses, plus a zone-ahead-of-UTC Jan 1 row; `internal/cli/acb_test.go` flag-help pin mirroring `:93-100`; cmd row in `run_acb_year_test.go`: bad `--year` with no store → ruled line, exit 2 (not the no-store refusal)
- [ ] Step 5: `internal/report/document/acb.go:97-119` `NewACB` sets `year` from `ACB.Year` (null without); `internal/report/document/acb_warnings.go:18-29` slot 2 between `adjustmentWarnings` and `superficialLossWarnings`, forms in ruled order (:257): form 1 when `Securities` empty (with or without `--year`); else `--year` only: `…, nor in any other year` when no year has a counted sale; else the span `<first>–<last>` (U+2013), bare `<first>` when first = last; fires under `--year` only when that year has nothing. Tests: `document/acb_test.go:83` OWED `years[]` key-order pin for a `--year` document, `return_of_capital_gain` right after `gain`, `"0.00"`; JSON form-1 shapes (`"year":null,"years":[],"securities":[]`; `--year 2025` → `"year":2025`, one zero entry, `"sales":[]`); `acb_warnings_test.go`: each of the four slot-2 outputs; silent for a year with sales, `Test_ACBWarnings_is_silent_for_a_year_with_only_a_return_of_capital_gain`, buy-only default; span skips ROC-only and no-rate-excluded years; full 1–9 order with every slot firing (adjustment issue + empty year + 3..9) in one hand-built `report.ACB`
- [ ] Step 6: `internal/cli/render_acb.go:19-38` `renderACB` branches on `ACB.Year` ONLY (never on emptiness) to a sales renderer: caption `Sales in <year>, in CAD`; columns `Date  Security  Shares  Proceeds  Outlays  ACB  Gain or loss` + unheaded suffix column (`possible superficial loss`, `unknown cost`, ", "-joined, that order; year suffix order `:41-53` untouched); security = ticker else name via `escapeCell`; Total row ALWAYS (Date `tableTotalLabel`, Security/Shares blank, four sums, Gain sales only, suffix blank; 0.00 when no sale); ROC row after it only when > 0. `acb.go:66-70`: warnings from the uncut report, text and JSON from `InYear()`. Tests: render internal — each suffix alone, both stacked, ticker / name / escape, empty year (Total 0.00, no ROC row), ROC-only year (Total 0.00 + ROC row), Total sums; headers-only pin `render_acb_internal_test.go:209-217` stays green unchanged; cmd `run_acb_year_test.go`, each text AND `--json`: stacked marks; empty year with sales elsewhere (span warning); ROC-only year (security listed, no warning 2); no-rate-only year (warnings 2 and 6 together); gap year; buy-only and all-pre-rate default (year header no rows, positions shown, no warning 2); `Test_run_acb_year_still_warns_of_a_security_it_did_not_sell_that_year` (warning 4 or 7 from an unsold security); `--year` stderr equals the default run's warnings plus slot 2

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `InYear`, `ParseACBYear`, the predicates, `ACB.Year`, `ACBRequest.Year`

### Verify
- [ ] Step 8: full verification + `spec-check.py phase4de-acb` → tick SCENARIO-15 with its acceptance test, and SCENARIO-18 as `delivered by SCENARIO-15 —` its folded test (test reference last on the line); STATE.md: strike the `--year` open debt, the S15/S18 Left-unbuilt rows, and S14's "test years, not securities" trap (superseded below)

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `ACBRequest.Year` → `ACB.Year` → `(ACB).InYear()` is the ONE year cut — S19's MCP `year` sets the same field; a second cut lets CLI and MCP disagree
- Warnings 1 and 3-9 come from the UNCUT report; only slot 2 reads the year (ruled) — the `--year` stderr is the default run's plus slot 2
- Emptiness (warning 2 form 1) = no pool events (`Securities` empty), NOT `Years` empty (ruled) — supersedes S14's STATE trap: a buy-only or all-pre-rate pool prints a position table
- The span counts only years with a counted sale (not ROC-only, not warning-6-excluded)
- The renderer never branches on emptiness; stdout is never blank on exit 0 (ruled)
- `--year` parse and both refusals live here (`ParseACBYear`, exit 2, before config/store) — S17's plan drops the bad and future `--year` rows
- `--year` `securities` = a counted sale or ROC excess that year; text is the sales table alone

**Left unbuilt** — named so nobody assumes it exists:
- `--year` combined with `--security` — S16 (unruled)
- MCP `acb` `year` param — S19

**Traps** — things that look right and are not:
- Passing `acb.InYear()` to `document.ACBWarnings` compiles and drops warning 4/6/7 for a security unsold that year
- `acbYears` copies sales by value; `InYear` must not reorder `Securities` (warnings 4 and 7 rely on walk order)
- Do not retype the headers-only stdout in the S18 test with `acbYearLine`: its widths are the filled fixture's

## Phase report

Run A. The committed plan body was missing: the file held only the orchestrator ruling block (e5af2b5), so steps 1-2 above are reconstructed from the spec's S15/S18 scenarios and `--year` ruling. Build, Sweep and Verify steps do not exist; re-plan before run B.
- `cmd/quarry/run_acb_year_test.go` (new): helper `acbSaleLine`, const `acbNothingToShowWarning`, both acceptance tests. No production code touched; `--year` is still unregistered.
- Red 1: `run_acb_year_test.go:33` exit code expected 0, actual 2 (`quarry: unknown flag: --year`). Red 2: `run_acb_year_test.go:60` stderr expected the form-1 warning line, actual empty (stdout assertions above it already pass: headers-only pin).
- The `--year 2025` expected table (widths, `Total` row, blank Security/Shares) is unverified until green: the red stops at the exit code. Re-check column widths when `--year` lands.
- Do not retype the headers-only stdout in test 2 with `acbYearLine`: its widths are the filled fixture's, not the empty table's.
