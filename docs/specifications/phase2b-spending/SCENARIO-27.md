---
id: SCENARIO-27
status: done
---

# SCENARIO-27: spending leaves out accounts that use Quicken's linked account tracking (absorbs 28, 29)

Cadence: code-first (a Gate rule change, not a `Failure:` finding; no write-safety or atomicity item touched)
Acceptance test: `cmd/quarry/run_cashflow_linked_test.go` `Test_run_cashflow_leaves_out_accounts_that_use_linked_account_tracking`
Acceptance test (SCENARIO-28, folded): `cmd/quarry/run_spend_account_test.go` `Test_run_spend_warns_that_a_named_linked_tracking_account_is_left_out`
Acceptance test (SCENARIO-29, folded): `cmd/quarry/run_accounts_test.go` `Test_run_accounts_all_marks_accounts_that_use_linked_account_tracking`
Narrow loop: `go test ./internal/importer/ ./internal/store/... ./internal/cli/ -run 'linked|Linked|reported|Reported|left_out|cash_flow|accountStatus|renderAccountsJSON|help' && go test ./cmd/quarry/ -run 'linked|Test_run_accounts'`
Mutation checks: (each reddens the named test, per `proof.md`)
- drop `linked_tracking` from the shared "reported" SQL fragment → the acceptance test and the linked case in `Test_cash_flow_leaves_out_what_quicken_reports_leave_out` (`views_test.go`)
- `transactionRangeQuery` (`spending.go:51-58`) not using the fragment → `Test_spending_ranges_over_the_reported_accounts_it_is_named_for`, its cashflow twin, and `Test_run_spend_ranges_a_linked_and_a_reported_named_account_over_the_reported_one`
- W2 emitted for an account both not in reports and linked (W3 branch not first) → the 28 acceptance test and `Test_spend_warns_w3_only_for_an_account_both_linked_and_not_in_reports`
- `Account.LeftOutOfReports` ignoring `LinkedTracking` → `Test_spend_says_nothing_of_an_empty_window_when_every_named_account_is_linked` (and its cashflow twin)
- importer polarity/coalesce (`= 0` read as on, NULL as on) → `Test_import_marks_an_account_that_uses_linked_account_tracking`
Runs: A (1-4) | B1 (5-6) | B2 (7-8) | V (9-10)
Size: OWNS A RUN — 4 batches, 1 feature package (importer) plus store/duckstore/cli; absorbs 28, 29

Survey (step-5 grep): every reader of `a.in_reports`/`NotInReports` in production is `schema.go:146`, `spending.go:57`, `accounts.go:14/36-46`, `duckstore.go:470`, `cli/empty_window.go:15,35`, `render_accounts.go:23,66`, `json_accounts.go:36`, `importer/accounts.go:43,61-95`. `transactionRangeQuery` is shared by spend (`spending.go:178`) and cashflow (`cashflow.go:79`) — one fix covers both. `report/accounts.go:94,100` returns `a.Account` whole, so `LinkedTracking` reaches `Spending.Accounts` once the Accounts read fills it. No port signature changes: no new `Store`/fake edits.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_cashflow_linked_test.go` (new) `Test_run_cashflow_leaves_out_accounts_that_use_linked_account_tracking` — `replaceStore` + `cashFlowRows` (shape of `run_cashflow_invariant_test.go:28-75`): linked account with an expense and an uncategorized deposit, unlinked account with spending; run spend and cashflow; cashflow Income/Spent hold only the unlinked rows and `totalsColumn` spend Total == cashflow Spent
- [x] Step 2: `cmd/quarry/run_spend_account_test.go:54-73` `Test_run_spend_warns_that_a_named_linked_tracking_account_is_left_out` — account "Netskope 401(k)" linked + not in reports; stdout caption and header only, stderr exactly the W3 line (spec Surface & Copy, W3), exit 0
- [x] Step 3: `cmd/quarry/run_accounts_test.go:175-206` `Test_run_accounts_all_marks_accounts_that_use_linked_account_tracking` + `syncLinkedTrackingFixture` (sync path, v9fixture `SimpleInvesting`): open linked, inactive linked, closed + not-in-reports + linked; Status `linked tracking`, `inactive, linked tracking`, `closed, not in reports, linked tracking`
- [x] Step 4: `store.go:10-22` `Account.LinkedTracking` and `v9fixture/builder.go:35-48` `AccountRow.SimpleInvesting *int64` — fields only so steps 1-3 compile; all three fail at their assertions

### Build
- [x] Step 5: **Import the flag** — `importer/accounts.go:40-47,61-63,93-96` (`COALESCE(a.ZSIMPLEINVESTING, 0) <> 0`, scan, `LinkedTracking` set, not inverted); `v9fixture/builder.go:301-303` INSERT `ZSIMPLEINVESTING` via `nullableInt`; `schema.go:13-23` `linked_tracking BOOLEAN NOT NULL` LAST after `in_reports`; `duckstore.go:467-473` `accountRows` appends it last. Tests: `importer/accounts_test.go:92` `Test_import_marks_an_account_that_uses_linked_account_tracking` (`SimpleInvesting` 1/0/NULL → true/false/false); `duckstore_test.go:105-128` `Test_replace_stores_linked_tracking_per_account` (round trip, beside the `in_reports` asserts)
- [x] Step 6: **One "reported" definition** — `schema.go:131-152` and `spending.go:49-58`: one SQL fragment const (`a.in_reports AND NOT a.linked_tracking`, alias `a`; a const so `cashFlowViewDDL` can concatenate it) used by the view predicate and `transactionRangeQuery`; COMMENT ON VIEW text = spec "Changes to 2a surfaces" last bullet. Tests: linked-account case in `views_test.go:106-155` (expense + uncategorized deposit both absent; `v_spending` too); `views_test.go:307-313` pin; `spending_range_test.go:57-85` `Test_spending_ranges_over_the_reported_accounts_it_is_named_for` (name a linked account with far-outside dates plus a reported account; and an all-linked → zero range) + twin in `cashflow_test.go:308-321`; `spending_account_test.go:136-147` counts nothing for a named linked account; add `acctLinked` to the duckstore fixture consts
- [x] Step 7: **Name a linked account, W3** — `duckstore/accounts.go:12-18,36-47` Accounts read carries `LinkedTracking` (`a.linked_tracking`; `v_account_balances` NOT widened) + test in `accounts_test.go`; `store.go:19-22` `Account.LeftOutOfReports() bool` (`NotInReports || LinkedTracking`); `cli/empty_window.go:10-43` `leftOutWarnings` W3 branch first (W3 wins over W2), `linkedTrackingWarning(a, command)` with the spec W3 copy, `appendEmptyWindowWarning` counts `LeftOutOfReports()`. Tests in `spend_account_test.go:98-116` (argv-order W2/W3 interleave; both-flags → W3 only), `spend_empty_test.go:84-110` (every named left out incl. linked → no E; some left out → W3 then E1a), cashflow twins in `cashflow_test.go` (cli), cmd `Test_run_spend_ranges_a_linked_and_a_reported_named_account_over_the_reported_one`
- [x] Step 8: **cli copy** — `render_accounts.go:23,55-70` `accountStatus` third part `linked tracking` (order state, `not in reports`, `linked tracking`); `json_accounts.go:16-36` `LinkedTracking bool json:"linked_tracking"` directly after `in_reports`; `spend.go:20-31` and `cashflow.go:43-53` Long, copied from the spec blocks (spec `### spend` Long, `### cashflow` Long), NOT edited in place — wrapping differs. Tests: `render_accounts_internal_test.go:72-90` rows for every spec Status example; `json_accounts_internal_test.go:14-80`; `run_accounts_json_test.go` `Test_run_accounts_json_carries_linked_tracking_per_account`; pins `report_help_test.go:11-31,62-79`

