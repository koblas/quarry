---
id: SCENARIO-22
status: done
---

# SCENARIO-22: cashflow --json returns cash flow as a document

Size: LIGHT — 2 steps, internal/cli
Cadence: code-first (no write-safety, atomicity or bug-fix item touched)
Runs: L | V
Acceptance test: `cmd/quarry/run_cashflow_json_test.go` `Test_run_cashflow_json_returns_cash_flow_as_a_document`
Narrow loop: `go test ./internal/cli/ -run 'cashflow' && go test ./cmd/quarry/ -run 'test_run_cashflow'`
Mutation checks: none (no mandatory test-first item)

## Implementation Plan

### Acceptance (red)
- [x] `run_cashflow_json_test.go` (new): exact-bytes `cashflow --json --since 2026-01 --until 2026-02` over CAD salary, groceries and fuel in January and spending only in February (rate 31.9, null, total 30.8); red while `--json` prints the text table. Replace the interim `Test_cashflow_json_prints_the_text_table_until_the_document_exists` (`internal/cli/cashflow_test.go:164`) with a cli-level `--json` test.

### Build
- [x] 1. `internal/cli/json_cashflow.go` (new): `cashFlowDocument` (since, until, by, account_filter, periods, totals, warnings), period/total row documents, `renderCashFlowJSON(c, warnings)` via `marshalDocument`; `periods`/`totals`/`account_filter`/`warnings` made, never nil; `savings_rate_pct` is `*float64`. Reuses `spendAccountDocument`. White-box `json_cashflow_internal_test.go`: empty lists as `[]`, partial true, negative amounts, year period.
- [x] 2. `cashflow.go`: pass `renderCashFlowJSON(flow, warnings)` to `renderResult`; delete the `// unreachable:` marker (`cashflow.go:87`). Fold: `cashflow --help` test pinning `--by`, `--account` and Long's first line.

## Handoff
- None: last cashflow scenario; `--json` warnings are unprefixed, as in spend.

## Phase report
Runs L and V done. Start 634f2a0.
- Verify: covered full suite exit 0; `uncovered-diff.py` 0 uncovered since 634f2a0; `go test -race ./internal/cli/... ./cmd/quarry/...` ok; `golangci-lint run ./...` 0 issues.
- test-stats --base 634f2a0 --changed: cmd/quarry 121 (+1), internal/cli 119 (+4), TOTAL 240 (+5); tempdir 115 (+1), disk 100 (+1).
- Ticked SCENARIO-22 in specification.md; STATE.md rewritten (renderCashFlowJSON, interim trap and help-copy debt removed).
