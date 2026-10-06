---
id: SCENARIO-06
status: open
---

# SCENARIO-06: First month of data shows no change (folds SCENARIO-07 A quiet month)

Cadence: code-first (pure rendering; no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_summary_empty_test.go` `Test_run_summary_shows_no_change_in_the_first_month_of_data`
Acceptance test (SCENARIO-07, folded): `cmd/quarry/run_summary_empty_test.go` `Test_run_summary_says_so_in_a_month_with_no_unusual_charge_and_no_new_recurring_charge`
Narrow loop: `go test ./internal/cli/ -run 'Summary|NetWorthWithChange' && go test ./cmd/quarry/ -run 'Summary'`
Mutation checks: anomalies empty-section condition `len(a.Listed)==0` → `a.Checked==0` → `Test_summaryAnomaliesSection_says_none_when_no_charge_is_listed` (row Checked>0, Listed empty); drop the "end day also empty" test in the no-balance branch of `renderNetWorthWithChange` → `Test_renderNetWorthWithChange_says_no_account_has_a_balance_when_neither_month_end_has_one` plus the S06 acceptance test
Runs: A (1-2) | B1 (3) | B2 (4) | V (5-6)
Size: OWNS A RUN — 2 batches, 1 package (`internal/cli`; tests also in `cmd/quarry`); S07 FOLDED (same three renderers, no report change)

No new port or adapter: nothing to survey. No fallible call and no numeric bound added (strings only), so no fault or bound rows. `internal/report` needs no change: `NetWorth.Change()` nil already means "first month end has no row" (networth_change.go:16-19) and `Dates` always holds both month ends (networth.go:119-144).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: new `cmd/quarry/run_summary_empty_test.go` `Test_run_summary_shows_no_change_in_the_first_month_of_data` — `spendRows` store whose only transactions are in September (template `run_summary_change_test.go:78-96`, clock `summaryClock`), `summary --month 2026-09`; full-document pin: heading, Snapshot/Dates/Findings, section bodies, history table (Aug 31 line date-only, Sep 30 line), then blank line and `No change shown: no account has a balance on 2026-08-31.`, no Change line; stderr empty; fails at the stdout assertion
- [x] Step 2: same file `Test_run_summary_says_so_in_a_month_with_no_unusual_charge_and_no_new_recurring_charge` — default month (September), store whose September holds only ordinary charges (non-zero `N charges checked`, none unusual, no series recognized), balances both month ends; pins `No unusually large charges.` + footer, `No new recurring charges.`, normal net worth with Change; stderr empty

### Build
- [x] Step 3: `internal/cli/render_summary.go:22-32` `renderSummary` + new `summaryAnomaliesSection`, `summaryRecurringSection`, shared `renderEmptySection(caption, line)` (= `renderTable` with one left-aligned cell, `render_table.go:26-50`); `render_anomalies.go:44` hoist the `"Unusually large charges"` literal to a const both renderers use — quiet-month sections (SCENARIO-07)
  - Anomalies: `len(Listed)==0` → caption (`windowCaption`), blank line, `No unusually large charges.`, blank line, `anomaliesFooter(Checked, NotJudged)` (footer keeps the not-judged clause when Checked>0); otherwise `renderAnomalies` untouched — `quarry anomalies` output stays byte-identical
  - Recurring: `r.Empty()` → caption `windowCaption(summaryRecurringTitle, …)`, blank line, `No new recurring charges.`; otherwise `renderRecurringTitled` untouched
  - Tests (`render_summary_internal_test.go`, white-box, after `:74-80`): `Test_summaryAnomaliesSection_says_none_when_no_charge_is_listed` (rows: Checked 0; Checked 412 + NotJudged 37; one Listed row = control that the header row still prints; native heading adds no amounts clause), `Test_summaryRecurringSection_says_none_when_no_series_is_new` (empty vs one series control; CAD vs native caption). Update fixture `septemberSummary` (`:19-26`) to carry both month ends in `NetWorth.Dates` (as `Server.Summary` always does) and re-pin `Test_renderSummary_says_when_the_store_holds_no_transactions` (`:74-80`) to the full empty document
- [ ] Step 4: `internal/cli/render_summary.go:34-42` `renderNetWorthWithChange` — net worth empty lines
  - Neither month end has a row (`Change()==nil` and last day has no rows): `renderEmptySection(netWorthHistoryCaption(n), "No account has a balance on <d0> or <d1>.")`, no table
  - First month end empty, last not (`Change()==nil`): history table unchanged, then blank line, `No change shown: no account has a balance on <d0>.`
  - Else (including U6 end-empty): table + Change rows as today, no new branch; dates from `n.Dates[0]` / `n.Dates[len-1]`, `time.DateOnly`
  - Tests (`render_summary_internal_test.go`, extend `:185-196` which already holds `augustEnd`/`septemberEnd` and `monthEndHolding`): `Test_renderNetWorthWithChange_says_no_account_has_a_balance_when_neither_month_end_has_one` (CAD, USD, native: same line, caption unchanged), `Test_renderNetWorthWithChange_says_no_change_is_shown_when_only_the_first_month_end_is_empty` (CAD and native, the latter with CAD and USD on the end day: U10 — fires only when the start day has no row in any currency; control: start day with a row in one currency only → Change rows, no line). Command-level cells (`run_summary_empty_test.go`): `Test_run_summary_prints_the_ruled_empty_lines_for_a_store_without_transactions_and_a_month_before_all_data` (table of: store with no transactions → `Dates     no transactions` and all four empty lines; `--month 2020-01` on the seeded store → heading plus empty lines, month named in `no account has a balance on 2019-12-31 or 2020-01-31.`; empty month between data via `--month 2026-03` on `seedSummaryStore` → `0 charges checked`, `No new recurring charges.`, normal Change; each asserts empty stderr and exit 0) and `Test_run_summary_shows_no_change_in_native_when_no_currency_has_a_balance_on_the_first_month_end` (`--currency native` on a CAD+USD store first balanced in September: no Change rows, one line)

### Sweep
- [ ] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on the new helpers (1-2 lines each)

### Verify
- [ ] Step 6: full verification (`agent-briefs.md` → *Verification*) + `.claude/scripts/spec-check.py phase4f-summary` → tick SCENARIO-06 with its acceptance test and SCENARIO-07 ("delivered by SCENARIO-06") in `specification.md`; rewrite STATE.md

## Handoff

**Binding decisions:**
- Empty section bodies are summary-only composers in `render_summary.go`; `renderAnomalies`, `renderRecurringTitled`, `renderNetWorthHistory` and every other command's output stay byte-identical — `quarry anomalies/recurring/networth` have their own ruled empty behaviour (header row plus stderr warning)
- The empty-section test is `len(Listed)==0` (anomalies) and `Recurring.Empty()`, not `Checked==0`: a month with charges all too young to judge says `No unusually large charges.` above `N charges checked; M had too little history to judge`
- No-change vs no-balance both key on `NetWorth.Change()==nil`; they differ only by whether the last month end has rows — no report-package change, and no branch for the unreachable end-day-empty case (U6)

**Defaults pending ruling** (no copy exists in `## Surface & Copy`):
- Gherkin S06 says the table "shows September 30 only"; Surface says "table, no Change row". Default: the history table is unchanged, so the Aug 31 line prints date-only (blank cells), as `quarry networth` does
- "Anomalies section byte-identical to `quarry anomalies`" holds for non-empty months only; for an empty month the ruled `No unusually large charges.` variant differs from `quarry anomalies` (which prints the header row). `Test_run_summary_anomalies_section_equals_quarry_anomalies_of_the_month` stays on the non-empty store
- Native empty lines are the converted ones: caption without an amounts clause, same sentences

**Left unbuilt:**
- Suppression of the empty-window lines of `AnomaliesWarnings` (warnings.go:61-68, fires on `Checked==0`) / `RecurringWarnings` (:53-59) and `emptyNetWorthWarnings` — SCENARIO-14 wires W4-W6; `internal/cli/summary.go` calls none of them today (grepped), so this scenario removes nothing and its empty-stderr pins are green on arrival; they become load-bearing when S14 lands and S14 must keep them green
- `--json` empty shapes (`{"types":[],"totals":[]}`, `dates` null/null) — SCENARIO-12

**Traps:**
- `septemberSummary` has no `NetWorth.Dates`; a renderer indexing `Dates[0]` panics on it, so give the fixture both month ends rather than adding an untested length guard
- The recurring empty-window warning fires on `Recurring.Empty()` but the anomalies one on `Checked==0` — different conditions from the body's; S14 must drop only that last line of each, keeping the left-out/unconverted lines
- `spendEnv`'s clock refuses `--month 2026-09`; use `summaryClock` (`run_summary_test.go:23`) and `pinLocalZone` where the Snapshot age prints

**Orchestrator rulings 2026-10-06 (pre-dispatch):** all four defaults stand — (1) first-month table is the unchanged history table (Aug 31 date-only, as `quarry networth`), Gherkin "September 30 only" read as only Sep 30 has balances; (2) ruled empty variant wins over anomalies byte-identity for an empty month — the identity invariant holds for non-empty months; (3) native empty lines same sentences, no amounts clause; (4) U6 no branch.

## Phase report

Run B1 (step 3) done. Start commit 83c1608. Step 3 ticked; steps 4-6 open.

Files: `internal/cli/render_summary.go:22-60` (`renderSummary` calls `summaryAnomaliesSection` / `summaryRecurringSection`; shared `renderEmptySection(caption, line)`); `internal/cli/render_anomalies.go:13-14,45` (const `anomaliesTitle`, used by `renderAnomalies` and the summary composer); `internal/cli/render_summary_internal_test.go` (fixture `septemberSummary` now carries both windows and both month ends in `NetWorth.Dates`; empty-store test re-pinned to the Dates-through-recurring part of the document; new `Test_summaryAnomaliesSection_says_none_when_no_charge_is_listed` (3 rows: Checked 0; 412/37 not-judged clause; native no amounts clause), `Test_summaryAnomaliesSection_keeps_the_anomalies_table_when_a_charge_is_listed`, `Test_summaryRecurringSection_says_none_when_no_series_is_new` (CAD, native), `Test_summaryRecurringSection_keeps_the_recurring_table_when_a_series_is_new`).

Green: `go test ./internal/cli/` (603, +4); S07 acceptance `Test_run_summary_says_so_in_a_month_with_no_unusual_charge_and_no_new_recurring_charge` passes. Red, as planned: `Test_run_summary_shows_no_change_in_the_first_month_of_data` differs only by the missing trailing `\nNo change shown: no account has a balance on 2026-08-31.\n` (step 4).

Mutation (plan line, anomalies condition): `render_summary.go:37` `len(a.Listed) > 0` -> `a.Checked > 0` reddened `Test_summaryAnomaliesSection_says_none_when_no_charge_is_listed/charges_checked_and_some_too_young_to_judge` (actual printed the header row `Date  Account  Payee ...` instead of `No unusually large charges.`) and `/native_adds_no_amounts_clause`; restored, diff identical.

Next runs: B2 must keep `renderNetWorthWithChange` (render_summary.go) working on `septemberSummary`'s two empty month ends: re-pin `Test_renderSummary_says_when_the_store_holds_no_transactions` to the FULL empty document there once the no-balance line exists (this run pins only up to the recurring section; the net worth part was left because it is step 4). `Test_renderNetWorthWithChange_adds_no_line_when_the_first_month_end_has_no_balance` (:185 area) asserts NotContains "Change"; step 4 adds the `No change shown` line, so it needs tightening, not deleting. Do not edit the acceptance expectations.
