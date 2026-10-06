---
id: SCENARIO-16
status: open
---

# SCENARIO-16: Claude asks for last month's summary

Cadence: code-first (no mandatory test-first item: no write-safety guard, atomic adapter or bug fix)
Acceptance test: `cmd/quarry/run_mcp_summary_test.go` `Test_run_mcp_monthly_summary_returns_the_summary_json_document`
Narrow loop: `go test ./internal/mcp/ ./internal/cli/ -run 'monthly_summary|each_tool|logLine|mcp'` and `go test ./cmd/quarry/ -run 'run_mcp|run_skill'`
Mutation checks: ignore list read when `currency` is given (`summaryChoices`) → `Test_monthly_summary_counts_an_ignored_finding_when_currency_is_given`; `s.now()` read once per call → `Test_monthly_summary_reads_today_once_at_the_start_of_every_call`
Runs: A (1-2) | B1 (3-4) | B2 (5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (internal/mcp; cli Tools line, cmd/quarry tests, SKILL/PRD rows ride along)

Sources: `internal/mcp` read in full for net_worth.go, anomalies.go, result.go, server.go, tools.go, cap.go. No new port or Store method: `report.Server.Summary` (internal/report/summary.go:64-82), `document.NewSummary`, `document.SummaryWarnings(s, again, advice)`, `report.ParseMonth`, `report.MonthError{Kind,Value,Example}` all exist. Surface survey: the tool calls `s.now`, `s.newConfig`, `s.newReport`, `srv.Summary`, `report.CountFindings`, `capList`; nothing else.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_mcp_documents_helpers_test.go:21-60` add `now time.Time` to `toolDocumentRun` (zero = `toolClock`) and `startMCPAt(ctx,t,now)` that `startClockedMCP` wraps; new `run_mcp_summary_test.go` `Test_run_mcp_monthly_summary_returns_the_summary_json_document` — `runBothSurfaces` vs `quarry summary --json` at `summaryClock` (2026-10-06; toolClock's default month is August). Seeder writes into the helper's `home` (`replaceStore(t, home, summaryRows(false))` then `(true)`): `seedSummaryStore` Setenv's its own HOME and would orphan `writeConfig`. Rows: default month; `month` 2026-08; `native`; `USD`; unknown-key config; unreadable config + `currency` (W2, `ignored` null); first-month store (`firstMonthRows`)
- [x] Step 2: `internal/mcp/tools.go:17-30,282-346` `toolMonthlySummary`, `monthlySummaryInput{Month *string; Currency string}`, registration LAST after acb (U12), description exactly spec :250 as one string; `month` = plain `{"type":"string"}` described by U11, no `pattern` (the SDK would refuse before the ruled isError text); stub `summary.go` `(*Server).monthlySummary` returns `document.Summary{}`. Stub turns `run_mcp_test.go:41-45` and `run_mcp_descriptions_test.go:272-290` red until step 5

### Build
- [x] Step 3 (batch 1, handler): new `internal/mcp/summary.go`; `result.go:24-37` `monthRefusedLog`. Order: `now := s.now()` once → `report.ParseMonth(in.Month, now)` → config → `s.newReport` → ONE `srv.Summary` → `document.NewSummary(summary, FindingsTally{CountFindings(summary.Status, ignore, class), IgnoreKnown}, slices.Concat(W1, W2, document.SummaryWarnings(summary, "call monthly_summary again", document.NativeParameter)))` (plain concat, as `cli.withConfigWarnings` internal/cli/currency.go:116). `summaryChoices(name)` loads config ONCE, never `resolveCurrency` (spending.go:49 skips the config when currency is given): readable → `cfg.WarningsAbsolute`, ignore, classification (even with `currency`); unreadable + no currency → `withLog(err, configRefusalLog("summary"))`; unreadable + currency → W2 `CannotTellChoices(config.ProblemAbsolute(err))`, `IgnoreKnown=false`. `monthRefusal` (model on `asOfRefusal`, holdings.go:63) words from `Kind/Value/Example`, never `MonthError.Error()` (CLI wording); fallback arm `// unreachable:` citing month.go's two return sites. Test support: `query_helpers_test.go:46-80` `fakeStore.Summary` (record params; `NetWorth` may stay empty: core builds both dates from `params.Dates`, summary.go:71) + `harness.monthlySummary`; `internal/mcp/summary_test.go` tests: `Test_monthly_summary_reads_today_once_at_the_start_of_every_call` (`steppingClock`, anomalies_test.go:39), `…_defaults_to_last_month`, `…_refuses_a_month_it_cannot_summarize` (NotAMonth: `2026-9`, `""`, `0000-01`, `2026-13`, `2026-09-15`; NotEnded: `2026-10`, `2027-01`; control `2026-09`; exact U7/spec :251 text and `monthRefusedLog` stderr), `…_refuses_a_month_before_it_reads_config_or_store` (`h.built==0`, config stub untouched), `…_reads_the_store_once` (`summaryAsked` length 1, `Through`=month end, `Dates`=two month ends; `statusReads==0`), `…_loads_the_config_once_per_call` (with and without `currency`), `…_counts_an_ignored_finding_when_currency_is_given`, `…_keeps_config_warnings_when_currency_is_given`, `…_refuses_an_unreadable_config_without_currency` (client text = err, stderr `cannot read quarry's config file; run quarry summary to see why`), `…_warns_when_the_config_is_unreadable_with_currency`; fault: factory error and `fakeStore.err` pass through unchanged
- [x] Step 4 (batch 2, warnings + caps): `summary_test.go`: `Test_monthly_summary_words_the_snapshot_warning_tail_for_the_tool` (W3a tail `then call monthly_summary again`, month absent AND present; W3b control with no tail), `…_words_the_rate_advice_for_a_tool_parameter` (W6 `pass currency native`), `…_caps_anomalies_charges_and_recurring_series_at_500` (500 = no line, 501 = cut + line, for each list; both capped → charges line then series line; nouns `charges`/`series`, advice `pass an earlier or later month`, tool name `monthly_summary`; fixtures from `payeeNamed`, anomalies_test.go / recurring_charges_test.go, dated so the listing charge falls in the month). `doc.Anomalies.Charges, doc.Warnings = capList(…)` then `doc.Recurring.Series, doc.Warnings = capList(…)`. `cmd/quarry/run_mcp_summary_test.go`: `Test_run_mcp_monthly_summary_words_the_tool_advice_not_the_flag` (CLI vs tool side by side as run_mcp_net_worth_test.go:77: W3a with and without `month`, W6), `Test_run_mcp_monthly_summary_refuses_a_month_in_mcp_words` (clock Oct 6 so the example reads 2026-09; text + stderr line, as :139)
- [ ] Step 5 (batch 3, registry, copy, tables): `internal/mcp/log_classes_internal_test.go:60-100` rows for `monthRefusedLog` in the classification table and the never-logs-the-value table; `timeout_test.go:57-100,125-138` `stallingStore.Summary` (nil embedded `report.Store` would panic) + row `monthly_summary stopped after 1 second; try again`; `cmd/quarry/run_mcp_test.go:41-45` tool list; `run_mcp_descriptions_test.go:230-303` description + schema consts and map row, Tools-line pin (U12); `run_mcp_no_store_test.go:34` row; `run_mcp_store_faults_test.go:31-110` rows `directoryStore` and `DELETE FROM import_runs` → `the store has no import history` (deterministic: status read first; not a dropped view); `internal/cli/mcp.go:38-40` + `mcp_test.go:43-45` Tools line (U12, ends `net_worth, acb, monthly_summary.`); `plugin/skills/quarry/SKILL.md:99` + `run_skill_text_test.go:241` (U13); `docs/initial-prd.md` new row between :205 and :206 (U14)

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments (budget: exported ~4 lines); `go doc ./internal/mcp` unchanged surface

### Verify
- [ ] Step 7: full verification per `.claude/rules/agent-briefs.md`, `spec-check.py phase4f-summary`, tick SCENARIO-16 with its acceptance test, STATE.md rewrite

## Handoff

**Binding decisions:**
- Explicit `month` gets the same W3a tail as the default: `then call monthly_summary again` (ruled in the dispatch brief) — the MCP call has no flags to echo.
- The MCP handler loads config once and never goes through `resolveCurrency` — summary is STATE's third config class (readable config always feeds ignore/classification/W1; unreadable refuses only without `currency`).
- `monthly_summary` is registered last (U12) and `month` is a plain string schema: a `pattern` would pre-empt the ruled isError wording.

**Default, pending ruling (orchestrator: scoped product-vision copy ruling before B1, or accept):**
- Month-refusal stderr line `quarry: mcp: monthly_summary: refused the call's month; details went to the client only` (class line, precedent `windowRefusedLog`/`yearRefusedLog`).
- Cap-line order when both lists cut: charges, then series.
- `instructions` (tools.go:43-54) left unchanged: no ruling adds the tool to it.
- W1 config warnings kept when `currency` is given (CLI `--json` parity; other MCP tools drop them in that case).

**Left unbuilt:** `references/monthly-summary.md`, README section, SKILL §4/§10 rows, PRD Decisions row, `run_skill_json_fields_test.go:33` `summary --json` row — SCENARIO-17.

**Traps:**
- `toolClock` (2026-09-29) makes the default month August and `month: 2026-09` "not ended": summary tests need the clock parameter from step 1.
- `fakeStore` and `stallingStore` embed a nil `report.Store`: an unimplemented `Summary` panics every test touching the tool.
- `MonthError.Error()` is CLI wording (`--month …`, `summary covers…`); the MCP text differs.
- Registering the stub in step 2 turns the tool-list and description tests red until step 5; that is expected.

**Orchestrator rulings 2026-10-06 (pre-dispatch):** defaults 1-4 stand — (1) `quarry: mcp: monthly_summary: refused the call's month; details went to the client only` (windowRefusedLog/yearRefusedLog precedent); (2) both caps: charges line then series line; (3) `instructions` unchanged; (4) W1 kept when `currency` given — parity with `summary --json` beats the other tools' precedent. Explicit-month W3 tail = `then call monthly_summary again` (binding).

## Phase report

Runs A and B1 done (steps 1-4 ticked). B1 started from a6ab29c1.

**Files (B1)**
- `internal/mcp/summary.go`: real handler `monthlySummary` (`now` once, `ParseMonth`, `s.summaryChoices(in.Currency)` loads config once, `newReport`, one `srv.Summary`, `NewSummary`, two `capList` calls, charges then series); `summaryChoice` (one `warnings` slice: W1 or W2, they never co-occur); `monthRefusal`/`monthWording` (from Kind/Value/Example); `parseCurrency` (also used by `resolveCurrency`, spending.go:49). `summaryAgain` = `call monthly_summary again`.
- `internal/mcp/result.go:37`: `monthRefusedLog`.
- `internal/mcp/query_helpers_test.go`: `fakeStore.summary/summaryAsked` + `Summary`, `harness.monthlySummary`.
- `internal/mcp/summary_test.go` (new, 35 leaf tests): every B1 test the plan names, plus the W3a/W3b/W6/cap tests of step 4 (`coveredSummary()` = status with a snapshot taken after September, so no W3 line). `anomalies_test.go`/`recurring_charges_test.go`: `unusualChargesIn(month, first, n)`, `monthlyChargesFrom(first, n, amount)`; the old helpers call them.
- `cmd/quarry/run_mcp_summary_test.go`: `Test_run_mcp_monthly_summary_words_the_tool_advice_not_the_flag` (3 rows), `Test_run_mcp_monthly_summary_refuses_a_month_in_mcp_words` (2 rows); seeders `seedSummaryNativeStoreAt`, `seedSummaryRatedStoreAt`, `seedSummaryFirstMonthStoreAt` (thelper).

**Green now**: acceptance test (7 subtests), all `internal/mcp`, `internal/cli`, `cmd/quarry -run run_mcp_monthly_summary`; `golangci-lint run ./...` 0 issues. Still red by design until step 5 (B2): `Test_run_mcp_describes_every_tool/tools/list_and_instructions`, `Test_run_mcp_lists_quarrys_tools_over_json_rpc`.

**Mutations (B1)**: (1) `summaryChoices` returns early when `name != ""` (skips config): reddens `…_loads_the_config_once_per_call/with_a_currency` (commands nil), `…_counts_an_ignored_finding_when_currency_is_given` (Ignored nil), `…_keeps_config_warnings_when_currency_is_given`, `…_warns_it_cannot_tell…_with_currency`. (2) a second `s.now()` read beside the first: reddens `…_reads_today_once_at_the_start_of_every_call` (month 2026-10 not 2026-09; reads 4 not 2). Both restored, diff clean.

**Next runs must not undo**: no `pattern` on `month`; the handler is not routed through `resolveCurrency`; every other `run_mcp_*` test runs at `toolClock`. B2 (step 5): registry/copy/tables only — nothing in `summary.go`.
