---
id: SCENARIO-02
status: open
---

# SCENARIO-02: spending returns the spend --json document

Cadence: code-first — no write guard, atomic adapter or bug fix touched
Acceptance test: `cmd/quarry/run_mcp_spending_test.go` `Test_run_mcp_spending_returns_the_spend_json_document`
Acceptance test (SCENARIO-06, folded): `cmd/quarry/run_mcp_spending_test.go` `Test_run_mcp_spending_reads_the_config_only_when_currency_is_absent`
Acceptance test (SCENARIO-07, folded): `cmd/quarry/run_mcp_spending_test.go` `Test_run_mcp_spending_words_its_warnings_with_the_tool_name`
Acceptance test (SCENARIO-12, folded): `internal/mcp/spending_test.go` `Test_spending_reads_today_once_at_the_start_of_every_call`
Narrow loop: `go test ./internal/mcp/ ./internal/report/document/ && go test ./cmd/quarry/ -run 'Test_run_mcp|Test_run_prints_spend'`
Mutation checks: per-call clock read in `(*Server).spending` hoisted to `NewServer`/`Serve` (or read twice per call) → `Test_spending_reads_today_once_at_the_start_of_every_call`; currency-absent condition dropped (config always loaded) → S06 row "CAD + unparseable config"
Runs: A (1-2) | B1 (3-5) | V (6-7)
Size: OWNS A RUN — 3 batches + equality harness, 0 new feature packages (mcp delivery + cmd/quarry tests)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: new `cmd/quarry/run_mcp_documents_helpers_test.go` — reusable CLI-vs-MCP equality harness (S09/S10 reuse): runs `<cli> --json` via `runWith` with `spendEnvAt(fixed)` (`run_helpers_test.go:83-92`), starts `startMCP` (`run_mcp_test.go:144-165`) with `env.ServeMCP = newMCPServe(nil, mcp.WithClock(fixed))`, `json.Compact`s CLI stdout, strips the `warnings` member from both compact byte strings and compares them byte-equal against the tool's `TextContent` (never the decoded map); maps CLI warnings on the left-out template only (`, so <cliWord> leaves it out` → `, so <tool> leaves it out`) with two controls: mapped list ≠ raw CLI list, and the before-first-rate line passes unchanged. New `cmd/quarry/run_mcp_spending_test.go` `Test_run_mcp_spending_returns_the_spend_json_document` over `populatedAnalysisStore` (`run_analysis_documents_test.go:132-157`): row "all five given" (since, until, the four `populatedAccounts`, currency CAD, by payee), row "none given" (defaults: by category, window from the clock, CAD with no config)
- [x] Step 2: `internal/mcp/server.go:36-41,71-78` `WithClock` option + `now` field (signature only); `internal/mcp/tools.go:116-122,172-201` `toolSpending` const, `spendingInput`, registration with a placeholder schema and a stub `(*Server).spending` in new `internal/mcp/spending.go` that returns an empty document — the test must fail at the equality assertion, not on "unknown tool"

