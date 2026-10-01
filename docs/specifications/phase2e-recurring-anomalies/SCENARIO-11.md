---
id: SCENARIO-11
status: open
---

# SCENARIO-11: recurring --account lists only series charged in that account (folds 12 empty window, 13 refusal outline)

Cadence: code-first (no write-safety guard, no atomic adapter; the read adapter change is a scoped SELECT)
Acceptance test: `cmd/quarry/run_recurring_account_test.go` `Test_run_recurring_lists_only_the_series_charged_in_the_named_account`
Acceptance test (SCENARIO-12, folded): `cmd/quarry/run_recurring_empty_test.go` `Test_run_recurring_says_when_no_series_runs_in_the_period`
Acceptance test (SCENARIO-13, folded): `cmd/quarry/run_recurring_refusals_test.go` `Test_run_recurring_refuses_usage_and_account_problems`
Narrow loop: `go test ./internal/report/ -run 'Recurring' && go test ./internal/cli/ -run 'Recurring' && go test ./internal/store/duckstore/ -run 'Charges' && go test ./cmd/quarry/ -run 'Recurring|ReadCommands'`
Mutation checks: any-run-charge account match in `(*Server).Recurring` → `Test_recurring_lists_a_series_when_any_charge_of_its_run_is_in_a_named_account`
Runs: A (1) | B1 (2) | B2 (3-4) | V (5-6)
Size: OWNS A RUN — 3 Build batches, 1 feature package (`report`) plus `cli`, `cmd/quarry` and one adapter parameter

## Implementation Plan

Existing, no change: `noArgs` (U8), `flags.window` (S1/S2/S2d/S3 before `openReport`), `openReport`, `readRefusal` (R1/I1), `namedAccounts` (`accounts.go:71`, S5/S6, dedupe), `leftOutWarnings`, `appendEmptyWindowWarning` (`empty_window.go`), `emitReport` warnings plumbing, `windowCaption(…, accounts)` (`render_table.go:52`), `accountFilterDocuments`.

### Acceptance (red)
- [ ] Step 1: all three acceptance tests, each seen red at its assertion (SCENARIO-07 precedent: folded tests are written red in run A). All compile today.
  - `cmd/quarry/run_recurring_account_test.go` `Test_run_recurring_lists_only_the_series_charged_in_the_named_account` — real store via `chargeRows` (`run_helpers_test.go:184`): a monthly series on Visa and one on Chequing, `recurring --since 2000 --account Visa`; caption `… in Visa`, one row, Total over that row only. Red: the flag is ignored.
  - `cmd/quarry/run_recurring_empty_test.go` `Test_run_recurring_says_when_no_series_runs_in_the_period` — E1 over `run`: caption plus header on stdout, `no recurring charges from … to …; the store's transactions run …` on stderr, exit 0. Red: warnings are `[]string{}`.
  - `cmd/quarry/run_recurring_refusals_test.go` `Test_run_recurring_refuses_usage_and_account_problems` — the outline's four rows (U8, S1, `--account Nope`, no store); `--account Nope` exits 0 today.

### Build
- [ ] Step 2 (B1, report + port param): one batch, `go test ./internal/report/ ./internal/store/duckstore/ -run 'Recurring|Charges'`
  - `internal/store/store.go:523-525` `ChargeParams.AccountIDs` — scopes only `Charges.Transactions` (doc: rows stay unfiltered); `duckstore/charges.go:35-56` passes `accountFilter(params.AccountIDs)` to `transactionRange` (as `spending.go:109`). Adapter tests in `charges_test.go` after `:269`: span over named reported accounts only (a `NotInReports` and a linked account excluded; control: none named = whole store); zero span when the named accounts have none.
  - `internal/report/recurring.go:117` `RecurringRequest.Accounts []string`; `:164` `Recurring` gains `Accounts []store.Account`, `Transactions store.TransactionRange` and `Empty() bool` (`len(Series)==0`); `:175-199` `Server.Recurring`: `namedAccounts(ctx, recurringCommand, req.Accounts)` first (refusal means no `Charges` read), `ChargeParams{Through, AccountIDs}`, keep series with `Series.Accounts` holding a named id after the gate and `runsDuring`, before `yearlyTotals`.
  - Tests (new `internal/report/recurring_account_test.go`, fake accounts via `accountsOf`): `Test_recurring_lists_a_series_when_any_charge_of_its_run_is_in_a_named_account` (named account only on a middle charge; amount, first/last from the whole run; control: other account named, not listed); match by id and by name case-insensitively; two named accounts list the union; Totals count only listed series; a gate-dropped series stays out even when named; accounts echoed in argv order without repeats; passes ids to `Charges` and reads once (`chargesReads`); unknown, ambiguous and empty-argument refusals with no `Charges` read; accounts-read store fault is the R1 refusal naming the store; cancelled ctx gives `recurring interrupted` on the accounts read and on the `Charges` read (`spending_account_test.go:151-165` twins); `Charges` read fault is a refusal; `Transactions` carried through.
