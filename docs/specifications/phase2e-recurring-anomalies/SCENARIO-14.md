---
id: SCENARIO-14
status: open
---

# SCENARIO-14: a charge over twice the payee's usual is listed

Cadence: code-first (read-only report; no write-safety guard, atomic adapter or bug fix touched)
Acceptance test: `cmd/quarry/run_anomalies_test.go` `Test_run_anomalies_lists_a_charge_over_twice_the_payees_usual`
Acceptance test (SCENARIO-15, folded): `internal/report/anomalies_test.go` `Test_anomalies_thresholds_hold_at_their_boundaries`
Acceptance test (SCENARIO-16, folded): `internal/cli/report_help_test.go` `Test_each_reports_window_flags_describe_what_it_does_with_them`
Narrow loop: `go test ./internal/report/ -run 'Anomal'` (B1); `go test ./internal/cli/ ./cmd/quarry/ -run 'Anomal|help|Help|ignore_a_malformed_config|Available'` (B2)
Mutation checks: `amount > AnomalyPayeeMultiplier*median` → `>=` in `anomalies.go` → `Test_anomalies_thresholds_hold_at_their_boundaries` (60.00/120.00 row); `AnomalyMinAmount` floor `<` → `<=` → `Test_anomalies_never_list_a_charge_under_the_minimum`; strictly-earlier `Before` → `!After` → `Test_anomalies_do_not_count_same_day_charges_as_history`; history gate `>= AnomalyPayeeMinHistory` → `>` → `Test_anomalies_need_three_earlier_charges_for_a_payee_baseline`
Runs: A (1-4) | B1 (5-6) | B2 (7-8) | V (9-10)
Size: OWNS A RUN — 4 batches, 1 feature package (`report`; `cli` + `cmd/quarry` wiring); orchestrator overruled SPLIT, B1 = 14a, B2 = 14b

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_anomalies_test.go` (new) `Test_run_anomalies_lists_a_charge_over_twice_the_payees_usual` — `chargeRows` fixture (`run_helpers_test.go:169-200`): one payee, ≥3 earlier charges last year with median 96.05, then 412.00 this year; assert exact stdout (caption `Unusually large charges … in all accounts`, header `Date  Account  Payee  Category  Amount  Usual  Times  Compared with`, row `… 412.00  96.05  4.3x  payee, N earlier`, blank line, footer), empty stderr, exit 0. Table helper modelled on `run_recurring_test.go:18-50` (Amount, Usual, Times right-aligned)
- [ ] Step 2: `internal/report/anomalies_test.go` (new) `Test_anomalies_thresholds_hold_at_their_boundaries` — `Server.Anomalies` table over the three SCENARIO-15 rows (40.00/99.99 not listed, 60.00/120.00 not listed, 60.00/120.01 listed) against `fakeStore` (`fakes_test.go:54-66`), reusing `chargeOn`/`paidTo`/`ofAmount` (`recurring_test.go:17-60`)
- [ ] Step 3: `internal/cli/report_help_test.go` `Test_each_reports_window_flags_describe_what_it_does_with_them` — table over spend, cashflow, recurring, anomalies `--help`: `--since`/`--until`/`--account` lines verbatim (spend/cashflow = `transactionFlagHelp`, `window.go:22-27`; recurring = `recurring.go:14-18`; anomalies = `## Surface & Copy`)
- [ ] Step 4: `internal/report/anomalies.go` (new) `AnomaliesRequest{Window, Now, Accounts}`, `Anomalies`, `Anomaly`, `AnomalyBaseline` (`BaselinePayee` only), `(*Server).Anomalies` returning zero value; `internal/cli/anomalies.go` (new) stub `newAnomaliesCommand(newReport, now)` with `Use`/`Short` only, RunE calling the stub; `root.go:33` register + `run_status_test.go:121-131` root pin gains `anomalies` (cobra order, after `accounts`) in the same step. Red: S14 on stdout, S15 on empty `Anomalies`, S16 on the missing flag lines

