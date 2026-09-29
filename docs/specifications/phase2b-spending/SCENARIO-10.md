---
id: SCENARIO-10
status: open
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
Run L done: acceptance red at its assertion (`quarry: unknown flag: --by`, exit 2 not 0), then green; `golangci-lint run ./...` 0 issues.
- `internal/store/store.go` `SpendByPayee`; `duckstore/spending.go` `spendingQuery(key, rowOrder)` + `spendingQueries` map (a grouping absent from it is `ErrUnsupportedGrouping`; test for 99 still there); `duckstore/spending_payee_test.go` (4 tests).
- `internal/report/spending.go` `SpendRequest.By`, `Spending.By`; `internal/cli/spend_grouping.go` (`spendGroupings` array indexed by group, `parseSpendGrouping`, `errSpendByUnknown`), `spend.go` `--by`, `render_spend.go` header/missing label, `json_spend.go` `spendCategoryRowDocument`/`spendPayeeRowDocument` + `spendRowDocumentFor`.
- Tests: `cmd/quarry/run_spend_by_test.go` (acceptance; `spendSplit.payee`, `spendRows` payees in `run_spend_test.go`), `internal/cli/spend_test.go` (payee wiring, S4 table incl. tag/month), white-box render and json cases, `internal/report/spending_test.go`.
- Run V still to do: Verify block, spec tick (append acceptance test), `spec-check.py`, STATE.md rewrite (json row-key binding now generalised; `--by` and SpendByPayee out of Left unbuilt; tag/month refused via absence from `spendGroupings`), `status: done`. No mutation checks were planned.
