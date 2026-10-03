---
id: SCENARIO-10
status: open
---

# SCENARIO-10: search_transactions refuses bad input without the caller's values on stderr (folds SCENARIO-13)

Cadence: code-first (no bug fix, write-safety guard or atomic adapter; the Rule 4 stderr guards are red-proved by mutation, below)
Acceptance test: `cmd/quarry/run_mcp_search_refusals_test.go` `Test_run_mcp_search_transactions_refuses_bad_input_without_the_callers_values_on_stderr`
Acceptance test (SCENARIO-13, folded): `cmd/quarry/run_mcp_descriptions_test.go` `Test_run_mcp_describes_every_tool`
Narrow loop: `go test ./internal/mcp/ ./internal/cli/ -run 'Search|Refusal|logLine|MCP|Mcp'` and `go test ./cmd/quarry/ -run 'Test_run_mcp_search|Test_run_mcp_describes'`
Mutation checks: blank-text wrapper returns err unchanged → `Test_search_transactions_refuses_blank_text_and_a_bad_amount_before_building_a_report`; amount wrapper words from `Error()` with `--` stripped (string replace) or returns err unchanged → `Test_amountRefusal_words_each_amount_error_from_its_parts`; category wrapper identity → `Test_categoryRefusal_words_unknown_and_leaves_the_rest`; text/amount `withLog` swapped for `verbatim` → acceptance test (caller value on stderr); handler order amounts-before-text → `Test_run_mcp_search_transactions_refuses_in_the_ruled_order`
Runs: A (1-2) | B1 (3-4) | B2 (5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (`internal/mcp`; `internal/cli` Long const and `cmd/quarry` tests are wiring). Absorbs SCENARIO-13.

Read: STATE.md only (no prior SCENARIO file); `go doc`-level survey skipped, no new port. Glob/broad grep: none needed beyond string greps for copy (`David`, `Tools:`) which LSP cannot do.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_mcp_search_refusals_test.go` (new) acceptance test — table over spec's 8 outline rows (text `"  "`, min `"-12"`, min 50/max 20, since `"2024-13"`, accounts `["Nope"]`, category `"Fod"`, min as JSON number, limit 501) via `startClockedMCP` + `replaceSearchStore` (`run_mcp_search_test.go:102`); each row asserts IsError, exact client text (SDK rows: `Contains` on the field name only), exact stderr `searchLogPrefix + class + "\n"` (`run_mcp_search_test.go:13`), caller value (`-12`, `Fod`, `Nope`, `2024-13`) absent from stderr. Red today for text/min/min>max/category rows (client text is the CLI line; stderr is `failed; …` or the CLI line); since/accounts/SDK rows are green on arrival.
- [ ] Step 2: `cmd/quarry/run_mcp_descriptions_test.go:14-33` `mcpInstructions`, `mcpQueryDescription`; `:250` Long fragment; `internal/cli/mcp_test.go:43-44` Long pin — byte-copy spec §2.8 items 1-3 verbatim (instructions says "the user's", `search_transactions` sentence, query description redirect, Tools list ending `anomalies, search_transactions.`). `Test_run_mcp_describes_every_tool` and the cli Long test go red at their assertions.

### Build
- [ ] Step 3 (batch 1, MCP wrappers): `internal/mcp/result.go:25-32` add `textRefusedLog` and `amountRefusedLog` (class lines from spec §4.2); `internal/mcp/search.go:15-24` replace both `//nolint:wrapcheck` returns with wrappers (new `textRefusal`/`amountRefusal` in `search.go`, `categoryRefusal` beside `accountRefusal` at `accounts.go:22-31` or one shared helper; built like `windowRefusal` `window.go:12-18`: `withLog(<typed text error>, <class const>)`; category keeps `Unwrap` to the `RefusalError` so `logLine` stays `unknownCategoryLog`) and wire `categoryRefusal` at the `Search` error site (`search.go:34-36`). Words come from `report.AmountError` parts (`Bound`, `Value`, `Other`; `report/amount.go:36-49`) and `errors.Is(err, report.ErrBlankSearchText)`; never string-replace `Error()`. Exact lines: spec §3 MCP column. Tests: `internal/mcp/search_test.go:58-81` gains per-case exact client text, exact stderr (`HasPrefix(searchLogPrefix)` + class), `NotContains` caller value (closes S09 checkpoint MINOR); new `Test_amountRefusal_words_each_amount_error_from_its_parts` (min and max bound, not-an-amount incl. `%q` escape of a quote, min>max with raw values `50.00`/`20`, non-AmountError passes through unchanged); new `Test_categoryRefusal_words_unknown_and_leaves_the_rest` (empty `""` name → `no category named ""`, ambiguous/store/non-refusal pass through; reuse `categoryRefusalFor` `log_classes_internal_test.go:57`); `log_classes_internal_test.go:71-123` rows for the two new class lines through `logLine` of the wrapped errors.
- [ ] Step 4 (batch 2, matrix completion): `run_mcp_search_refusals_test.go` add `limit 0` SDK row to step 1's table (control: `limit 1` is not refused); new `Test_run_mcp_search_transactions_refuses_in_the_ruled_order` (min `"-12"` + since `"2024-13"` → min line and min class; text `"  "` + min `"-12"` → text line and text class; control row: since alone → window line); new `Test_run_mcp_search_transactions_over_stdio_answers_invalid_utf8_text_with_a_result` — raw JSON-RPC frames (pattern `run_mcp_exit_test.go:17,110-123`, `run_mcp_test.go:25-60`) with a literal `0xFF` byte inside `text`, asserts a non-error result with `matched` 0 and empty stderr (guards a decoder change; `Server.Search` never calls `CheckUTF8`). Folds (behaviour-neutral): `cmd/quarry/run_search_category_test.go:166-195` `Contains(stdout, "")` → `Equal` (`""` for the refusal row, exact for the no-match row); `internal/store/duckstore/search_amount_test.go:159-175` expected Amount into the case struct (no map/loop in the subtest body); `internal/mcp/search.go:13` order comment shortened to one line.
- [ ] Step 5 (batch 3, S13 production copy + PRD): `internal/mcp/tools.go:39-49` `instructions`, `:51-59` `queryDescription` (keep the `` `limit` `` concat) → §2.8 items 1-2 verbatim; `internal/cli/mcp.go:38-39` Long Tools line → item 3; `docs/initial-prd.md:~170` CLI table row `quarry search` and a Decisions line in `:321-340` (item 7 text, verbatim); `newRootCommand` doc comment (`internal/cli/root.go`) only if it lists subcommands without `search`. Step 2's tests go green.

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues` (`golangci-lint run --fix` rewrites asserted copy: review); doc comments on the new wrappers.

### Verify
- [ ] Step 7: full verification (`.claude/rules/agent-briefs.md`) + `spec-check.py phase3c-search`; tick SCENARIO-10 with its acceptance test and SCENARIO-13 as `— delivered by SCENARIO-10 — ` + `cmd/quarry/run_mcp_descriptions_test.go` `Test_run_mcp_describes_every_tool` (test ref last on line, line not wrapped, `specification.md:755-756`); rewrite STATE.md (drop the S10 Left-unbuilt and "David's" debt, S09 checkpoint MINOR debt, `nolint` lines).

## Handoff

**Binding decisions**
- Text and min/max class lines are `withLog` consts in `result.go`, not new `refusalLine` arms — `refusalLine` takes `report.RefusalError` and blank text / `AmountError` are not one; an unwrapped one still falls to `failedLog` (no value leaks).
- MCP wording is built from `AmountError` parts and `ErrBlankSearchText`; CLI `Error()` text is never edited — the CLI has `--` and unquoted values the MCP lines do not.
- Handler order stays `CheckSearchText` -> `ParseSearchAmounts` -> `ParseSearchWindow` -> `newReport` -> `Search` (Rule S9); S10 only wraps at those sites.
- Invalid UTF-8 has no MCP wrapper or class line (mid-feature ruling): pinned over stdio as a plain result only.

**Left unbuilt** (after S10, nothing is owed by this feature's scenarios)
- `platform/sqlite` `QueryRows` per-row `ctx.Err()`; duckdb `CAST(x*100 …)` overflow outside search — unowned debts.

**Traps**
- `Test_run_mcp_describes_every_tool` is already ticked by phase3b (`phase3b-analysis-tools/specification.md:747`); S13 reds it by editing the pin first (step 2), not by renaming. Two specs now cite one test.
- SDK rows (min number, limit 0/501): assert the SDK text only by field name; the exact wording is the SDK's and `argumentsRefusedLog` is the stderr line.
- Category refusal text must come from `refusal.Arg` with `%q`; `Arg` may be `""`. The CLI line (`list them with quarry sql …`) must not reach MCP clients.
