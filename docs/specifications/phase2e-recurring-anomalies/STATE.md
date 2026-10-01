# phase2e-recurring-anomalies — current state

Scenarios complete: SCENARIO-01..04 (02-04 folded into 01). Last updated by SCENARIO-01.

## Binding decisions
- One port method `report.Store.Charges(ctx, store.ChargeParams{Through})` returns `store.Charges{Rows []Charge, Transactions TransactionRange}`: no window, no account ids, `Transactions` always filled (whole store). Anomalies (S14) and the empty window (S11, S20) read this; reopening it reopens three fakes, the adapter and the `cmd/quarry/run.go:29` guard (SCENARIO-01)
- `store.Charge` carries TransactionID, SourceID, Date, Account (`store.Account`: ID/Name/Currency/Closed/Active, for `accountLabel`), PayeeID/Payee `*string`, Currency, Amount (cents), Category `*ChargeCategory{ID, Path}` (set only when every expense row shares one non-NULL category), ExpenseSplits (count of `v_spending` rows). S14/S17: category baseline when Category != nil; cell `(split)` when ExpenseSplits > 1, else `(uncategorized)` when Category is nil. One kind enum cannot express two same-category splits (SCENARIO-01)
- Detection lives in package `report` (no leaf package): `charges.go` holds the group key (tagged `keyKind` PayeeKey vs payee_id, so the two never merge; S07's `payee_key: null` reads the tag; S14 adds its median here); `recurring.go` holds the named cadence consts (`cadenceRules`, gap range / min charges / ended-after / per-year factor), `latestRun` walk and `Server.Recurring` (SCENARIO-01)
- Through = `DefaultWindow(req.Now).Until`, the local civil date, never `Window.Until`; the adapter computes no date. Duckstore filters `date <= Through` in SQL because `v_spending` keeps future-dated rows (SCENARIO-01)
- `Server.Recurring` makes one `Charges` read (pinned by `chargesReads`); a read fault is `readRefusal` naming recurring (SCENARIO-01)
- Interim until S07/S11: `newRecurringCommand(newReport, now)` has no `jsonOut`, so `recurring --json` prints text (`emitReport` gets `false`, JSON closure carries `// unreachable:`); `--account` is bound but ignored (SCENARIO-01)
- `reportFlags.bind(cmd, reportFlagHelp)` takes per-command flag help (spend/cashflow pass today's strings, byte-identical); `renderTable` takes per-column alignment and trims trailing spaces (SCENARIO-01)
- Recurring table column widths come from the padding rule (widest cell after `escapeCell`, two-space gaps), not from the spec sample's bytes; the sample header implies Every 6 wide but its Total row is 48 chars (SCENARIO-01)
- Read-command pins already include recurring: root Available Commands pin (`cmd/quarry/run_status_test.go`), `Test_run_read_commands_ignore_a_malformed_config` (fixture holds one active Costco monthly series so stderr stays empty once S11's E1 lands) (SCENARIO-01)

## Left unbuilt
- `renderRecurringJSON`, `recurringDocument`, `Series.PriceChanges`, `Series.Payees`/`Accounts`, the steady gate (P2e-7) — S07
- `RecurringRequest.Accounts`, the `namedAccounts` call, `recurringWarnings` (W2/W3, E1/E2), refusal outline, I1 — S11
- `Series.New`, the `, new` suffix, ended-after bound pins (bound and bound+1 per cadence) — S05/S06. Every sort tier, nil `PerYear` and Total skipping ended are already built and pinned
- `report.Server.Anomalies`, `anomalies` command, `anomalies.go`, anomalies pins in root help / never-load-config / PRD P2d-10 list — S14
- `docs/initial-prd.md` change 1 and `report/doc.go` / `cli/root.go` recurring mentions are done; anomalies mentions land with S14

## Traps
- `v_spending` keeps future-dated rows (spend and cashflow count them); any new `Charges`-style read must filter on Through in SQL (SCENARIO-01)
- `fakeStore` / `fakeReportStore` methods have value receivers, so counters need pointer fields (`accountsReads` precedent) (SCENARIO-01)
- A store fault through `Server.Recurring` is a `RefusalError` only for recognised kinds; the cli test asserts `ErrorIs`, exit 1 comes from `Execute` unwrapping `runtimeError` (SCENARIO-01)

## Open debts
- Reference check on the real Quicken file after SCENARIO-22, before the gate round (`REFERENCE-CHECK.md`) — unowned until the architect schedules it; dies unless run
- Checkpoint (5a) MINOR/NIT findings for SCENARIO-01, if any, are recorded here by the orchestrator
