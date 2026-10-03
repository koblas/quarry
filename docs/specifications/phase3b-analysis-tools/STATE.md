# phase3b-analysis-tools — current state

Scenarios complete: SCENARIO-01, 02 (folds 06, 07, 12), 03. Last updated by SCENARIO-03. The phase3a MCP decisions (`docs/specifications/phase3a-mcp-core/STATE.md`: transport, `handler`/`errorLog`, `absentNullArguments`, `WithReport`/`WithConfig`, per-call deadline) still bind every tool added here.

## Binding decisions
- `document.NewSpending/NewCashFlow/NewRecurring/NewAnomalies(result, warnings)` (`internal/report/document`) are the only builders of the four documents; MCP tools render from them, cli only indents via `marshalDocument` (SCENARIO-01)
- `document.SpendingWarnings/CashFlowWarnings/RecurringWarnings/AnomaliesWarnings(result, word)`: `word` feeds only the linked-tracking and not-in-reports lines; subjects, nouns, no-rates and multi-tag lines are fixed (spec §4.1). cli passes `spend`/`cashflow`/`recurring`/`anomalies` (`*Command` consts), mcp passes the tool name. Order: left-out, unconverted, multi-tag, empty window; the caller prepends config lines (cli `withConfigWarnings`, mcp `cfg.WarningsAbsolute`) and mcp appends the cap line (SCENARIO-01)
- The `by` vocabulary is `store.SpendingGroups()` / `store.CashFlowPeriods()` plus `String()`, in order; schema enums come from them, never hand-typed (`finding.Types()` precedent). cli keeps only the `header`/`missing` presentation tables and parses `--by` over those lists (SCENARIO-01)
- `money.NativeOf` is the one home of "the other of CAD/USD"; cli `accountsFXWarnings` and document both call it. `document.RecurringStatus` / `document.BaselineWord` are shared by the text renderers and the JSON documents (SCENARIO-01)
- document imports only report, store, finding, platform (money, humanize, tomlstr); never cli, mcp, config (SCENARIO-01)
- Output equality is pinned byte for byte by `cmd/quarry/run_analysis_documents_test.go` (18 rows, `--json` and text, stdout and stderr exact, fixed `Env.Now`) with goldens in `run_analysis_documents_golden_test.go`; S02's CLI-vs-MCP harness may rely on them as the CLI side (SCENARIO-01)
- `mcp.WithClock(func() time.Time)`, default `time.Now` in `NewServer`, read once at handler start; later tools pass that one value to the window parse and `Request.Now` (two reads straddle midnight). Departs from the skill's "no injected clock" because the equality harness needs one fixed instant on both surfaces (SCENARIO-02)
- `(*Server).resolveCurrency(name, twin)` is the one currency path: config read only when currency is absent, `cfg.WarningsAbsolute` first; cash_flow/recurring/anomalies reuse it. `configRefusalLog(twin)` names the CLI twin (`spend`, `cashflow`, `recurring`, `anomalies`; data_quality `findings`); `newConfig`/`newReport` keep `commandName` `mcp` (SCENARIO-02)
- Shared schema builders in `internal/mcp/tools.go` (`described`, `accountsSchema`, `currencySchema`, `sinceDescription`/`untilDescription`/`accountsDescription`/`currencyDescription`): S09 reuses them; S10 swaps its own since/until/accounts strings, keeps currency. `currency` never gets a schema `default` (it overrides `reporting.currency`); `by` enum comes from `spendingGroups()` over `store.SpendingGroups()` (SCENARIO-02)
- Handler order: window, config (only when currency absent), factory, `Spend`; warnings are `cfg.WarningsAbsolute`, then `document.*Warnings(result, toolName)`; copy it for the other three tools (SCENARIO-02)
- Equality harness `cmd/quarry/run_mcp_documents_helpers_test.go` is the acceptance shape for S09/S10: CLI `--json` compacted vs tool `TextContent`, compared as bytes with `warnings` stripped from both, warnings mapped separately; fixtures stay under the 500 cap (SCENARIO-02)
- `report.WindowError` is a value of exported parts (`Kind`, `Bound` bare `since`/`until`, `Value`, `Other`, `DefaultSince`, `Command`); only `Error()` words the CLI line with `--`, so cli keeps `UsageError{msg: err.Error()}` and reads no parts. `mcp.windowRefusal(err)` is the one MCP wording of a window refusal and attaches the class line `windowRefusedLog` (`result.go`) itself; S09/S10 call it unchanged, none re-words. S10 passes the tool name to `ParseChargeWindow` and the charge variant says it through `Command` (SCENARIO-03)

