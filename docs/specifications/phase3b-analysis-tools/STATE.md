# phase3b-analysis-tools — current state

Scenarios complete: SCENARIO-01. Last updated by SCENARIO-01. The phase3a MCP decisions (`docs/specifications/phase3a-mcp-core/STATE.md`: transport, `handler`/`errorLog`, `absentNullArguments`, `WithReport`/`WithConfig`, per-call deadline) still bind every tool added here.

## Binding decisions
- `document.NewSpending/NewCashFlow/NewRecurring/NewAnomalies(result, warnings)` (`internal/report/document`) are the only builders of the four documents; MCP tools render from them, cli only indents via `marshalDocument` (SCENARIO-01)
- `document.SpendingWarnings/CashFlowWarnings/RecurringWarnings/AnomaliesWarnings(result, word)`: `word` feeds only the linked-tracking and not-in-reports lines; subjects, nouns, no-rates and multi-tag lines are fixed (spec §4.1). cli passes `spend`/`cashflow`/`recurring`/`anomalies` (`*Command` consts), mcp passes the tool name. Order: left-out, unconverted, multi-tag, empty window; the caller prepends config lines (cli `withConfigWarnings`, mcp `cfg.WarningsAbsolute`) and mcp appends the cap line (SCENARIO-01)
- The `by` vocabulary is `store.SpendingGroups()` / `store.CashFlowPeriods()` plus `String()`, in order; schema enums come from them, never hand-typed (`finding.Types()` precedent). cli keeps only the `header`/`missing` presentation tables and parses `--by` over those lists (SCENARIO-01)
- `money.NativeOf` is the one home of "the other of CAD/USD"; cli `accountsFXWarnings` and document both call it. `document.RecurringStatus` / `document.BaselineWord` are shared by the text renderers and the JSON documents (SCENARIO-01)
- document imports only report, store, finding, platform (money, humanize, tomlstr); never cli, mcp, config (SCENARIO-01)
- Output equality is pinned byte for byte by `cmd/quarry/run_analysis_documents_test.go` (18 rows, `--json` and text, stdout and stderr exact, fixed `Env.Now`) with goldens in `run_analysis_documents_golden_test.go`; S02's CLI-vs-MCP harness may rely on them as the CLI side (SCENARIO-01)

## Left unbuilt
- mcp clock (`WithClock`), compact encoding of these documents, list caps and their warning lines, MCP currency resolution, the four tool registrations and schemas — S02 (spending), S09 (cash_flow, caps), S10 (recurring_charges, anomalies); description/instructions copy — S13
- Window refusal wording in MCP words (`WindowError` parts) — S03; account refusal parts, `logLine` classification and the OpenFaultOther withheld line — S04
- `withConfigWarnings`, `currencyFlag.resolve`, `accountsFXWarnings`, `recurringEvery`, the `*Command` consts stay in cli; accounts/snapshots/sync documents stay in `internal/cli/json_*.go` (SCENARIO-01)

## Traps
- recurring/anomalies `--json` pins decode into structs (`decodeRecurringJSON`, `decodeAnomaliesJSON`): blind to key order and dropped fields; `JSONEq` is blind too. Only the step-1 goldens and the document-package literal tests see bytes (SCENARIO-01)
- Left-out lines appear only for `--account`-named accounts; naming only left-out accounts suppresses the empty-window line (SCENARIO-01)
- `cashFlowCommand` is `cashflow` but its empty-window subject is `income or spending`; the two never mix (SCENARIO-01)
- `String()` on the enums is safe only because duckstore formats them with `%d` (`duckstore/spending.go:115`, `cashflow.go:64`); a `%v` would change error text (SCENARIO-01)
- The builders copy warnings with `append([]string{}, warnings...)`: neutral only because cli `withConfigWarnings` (`currency.go:71`) never returns nil; the document always emits `[]` (SCENARIO-01)
- Tests must go through `runWith` with a fixed `Env.Now`, never `run()`: the real clock drifts the default window (SCENARIO-01)

## Open debts
- OWNED BY S04: `internal/mcp/result.go:55` logs `report.RefusalError` verbatim; account refusals (`internal/report/refusal.go:63,68`) embed caller text, so classify them to class lines before the four tools ship; flip the pin at `internal/mcp/query_refusal_test.go:99-101` (phase3a gate R2)
- OWNED BY S04 (folds S05): statement-time DuckDB read faults log the reason verbatim via `OpenFaultOther` (`internal/store/duckstore/schema_read.go:57,69,81,89`, `status.go:71,92`, `findings_read.go:88`); `logLine` maps `OpenFaultOther` to the withheld line for open- and statement-time faults, with a per-tool table (phase3a gate R2)
- OWNED BY S13: per-parameter `description`s on the 3a tools (e.g. `data_quality.limit` counts findings) and the instructions string naming "David's" (phase3a final product-vision)
- Phase3a gate R1/R2/R4 MINOR/NIT debts not listed here remain in the phase3a STATE (unowned); this feature does not close them
