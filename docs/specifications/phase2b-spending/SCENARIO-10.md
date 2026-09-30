---
id: SCENARIO-10
status: done
---

# SCENARIO-10: spend groups by payee

Size: LIGHT — 3 steps, store + duckstore + report + cli
Cadence: code-first (no write-safety, atomicity or bug-fix item touched)
Runs: L | V
Acceptance test: `cmd/quarry/run_spend_by_test.go` `Test_run_spend_by_payee_groups_spending_by_payee_and_currency_biggest_first`
Narrow loop: `go test ./internal/store/duckstore/ -run 'pending' && go test ./internal/report/ ./internal/cli/ -run 'pend' && go test ./cmd/quarry/ -run 'Test_run_spend'`
Mutation checks: none (no mandatory test-first item)

## Implementation Plan

### Acceptance (red)
- [x] `cmd/quarry/run_spend_by_test.go` (new): `spend --by payee` over CAD payees (Costco, Bakery in two splits, no payee) and a USD Costco; `spendSplit` gains `payee`, `spendRows` gains the payees table. Red while `--by` is an unknown flag.

### Build
- [x] 1. `store.go` `SpendByPayee`; `duckstore/spending.go` one query builder per grouping (`payee`: `ORDER BY GROUPING, currency, sum(spent) DESC, lower(payee), payee`); a grouping with no query (tag, month, 99) stays `ErrUnsupportedGrouping`. Tests: order/tie-break, nil key, totals unchanged.
- [x] 2. `report`: `SpendRequest.By`, `Spending.By` carried. Test: params reach the store, By comes back.
- [x] 3. `cli`: `spendGroupings` table (flag name, header, missing label) indexed by `store.SpendingGroup`; `--by` (default `category`) parsed before `openReport`, any name outside the table is S4 (`--by must be category, payee, tag or month`, exit 2); `renderSpending` header/label and `by`/row key in `spendDocument` follow it (one row struct per grouping; the embedded-field lint forbids sharing a tail without moving the key after `currency`). Tests: S4 table, payee text, payee JSON with null key.

## Handoff
- `tag` and `month` are refused by the S4 branch only because they are absent from `spendGroupings`; SCENARIO-11/12 add a table entry, a store query and a row struct each. Month rows also need `partial` and a Status column.

## Phase report
Run L done (code and tests in commit `60b1d7c`); run V done: `go build`, `golangci-lint run ./...` 0 issues, covered full suite green, `uncovered-diff.py` 0 uncovered added lines since `24692f5`, `go test -race` on cli/report/store green, spec ticked, `spec-check.py` OK, STATE.md rewritten (json row-key binding generalised; `--by` grouping table added; `SpendByPayee` out of Left unbuilt).
- test-stats vs `24692f5`: cmd/quarry 108 (+1), internal/cli 70 (+3), internal/report 23 (+1), internal/store/duckstore 117 (+4); TOTAL 318 (+9).
- Nothing left for later runs. No mutation checks were planned.
