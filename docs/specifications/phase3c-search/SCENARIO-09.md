---
id: SCENARIO-09
status: open
---

# SCENARIO-09: search_transactions returns the quarry search --json document

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_mcp_search_test.go` `Test_run_mcp_search_transactions_returns_the_search_json_document`
Acceptance test (SCENARIO-11, folded): `cmd/quarry/run_mcp_search_test.go` `Test_run_mcp_search_transactions_cuts_to_its_limit_with_the_mcp_cut_line`
Acceptance test (SCENARIO-12b, folded): `cmd/quarry/run_config_test.go` `Test_run_mcp_search_transactions_ignores_a_malformed_config`
Narrow loop: `go test ./internal/mcp/ ./cmd/quarry/ -run '(?i)search|describes_every_tool|lists_quarrys_tools|before_the_first_sync|store_read_fault|deadline|absent_null'`
Mutation checks: limit schema `Default` deleted → `Test_a_search_call_with_absent_null_or_empty_arguments_searches_the_newest_500`; `windowRefusal(err)` → `err` in the handler → `Test_search_transactions_refuses_a_bad_window_in_the_tools_words`; `accountRefusal(err)` → `err` → `Test_run_mcp_search_transactions_refuses_an_account_without_its_name_on_stderr`; `Search` error site returns `errors.New(err.Error())` (breaks the deadline chain; a detached-ctx mutant hangs the stalling store, do not use it) → timeout row `search_transactions` in `Test_each_tool_answers_its_deadline_with_its_ruled_line`; cut-wording condition inverted → `Test_search_transactions_words_its_cut_line_by_the_limit_asked` and S11's acceptance; cut append deleted → S11's acceptance; handler passes `nil` warnings instead of `document.SearchWarnings` → no-match row of the acceptance test; handler calls `s.resolveCurrency("", …)` → S12b's acceptance
Runs: A (1-2) | B1 (3-5) | V (6-8)
Size: OWNS A RUN — 3 batches, no feature package changed (internal/mcp delivery + cmd/quarry pins; report and document reused unchanged)

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_mcp_search_test.go` (new) `Test_run_mcp_search_transactions_returns_the_search_json_document` — table through `runBothSurfaces` (`run_mcp_documents_helpers_test.go:40-64`, unchanged) over `searchStore()` (`run_search_json_test.go:16`): rows "nothing given" (every row, transfer/excluded/left-out/linked flags), "all eight given" (text, since, until, accounts, category, min, max, limit; ≥1 match), "no match" and "no match with accounts named". Assert `cliBody == toolBody` AND `toolWarnings == cliWarnings` directly (no `inToolWords`: no word to map; without the warnings assert the no-match mutant survives). Red must FAIL at assertion, never panic: harness already `require`s `IsError` false first
- [ ] Step 2: `internal/mcp/tools.go:17-26` `toolSearch`; `:144-188` `searchInput` (`Text`, `Since`, `Until`, `Category`, `Min`, `Max` all `*string`; `Accounts []string`; `Limit int`); `:115-142` `searchDescription` + 8 `search*Description` consts verbatim from spec §2.7; `:190-229` `sdk.AddTool` with `objectSchema` (strings for text/since/until/category/min/max, `accountsSchema(...)`, limit `described(searchLimitDescription, limitSchema(maxRows))`) and a stub `(*Server).searchTransactions` in new `internal/mcp/search.go` returning an empty `document.Search`. Registration turns `run_mcp_descriptions_test.go:202` (`require.Len`) and `run_mcp_test.go:42-45` red — expected; run A reports them, Step 3 fixes them