- [ ] Step 3 (B2, cli): one batch, `go test ./internal/cli/ -run Recurring`
  - `internal/cli/recurring.go:41-58` pass `Accounts: flags.accounts`; new `recurringWarnings(rec report.Recurring) []string` beside it (twin `cashflow.go:122-129`): `leftOutWarnings(rec.Accounts, recurringCommand)` then, when `rec.Empty()`, `appendEmptyWindowWarning(…, "recurring charges", rec.Accounts, rec.Window, rec.Transactions)`; the same `warnings` slice goes to `emitReport` (stderr) and `renderRecurringJSON`. Export or reuse `report.recurringCommand` as cashflow does (`cashFlowCommand` precedent: a cli const equal to the word).
  - `json_recurring.go:80` `accountFilterDocuments(r.Accounts)`; `render_recurring.go:80` `windowCaption("Recurring charges", r.Window, r.Accounts)`.
  - E1 turns empty-result tests red as expectation updates, not regressions (grep of `Empty(t, stderr` hits): `cmd/quarry/run_recurring_price_test.go:50` (gate-dropped bill, empty result) and `internal/cli/recurring_test.go:37,60,94` (fake store, empty `charges`) — assert the E1 line instead; `cmd/quarry/run_recurring_{test,json_test,state_test}.go` stderr checks sit on non-empty results, so should stay green (verify).
  - Tests (`recurring_test.go`, `render_recurring_internal_test.go`, `json_recurring_internal_test.go`, `fakeReportStore`): caption names the accounts (escaped, joined by `, `); `account_filter` holds id and name (`[]` when none); W2 and W3 each once, argv order, in stderr and in `warnings[]` unprefixed; E1 / E2 (zero span `; the store has no transactions`) / E1a / E2a (`in the named accounts; they have no transactions`) in stderr and `warnings[]`, W lines before E; every named account left out gives W only (no E, even zero span); some left out gives W then E1a; non-empty result gives no E; the empty text is caption plus header only; second `Charges` call returns different data and cannot reach the output (`chargesReads == 1`).
- [ ] Step 4 (B2, pins beyond the acceptance tests — green on arrival except where noted; a red pin is a real finding): `go test ./cmd/quarry/ -run 'Recurring|ReadCommands'`
  - `run_recurring_empty_test.go` rows: E2 (zero transactions), E1a/E2a (`in the named accounts; their transactions run …` / `they have no transactions`), W2 and W3 alone (every named account left out: W only, no E), W then E1a; each asserts stdout, stderr and the `--json` `warnings[]` (same text, no prefix). `run_spend_empty_test.go:15` and `run_cashflow_refusals_test.go:14` are the table shape.
  - `run_recurring_refusals_test.go` rows: S2 (2b spec :194), S2d (:195), S3 (:196), S5 unknown and S6 ambiguous account (:198-199, exit 1), copy has no command word; one `--json` refusal with empty stdout.
  - Existing tables gain a recurring row: `run_read_usage_test.go:41` (U8), `run_read_refusals_test.go:56-63` (R1, home stays empty; second row adds `--account Visa`, the accounts read opens first) and `:165-170` (I1 `quarry: recurring interrupted`).

### Sweep
- [ ] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `ChargeParams.AccountIDs`, `RecurringRequest.Accounts`, `Recurring.Accounts`/`Transactions`/`Empty`, `recurringWarnings`; update `go doc` for `store.Charges` ("span of every transaction in the store" becomes "or of the named reported accounts").

### Verify
- [ ] Step 6: full verification + `.claude/scripts/spec-check.py phase2e-recurring-anomalies`; tick SCENARIO-11 with its acceptance test and SCENARIO-12, 13 with "delivered by SCENARIO-11:" and their tests; rewrite STATE.md (drop the `--account` interim and the S11 "left unbuilt" line; add the `AccountIDs` decision below).

## Handoff

**Binding decisions**
- `store.ChargeParams.AccountIDs` scopes only `Charges.Transactions` (named reported accounts, as spend's E1a range), never `Rows` — E1a/E2a say "their transactions run …", which the whole-store span cannot give; the departure from STATE's "no account ids" keeps the single read (no second store call) and leaves `Charges` signature, three fakes and the `run.go:29` guard untouched. S20 (anomalies `--account`) reuses it.
- `--account` matches `Series.Accounts` (run's distinct accounts), after the steady gate and window filter, before `yearlyTotals`: Totals sum listed series only.
- `Recurring.Empty()` is `len(Series)==0` after the account filter; the empty warning therefore fires for "series exist but none in the named accounts".
- Warnings are built once (`recurringWarnings`) and the same slice feeds stderr and `warnings[]`; pins assert both.

**Left unbuilt**
- `report.Server.Anomalies`, `anomalies` command, its `--account` / W2/W3 / `0 charges checked` footer / refusal pins — S14, S20-22.
- Reference check (`REFERENCE-CHECK.md`) — unchanged debt.

**Traps**
- Unknown/ambiguous account copy (S5/S6) carries no command word; only U8, R1, I1 and W2/W3 name `recurring`. I1 for the accounts read and the `Charges` read are two different code paths: pin both.
- `chargeRows` accounts need `CAD` currency and a name `Visa`; a `NotInReports` or `LinkedTracking` account has no charges in `v_spending`, so naming it yields W only and no series.
- Series match must use the run's accounts, not the latest charge's account — a card change mid-run keeps the series listed under either card.
