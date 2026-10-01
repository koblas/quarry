---
id: SCENARIO-20
status: done
---

# SCENARIO-20: anomalies --account lists and counts only the named accounts, history from every account (folds 21 empty window, 22 refusal outline)

Cadence: code-first (no write-safety guard, no atomic adapter, no bug fix; the port is unchanged)
Acceptance test: `cmd/quarry/run_anomalies_account_test.go` `Test_run_anomalies_account_judges_the_named_accounts_charge_against_history_from_every_account`
Acceptance test (SCENARIO-21, folded): `cmd/quarry/run_anomalies_empty_test.go` `Test_run_anomalies_says_when_no_charge_falls_in_the_window`
Acceptance test (SCENARIO-22, folded): `cmd/quarry/run_anomalies_refusals_test.go` `Test_run_anomalies_refuses_usage_and_store_problems`
Narrow loop: `go test ./internal/report/ -run 'Anomal|anomal' && go test ./internal/cli/ -run 'Anomal|anomal|Window' && go test ./cmd/quarry/ -run 'anomal|Anomal|ReadCommands|read_commands'`
Mutation checks: account scope applies to tally only, never to `groupCharges` history → `Test_anomalies_keep_charges_of_other_accounts_in_the_payees_history`
Runs: A (1) | B1 (2) | B2 (3) | B3 (4) | V (5-6)
Size: OWNS A RUN — 3 Build batches, 1 feature package (`report`) plus `cli` and `cmd/quarry`; absorbs 21 and 22 (pure coverage of the same warnings/refusal path)

Existing, no change (S11 precedent): `namedAccounts` (`accounts.go:71`, S5/S6, dedupe, empty arg), `leftOutWarnings` and `appendEmptyWindowWarning` (`empty_window.go:11,42`), `accountFilterDocuments` (`json_spend.go:26`), `windowCaption(…, accounts)` (`render_table.go:52`), `noArgs` (U8), `flags.window` (S1/S2/S2d/S3 before `openReport`), `readRefusal` (R1/I1), `emitReport` warnings plumbing, `store.ChargeParams.AccountIDs` (scopes `Transactions` only; `Rows` unfiltered). No port, adapter or fake change. Triage-style survey of a new port: none, nothing replaced.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: three acceptance tests over `run`, each seen red at its assertion; all compile today, no stubs. Fixtures via `chargeRows`/`groceryCharge` (`run_helpers_test.go:184`), twins `run_recurring_account_test.go:27`, `run_recurring_empty_test.go:17`, `run_recurring_refusals_test.go:25`; table helper `anomaliesTable` (`run_anomalies_test.go:21`).
  - `run_anomalies_account_test.go`: payee's three earlier charges last year on Chequing, a large charge this year on Visa, plus a large charge this year on Chequing as control; `anomalies --account Visa` lists only the Visa row `payee, 3 earlier`, caption `… in Visa`, footer `1 charge checked`, stderr empty. Red: flag ignored (control row listed, caption `all accounts`).
  - `run_anomalies_empty_test.go`: E1 over `run`: caption, header, footer `0 charges checked` on stdout; stderr `quarry: warning: no unusually large charges from 2026-01-01 to 2026-09-29; the store's transactions run <first> to <last>`; exit 0. Red: stderr empty.
  - `run_anomalies_refusals_test.go`: outline's four rows (U8 `quarry: anomalies takes no arguments` 2; S1 bad date 2; `--account Nope` unknown-account line 1; no store R1 line naming anomalies 1), stdout empty. Red: `--account Nope` exits 0.