### Build
- [ ] Step 3: `internal/mcp/search.go` `(*Server).searchTransactions` — order `report.CheckSearchText` → `report.ParseSearchAmounts` → `report.ParseSearchWindow` (`windowRefusal` on its error) → `s.newReport(ctx, commandName)` → `srv.Search` (`accountRefusal` at its error site) → `document.NewSearch(found, document.SearchWarnings(found))`; `Limit`, `Category` pointer (a given `""` stays non-nil) and `Text` passed through; no clock, no config, no `capList`. `internal/mcp/query_helpers_test.go:35-55` `fakeStore` gains a `Search` recording params and cutting its rows to `params.Limit` as the real store does; harness `search` method after `:183-190`. New `internal/mcp/search_test.go`: `Test_search_transactions_refuses_a_bad_window_in_the_tools_words` (not-a-date + since-after-until, stderr `windowRefusedLog`), factory-failure row (`failedLog`), limit/category/text passthrough row. Pins: `run_mcp_descriptions_test.go:179` rename to `Test_run_mcp_describes_every_tool`, `search_transactions` row in `wantTools` (`:191-201`) with byte-copy consts `mcpSearchDescription`/`mcpSearchInputSchema` beside `:150-175`; `run_mcp_test.go:42-45` name added; repoint `docs/specifications/phase3b-analysis-tools/specification.md:747` to the new name
- [ ] Step 4: all-tools rows — `run_mcp_no_store_test.go:20-32` `{tool: "search_transactions"}`; `run_mcp_store_faults_test.go:72-74` open row (`directoryStore`) and `:100-102` statement row (a table search reads dropped, `CASCADE`); `internal/mcp/timeout_test.go:80-83` `stallingStore.Search` (same shape as `Charges`; must land with the row or the embedded nil `report.Store` panics) + row at `:105-115`; `cmd/quarry/run_mcp_search_test.go` `Test_run_mcp_search_transactions_refuses_an_account_without_its_name_on_stderr` → `refuseAccountKeepingItsNameOffStderr` (pattern `run_mcp_anomalies_test.go:77-79`, prefix const `searchLogPrefix`); `internal/mcp/server_test.go` after `:181-196` `Test_a_search_call_with_absent_null_or_empty_arguments_searches_the_newest_500` (omitted/null/`{}`: store asked `Limit` 500, doc `limit` 500). Cancel row n/a: `errorLog` silences every tool on `ctx.Err()` (`result.go:145`), pinned once by `run_mcp_cancel_test.go`
- [ ] Step 5: `internal/mcp/search.go` cut line appended as the only warning when `found.Truncated()`: limit == `maxRows` → §2.6 "narrow the search" wording, limit < `maxRows` → "pass a higher limit, up to 500, or narrow" wording; N = rows listed, M = `Matched`, both `humanize.Thousands`. `internal/mcp/search_test.go` `Test_search_transactions_words_its_cut_line_by_the_limit_asked`: limit 500 vs 499 (just outside), limit 20, matched 1,234 (comma visible), not truncated → no line. S11 acceptance `Test_run_mcp_search_transactions_cuts_to_its_limit_with_the_mcp_cut_line` over `manySearchTxns(501)` (`run_search_helpers_test.go:134`, pattern `run_search_limit_test.go:13-47`): limit 20 and absent; rows, newest/oldest id, `matched` 501, `truncated`, exact warnings, empty stderr. S12b `cmd/quarry/run_config_test.go` after `:310-319` `Test_run_mcp_search_transactions_ignores_a_malformed_config`: `malformedConfigFixture` + `startClockedMCP` (startup reads no config: `run.go:137-152`), not `IsError`, a known payee in the doc, `warnings` `[]`, empty stderr. Do NOT add search or mcp to `readCommandArgs` (`run_config_test.go:218-226`)

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `searchTransactions`, `searchInput`, the cut-line func (budget: 1-2 lines)
- [ ] Step 7: fold S04 checkpoint MINORs/NITs from STATE.md `## Open debts` (duckstore `search_category_test.go:~108` named-accounts UnknownCategory row; `run_search_category_test.go` text-mode `--category Travel` row; `run_search_refusals_test.go:111-113` blank line, `:33-38` literals; `duckstore/search.go:~27-30` one-line doc; top-value Amount assert) — tests and docs only, no runtime behaviour

### Verify
- [ ] Step 8: full verification + `spec-check.py phase3c-search` and `spec-check.py phase3b-analysis-tools` → tick SCENARIO-09, SCENARIO-11 and SCENARIO-12b (folds "delivered by SCENARIO-09") with their acceptance tests

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Handler order is `CheckSearchText` → `ParseSearchAmounts` → `ParseSearchWindow` → factory → `Search` — Rule S9; S10's wrappers wrap at those sites, never reorder
- `searchInput` fields are pointers (`Text`, `Category`, `Min`, `Max`, `Since`, `Until`) — nil = absent, `""` = given and refused (§2.7 table); a `string` field loses the distinction
- Cut line lives only in the MCP handler, keyed on the limit asked (`== maxRows` vs less), appended after `SearchWarnings`; no `capList` — Rule S7 store-side limit
- Description and parameter text live in `tools.go` consts, byte-copied in `Test_run_mcp_describes_every_tool` — S10 edits both for instructions/query/Long

**Left unbuilt** — named so nobody assumes it exists:
- MCP wording wrappers for blank text, amount (not-an-amount, min>max) and unknown category, plus their class lines for text and min/max — SCENARIO-10. Until then: blank/amount client text is the CLI line (`search text is blank…`, `--min "-12"…`) and stderr is `failed; details went to the client only` (`logLine` → `failedLog`, `result.go:60-70`); category client text is the CLI `quarry sql` line, stderr already `unknownCategoryLog` (`result.go:79`). No caller value reaches stderr: the gap is client wording and class lines, not a Rule 4 leak
- Rule S9 MCP order rows (bad min + bad since → min refusal; blank text + bad min → text refusal) and SDK rows (min as JSON number, limit 0, limit 501) — SCENARIO-10
- `instructions`, `queryDescription`, `quarry mcp` Long tool list + the pin's Long fragment (`run_mcp_descriptions_test.go:219`), PRD row — SCENARIO-10 (S13 fold)

**Traps** — things that look right and are not:
- No S09 test may pin blank/amount/category client text or the `failed;` stderr line — S10 changes both
- `limitSchema`'s `Minimum: 1` (`tools.go:255`) is the only thing keeping limit 0 (store: "no limit") off MCP
- Equality harness strips warnings: no-match identity is proven only by the explicit `toolWarnings == cliWarnings` assert
- `fakeStore`'s `Search` must cut to `params.Limit`, or the cut line's "newest N" tests nothing
