---
id: SCENARIO-01
status: open
---

# SCENARIO-01: recurring lists a monthly subscription with its yearly cost

Cadence: code-first — read-only port, query and renderer; no bug fix, write-safety guard or atomic adapter
Acceptance test: `cmd/quarry/run_recurring_test.go` `Test_run_recurring_lists_a_monthly_subscription_with_its_yearly_cost`
Acceptance test (SCENARIO-02, folded): `cmd/quarry/run_recurring_test.go` `Test_run_recurring_detects_weekly_quarterly_and_yearly_series`
Acceptance test (SCENARIO-03, folded): `internal/report/recurring_test.go` `Test_recurring_starts_the_series_again_after_a_charge_off_schedule`
Acceptance test (SCENARIO-04, folded): `cmd/quarry/run_recurring_test.go` `Test_run_recurring_counts_a_split_charge_once_and_leaves_a_refund_out`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/report/ ./internal/cli/ ./cmd/quarry/ -run '(?i)recurring|charges|_reads_|help|table|spend|cash_?flow|ignore_a_malformed'`
Mutation checks: `sum(spent) > 0` filter in duckstore `Charges` → `Test_charges_drops_refunds_and_net_zero_transactions`; `date <= Through` in duckstore `Charges` → `Test_charges_ignores_rows_dated_after_through`; gap-outside-range break in the run walk → `Test_recurring_starts_the_series_again_after_a_charge_off_schedule`; group-key kind tag → `Test_recurring_keeps_a_payee_id_fallback_apart_from_an_equal_payee_key`
Runs: A (1-2) | B1 (3-4) | B2 (5) | B3 (6-7) | B4 (8) | V (9-10)
Size: OWNS A RUN — 6 batches, 1 feature package (orchestrator overruled SPLIT; B1–B2 = 01a, B3–B4 = 01b)

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_recurring_test.go` (new) main + folded 02 (outline rows 7×4, 91×3, 365×2 — First, Every, Amount) + folded 04 (split month and refund mid-series, so a regression shifts First or Amount); `cmd/quarry/run_helpers_test.go:109-180` new `chargeRows` fixture (multi-split transactions, any payee — `spendSplit` is single-split, fixed payees); `internal/report/recurring_test.go` (new) folded 03 through `Server.Recurring` (charge count 3, First = first after the 70-day gap — the text table has no count column)
- [ ] Step 2: signature-only stubs: `internal/store/store.go:519` (append) `ChargeParams`, `Charges`, `Charge`, `ChargeCategory`; `internal/report/store.go:11-24` `Store.Charges`; `internal/store/duckstore/charges.go` (new) `(*Store).Charges`; `internal/report/fakes_test.go:11-49` + `internal/cli/fakes_test.go:18-53` `Charges` with `charges`, `gotCharges`, `chargesReads` fields; `internal/report/recurring.go` (new) `RecurringRequest{Window, Now}`, `Recurring`, `Series`, `(*Server).Recurring`. Port guard `cmd/quarry/run.go:29` and `cli/status_test.go:19` embed need no edit. All four acceptance tests red at an assertion

