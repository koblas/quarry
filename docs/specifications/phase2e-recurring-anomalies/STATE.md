# phase2e-recurring-anomalies — current state

Scenarios complete: SCENARIO-01..16 (02-04 folded into 01, 06 into 05, 08-10 into 07, 12-13 into 11, 15-16 into 14). Last updated by SCENARIO-14.

## Binding decisions
- One port method `report.Store.Charges(ctx, store.ChargeParams{Through, AccountIDs})` returns `store.Charges{Rows []Charge, Transactions TransactionRange}`: no window; `Rows` are never filtered by account; `Transactions` is the whole store's span, or with `AccountIDs` set the span of those reported accounts only (spend's E1a range, so E1a/E2a say "their transactions"). Anomalies (S14 built, S17/S20 extend) read this single call; reopening it reopens three fakes, the adapter and the `cmd/quarry/run.go:29` guard (SCENARIO-01, SCENARIO-11)
- `store.Charge` carries TransactionID, SourceID, Date, Account (`store.Account`: ID/Name/Currency/Closed/Active, for `accountLabel`), PayeeID/Payee `*string`, Currency, Amount (cents), Category `*ChargeCategory{ID, Path}` (set only when every expense row shares one non-NULL category), ExpenseSplits (count of `v_spending` rows). S14/S17: category baseline when Category != nil; cell `(split)` when ExpenseSplits > 1, else `(uncategorized)` when Category is nil. One kind enum cannot express two same-category splits (SCENARIO-01)
- Detection lives in package `report` (no leaf package): `charges.go` holds the group key (tagged `keyKind` PayeeKey vs payee_id, so the two never merge; S07's `payee_key: null` reads the tag; S14 adds its median here); `recurring.go` holds the named cadence consts (`cadenceRules`, gap range / min charges / ended-after / per-year factor), `latestRun` walk and `Server.Recurring` (SCENARIO-01)
- Through = `DefaultWindow(req.Now).Until`, the local civil date, never `Window.Until`; the adapter computes no date. Duckstore filters `date <= Through` in SQL because `v_spending` keeps future-dated rows (SCENARIO-01)
- `Server.Recurring` makes one `Charges` read (pinned by `chargesReads`); a read fault is `readRefusal` naming recurring (SCENARIO-01)
- `recurring --account`: `Server.Recurring` calls `namedAccounts` first (a refusal means no `Charges` read), then keeps a series when `Series.Accounts` (the run's distinct accounts, not the latest charge's) holds a named id, after the steady gate and `runsDuring`, before `yearlyTotals` (Totals sum listed series only). `Recurring.Empty()` is `len(Series)==0` after that filter, so "series exist but none in the named accounts" is empty too (SCENARIO-11)
- `recurringWarnings(rec)` builds the one slice that feeds stderr and `warnings[]`: `leftOutWarnings` (W2/W3, argv order) first, then E1/E2/E1a/E2a via `appendEmptyWindowWarning(…, "recurring charges", …)` only when `Empty()`; every named account left out gives W only. S20 anomalies reuses the order (SCENARIO-11)
- Price change and steady gate use integer cents, never floats (a float `> 0.05` misfires at exactly 5%): `|to-from|*100 > 5*from`; `Series.PriceChanges[i].Tenths` and `Series.ChangeTenths` (first to latest) are tenths of a percent, half away from zero. Text shows tenths with `+`/`-`, none at 0; JSON `change_pct` is `float64(tenths)/10`. A price change carries the later charge's date (SCENARIO-07)
- Steady gate sits in `Server.Recurring` before listing (`changes <= steps/4`, floor): a dropped group is in neither `Series` nor `Totals`; the window and `--account` filters apply to survivors only. A fixture with a >5% step needs enough charges for the gate to keep it (3 charges allow 0 changes) (SCENARIO-07)
- `Series.PayeeKey` is nil only for the payee_id fallback (reads `key.kind`); `Series.Payees` (`SeriesPayee{ID, Name}`) and `Series.Accounts` are the run's distinct values by id in first-appearance order, never nil. `--account` match and JSON `accounts` read `Series.Accounts` (SCENARIO-07)
- `recurring --json` is `renderRecurringJSON(r, warnings)` in `internal/cli/json_recurring.go`; every array built non-nil (`[]`), `per_year` null when ended, `payee_key` null on fallback; text and JSON iterate `r.Series` once, same order. `jsonOut` is the persistent flag at `root.go:26` (SCENARIO-07)
- `reportFlags.bind(cmd, reportFlagHelp)` takes per-command flag help (spend/cashflow pass today's strings, byte-identical); `renderTable` takes per-column alignment and trims trailing spaces (SCENARIO-01)
- Recurring table column widths come from the padding rule (widest cell after `escapeCell`, two-space gaps), not from the spec sample's bytes; the sample header implies Every 6 wide but its Total row is 48 chars (SCENARIO-01)
- `Series.New` is `!First.Before(Window.Since)`, set after the `runsDuring` listing filter (a listed series already starts by `Until`, so no upper check); the Status cell is the state word plus `, new` (`active, new`, `ended, new`), no trailing space. Any `--since 2000` fixture therefore reads `, new`. S07's `new` JSON field reads `Series.New` (SCENARIO-05)
- Ended bound is pinned at bound and bound+1 per cadence (weekly 14/15, monthly 45/46, quarterly 120/121, annual 400/401), local-date pin `Test_recurring_counts_days_since_the_last_charge_from_the_local_date` (SCENARIO-05)
- Read-command pins already include recurring: root Available Commands pin (`cmd/quarry/run_status_test.go`), `Test_run_read_commands_ignore_a_malformed_config` (fixture holds one active Costco monthly series so stderr stays empty now that E1 is live) (SCENARIO-01)

- `Server.Anomalies` makes one `Charges` read (`chargesReads == 1`), no window in the read; history = same `groupKey`, strictly earlier date (same-day is not history), every account, same currency, all time. S17/S20 add to that read, never a second one (SCENARIO-14)
- Flag rule in integer cents: `Amount >= AnomalyMinAmount (10000) && Amount > AnomalyPayeeMultiplier(2)*median`, history gate `>= AnomalyPayeeMinHistory (3)`; even-count median `(a+b+1)/2`; `TimesTenths` half away from zero. `Anomaly` carries the whole `store.Charge` plus `Baseline`, `Usual`, `Earlier`, `TimesTenths`; S19's JSON reads those same fields (SCENARIO-14)
- Ruled at S14 planning (product-vision, three): (1) `not_judged` = in-window charges of 100.00 or more with no baseline; a charge under 100.00 is judged by the floor and never counted (spec P2e-13); (2) empty-window warning only when `checked` = 0 (subject `unusually large charges`), none when charges exist but none is unusual (stderr empty, `warnings` `[]`); (3) NULL payee reads `(no payee)` in text and `null` in `--json` `payee`; `checked` includes NULL-payee charges (SCENARIO-14)
- Category cell is one shared helper `categoryText(splits, path)` (findings + anomalies): `(split)` when splits > 1, else `(uncategorized)` when no category, else escaped path; findings' pins unchanged control (SCENARIO-14)
- Interim until S19/S20: `anomalies --json` prints the text table (`emitReport(cmd, false, []string{}, nil, text)`), and `--account` is bound for help only (`Accounts` passed, ignored; caption always says "all accounts") (SCENARIO-14)
- Window-flag help is per report (`reportFlagHelp`): spend/cashflow `transactionFlagHelp`, recurring and anomalies own strings; pinned by `Test_each_reports_window_flags_describe_what_it_does_with_them` (SCENARIO-14)

## Left unbuilt
- `BaselineCategory`, `AnomalyCategoryMinHistory`, `AnomalyCategoryMultiplier`, the `category, N earlier` cell (hook: `anomaliesBaselineWord`, `render_anomalies.go`) — S17
- `renderAnomaliesJSON` and the `jsonOut` parameter of `newAnomaliesCommand` — S19
- `AnomaliesRequest.Accounts` honoured: `namedAccounts`, listing filter, W2/W3, E1/E2 `anomaliesWarnings` via `appendEmptyWindowWarning`; caption naming accounts — S20
- `run_read_usage_test.go:42` / `run_read_refusals_test.go:55,171` anomalies rows (U8, R1, I1) — S20
- Anomalies pins already in place: root Available Commands (`run_status_test.go`) and never-load-config (`run_config_test.go`, fixture holds a Bakery anomaly); no doc list to amend for P2d-10 (SCENARIO-14)

## Traps
- `v_spending` keeps future-dated rows (spend and cashflow count them); any new `Charges`-style read must filter on Through in SQL (SCENARIO-01)
- `fakeStore` / `fakeReportStore` methods have value receivers, so counters need pointer fields (`accountsReads` precedent) (SCENARIO-01)
- A store fault through `Server.Recurring` is a `RefusalError` only for recognised kinds; the cli test asserts `ErrorIs`, exit 1 comes from `Execute` unwrapping `runtimeError` (SCENARIO-01)

- Not_judged fixtures need `Category == nil`: a one-category fixture becomes category-judged in S17 and breaks S14's pins (SCENARIO-14)
- Payee cell dereferences a non-nil payee (nil reads `""` so `payeeLabel` gives `(no payee)`); only S17's category baseline can list a NULL-payee charge (SCENARIO-14)
- `Charges.Rows` run through today only: `--until` past today lists nothing extra, but `checked` uses the request window, not today (SCENARIO-14)
- `-run` patterns are case-sensitive: the plan's `Anomal` misses `Test_run_anomalies_*` in `cmd/quarry`; use `anomal|Anomal` in narrow loops (SCENARIO-14)

## Open debts
- I1 (`recurring interrupted`) on the accounts read and on the `Charges` read is pinned in `internal/report` only; the cmd-level I1 row cancels before the store opens. Low risk, no owner — dies unless re-opened (SCENARIO-11)
- Reference check on the real Quicken file after SCENARIO-22, before the gate round (`REFERENCE-CHECK.md`) — unowned until the architect schedules it; dies unless run
- Checkpoint (5a) MINOR/NIT findings for SCENARIO-01, if any, are recorded here by the orchestrator
- Checkpoint 05 MINOR: `cmd/quarry/run_recurring_state_test.go:19-40` `quietSeriesOutput` asserts inside the helper and returns three unnamed strings — return a struct or move the assertions to test bodies
- Checkpoint 14 MINORs: `internal/report/anomalies.go:60` `NotJudged` doc must say "of 100.00 or more"; no stdout-write-fault test for `anomalies` or `recurring` (`failingWriter`, mirror `spend_test.go:144`); `internal/cli/report_help_test.go:170-172` flag-help regexps unanchored; anomalies Through not pinned to the local civil date (non-UTC `Now`); `render_findings.go:163-165` `categoryCell` doc restates `categoryText` — one line. S20 owns flipping `Test_anomalies_without_charges_prints_the_empty_table_and_footer` to expect E1 and pinning "charges exist, none unusual → no warning"
- Orchestrator: `internal/platform/duckdb` `Test_query_rows_fails_when_the_context_is_cancelled_mid_iteration` (`exec_query_test.go:170`) flaked twice under a loaded full `-coverpkg` run (passes alone) — watch at the gate (unowned)
