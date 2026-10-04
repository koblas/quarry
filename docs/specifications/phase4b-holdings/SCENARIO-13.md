---
id: SCENARIO-13
status: done
---

# SCENARIO-13: MCP holdings tool

Cadence: code-first (no write-safety guard, atomic adapter or bug fix)
Acceptance test: `cmd/quarry/run_mcp_holdings_test.go` `Test_run_mcp_holdings_returns_the_holdings_json_document`
Narrow loop: `go test ./internal/mcp/ -run 'Holdings'` and `go test ./cmd/quarry/ -run 'run_mcp'` and `go test ./internal/cli/ -run 'MCP'`
Mutation checks: none
Runs: L | V
Size: LIGHT — 3 steps, mcp
Copy: ruled — tool description (S.5, one line, `Test_run_mcp_describes_every_tool`). Worded here from sibling shape, no S.5 text: `as_of` / `accounts` parameter descriptions; refusals `as_of "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD` and `as_of 2027-01-01 is after today; holdings are valued up to today only, so pass an earlier as_of`; stderr class line `refused the call's as_of; details went to the client only`; `holdings` appended to the `quarry mcp --help` Tools line (S.7 does not list it).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_mcp_holdings_test.go` — `runBothSurfaces` (`run_mcp_documents_helpers_test.go:40`) over `holdingsRows()` + `usdRate`, rows: as_of + currency given; none given (today = `toolClock`, no warnings); `as_of: "2026"` (current year resolves to today); `accounts: []` on an empty day (store-wide wording, `account_filter` `[]`); `accounts` + native. CLI body == tool body, warnings equal. Red: unknown tool

### Build
- [x] Step 2: `internal/mcp/holdings.go` handler `s.holdings` (as_of → config → report → `Server.Holdings`, `s.now()` once, no `capList`), `tools.go` registration, `holdingsInput`, `asOfRefusal` (`withLog`, wording above); unit tests in `internal/mcp/holdings_test.go` (clock once, bad and future `as_of` before config and store, `""`, config log, currency skips config, schema refusals, factory/store faults) + `fakeStore.Holdings`; cmd tests: unknown/ambiguous account via `refuseAccountKeepingItsNameOffStderr`, as_of refusals with stderr class line
- [x] Step 3: tool-list pins — `run_mcp_test.go:42-45`, `run_mcp_descriptions_test.go` (description, `mcpHoldingsInputSchema`, Tools line), `run_mcp_no_store_test.go`, `run_mcp_store_faults_test.go`, `internal/cli/mcp.go:38-39` + `mcp_test.go:44`

### Sweep
- [x] Step 4: `go build ./... && golangci-lint run ./...` down to `0 issues`

### Verify
- [x] Step 5: full verification, `spec-check.py`, tick SCENARIO-13, rewrite STATE.md

## Handoff

- MCP `holdings` lists at most 500 holdings (ruled, S.5); `internal/cli/mcp.go` "every list stops at 500 entries" is true again.
- SKILL.md §9 tool mapping and the PRD MCP table name no `holdings` yet — SCENARIO-15.

## Phase report

Run V done: sweep (`go build`, lint `0 issues`), full covered suite green (rc 0), `uncovered-diff.py` 0 uncovered (1 declared unreachable, `holdings.go:58`), `go test -race ./internal/mcp/` ok, spec tick, `spec-check.py` OK, STATE.md rewritten.
- Ruled cap built test-first in `internal/mcp/holdings.go:40-43`: `doc.Holdings` cut to `maxRows` after `document.NewHoldings` (totals and warnings over all), cut note appended last. `listCutWarning` (`describe_schema.go:33`) gained `tool` and `table` params; describe_schema call sites pass `toolDescribe`.
- Tests (`internal/mcp/holdings_test.go`): 501 -> 500 listed + note + total 501.00 (red: 501 != 500); exactly 500 no note (control, green on arrival); note ordered after config and no-price warnings (red: 2 != 3 warnings). Mutation `total > maxRows` -> `>=` reddened `Test_holdings_lists_exactly_500_with_no_cut_note`; restored.
- Counts vs a9a73b1: cmd/quarry 716 (+3), internal/cli 476 (+0), internal/mcp 160 (+14), total 1352 (+17).