### Build
- [ ] Step 3: `duckstore/charges.go` `(*Store).Charges` + `duckstore/charges_test.go` (new) — arms: split summed once; refund (<0) and net-zero dropped, 0.01 kept (`Test_charges_drops_refunds_and_net_zero_transactions`); Through day kept, Through+1 dropped (`Test_charges_ignores_rows_dated_after_through`); transfer and not-in-reports account absent (view); category set for one category, for two splits of the same category (ExpenseSplits 2), nil for all-NULL, two different, and NULL-plus-one; NULL payee row kept with nil payee; account Name/Currency/Closed/Active filled; order date then numeric source id (9 before 10) for both insertion orders; `Transactions` filled even with rows. Faults: `read_faults_test.go:21-37` `rowReads` gains `Charges` (open, query, scan, close); second query (`transactionRange`) fault via `spyReadDB{passQueries: 1}`
- [ ] Step 4: `internal/report/charges.go` (new) group key + `internal/report/recurring.go` cadence constants and latest-run walk, wired end to end in `(*Server).Recurring` (Charges → group → latest run → series); tests through `Server.Recurring` on `fakeStore` with a window from 2000 (step 5's overlap filter extends them, never flips them) — group arms: PayeeKey merge ("NETFLIX.COM 1234" + "Netflix.com"), CAD/USD split, two accounts one series, two categories one series, empty key → `payee_id` (same id one series, two ids two), NULL payee never a series, `Test_recurring_keeps_a_payee_id_fallback_apart_from_an_equal_payee_key`; each cadence row: gap lo-1 / lo / hi / hi+1, charges min-1 / min, factor; gap 14 (biweekly) no series; last gap picks the cadence both ways (weekly then monthly → monthly only; monthly then weekly → weekly only); short gap mid-run (charged twice one month) truncates the run; folded 03 green
- [ ] Step 5: `recurring.go` `(*Server).Recurring` — Through = `DefaultWindow(req.Now).Until`, never `Window.Until` (`gotCharges` with `--until` past today; `utcMinus5`-style clock whose local date differs from UTC); listed when span [first, today if active else last] overlaps window (last = since, last = since−1, first = until, first = until+1; last before since listed when active, not when ended — one variable); state active/ended (one control ended series well past its bound — S05 owns bounds); Amount and `PerYear` from the latest charge (first amount differs within 5%), `PerYear` nil when ended; `Totals` per currency of active series only, CAD before USD; payee = latest charge's name; sort — one pin per tier: currency, active before ended, per year desc, ended by last desc, lower(payee), key; `Test_recurring_reads_the_charges_once` (`chargesReads`); Charges fault → `readRefusal` `RefusalError` naming recurring

- [ ] Step 6: `internal/cli/window.go:11-18` `reportFlags.bind` takes the command's flag help; `spend.go:75` and `cashflow.go:92` pass today's strings — controls `spend_window_test.go:152-171`, `report_help_test.go:104-125` stay green unchanged
- [ ] Step 7: `internal/cli/render_table.go:17-51` per-column alignment, trailing spaces trimmed — `render_spend.go:32`, `render_cashflow.go:29` byte-identical (their render tests are the control); `internal/cli/render_recurring.go` (new) `renderRecurring` — caption `windowCaption("Recurring charges", …)`, header, rows (Payee `escapeCell`, Every word, Amount/Per year `formatMoney` right-aligned, First/Last/Status left-aligned, Price changes empty), one Total per currency (only Per year filled); `render_recurring_internal_test.go` (new) pins: payee widths measured after escaping, nil `PerYear` a blank cell, both Status words, no trailing space on a row with empty last cells or on Total, Totals in given order
- [ ] Step 8: `internal/cli/recurring.go` (new) `newRecurringCommand(newReport, now)` — no `jsonOut` parameter until S07 (Use, Short, Long, Example, `noArgs`, `flags.window`, `openReport`, `Server.Recurring` with `now()`, `emitReport` with `false` and `[]string{}` warnings); `root.go:28-33` register; `internal/cli/recurring_test.go` (new) `Server.Recurring` fault → `runtimeError` exit 1; `report_help_test.go` recurring pins: Short, Long, Examples, `--since`/`--until`/`--account` verbatim from `## Surface & Copy`; `cmd/quarry/run_status_test.go:121-130` root pin gains `recurring` in cobra order; `cmd/quarry/run_config_test.go:209-243` adds `recurring` and gives the shared fixture one active monthly series (keeps stderr empty after S11's E1 lands). Main acceptance test and folded 02/04 green

### Sweep
- [ ] Step 9: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on new exported symbols; `internal/report/doc.go:1-2` and `internal/cli/root.go:8-10` list recurring; PRD change 1 at `docs/initial-prd.md:129` (delete `` `v_recurring` (detected series), ``, add the ruled sentence verbatim)

### Verify
- [ ] Step 10: full verification + `spec-check.py phase2e-recurring-anomalies` → tick SCENARIO-01, and 02/03/04 each "delivered by SCENARIO-01" with its folded test last on the line

## Handoff

**Binding decisions:**
- One port method `Store.Charges(ctx, store.ChargeParams{Through})` → `store.Charges{Rows []Charge, Transactions TransactionRange}`, no window and no account ids; `Transactions` always filled (whole store). Anomalies (S14) and the empty window (S11, S20) read this — reopening it reopens three fakes and the adapter.
- `store.Charge` carries TransactionID, SourceID, Date, Account (`store.Account` with ID/Name/Currency/Closed/Active for `accountLabel`), PayeeID/Payee `*string`, Currency, Amount (cents), Category `*ChargeCategory{ID, Path}` (set only when every expense row has the same non-NULL category), ExpenseSplits (count of `v_spending` rows). S14/S17: category baseline when Category != nil; cell `(split)` when ExpenseSplits > 1, else `(uncategorized)` when Category is nil. A single kind enum cannot express two same-category splits.
- The group key is tagged PayeeKey or payee_id, so the two never merge. S07's `payee_key: null` reads the tag.
- Through = `DefaultWindow(req.Now).Until`, the local civil date. The adapter computes no date.
- Cadence constants live in `report/recurring.go`; the group key in `report/charges.go` is shared with anomalies, which adds its median there.
- `reportFlags.bind` takes per-command help; `renderTable` takes per-column alignment and trims trailing spaces.

**Left unbuilt:**
- `renderRecurringJSON`, `recurringDocument`, `Series.PriceChanges`, `Series.Payees`/`Accounts`, the steady gate → S07. Until then `recurring --json` prints text: `newRecurringCommand` takes no `jsonOut` and passes `false` to `emitReport`; S07 adds the parameter.
- `RecurringRequest.Accounts`, the `namedAccounts` call, `recurringWarnings` (W2/W3, E1/E2) → S11. `--account` is bound but ignored until then.
- `Series.New`, the `, new` suffix and the ended-after bound pins (bound and bound+1 per cadence) → S05. Every sort tier, `PerYear` nil and Total skipping ended are built and pinned here.

**Traps:**
- `v_spending` keeps future-dated rows (spend and cashflow count them). `Charges` must filter on Through in SQL.
- The spec's text sample is internally inconsistent: the header implies Every is 6 wide, but the Total row is 48 chars. Pin widths computed from the padding rule, not the sample bytes.
- The `fakeStore` methods have value receivers, so counters need pointer fields (`accountsReads` precedent).
