# phase2c-snapshots-config — current state

Scenarios complete: PRE-01 (behaviour-neutral pre-step; no SCENARIO ticked yet). Last updated by PRE-01.

## Binding decisions
- R3 reason phrase is `(*store.OpenError).UnreadableReason(at string) string` in `internal/store/open.go`: `at` is the display path (caller abbreviates with `homepath.Abbreviate`); returns the bare phrase (`the file is not a DuckDB database`, `permission denied`, `another program has it open for writing`, or Reason with every Path replaced by `at`), with no `cannot read the store at …:` prefix and no remedy. Missing/OtherFormat give the Reason fallback (empty). `snapshot` may not import `report`, so SCENARIO-15/16/21 render refusals from this method (PRE-01)
- `internal/store/duckstore/filter.go` owns `accountFilter`, `readArgs`, `civilDay`, `transactionRangeQuery`, `transactionRange`; a new windowed read reuses them, never a copy. `transactionRange` stays the LAST statement of `Spending` and `CashFlow` (tag-fault tests count statements positionally) (PRE-01)
- `report/period.go` `fillSeries` is the one fill helper for every period-series report; `DefaultWindow` lives in `report/window.go`; `period.First/Last` fields are gone (PRE-01)
- `internal/cli/window.go` `reportFlags` (since, until, accounts) owns `--since`, `--until`, `--account`; each command registers `--by` first, then `reportFlags.bind`. Flag order and help strings are pinned by `report_help_test.go` (PRE-01)
- `internal/cli/output.go` `emitReport(cmd, asJSON, warnings, renderJSON, renderText)` is the one spend/cashflow tail (renderResult then emit). Its single `// unreachable:` comment must hold for every caller: spend (marshalDocument only), cashflow (`savings_rate_pct` finite). A new caller must extend that reason or be tested (PRE-01)
- `internal/cli/render_table.go` `renderTable(caption, rows)` is the one period-table renderer; `windowCaption`, `accountsCaption`, `tableTotalLabel`, `tablePartialStatus` are report-neutral (PRE-01)
- `accountFilterDocument` / `accountFilterDocuments(accounts)` in `internal/cli/json_spend.go` build every report's `account_filter` (not `accountDocument`: that is sync's `never_reconciled` entry in `json.go`); `allLeftOut(accounts)` in `empty_window.go` (PRE-01)
- Command-name const pairs, unexported, must stay equal: cli `spendCommand="spend"` / `cashFlowCommand="cashflow"` (Use: and `leftOutWarnings`) and report `spendCommand` / `cashFlowCommand` (refusal word, e.g. `spend interrupted`). `jsonDateLayout = time.DateOnly` (value unchanged) (PRE-01)

## Left unbuilt
- `render.go` `:262,290,323` date literals stay (sync validation output, outside the report pipeline) — unowned (PRE-01)

## Traps
- `exhaustive` rejects a partial `OpenFault` switch even with `default:`; an arm only Missing/OtherFormat reach is uncovered (report handles them first) and fails `uncovered-diff.py` (PRE-01)
- Cobra renders flag help from registration order; keep `--by`, `--since`, `--until`, `--account` exactly (PRE-01)
- P2c-12: the existing black-box suite must pass with no `*_test.go`, `testdata` or `cmd/` hunk from PRE-01 (`git diff --exit-code b9bb83c -- '*_test.go' cmd/ ':(glob)**/testdata/**'` is empty); later scenarios add tests, so this is a PRE-01-range check only (PRE-01)

## Open debts
- PRE-01 checkpoint MINORs (comment budget): `internal/store/duckstore/cashflow.go:18-20` and `spending.go:88-90` sentinel docs 3 lines → 1 (reachability note at the `!ok` return); `internal/report/period.go:62-64` `fillSeries` doc → 2 lines; `internal/cli/render_table.go:20-22` `renderTable` doc → 2 lines. NIT: `store.OpenError.UnreadableReason` empty-result branch for Missing/OtherFormat unexecuted — pin it if SCENARIO-15/16/21 reach those faults.
- `store.CashFlowFigures` (shared figures struct embedded in `store.CashFlowRow` / `store.CashFlowTotal`) — not built: it rewrites composite literals in `internal/cli/cashflow_test.go` (4) and `internal/store/duckstore/cashflow_test.go` (10), a test hunk PRE-01 forbids. Unowned — dies unless re-opened (PRE-01)
- 2b `STATE.md` debt about the same duplication is closed by the orchestrator at SHIP (surface #9), not edited here (PRE-01)
