---
id: SCENARIO-17
status: done
---

# SCENARIO-17: spend says when the period holds nothing

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_spend_empty_test.go` `Test_run_spend_says_when_the_period_holds_nothing`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/report/ ./internal/cli/ ./cmd/quarry/ -run 'spend|spending|empty'`
Mutation checks: none (code-first)
Runs: L | V
Size: LIGHT — 3 steps, store + duckstore + report + cli

"Nothing in the window" = `len(Totals) == 0` (Totals keep zero-net currencies, so a net-zero currency prints `Total … 0.00` and no E line). Stdout needs no change: caption + header already print for an empty read.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_spend_empty_test.go` (new) — table over `replaceStore(spendRows(...))`, argv `spend --since 2026-01 --until 2026-02`: E1 (splits 2003-01-04 and 2025-12-31) and E2 (no splits); exact stdout (caption `2026-01-01 to 2026-02-28 in all accounts`, blank line, header) and stderr; no new symbols, so red is the stderr assertion

### Build
- [x] Step 2: `store.TransactionRange{First, Last}` (zero = none) as `store.Spending.Transactions`; `duckstore.(*Store).Spending` reads `min/max(date)` over `transactions` (whole store; with `AccountIDs`, joined to `accounts.in_reports` so named excluded accounts do not widen it); `report.Spending.Transactions` copies it. Tests: `spending_range_test.go` (income-only earliest txn, excluded named account older than the in-report one, none → zero, fault on the new statement)
- [x] Step 3: `internal/cli/empty_window.go` phrase builder (subject, window, named?, range) → E1/E2/E1a/E2a, reusable by cashflow; `spendWarnings` appends it after W1 unless `len(Accounts) > 0` and all `NotInReports`. Tests in `internal/cli/spend_empty_test.go`: four phrases, all-excluded suppression with zero transactions, W2 → E order in stderr and `warnings[]`, net-zero Totals → no E line

## Handoff

- `store.Spending.Transactions` is read only when the window has no Totals (a per-read extra statement would have changed ten exact-struct duckstore asserts); scope = every transaction (no `AccountIDs`) or the in-report subset of `AccountIDs` (join on `accounts.in_reports` inside the range query only, so the STATE.md empty-list-means-all trap cannot hit: an all-excluded list yields the zero range).
- S20 reuses `emptyWindowWarning` (`internal/cli/empty_window.go`) with subject `income or spending` and its own emptiness rule; `report.Spending.Empty()` is `len(Totals) == 0`.

## Phase report

Run V done: scenario complete, `status: done`.
- `go build ./...` green; `golangci-lint run ./...` `0 issues`; covered full suite green (`go test -count=1 -coverpkg=./... ./...`); `go test -race` on duckstore, report, cli green.
- `uncovered-diff.py --profile <cover> ff97ec0`: 0 uncovered added lines in 0 runs.
- `test-stats.py --base ff97ec0 --changed`: cmd/quarry 116 (+1), internal/cli 98 (+5), internal/report 54 (+1), internal/store/duckstore 154 (+7); TOTAL 422 (+14).
- Ticked SCENARIO-17 in `specification.md` with `Test_run_spend_says_when_the_period_holds_nothing`; `spec-check.py phase2b-spending` OK.
- `STATE.md` rewritten: E1/E1a/E2/E2a removed from Left unbuilt; new binding entry for the empty-window design.
- Nothing left for a later run; no production code changed in V.