### Build
- [x] Step 2 (B1, `report`): `go test ./internal/report/ -run 'Anomal|anomal'`
  - `anomalies.go:43` `AnomaliesRequest` (field already there, doc says honoured), `:63` `Anomalies` gains `Accounts []store.Account` (the request's named accounts, argv order, no repeats); `:75-100` `Server.Anomalies`: `namedAccounts(ctx, anomaliesCommand, req.Accounts)` first (refusal means no `Charges` read), `ChargeParams{Through, AccountIDs}`; a charge is tallied (`:85`, `:95`) only when in window AND (no ids named or `c.Account.ID` among them) — `groupCharges`, `categories.before` and `groupByCategory` keep every row (history from every account, both baselines). Update doc comments on `AnomaliesRequest`, `Anomalies`, `Server.Anomalies`.
  - Tests (new `internal/report/anomalies_account_test.go`, `accountsOf`, fakeStore counters as `recurring_account_test.go:156`): `Test_anomalies_keep_charges_of_other_accounts_in_the_payees_history` (payee baseline; control: no account named, same listing); same for the category baseline (10 earlier in other accounts); `checked` and `not_judged` count only named accounts' in-window charges (control: unnamed counts all); match by id and by name case-insensitively; two named accounts list the union; accounts echoed argv order without repeats, none when none named; passes ids to the one `Charges` read (`chargesReads == 1`, `gotCharges.AccountIDs`); `Transactions` carried through; unknown, ambiguous and empty-argument refusals with zero `Charges` reads; accounts-read open fault is the R1 refusal naming anomalies; cancelled ctx gives `anomalies interrupted` on the accounts read and on the `Charges` read after accounts were named (I1; `recurring_account_test.go:247,259` twins); a closed named account matches. Debt fold: `Test_anomalies_read_the_charges_through_the_local_date_when_the_utc_date_is_later` with `windowNow` (twin `recurring_window_test.go:31`).
- [x] Step 3 (B2, `cli`): `go test ./internal/cli/ -run 'Anomal|anomal|Window'`
  - `anomalies.go:52,57` keep `Accounts: flags.accounts`; replace `warnings := []string{}` with new `anomaliesWarnings(a report.Anomalies) []string` beside it (twin `recurring.go:75`): `leftOutWarnings(a.Accounts, anomaliesCommand)` then, only when `a.Checked == 0` (ruled; not `Recurring.Empty()`'s shape), `appendEmptyWindowWarning(…, "unusually large charges", a.Accounts, a.Window, a.Transactions)`. Same slice feeds `emitReport` and `renderAnomaliesJSON`.
  - `render_anomalies.go:47` `windowCaption("Unusually large charges", a.Window, a.Accounts)`; `json_anomalies.go:46` `accountFilterDocuments(a.Accounts)`; drop the interim doc lines about `account_filter []`.
  - Tests (`anomalies_test.go`, `render_anomalies_internal_test.go`, `anomalies_json_test.go`, `fakeReportStore`): flip `Test_anomalies_without_charges_prints_the_empty_table_and_footer` (`:122`) to expect E1 on stderr (zero span: `; the store has no transactions` as E2); `Test_anomalies_charges_exist_but_none_unusual_prints_no_warning` (stderr empty, `warnings` `[]`, footer `N charges checked`); E1a / E2a (`in the named accounts; their transactions run …` / `they have no transactions`); W2 and W3 each once, argv order, in stderr and `warnings[]` unprefixed, W before E; every named account left out gives W only (no E); some left out gives W then E1a; E not added when `checked` > 0 but nothing listed; caption names accounts escaped, joined `, `; `account_filter` holds id and name (`[]` when none); `warnings[]` equals stderr lines without prefix; second `Charges` call returns different data and cannot reach output.
  - Debt folds: `report_help_test.go:170-172` anchor the three flag-help regexps to line end (`$` with `(?m)`) so a longer help string no longer passes; `render_findings.go:163-165` `categoryCell` doc to one line.
- [x] Step 4 (B3, `cmd/quarry` pins beyond the acceptance tests; green on arrival, a red pin is a finding): `go test ./cmd/quarry/ -run 'anomal|Anomal|read_commands'`
  - `run_anomalies_empty_test.go` rows (stdout, stderr and `--json` `warnings[]` each; twin `run_recurring_empty_test.go:51`): E2 (zero transactions), E1a/E2a, W2 and W3 alone (every named account left out), W then E1a; "charges exist, none unusual" stderr empty and `warnings` `[]`.
  - `run_anomalies_refusals_test.go` rows: S2, S2d, S3, S5 unknown, S6 ambiguous (exit 1), copy has no command word; `--json` unknown-account and `--json` bad-date with stdout empty.
  - `run_anomalies_account_test.go`: closed-account row (`--account "Old Card"` lists its large charge, label via `accountLabel`; twin `run_recurring_account_test.go:45`); `--json --account Visa` `account_filter` holds id and name and `checked`/`not_judged` count Visa only.
  - Existing tables gain an anomalies row: `run_read_usage_test.go:42` (U8), `run_read_refusals_test.go:55-56` (R1, plain and `--account Visa`) and `:171` (I1 `quarry: anomalies interrupted`).

### Sweep
- [x] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `anomaliesWarnings`, `Anomalies.Accounts`; re-run `go doc ./internal/report Anomalies`.

### Verify
- [x] Step 6: full verification + `.claude/scripts/spec-check.py phase2e-recurring-anomalies`; tick SCENARIO-20 with its acceptance test and SCENARIO-21, 22 with "delivered by SCENARIO-20:" and their tests; rewrite STATE.md (drop S20 "Left unbuilt" and the `--account` interim, retire the S14 empty-test debt and flag-regexp debt).

## Handoff

**Binding decisions**
- `--account` scopes only the tally (listing, `checked`, `not_judged`); `groupCharges`, `groupByCategory` and `categories.before` read every row — history from every account for both baselines. The one `Charges` read carries `AccountIDs` only for `Transactions` (E1a/E2a "their transactions").
- Empty-window warning is `a.Checked == 0` alone (ruled), subject `unusually large charges`; "charges exist, none unusual" gives no warning. W lines first, then E; every named account left out gives W only.
- `anomaliesWarnings` builds the one slice for stderr and `warnings[]`.

**Left unbuilt**
- Reference check on the real Quicken file (`REFERENCE-CHECK.md`) — after S22 delivered here, scheduled before the gate round; unowned until the orchestrator runs it.

**Traps**
- A named account left out of reports has no `Rows`, so `checked` is 0 for it: expect W only, never E (the `allLeftOut` rule in `appendEmptyWindowWarning`).
- Scope on `c.Account.ID` of the charge, not `Series`-style run accounts; a closed account still matches.
- The cmd-level I1 row cancels before the store opens; the accounts-read and `Charges`-read I1 pins live in `internal/report` only.

## Phase report

Runs A, B1, B2, B3 done. Full covered suite rc=0, `uncovered-diff.py` 0 uncovered, `golangci-lint` 0 issues, `-race ./internal/cli/` ok. Counts vs 7cb2f1d: cmd/quarry 383 (+8), internal/cli 344 (+6), internal/report 224 (+14), total 951 (+28). All three acceptance tests green.

Production (B2): `internal/cli/anomalies.go` new `anomaliesWarnings(a)` (W2/W3 via `leftOutWarnings`, then `appendEmptyWindowWarning(..., "unusually large charges", ...)` only when `a.Checked == 0`) feeds `emitReport` and `renderAnomaliesJSON`; `render_anomalies.go` caption `windowCaption(..., a.Accounts)`; `json_anomalies.go` `accountFilterDocuments(a.Accounts)`. Debt folds: `report_help_test.go` three flag-help regexps anchored `(?m)...$`; `render_findings.go` `categoryCell` doc one line. No `report` change in B2/B3.

Tests: `internal/cli/anomalies_account_test.go` (new: caption + ids passed, caption escape, `account_filter`, warnings table 10 cases on stderr AND `warnings[]`, one `Charges` read); `anomalies_test.go` S14 empty test flipped to E1 on stderr and "charges exist, none unusual -> no warning" pinned; `anomalies_json_test.go` empty-arrays test now uses an ordinary charge (checked 1, `warnings` `[]`). `cmd/quarry`: empty (E2/E1a/E2a/W/W+E1a rows, text and `--json`, stderr and `warnings[]`; no-warning row), refusals (S5 ambiguous, `--json` unknown-account and bad-date with stdout empty, S2/S2d/S3 period table), account (closed-account row, `--json` `account_filter` + checked/not_judged Visa only), shared tables U8 / R1 (plain and `--account Visa`) / I1 rows for anomalies.

Green on arrival (code-first, production written before these tests); the acceptance tests had been red from A/B1. Mutations (restored, diff clean): `Checked == 0` -> `true` reddened the no-warning row and the json empty-arrays test; caption accounts -> nil reddened caption + escape tests; `account_filter` -> nil reddened the json account-filter test; drop `leftOutWarnings` reddened 3 warnings rows; drop accounts from `appendEmptyWindowWarning` reddened E1a/E2a rows; lengthening anomalies' `--account` help reddened the anchored regexp (`.../anomalies`).

Run V done: build ok, lint 0 issues, covered full suite rc=0, uncovered-diff 0, `-race` cli/report/cmd ok, spec-check OK, S20/21/22 ticked, STATE.md rewritten, status done.
