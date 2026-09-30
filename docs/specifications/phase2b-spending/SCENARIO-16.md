---
id: SCENARIO-16
status: done
---

# SCENARIO-16: spend --json returns spending as a document

Size: LIGHT — 2 steps, internal/cli
Cadence: code-first (no write-safety, atomicity or bug-fix item touched)
Runs: L | V
Acceptance test: `cmd/quarry/run_spend_json_test.go` `Test_run_spend_json_returns_spending_as_a_document`
Narrow loop: `go test ./internal/cli/ -run 'pend' && go test ./cmd/quarry/ -run 'Test_run_spend'`
Mutation checks: none (no mandatory test-first item)

## Implementation Plan

### Acceptance (red)
- [x] `run_spend_json_test.go` (new): exact-bytes `spend --json` over CAD groceries + fuel, an uncategorized negative row and a USD row (fixture via `spendRows`); red while `--json` still prints the text table. Replace the interim `Test_spend_prints_the_text_table_for_json_until_the_json_document_exists` in `internal/cli/spend_test.go`.

### Build
- [x] 1. `internal/cli/json_spend.go` (new): `spendDocument` (since, until, by, account_filter, rows, totals, warnings) + `renderSpendingJSON(report.Spending) ([]byte, error)` via `marshalDocument`. `spendRowDocument.Category *string` (`null` for uncategorized), then currency, spent. `rows`/`totals`/`account_filter`/`warnings` made, never nil. `by` is `"category"` (only grouping today). White-box test `json_spend_internal_test.go`: null key, empty rows/totals as `[]`, negative spent.
- [x] 2. `spend.go`: pass `renderSpendingJSON` to `renderResult`; delete the `// unreachable:` marker and the `if err != nil { return err }` stays, as in `accounts.go:38`.

## Handoff
- `partial` on month rows arrives with SCENARIO-12 (row document then needs a `Partial` field); `by` and row key come from `--by` in SCENARIO-10; `account_filter` entries `{"id","name"}` fill in SCENARIO-14.

## Phase report
Runs L and V done. Verify: `go build ./...` ok; `golangci-lint run ./...` 0 issues; covered full suite (`-count=1 -coverpkg=./...`) exit 0; `go test -race ./internal/cli/... ./cmd/quarry/...` ok; `uncovered-diff.py` 0 uncovered added lines since 7e1f486 (`spend.go` `return err` after renderResult is counted covered, same shape as `accounts.go:38`, no marker needed).
- test-stats --base 7e1f486 --changed: cmd/quarry 107 (+1), internal/cli 67 (+2), TOTAL 174 (+3), tempdir 101 (+1), disk 86 (+1).
- Ticked SCENARIO-16 in specification.md; `spec-check.py phase2b-spending` OK. STATE.md rewritten (interim-json binding replaced by the document shape; `renderSpendingJSON` out of Left unbuilt).
