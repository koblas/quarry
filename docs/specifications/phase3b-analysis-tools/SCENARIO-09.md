---
id: SCENARIO-09
status: open
---

# SCENARIO-09: cash_flow returns the cashflow --json document (folds SCENARIO-08 caps, SCENARIO-14 defaults)

Cadence: code-first (no write guard, atomic adapter or bug fix; no new `report.Store` method)
Acceptance test: `cmd/quarry/run_mcp_cash_flow_test.go` `Test_run_mcp_cash_flow_returns_the_cashflow_json_document`
Acceptance test (SCENARIO-08, folded): `cmd/quarry/run_mcp_spending_cap_test.go` `Test_run_mcp_spending_cuts_a_list_over_500_rows_with_the_cap_warning`
Acceptance test (SCENARIO-14, folded): `internal/mcp/server_test.go` `Test_a_by_tool_call_with_absent_null_or_empty_arguments_applies_the_by_default`
Narrow loop: `go test ./internal/mcp/ -run 'CashFlow|cash_flow|Cap|Spending|absent_null|deadline' && go test ./cmd/quarry/ -run 'MCP|mcp'`
Mutation checks: `accountRefusal` call at the `CashFlow` error site -> `Test_run_mcp_cash_flow_refuses_an_account_without_its_name_on_stderr`; cap test `len(list) > maxRows` -> `Test_capList_*` rows 500 and 501; cap line appended last -> `Test_cash_flow_puts_the_cap_line_after_every_other_warning`; twin `"cashflow"` passed to `resolveCurrency` -> `Test_cash_flow_refuses_an_unreadable_config_before_building_the_report`
Runs: A (1-3) | B1 (4-5) | B2 (6) | V (7-8)
Size: OWNS A RUN — 3 build batches (tool, all-tools rows, cap helper), 1 feature package (`internal/mcp`); S08/S14 folded per sizing

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_mcp_cash_flow_test.go` (new) `Test_run_mcp_cash_flow_returns_the_cashflow_json_document` — `runBothSurfaces` (`run_mcp_documents_helpers_test.go:40`) over `populatedAnalysisStore` (`run_analysis_documents_test.go:132`); rows: all five given (accounts `Linked` and `Old Card` so a left-out and a before-first-rate warning exist), none given, `currency native`, `by year`; `cliBody == toolBody`, warnings via `inToolWords(.., "cashflow", "cash_flow")`
- [x] Step 2: `cmd/quarry/run_mcp_spending_cap_test.go` (new) `Test_run_mcp_spending_cuts_a_list_over_500_rows_with_the_cap_warning` — `chargeRows` (`run_helpers_test.go:~172`) with one distinct payee per charge, `by payee`; 501 payees: `rows` is `spend --by payee --json`'s first 500 in order, `totals` equal the CLI's, last warning is the §5 spending line with `501`; 500 payees: 500 rows, no cap line (control)
- [x] Step 3: `internal/mcp/server_test.go:163` `Test_a_by_tool_call_with_absent_null_or_empty_arguments_applies_the_by_default` — spending and cash_flow x omitted/null/`{}` through the existing `newHarness`; asserts decoded `by` is `category` / `month`. Support: `query_helpers_test.go` `fakeStore` (`:29-58`) gains a `flow store.CashFlow` field and `CashFlow` method; `harness.cashFlow` beside `harness.spending` (`:145`). Spending arms are green on arrival (S02 built them); cash_flow arms red (unknown tool). No production stubs needed

### Build
- [x] Step 4: cash_flow tool — `internal/mcp/tools.go:76-83` `toolCashFlow`; `:133-140` `cashFlowDescription` (spec §3.3 verbatim, hard line breaks); `:142-149` `cashFlowByDesc` (§3.4); `:163-170` `cashFlowInput` (pointer since/until like `spendingInput`); `:175-195` registration reusing `described`/`accountsSchema`/`currencySchema`/`sinceDescription`.. with `by` enum from a new `cashFlowPeriods()` over `store.CashFlowPeriods()` (model `spendingGroups`, `:244-252`), `Default: "month"`, `handler(s.timeout, stoppedLine(toolCashFlow), s.cashFlow)`. New `internal/mcp/cash_flow.go` `(*Server).cashFlow` mirroring `spending.go:17-36`: `s.now()` once -> `report.ParseWindow` -> `windowRefusal`; `resolveCurrency(in.Currency, "cashflow")`; `newReport(ctx, commandName)`; `srv.CashFlow(ctx, report.CashFlowRequest{..})` -> `accountRefusal(err)` at that site; `document.NewCashFlow(flow, append(configWarnings, document.CashFlowWarnings(flow, toolCashFlow)...))`; parse of `by` with `// unreachable:` default as `spending.go:52-61` (extract one shared helper if both stay under 6 lines). Unit tests `internal/mcp/cash_flow_test.go` mirroring `spending_test.go:44-155`, each its own row: reads today once; bad window refused with class line before config/store (include future-since `since 2099 is after today; pass until to include future-dated transactions`); unreadable config -> stderr `run quarry cashflow to see why`; config not read when currency given; config read as `mcp`; factory failure -> generic line; store refusal verbatim; plain fault -> generic line; schema-rejected args (`by` `weekday`/`week`, `cad`, null since, unknown property, non-list accounts) with no config read and no store build
- [x] Step 5: cash_flow rows in every all-tools table (found by grep for `"spending"` in tests): `cmd/quarry/run_mcp_no_store_test.go:28`; `internal/mcp/timeout_test.go:101` (+ `stallingStore.CashFlow`, `:66`, recording the deadline and stalling like `Spending`) with line `cash_flow stopped after 1 second; try again`; `cmd/quarry/run_mcp_store_faults_test.go:60,76` (`cash_flow, store cannot be opened`; `cash_flow, its view dropped` via `brokenStore("DROP VIEW v_cash_flow")`, reason `Table with name v_cash_flow does not exist!`); `cmd/quarry/run_mcp_test.go:62-132` cash_flow description and input-schema consts plus `wantTools` entry (pin count becomes 6; do not rename the test, S13 owns that); new `Test_run_mcp_cash_flow_refuses_an_account_without_its_name_on_stderr` in `run_mcp_cash_flow_test.go` copying the three cases of `run_mcp_spending_test.go:179-229` (S04's ticked test keeps its name) with prefix `quarry: mcp: cash_flow: `
- [ ] Step 6: cap — new `internal/mcp/cap.go` generic `capList` (list, tool/noun/advice -> cut list, §5 line): cuts to `maxRows` (`tools.go:87`) after the document is built, in order, numbers through `humanize.Thousands`; `spending.go:35` and `cash_flow.go` apply it to `doc.Rows` / `doc.Periods`, leave `Totals` untouched and append the line **last**, after config and document warnings. Lines verbatim from spec §5 (spending: `..., or query v_spending for the rest`; cash_flow: `pass a later since, or by year`). Unit tests white-box `cap_internal_test.go` `Test_capList_*`: 499/500 untouched with no line, 501 -> 500 + `of 501`, 1,234 -> `1,234`, order kept, empty list; and through `Server`: `Test_spending_*cap*` / `Test_cash_flow_puts_the_cap_line_after_every_other_warning` (fakeStore with 501 rows and a left-out warning plus a config warning; totals length equals the uncut result's). Cap line generic enough that S10 passes `series` / `anomalies` unchanged

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `cashFlow`, `capList`, `cashFlowPeriods`; `doc.go` unchanged

### Verify
- [ ] Step 8: full verification block + `.claude/scripts/spec-check.py phase3b-analysis-tools` -> tick SCENARIO-09, 08, 14 (08 and 14 lines "delivered by SCENARIO-09", test last) ; rewrite STATE.md

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- The cap lives in `internal/mcp` (`capList`), applied to the built document after `document.New*`; `document` and the CLI stay uncapped — Rule 1 says CLI is the uncut reference and the equality harness's fixtures stay under 500.
- Cap line is the last warning, after config lines and `document.*Warnings`; its numbers use `humanize.Thousands` — spec §4.1 order.
- `capList` is element-type generic and takes tool, noun and advice from the caller — S10 reuses it for `series` and `anomalies` without a second helper.
- cash_flow passes `"cashflow"` to `resolveCurrency` and `toolCashFlow` (`cash_flow`) to `CashFlowWarnings` — stderr names the CLI twin, warnings name the tool.

**Left unbuilt** — named so nobody assumes it exists:
- `toolRecurring`, `toolAnomalies`, their handlers, caps, and rows in the five all-tools tables (no-store, timeout, store-fault, tools/list pin, account refusal) — S10
- Per-param descriptions on the 3a tools, instructions, query description, mcp Long, rename of `Test_run_mcp_lists_quarrys_four_tools_over_json_rpc` — S13
- A `Test_a_tool_call_with_absent_null...` arm for recurring/anomalies (they have no `by`; not needed)

**Traps** — things that look right and are not:
- `document.Spending.Rows` is `[]any`, `document.CashFlow.Periods` is `[]CashFlowPeriodRow` — a non-generic cap helper fails to compile for one of them.
- S14's spending arms are green on arrival; only cash_flow arms are red, so the test's red output must be the cash_flow arm's.
- S04's ticked acceptance test is `Test_run_mcp_spending_refuses_an_account_without_its_name_on_stderr`; renaming or generalising it breaks `spec-check.py`.
- Warning map in the harness substitutes on `, so cashflow leaves it out`; `cashflow` has no bare-word hazard but keep the template form.
- cash_flow's `by` schema `Default` is the quoted JSON bytes `"month"` (as spending's `"category"`); the handler never defaults `by` itself (parse miss stays `// unreachable:`), so S14 depends on the schema default alone.

## Phase report

Run B1 (steps 4-5) done; step 6 (cap) untouched. S09 and S14 acceptance green; S08 cap acceptance still red (its test panics at `run_mcp_spending_cap_test.go:51`, aborting any `cmd/quarry` run that includes it: use `-skip 'Test_run_mcp_spending_cuts_a_list'` until step 6).

Files:
- `internal/mcp/cash_flow.go` (new) - `(*Server).cashFlow`, `parseCashFlowPeriod` (`// unreachable:` fallback), const `cashFlowTwin`.
- `internal/mcp/tools.go` - `toolCashFlow`, `cashFlowDescription`, `cashFlowByDesc`, `cashFlowInput`, registration, `cashFlowPeriods()`; `spendingGroups`/`cashFlowPeriods` are one-liners over new generic `stringEnum`.
- `internal/mcp/cash_flow_test.go` (new) - 11 tests (clock once, bad window, future since, config unreadable/not read/read as mcp, `by year`, factory fault, store refusal, plain fault, schema-rejected table incl. `week`/`weekday`/`category`).
- All-tools rows: `cmd/quarry/run_mcp_no_store_test.go`, `run_mcp_store_faults_test.go` (2 rows), `run_mcp_test.go` (cash_flow consts + `wantTools`, pin count 6), `internal/mcp/timeout_test.go` (`stallingStore.CashFlow` + row), `cmd/quarry/run_mcp_cash_flow_test.go` (`Test_run_mcp_cash_flow_refuses_an_account_without_its_name_on_stderr`).

Mutations (restored, diffed): `accountRefusal(err)` -> `err` at CashFlow site reddens the account test ("run quarry accounts --all to list them" vs expected class text); twin `"cashflow"` -> `"spend"` reddens `Test_cash_flow_refuses_an_unreadable_config_before_building_the_report` (stderr names `spend`).

Step 6 must: add `cap.go` `capList`, apply in `spending.go:35` and `cash_flow.go` (both build `document.New*(...)` inline: split into doc, cap Rows/Periods, append line last); add `Test_capList_*` and `Test_cash_flow_puts_the_cap_line_after_every_other_warning`; mutations `len(list) > maxRows`, cap line last.

Green: golangci-lint 0 issues on `internal/mcp`, `cmd/quarry`; test-stats vs start: cmd/quarry 541 (+4), internal/mcp 103 (+12).
