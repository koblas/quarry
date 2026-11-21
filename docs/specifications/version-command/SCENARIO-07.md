---
id: SCENARIO-07
status: open
---

# SCENARIO-07: The MCP server reports the same version string

Cadence: code-first (no mandatory test-first item; wiring already exists, so the acceptance test is expected green on arrival)
Acceptance test: `cmd/quarry/run_mcp_test.go` `Test_run_mcp_initialize_reports_the_resolved_version`
Narrow loop: `go test ./cmd/quarry/ -run 'Test_run_mcp_initialize|Test_newMCPServe_reports|Test_run_sync_records'`
Mutation checks: `envFor` `ServeMCP: newMCPServe(nil)`
Runs: L | V
Size: LIGHT — 3 steps, cmd/quarry

Surface & Copy delivered: Store/MCP line, MCP half (`serverInfo.version` = `(devel) 260d0d29f940+dirty`), asserted by the acceptance test. No new copy.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `run_version_test.go`: hoist SCENARIO-06's `(devel)`+revision+modified fixture to `dirtyDevelInfo()` and `dirtyDevelVersion` (reused, behaviour-neutral). `run_mcp_test.go`: `Test_run_mcp_initialize_reports_the_resolved_version` runs `quarry mcp` through `startMCP` whose tweak swaps the Env for `envFor(dirtyDevelInfo(), ...)` (keeping the pipe stdin), asserts `session.InitializeResult().ServerInfo` equals `{quarry, (devel) 260d0d29f940+dirty}`. Expected green on arrival (wiring exists); the mutation is the red evidence.

### Build
- [x] Step 2: add the `(devel)`+revision+dirty row to `Test_newMCPServe_reports_the_build_infos_module_version` (`run_mcp_test.go:109-117`).
- [x] Step 3: mutation `envFor` `ServeMCP: newMCPServe(nil)` must redden the acceptance test.

### Sweep
- [x] `go build ./... && golangci-lint run ./...` zero issues.

### Verify
- [ ] `verify.sh`; spec tick; `spec-check.py`; STATE.md.

## Handoff
Nothing left for a later scenario: MCP half wired and pinned. `(devel)` constants in `internal/mcp/server.go` and `internal/store/duckstore/duckstore.go` stay (out of scope).

## Phase report
Run L done. Acceptance `Test_run_mcp_initialize_reports_the_resolved_version` (`run_mcp_test.go:~35`) was GREEN ON ARRIVAL: `envFor` already passes `info` to `newMCPServe` (SCENARIO-06), so no production change. Red evidence is the mutation: `run.go` `envFor` `ServeMCP: newMCPServe(nil)` -> acceptance fails at its assertion (expected `(devel) 260d0d29f940+dirty`, actual `(devel)`); restored byte-identical.
Files: `run_version_test.go:~113` hoisted `dirtyDevelInfo()` / `dirtyDevelVersion` (06's acceptance now uses them, behaviour-neutral); `run_mcp_test.go` acceptance plus `a dirty development checkout` row in `Test_newMCPServe_reports_the_build_infos_module_version`. `startMCP` tweak swaps the Env for `envFor(...)` keeping the pipe stdin.
`go build ./... && golangci-lint run ./...`: 0 issues (Sweep ticked). Not yet run (run V): `verify.sh`, spec tick, `spec-check.py`, STATE.md rewrite (drop the SCENARIO-07 Left-unbuilt entry), `status: done`.