### Build
- [ ] Step 3: `server.go:71-78` `NewServer` defaults `now` to `time.Now`; `spending.go` reads `s.now()` exactly once at handler start. `internal/mcp/spending_test.go` `Test_spending_reads_today_once_at_the_start_of_every_call` — fake clock advancing per read (23:59 day 1, 00:01 day 2) with a read counter; two calls without since/until → document `until` day 1 then day 2, reads == 2. Shipped-default pin: the spending row in `run_mcp_no_store_test.go:20-29` (via `newMCPServe`, no `WithClock`; a nil clock panics before the store)
- [ ] Step 4: `tools.go:172-201` spending schema per §3.2: `since`/`until` `{"type":"string"}`, `accounts` string array, `currency` enum `CAD|USD|native` exact case with no `default`, `by` enum from `store.SpendingGroups()` `String()` (helper beside `findingTypes`, `tools.go:233-241`) default `category`, `additionalProperties:false`; §3.3 description const; §3.4 per-param descriptions (`since`/`until`/`accounts` shared with cash_flow, `currency` shared by all four — build them as reusable property builders). Pins: `cmd/quarry/run_mcp_test.go:20-85,87-127` spending row (description + schema consts, `by` enum literal `category, payee, tag, month`, `require.Len` 5; keep the test name, S13 renames it); SDK refusal rows in `spending_test.go` — `since: null`, `currency: "cad"`, unknown property → isError + `argsRefusedLog` on stderr
- [ ] Step 5: `spending.go` handler in §3.2 order: `*string` since/until → `report.ParseWindow(since, until, today)` → config via `s.newConfig(commandName)` only when currency absent (else `money.ParseCurrency`) → `s.newReport(ctx, commandName)` → `srv.Spend` → `cfg.WarningsAbsolute` then `document.SpendingWarnings(result, toolSpending)` → `document.NewSpending`. `data_quality.go:13-14,21` `configRefusalLog` becomes a per-twin line builder; spending passes `spend`, data_quality stays byte-identical (pins `data_quality_test.go:23`, `cmd/quarry/run_mcp_data_quality_test.go:81`). Tests: S06 `Test_run_mcp_spending_reads_the_config_only_when_currency_is_absent` (absent+USD config → USD doc; absent+no config → CAD; absent+unparseable → isError config line, stderr exactly `quarry: mcp: spending: cannot read quarry's config file; run quarry spend to see why`; CAD+unparseable → CAD doc, `warnings` exact with no config lines; absent+unknown key → absolute config line first); S07 `Test_run_mcp_spending_words_its_warnings_with_the_tool_name` (accounts Linked, Old Card, US Chequing → the §4.1 MCP lines verbatim, "run quarry sync" kept, before-first-rate line unchanged). Unit rows in `spending_test.go` over `fakeStore` (`query_helpers_test.go:33-74`, add `Spending` + `h.spending` beside `:129-135`): window error → isError + `failedLog`, config loader not called, factory not built (assert no client text); broken config + factory error → config refusal, `h.built == 0`; currency given → loader never called; factory error → `failedLog`; `Spend` `RefusalError` → its text verbatim; `Spend` plain error → `failedLog`. Rows in all-tools tables: `timeout_test.go:84-110` (+ `stallingStore.Spending`, `:21-65`, failing as the real adapter does) and `run_mcp_no_store_test.go:20-29`. Fold S01 checkpoint MINORs: `internal/report/document/warnings_composers_test.go:117,127` assert the exact lines, `:151-160` "none checked" asserts the empty-window string

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `WithClock`, `spending`, the property builders; S01 NITs: `internal/store/store_test.go:136,160` subtest `name` field, `internal/cli/json_internal_test.go:181` `spendWindow` doc line

### Verify
- [ ] Step 7: full verification + `.claude/scripts/spec-check.py phase3b-analysis-tools` → tick SCENARIO-02, -06, -07, -12 (folded lines name "delivered by SCENARIO-02" before the test reference); rewrite STATE.md

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `mcp.WithClock(func() time.Time)`, default `time.Now` in `NewServer`, read once at handler start; S10 passes that one value to `ParseChargeWindow` and `Request.Now` — Rule 12 (once per call), and two reads can straddle midnight. Departs from the `clean-architecture` skill's "no injected clock": spec Rule 12 allows it, `Env.Now` is the CLI precedent, and the equality harness needs one fixed instant on both surfaces over real DuckDB (synctest's clock cannot match the CLI's)
- `newConfig` and `newReport` keep `commandName` (`mcp`): the `$HOME` refusal says "run quarry mcp again". Only the config-refusal stderr line names the CLI twin, via the per-twin builder (`spend`, `cashflow`, `recurring`, `anomalies`; data_quality `findings`)
- Shared property builders for since/until/accounts/currency: S09 reuses since/until/accounts/currency as-is; S10 swaps in its §3.4 since/until/accounts strings, keeps currency. `currency` never gets a schema `default` (it would override `reporting.currency`)
- Handler order: window → config (only when currency absent) → factory → Spend; the cash_flow/recurring/anomalies handlers copy it
- Equality harness (`run_mcp_documents_helpers_test.go`) is the acceptance shape for S09/S10: compact bytes minus `warnings`, never the decoded map; fixtures must stay under the 500 cap