### Build
- [ ] Step 5: `internal/report/charges.go:79+` `medianCents` + `anomalies.go` consts `AnomalyPayeeMinHistory`, `AnomalyPayeeMultiplier`, `AnomalyMinAmount` (cents), payee judging (history = same `groupKey` charges strictly earlier by date, from every account and before the window) and times as tenths half away from zero (`changeTenths` rounding precedent, `pricechange.go:40-48`). Pins through `Server.Anomalies`, each differing from its control in one variable: history 2 → not_judged vs 3 → judged; same-day third charge not history vs one day earlier (`Test_anomalies_do_not_count_same_day_charges_as_history`); 99.99 vs 100.00 with a median low enough that both clear 2× (`Test_anomalies_never_list_a_charge_under_the_minimum`); odd-count median with date order ≠ amount order; even-count median whose two middles sum to an odd cent count (rounds up); times landing exactly on a `.x5` tenth; `Earlier` and Usual from >3 earlier charges (all of them, not the first 3); another currency's charges are not history; NULL payee → not_judged; under-100.00 charge with no baseline → not_judged; under-100.00 with a baseline → judged, not listed. Every not_judged fixture has `Category == nil` (S17 must not flip it)
- [ ] Step 6: `anomalies.go` `(*Server).Anomalies` — `today := DefaultWindow(req.Now).Until`, one `s.store.Charges` read (`recurring.go:191-195` shape), result carries `Window`, `Transactions`; `checked` = charges with `Window.Since ≤ Date ≤ Window.Until` (NULL payee included), `NotJudged`, listing sorted date desc then `SourceID` desc. Tests: `chargesReads == 1`; read fault → `RefusalError` naming `anomalies` via `readRefusal` (`refusal.go:29`), fault shaped as the adapter's; window bounds (charge on `since` listed / day before is history only; on `until` listed / day after not); sort both tiers in both input orders; an earlier anomaly still counts as history
- [ ] Step 7: `internal/cli/anomalies.go` full command — `anomaliesCommand` const, Short/Long/Example verbatim, `anomaliesFlagHelp` via `flags.bind`, `noArgs`, `flags.window`, `openReport`, `Server.Anomalies` with `Accounts: flags.accounts`, fault → `runtimeError`, `emitReport(cmd, false, []string{}, …)` (S01 no-`jsonOut` precedent, `recurring.go:47-67`). Tests in `internal/cli/anomalies_test.go` (new): help Long/Examples/each flag verbatim (`recurring_test.go:126-196` shape), report fault → exit 1. `root.go:7-10` and `internal/report/doc.go:1-3` doc comments name anomalies; `cmd/quarry/run_config_test.go:209-243` adds `anomalies` and gives the shared fixture one real anomaly from a payee other than Costco (stderr stays empty whichever E1 trigger S20 rules)
- [ ] Step 8: `internal/cli/render_anomalies.go` (new) `renderAnomalies` — `windowCaption("Unusually large charges", …)` (`render_table.go:52`), `renderTable` with Amount/Usual/Times right-aligned, cells: date, `accountLabel` (`render.go:264`), `escapeCell` payee, category cell, `formatMoney` amount/usual, `N.Nx`, `payee, N earlier`; footer after a blank line. Extract the category cell rule from `render_findings.go:163-173` into one helper over (splits, path) both renderers call — findings' category pins stay green unchanged as control. `render_anomalies_internal_test.go` (new): footer `0 charges checked`, `1 charge checked`, `; 1 had too little history to judge`, clause omitted at 0, thousands grouping on both counts from constructed values (1,204 / 1,087); category `(uncategorized)`, single path, `(split)` for two splits sharing one category (`Category != nil`, `ExpenseSplits > 1`); `\n\t\r` escaped in payee, account and category; empty listing = caption, header, footer

### Sweep
- [ ] Step 9: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on every new exported symbol and the three consts

### Verify
- [ ] Step 10: full verification + `spec-check.py phase2e-recurring-anomalies` → tick SCENARIO-14, and SCENARIO-15 / SCENARIO-16 each "delivered by SCENARIO-14" with their acceptance tests

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `Server.Anomalies` makes exactly one `Charges` read, no window in the read; history = same `groupKey` (charges.go), strictly earlier date, every account, all time. S17/S20 add to this read, never a second one — STATE's port decision
- `not_judged` = every in-window charge with no baseline, whatever its amount (P2e-13 as written; an under-100.00 first-time charge counts). `checked` includes NULL-payee charges
- Flag rule in integer cents: `Amount >= AnomalyMinAmount && Amount > AnomalyPayeeMultiplier*median`; median of an even count `(a+b+1)/2` (amounts positive); times stored as tenths, half away from zero
- `Anomaly` carries the whole `store.Charge` plus `Baseline`, `Usual`, `Earlier`, `TimesTenths`; S19's JSON reads the same fields as text, in the same order
- Category cell is one shared helper (findings + anomalies): `(split)` when splits > 1, else `(uncategorized)` when no category, else the escaped path

**Left unbuilt** — named so nobody assumes it exists:
- `BaselineCategory`, `AnomalyCategoryMinHistory`, `AnomalyCategoryMultiplier`, the `category, N earlier` cell — S17
- `renderAnomaliesJSON` and the `jsonOut` parameter of `newAnomaliesCommand`; `anomalies --json` prints text until then — S19
- `AnomaliesRequest.Accounts` is passed but ignored (no `namedAccounts`, no listing filter, no W2/W3, no E1/E2 `anomaliesWarnings`); `--account` is bound only for its help — S20
- `run_read_usage_test.go:42` / `run_read_refusals_test.go:55,171` anomalies rows (U8, R1, I1) — S20
- No doc list to amend for P2d-10: the 2d spec's list is closed; P2e-14 is the amendment, the pin is `run_config_test.go`

**Traps** — things that look right and are not:
- A not_judged fixture with one category becomes category-judged in S17 and breaks S14's pins — keep `Category == nil`
- The payee cell dereferences a non-nil payee: only S17's category baseline can list a NULL-payee charge, and its copy is unruled (`payeeLabel` `(no payee)`, `render.go:330`, is precedent only)
- `Charges.Rows` run through today only; `--until` past today lists nothing extra, but `checked` must still use the request window, not `today`

## Phase report
