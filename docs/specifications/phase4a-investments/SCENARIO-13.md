---
id: SCENARIO-13
status: open
---

# SCENARIO-13: Sync keeps a commission with fractions of a cent

Cadence: code-first
Acceptance test: `cmd/quarry/run_commission_test.go` `Test_run_sync_keeps_a_commission_with_fractions_of_a_cent`
Narrow loop: `go test ./internal/importer/ ./internal/store/... ./cmd/quarry/ -run '(?i)commission|investment'`
Mutation checks: none
Size: LIGHT — 3 steps, importer
Runs: L | V

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_commission_test.go` — buy 9.99 and sell 8.4998 commissions sync, exit 0, store reads `9.9900` / `8.4998`. Red: exit 1 (2-decimal refusal).

### Build
- [x] Step 2 (importer + store, one unit change cents → ten-thousandths): `price.go` fixed-point `parseCommission` (snap 1e-8, bound 10^14), `investments.go` commission path, `reasons.go` `… has a commission of <v>, which has more than 4 decimal places` (replaces the 2-decimal line for commission only; amount keeps 2), `store.go` `Commission` ten-thousandths, `duckstore` DDL `DECIMAL(18,4)` + `commissionScale`. Tests: re-pin `investments_test.go` commission rows (1.2345 kept, 1.23456 refused, 1e-9 snaps to NULL) and `duckstore_test.go` commission rows; cmd refusal row 1.23456 in `run_investments_test.go`; re-pin `9.99`/`4.95` → `9.9900`/`4.9500`.
- [x] Step 3 (copy): `report/sql_conventions.go` commission clause (S.7) + byte pins `internal/cli/sql_test.go`, `cmd/quarry/run_shared_documents_test.go`; regenerate `schema.md`; `docs/initial-prd.md` L125 `(commission DECIMAL(18,4))` + `run_plugin_notices_test.go:53` pin.

## Handoff
`store.InvestmentTransaction.Commission` is ten-thousandths of the currency unit; nothing else reads it.

## Phase report
Run L done (plan, Acceptance red, Build green; nothing for V undone).
- Red (run before code): `run_commission_test.go:39` `expected: 0, actual: 1` (2-decimal refusal).
- Green: acceptance test, `Test_parse_commission_snaps_residue_within_tolerance_and_refuses_beyond` (`price_internal_test.go`), commission rows in `investments_test.go` (stored/refused tables), `duckstore_test.go` commission rows, `run_investments_test.go` (1.23456 refusal row, `9.9900`/`4.9500` re-pins), shared-documents/sql/notices byte pins. Narrow packages importer, store, cli, report, cmd/quarry green; `golangci-lint` 0 issues.
- Files: `internal/importer/price.go` (`fixedPoint`, `parseFixed`, `parseCommission`), `investments.go` (`moneyColumn`), `reasons.go:148`, `internal/store/store.go`, `duckstore/{schema,duckstore}.go` (`commissionScale`), `report/sql_conventions.go`, `plugin/skills/quarry/references/schema.md` (regenerated), `docs/initial-prd.md` L125.
- V still to do: full covered suite + `uncovered-diff.py --profile`, `test-stats.py`, spec tick (`Test_run_sync_keeps_a_commission_with_fractions_of_a_cent`), `spec-check.py`, STATE.md rewrite, `status: done`.
- Not done by design: no mutation checks (code-first); `FormatVersion` stays 6 (unreleased 4a column change).
