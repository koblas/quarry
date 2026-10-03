---
id: SCENARIO-03
status: open
---

# SCENARIO-03: spending refuses a bad window in MCP words

Cadence: code-first (no mandatory test-first item: no write-safety guard, no atomic adapter, no bug fix)
Acceptance test: `cmd/quarry/run_mcp_spending_test.go` `Test_run_mcp_spending_refuses_a_bad_window_in_mcp_words`
Narrow loop: `go test ./internal/report/ -run 'Window' && go test ./internal/mcp/ -run 'Spending|Window|ErrorLog' && go test ./cmd/quarry/ -run 'McpSpending|mcp_spending|Refus'`
Mutation checks: stderr carries only the class line (drop `withLog` in `windowRefusal`) → `Test_run_mcp_spending_refuses_a_bad_window_in_mcp_words`; CLI text stays `--`-worded (drop the `--` prefix in `WindowError.Error`) → `Test_parse_window_refuses_a_value_that_is_not_a_date` and the cmd/quarry `run_*_refusals` pins
Runs: A (1) | B1 (2-4) | V (5-6)
Size: OWNS A RUN — 3 small batches (2 sized + the S02 checkpoint fold), 2 packages (`internal/report`, `internal/mcp`; cmd/quarry is the test boundary only)

Surface survey (`ParseWindow`/`ParseChargeWindow`/`WindowError` callers, grep over `*.go`): `internal/cli/window.go:48-56` (stays as is: `UsageError{msg: err.Error()}`, so `Error()` byte-identity is the whole CLI contract), `internal/mcp/spending.go:19`, `internal/report/window_test.go`. No type-switch on `WindowError` outside tests. S10 is the next caller (charge window).

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_mcp_spending_test.go` (new test after `:75`) `Test_run_mcp_spending_refuses_a_bad_window_in_mcp_words` — table over the five ruled rows (spec SCENARIO-03 Examples, plus `until "2024-13"` for the `until` spelling), each through `startClockedMCP` on `populatedAnalysisStore`: `IsError`, client text exact, stderr exactly `spendingLogPrefix + "refused the call's since or until; details went to the client only\n"`. Add the class line as a const beside `:14-17`. Needs no stubs; red at the text/stderr assertion (today: `--since …` text, `failed; …` stderr). Read the CLI twin's stderr only for the bound-spelling contrast, never to build the expected line.

### Build
- [ ] Step 2: `internal/report/window.go:10-17,41-105` `WindowError`, `ParseWindow`, `ParseChargeWindow`, `parseWindow`, `parseDateBound` — drop `msg`; value type keeps exported parts: `Kind` (`WindowNotADate`, `WindowSinceAfterToday`, `WindowChargeSinceAfterToday`, `WindowSinceAfterUntil`, `WindowUntilBeforeDefault`), `Bound` (`since`/`until`, bare), `Value`, `Other` (until value, for since-after-until), `DefaultSince` (`2006-01-02` string), `Command`. `Error()` composes today's text per kind with `"--"+Bound`; `parseWindow` takes the future-since kind and command instead of the `sinceAfterToday` closure and no longer spells `"--since"`/`"--until"` (`:66,74,97`). Order of checks and since-first reporting unchanged. Tests in `window_test.go`: `Test_window_error_carries_its_parts` — one row per kind through the real `ParseWindow`/`ParseChargeWindow` (not-a-date for both bounds, `""` value, charge command); `Error()` text per kind stays pinned by the existing tests at `:48-66,107-134,154-225` (must stay green unedited). Doc comment on `WindowError` and kinds is the parts contract; update `ParseChargeWindow`'s doc (command is a part).
- [ ] Step 3: `internal/mcp/window.go` (new) `windowRefusal(err error) error` + `internal/mcp/result.go:20-23` const `windowRefusedLog` + `internal/mcp/spending.go:19-22` wiring — `windowRefusal` words every kind from the parts with bare `since`/`until` and the MCP tail (`pass until to include…`, `pass since too`, `… is after until …`, charge variant `since V is after today; <Command> lists charges up to today only, so pass an earlier since`), returns `withLog(<MCP text>, windowRefusedLog)`; a non-`WindowError` returns unchanged (mark `// unreachable:`: ParseWindow returns only `WindowError`). The charge variant takes its command from the part, so S10 passes the tool name to `ParseChargeWindow` and calls `windowRefusal` unchanged (deviation from a `(err, tool)` signature: the part makes `tool` redundant). Replace the `//nolint:wrapcheck` comment at `spending.go:21`. Tests: `internal/mcp/window_internal_test.go` (white-box, like `log_internal_test.go`) `Test_windowRefusal_words_every_kind` — all five kinds incl. charge variant with `recurring_charges` and `anomalies`, `until` spelling, empty value, and that the recorded log line is the class line for each (value never in it); flip `spending_test.go:58-68` to `Test_spending_refuses_a_window_it_cannot_read_…` asserting the class line, not `failedLogLine`, and keep its config/store-not-touched asserts.
- [ ] Step 4: S02 checkpoint folds — `internal/mcp/spending.go:15-17` doc cut to its first sentence (ordering fact, "today is read once", stays as a body comment beside `s.now()`); `cmd/quarry/run_mcp_spending_test.go:34-52` two rows in `Test_run_mcp_spending_returns_the_spend_json_document`: `currency: "native"` (CLI `--currency native`) on `populatedAnalysisStore`, and `by: "tag"` on `multiTagAnalysisStore` (`run_analysis_documents_test.go:174`). Test-only apart from the doc; both rows must pass first run (document logic exists), say so.

### Sweep
- [ ] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues` (the `exhaustive` linter will list the kind switches); `go doc ./internal/report WindowError` reads as the parts contract.

### Verify
- [ ] Step 6: full verification block, `.claude/scripts/spec-check.py phase3b-analysis-tools`, tick SCENARIO-03 with its acceptance test, rewrite STATE.md, `status: done`. CLI pins `cmd/quarry/run_{spend,cashflow,recurring,anomalies}_refusals_test.go` must pass unedited.

## Handoff

**Binding decisions:**
- `report.WindowError` is a value type of exported parts (`Kind`, `Bound`, `Value`, `Other`, `DefaultSince`, `Command`); `Error()` alone words the CLI line with `--`, so cli keeps `UsageError{msg: err.Error()}` and no CLI code reads parts — S10's `ParseChargeWindow(<tool name>, …)` makes the charge variant say the tool name through `Command`.
- `windowRefusal(err)` is the only MCP wording of a window refusal and attaches the class line itself; S09 (`cash_flow`) and S10 call it, none re-words.
- `windowRefusedLog` is a `result.go` const beside `argumentsRefusedLog`/`failedLog`; S04's account class lines join it there.

**Left unbuilt:**
- account refusal parts, wording and class lines, `OpenFaultOther` withheld line, `logLine`'s verbatim `RefusalError` arm (`result.go:55`) — S04.
- `cash_flow`, `recurring_charges`, `anomalies` handlers (S09/S10); the charge variant is worded and unit-pinned here but reaches no tool until S10.

**Traps:**
- `Bound` is bare (`since`), the CLI prefix lives only in `Error()`; MCP re-spelling by string-replacing `--` on `Error()` would also eat `--` inside a caller value.
- Value quoting differs by kind: only not-a-date uses `%q`; the rest print the raw value (today's text). Do not unify.
- `windowRefusal` must not leak `Value`/`Other` into the log line; the class line is a constant.
