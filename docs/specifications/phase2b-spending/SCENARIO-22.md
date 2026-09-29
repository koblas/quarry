---
id: SCENARIO-22
status: open
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
Run L done (plan written, Acceptance + Build). Start 634f2a0.
- Red seen: `Test_run_cashflow_json_returns_cash_flow_as_a_document` failed at its exact-bytes assertion (actual = text table). Now green.
- New: `internal/cli/json_cashflow.go` (`renderCashFlowJSON`, `cashFlowDocument`), `json_cashflow_internal_test.go` (2 tests + `cashFlowTestRow` helper: embedlit lint forbids `{CashFlowRow: {...}}` and the compiler rejects the elided form), `cmd/quarry/run_cashflow_json_test.go`.
- Changed: `internal/cli/cashflow.go:83-89` (real renderer; `// unreachable:` marker deleted); `internal/cli/cashflow_test.go` (interim test replaced by 2 json tests + `Test_cashflow_help_shows_the_flags_and_what_the_command_is_for`; unused `cashFlowCaption` const removed).
- Already run: `golangci-lint run ./...` 0 issues; covered full suite exit 0; `uncovered-diff.py` 0 uncovered since 634f2a0.
- V still to do: race run, `test-stats.py --base 634f2a0 --changed`, tick SCENARIO-22 in specification.md (`cmd/quarry/run_cashflow_json_test.go` `Test_run_cashflow_json_returns_cash_flow_as_a_document`), `spec-check.py`, STATE.md (drop `renderCashFlowJSON` from Left unbuilt, drop the interim-json trap and the folded help-copy debt, remove the spec line 176 interim note is spec text: leave), `status: done`.