## Left unbuilt
- Account refusal wording/class lines and the `OpenFaultOther` withheld line — S04 (`logLine` still logs a `RefusalError` verbatim)
- charge-variant window wording is built and unit-pinned (`Test_windowRefusal_words_every_kind`) but reaches no tool until S10; cash_flow tool (calls `windowRefusal`), list caps and their warning lines, compact encoding of those documents — S09; recurring_charges and anomalies — S10
- `null`/omitted/`{}` arguments row for spending at `internal/mcp/server_test.go:163` — S09 (S14 in S09)
- Per-param descriptions on the 3a tools, instructions, query description, mcp Long, rename of the tools/list pin test (`cmd/quarry/run_mcp_test.go`) — S13
- `withConfigWarnings`, `currencyFlag.resolve`, `accountsFXWarnings`, `recurringEvery`, the `*Command` consts stay in cli; accounts/snapshots/sync documents stay in `internal/cli/json_*.go` (SCENARIO-01)

## Traps
- recurring/anomalies `--json` pins decode into structs (`decodeRecurringJSON`, `decodeAnomaliesJSON`): blind to key order and dropped fields; `JSONEq` is blind too. Only the step-1 goldens and the document-package literal tests see bytes (SCENARIO-01)
- Left-out lines appear only for `--account`-named accounts; naming only left-out accounts suppresses the empty-window line (SCENARIO-01)
- `cashFlowCommand` is `cashflow` but its empty-window subject is `income or spending`; the two never mix (SCENARIO-01)
- `String()` on the enums is safe only because duckstore formats them with `%d` (`duckstore/spending.go:115`, `cashflow.go:64`); a `%v` would change error text (SCENARIO-01)
- The builders copy warnings with `append([]string{}, warnings...)`: neutral only because cli `withConfigWarnings` (`currency.go:71`) never returns nil; the document always emits `[]` (SCENARIO-01)
- Tests must go through `runWith` with a fixed `Env.Now`, never `run()`: the real clock drifts the default window (SCENARIO-01)
- Warning map in the harness substitutes on the `, so <word> leaves it out` template, never the bare word: `spend` sits inside `spending` and `recurring` inside `recurring charges` (SCENARIO-02)
- No S02 test may assert a verbatim account-refusal stderr line: S04 flips it (SCENARIO-02)
- Value quoting differs by window kind: only not-a-date uses `%q`, the rest print the raw value; `windowRefusal` never puts `Value`/`Other` in the log line, and string-replacing `--` on `Error()` would eat `--` inside a caller value (SCENARIO-03)
- `go test -run 'Window'` is case-sensitive: older report tests are lowercase `window`; narrow loops use `(?i)window` (SCENARIO-03)
- The `by` parse miss in `parseSpendingGroup` is `// unreachable:` behind the schema enum; add no handler default (SCENARIO-02)
- `rows.Err()` alone misses a cancel: database/sql closes rows asynchronously, so a read can finish with a nil error. duckdb `QueryRows` and `QueryTable` check `ctx.Err()` per row; any new row loop over `*sql.Rows` needs the same. `platform/sqlite` `QueryRows` has the same unchecked loop (unowned) (fix(duckdb) pass)

## Open debts
- OWNED BY S04: `internal/mcp/result.go:55` logs `report.RefusalError` verbatim; account refusals (`internal/report/refusal.go:63,68`) embed caller text, so classify them to class lines before the four tools ship; flip the pin at `internal/mcp/query_refusal_test.go:99-101` (phase3a gate R2)
- OWNED BY S04 (folds S05): statement-time DuckDB read faults log the reason verbatim via `OpenFaultOther` (`internal/store/duckstore/schema_read.go:57,69,81,89`, `status.go:71,92`, `findings_read.go:88`); `logLine` maps `OpenFaultOther` to the withheld line for open- and statement-time faults, with a per-tool table (phase3a gate R2)
- OWNED BY S13: per-parameter `description`s on the 3a tools (e.g. `data_quality.limit` counts findings) and the instructions string naming "David's" (phase3a final product-vision)
- Phase3a gate R1/R2/R4 MINOR/NIT debts not listed here remain in the phase3a STATE (unowned); this feature does not close them
- Checkpoint S02 NITs: `internal/mcp/tools.go:~88` `spendingByDesc` naming vs `*Description` siblings; `internal/mcp/spending.go:49` `slices.Clone` unpinned (harmless)