**Left unbuilt** — named so nobody assumes it exists:
- MCP window refusal wording + `refused the call's since or until; ...` class line — S03 (today a `WindowError` logs `failedLog`, client text still `--since`-worded)
- Account refusal wording/class lines and `OpenFaultOther` withheld line — S04 (today `logLine` logs a `RefusalError` verbatim; the gap lives only on this branch until S04)
- `null`/omitted/`{}` arguments rows for spending at `internal/mcp/server_test.go:163` — S14 in S09 (`absentNullArguments` already covers it generically)
- List caps and their warning lines — S09; cash_flow, recurring_charges, anomalies — S09/S10
- tools/list test rename, 3a per-param descriptions, instructions, query description, mcp Long — S13

**Traps** — things that look right and are not:
- The warning map must substitute on the `, so <word> leaves it out` template, never the bare word: `spend` sits inside `spending` (empty-window subject) and `recurring` inside `recurring charges`
- No S02 test may assert window-refusal client text or a verbatim account-refusal stderr line: S03/S04 flip both
- The `by` parse miss is unreachable behind the schema enum; mark it `// unreachable:`, do not add a handler default (the schema default plus `absentNullArguments` supplies `category`)

## Phase report

Run A done (steps 1-2). Acceptance and the three folded acceptance tests are red at their assertions; nothing else is built.

Files:
- `cmd/quarry/run_mcp_documents_helpers_test.go` — `toolDocumentRun`, `runBothSurfaces` (CLI `--json` via `runWith`+`spendEnvAt(toolClock)`, then MCP via `startClockedMCP`), `splitWarnings` (compact bytes minus the trailing `warnings` member), `inToolWords(cliWarnings, cliWord, tool)` (maps only `, so <word> leaves it out`), `toolClock` (2026-09-29 12:00 UTC)
- `cmd/quarry/run_mcp_spending_test.go` — S02 `Test_run_mcp_spending_returns_the_spend_json_document` (rows: all five given, none given); S06 `..._reads_the_config_only_when_currency_is_absent` (4 document rows + refusal subtest, asserts stderr `quarry: mcp: spending: cannot read quarry's config file; run quarry spend to see why`); S07 `..._words_its_warnings_with_the_tool_name` (three verbatim lines); `Test_tool_warning_mapping_rewrites_only_the_left_out_template` (helper control, green on arrival by design)
- `internal/mcp/spending_test.go` — S12 `Test_spending_reads_today_once_at_the_start_of_every_call`, `steppingClock`, `decodeSpending`; `internal/mcp/query_helpers_test.go` — `fakeStore.Spending` (+ `spent` field), `harness.spending`
- stubs: `internal/mcp/server.go` `now` field + `WithClock` (no default yet); `internal/mcp/tools.go` `toolSpending`, `spendingInput`, placeholder registration (plain-typed props, no description); `internal/mcp/spending.go` returns `document.NewSpending(report.Spending{}, nil)`

Red now (all at their assertions, document `0001-01-01`/`native`/`[]` vs expected): the S02 test (both rows), S06 (all 5 subtests), S07, S12; plus `Test_run_mcp_lists_quarrys_four_tools_over_json_rpc` (5 tools, `require.Len` 4) which step 4 re-pins.

Next run (B1) must know:
- S06/S07/S12 tests already exist; steps 3 and 5 only add production + the remaining unit rows (window/config/factory/Spend fault rows, SDK refusal rows, all-tools table rows). Step 4 owns the tools/list pin (`run_mcp_test.go`) and the schema/description pins.
- S07 line for before-first-rate is `3 transactions dated before 2026-03-01 ...` over Linked, Old Card, US Chequing (verified against the CLI).
- `addTools` doc comment still says "four tools"; `NewServer` has no `now` default, so `Serve` with no `WithClock` would nil-panic in step 5 until step 3 sets it.
- Comparison is on `got.cliBody == got.toolBody`; warnings via `inToolWords`. S06 `config` field writes the file before both surfaces run.
