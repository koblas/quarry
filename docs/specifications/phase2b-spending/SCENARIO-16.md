---
id: SCENARIO-16
status: open
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
Run L done. Red quoted: `spend --json` printed the text table, not the document (`run_spend_json_test.go:38`). Green: `go test ./internal/cli/ -run pend`, `go test ./cmd/quarry/ -run Test_run_spend`, lint on cli+cmd clean.
- New: `internal/cli/json_spend.go`, `json_spend_internal_test.go`, `cmd/quarry/run_spend_json_test.go`. Changed: `spend.go` (renderer + marker deleted), `spend_test.go` (interim test replaced by `Test_spend_json_puts_the_report_window_and_rows_in_the_document`).
- Row key is a fixed `category` field; SCENARIO-10 must make it follow `--by`, SCENARIO-12 adds `partial`, SCENARIO-14 fills `account_filter`.
- V: full sweep/verify, check uncovered-diff on `spend.go` `return err`, tick SCENARIO-16 in specification.md, STATE.md (drop the interim-json line), `status: done`.
