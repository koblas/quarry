---
id: SCENARIO-26
status: done
---

# SCENARIO-26: accounts marks accounts Quicken leaves out of reports (absorbs SCENARIO-25)

Size: LIGHT — 3 steps, internal/store/duckstore + internal/cli
Cadence: code-first (no write-safety, atomicity or bug-fix item touched)
Runs: L | V
Acceptance test: `cmd/quarry/run_accounts_test.go` `Test_run_accounts_all_marks_accounts_left_out_of_reports`
Acceptance test (SCENARIO-25, folded): `cmd/quarry/run_accounts_test.go` `Test_run_accounts_says_how_to_list_them_when_every_account_is_closed`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/cli/ -run 'ccounts|Status|ccountStatus' && go test ./cmd/quarry/ -run 'Test_run_accounts|Test_run_sql'`
Mutation checks: none (no mandatory test-first item)

## Implementation Plan

### Acceptance (red)
- [x] `run_accounts_test.go`: new `Test_run_accounts_all_marks_accounts_left_out_of_reports` (open/inactive/closed, each `UsedInReports` 0; plus an in-reports control row); update the all-closed stderr assertions to `quarry: warning: ` (`run_accounts_test.go`, `run_accounts_json_test.go`) — red until steps 1-3.

### Build
- [x] 1. duckstore `Accounts`: join `accounts a ON a.id = v.id` for `NOT a.in_reports`, scan into `Account.NotInReports` (`accounts.go:12-49`). Join, not a `v_account_balances` column: the view is a queryable contract (`quarry sql`) that 2b does not need to widen, and `Accounts` already reads the store, so one extra join on the primary key is cheaper than a view change plus `minimalRows`/describe churn. Test: `accounts_test.go` `Test_accounts_reads_each_accounts_balance` gains a not-in-reports account.
- [x] 2. cli: `accountStatus(closed, active, notInReports)` joins `closed`/`inactive` with `not in reports` by `, ` (`render_accounts.go:57`); `accountRowDocument.InReports bool` `json:"in_reports"` after `active` (`json_accounts.go:23`). Tests: `render_accounts_internal_test.go` `Test_accountStatus` cases, `json_accounts_internal_test.go`, cmd `--json` document test gains `in_reports`.
- [x] 3. `accounts.go:41` prefix `quarry: warning: `; `sql.go` Long blank lines around the indented example; update `accounts_test.go:58-60`-adjacent stderr asserts and the sql help test.

### Sweep
- [x] `go build ./... && golangci-lint run ./...` zero; doc comments.

## Handoff
- `store.Account.NotInReports` now survives the read path: `Accounts` scans it from `accounts.in_reports` by join, `v_account_balances` unchanged. SCENARIO-06's view reads `a.in_reports` itself.

## Phase report
Run V done. `go build` ok, `golangci-lint run ./...` = 0 issues, covered full suite exit 0, `go test -race` duckstore+cli ok, `uncovered-diff.py --profile <cover> b2d4844` = 0 uncovered added lines, `spec-check.py phase2b-spending` OK.
- test-stats (base b2d4844, --changed): cmd/quarry 101 (+2) tempdir 95 (+2) disk 80 (+2); internal/cli 59 (+0); internal/store/duckstore 89 (+0); TOTAL 249 (+2), 154 (+2), 136 (+2).
- Ticked SCENARIO-25 (delivered by SCENARIO-26) and SCENARIO-26 in specification.md; STATE.md rewritten (`in_reports` Left-unbuilt line and sql.go Long debt removed; warning-prefix and `Accounts` join decisions recorded).
