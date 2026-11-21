---
id: SCENARIO-07
status: done
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
- [x] `verify.sh`; spec tick; `spec-check.py`; STATE.md.

## Handoff
Nothing left for a later scenario: MCP half wired and pinned. `(devel)` constants in `internal/mcp/server.go` and `internal/store/duckstore/duckstore.go` stay (out of scope).

## Phase report
Run L: acceptance green on arrival (wiring from SCENARIO-06); red evidence is the mutation. Run V: checkpoint pin-only fixes applied: the test keeps `testEnv` and takes only `envFor(dirtyDevelInfo(), ...).ServeMCP`, uses `newHome(t)` and `peer.waitForExit(ctx, t)`, how-comment dropped. Mutation `envFor` `ServeMCP: newMCPServe(nil)` re-run: acceptance fails at its assertion (expected `(devel) 260d0d29f940+dirty`, actual `(devel)`); `run.go` restored byte-identical.
Verify: `verify.sh 398439ab...`: go build rc=0, go test rc=0, uncovered-diff rc=0 (no production lines added), go test -race rc=0, golangci-lint rc=0 (0 issues). test-stats: cmd/quarry 991 (+1) tests, tempdir 26 (+0), disk 75 (+0).
