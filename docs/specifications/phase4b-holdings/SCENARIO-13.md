---
id: SCENARIO-13
status: open
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
- [ ] Step 4: `go build ./... && golangci-lint run ./...` down to `0 issues`

### Verify
- [ ] Step 5: full verification, `spec-check.py`, tick SCENARIO-13, rewrite STATE.md

## Handoff

- MCP `holdings` is uncapped (S.5 "same document"); `internal/cli/mcp.go` says every list stops at 500 entries — ruling owed.
- SKILL.md §9 tool mapping and the PRD MCP table name no `holdings` yet — SCENARIO-15.

## Phase report

Run L done: plan written, steps 1-3 green; V owns steps 4-5 (sweep, full verify, tick, STATE).
- Red (step 1): all 6 rows of `Test_run_mcp_holdings_returns_the_holdings_json_document` failed at `run_mcp_documents_helpers_test.go:54` `require.NoError`: `calling "tools/call": unknown tool "holdings"` (the CLI half ran; the tool was absent, so no body assertion is reachable). Green after step 2.
- Production: `internal/mcp/holdings.go` (`holdings`, `asOfRefusal`, `asOfWording`, `asOfRefusedError`); `tools.go` (`toolHoldings`, `holdingsDescription`, `holdingsAsOf/AccountsDescription`, `holdingsInput`, registration); `result.go` `asOfRefusedLog`; `internal/cli/mcp.go:39` Tools line gained `, holdings`. `asOfRefusal`'s non-`AsOfError` branch is marked `// unreachable:`.
- Tests: `cmd/quarry/run_mcp_holdings_test.go` (acceptance table of 6 + as_of refusals + account refusals via the shared helper); `internal/mcp/holdings_test.go` (11 tests; `fakeStore.Holdings`/`held`/`heldAsked` and `h.holdings` in `query_helpers_test.go`); pins extended: `run_mcp_test.go:44`, `run_mcp_descriptions_test.go` (description, `mcpHoldingsInputSchema`, Tools line), `run_mcp_no_store_test.go`, `run_mcp_store_faults_test.go` (2 rows), `internal/cli/mcp_test.go:44`. Instructions const unchanged and pinned unchanged (`mcpInstructions`). Narrow loop green: `cmd/quarry` mcp/holdings/skill/plugin, `internal/mcp`, `internal/cli`.
- Copy worded here, not ruled (see header `Copy:`): as_of/accounts descriptions, two as_of refusals, class line, Tools line. Uncapped by design (no `capList`).
- V must not redo: nothing in `internal/mcp` is capped; do not add `capList` without a ruling. V still owes: lint, full covered suite, `uncovered-diff.py`, tick SCENARIO-13, STATE.md (add: S15 owns SKILL.md §9 tool mapping line `plugin/skills/quarry/SKILL.md:94` + `run_skill_text_test.go:236` and the PRD MCP table; open debt: 500-cap vs `internal/cli/mcp.go` "every list a tool returns stops at 500 entries" and `mcp --help` Tools line addition unruled).