### Sweep
- [x] Step 9: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; exact-document bumps `json_accounts_internal_test.go:39-72`, `run_accounts_json_test.go:27,71-93` (struct + expected JSON); `accountStatus` signature in `Test_accountStatus`; doc comments on the fragment const, `LeftOutOfReports`, `linkedTrackingWarning`

### Verify
- [x] Step 10: full verification + `spec-check.py phase2b-spending`; tick SCENARIO-27, and 28/29 as `— delivered by SCENARIO-27 — <its acceptance test>`; rewrite STATE.md; set `status: done`

## Handoff

**Binding decisions:**
- "Reported" has one SQL owner (the duckstore fragment: view predicate + range query) and one Go owner (`store.Account.LeftOutOfReports`) — P2b-6; a third copy of the rule is a finding.
- `accounts.linked_tracking` is LAST in `accounts`, after `in_reports` — the build Appender is positional (`accountRows`). `FormatVersion` stays 3 (unshipped); S05 compares the symbol.
- W3 wins over W2: an account both not in reports and linked gets W3 only, in argv order with W2 lines, before W1 and E.
- `v_account_balances` is not widened (STATE, SCENARIO-26); the Accounts read joins `accounts a` for both flags.
- The importer flag is not inverted (`LinkedTracking` true = Quicken flag on), unlike `NotInReports`.

**Left unbuilt:**
- Re-running the Gate against the real Quicken file (needs `quarry sync` on the dev store first) — orchestrator/user, manual.
- Investment-action rows in the 401(k) Gate diff — not caused by this rule; unowned.

**Traps:**
- A format-3 store synced before this lacks `linked_tracking`: `checkFormat` only reads `format_version`, so reads fail until `quarry sync` (P2b-2a accepts it; untested).
- `spendingQueryFrom` / `cashFlowQuery` filter through views, so only `transactionRangeQuery` needs the fragment; do not add it to the account-id filter.
- A linked account named on `--account` still resolves (closed and not-in-reports accounts do); only the counting leaves it out.
- `store.Account` literals in tests rely on the zero value = reported; `spendRows` needs no change.

## Phase report

Run V done (steps 9-10 ticked, `status: done`). `go build ./...` ok; covered full suite `go test rc=0`; `uncovered-diff.py` 0 uncovered; `-race` on importer, store, duckstore, cli, report, cmd/quarry ok; `golangci-lint` 0 issues; `spec-check.py phase2b-spending` OK. SCENARIO-27 ticked, 28 and 29 ticked as delivered by 27. STATE.md rewritten (reported-rule owner, W3, format-3 re-sync trap). Runs A-B2 files and mutations: see git log 5444906, 41dbcac, ab98214.
