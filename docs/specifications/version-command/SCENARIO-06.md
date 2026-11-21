---
id: SCENARIO-06
status: done
---

# SCENARIO-06: The store records the same version string

Cadence: code-first (no mandatory test-first item: no write-safety guard, atomic adapter or bug fix)
Acceptance test: `cmd/quarry/run_version_test.go` `Test_run_sync_records_the_resolved_version_in_the_store`
Narrow loop: `go test ./cmd/quarry/ -run 'Test_run_sync_records|Test_defaultEnv|Test_run_stamps|Test_buildVersion'`
Mutation checks: `envFor` passes nil info to `newServerFactory`
Runs: L | V
Size: LIGHT — 3 steps, cmd/quarry

Surface & Copy delivered: Store/MCP line, store half (`store_info.quarry_version`, `status --json` `.store.quarry_version` = `(devel) 260d0d29f940+dirty`) asserted by the acceptance test. No new copy.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `Test_run_sync_records_the_resolved_version_in_the_store` in `run_version_test.go`: sync a one-account fixture over `envFor(devInfo, ..., fixedRates())`, assert `SELECT quarry_version FROM store_info` and `status --json` `.store.quarry_version`. Stub `envFor` (signature only, delegating to `defaultEnv`) in `run.go`. Red: store says `(devel)`.

### Build
- [x] Step 2: `newServerFactory(info, storeOpts...)` drops its own `debug.ReadBuildInfo()` (`run.go:57,70`); `defaultEnv` becomes `envFor(readInfo, ...)`, `envFor` builds the Env and passes its info to `newServerFactory` and `newMCPServe`. Callers get `info` (6 sites: `main_test.go:40`, `run_store_test.go:631`, `run_sync_lock_test.go:163`, `run_sync_rates_test.go:67,845`, `run.go:197`).
- [x] Step 3: wiring pin = the acceptance test over `envFor`; `defaultEnv` -> `envFor` delegation already pinned by `Test_defaultEnv_carries_this_binarys_build_info`.

### Sweep
- [x] `go build ./... && golangci-lint run ./...` zero issues; doc comments.

### Verify
- [x] `verify.sh`; spec tick; `spec-check.py`; STATE.md.

## Handoff
`newServerFactory` takes the info; MCP half (`newMCPServe(info)` already takes it) is SCENARIO-07, untouched here. SCENARIO-07's plan must drive `quarry mcp` initialize through `envFor(info, ...)` and assert `serverInfo.version`: the `ServeMCP: newMCPServe(info)` link in `envFor` is unpinned while the `startMCP` helpers (`run_mcp_test.go:319,414,1079`) hardcode `newMCPServe(nil, ...)`. Mutation `ServeMCP: newMCPServe(nil)` in `envFor` must redden it.

## Phase report
Run L done. Acceptance red at both assertions (`(devel)` vs `(devel) 260d0d29f940+dirty`), now green. Files: `cmd/quarry/run.go` (`newServerFactory(info, ...)` ~:57, `defaultEnv`/`envFor` ~:205-235), `cmd/quarry/run_version_test.go:107-139` acceptance, 5 caller sites in `main_test.go:40`, `run_store_test.go:631`, `run_sync_lock_test.go:163`, `run_sync_rates_test.go:67,845` (`valetServerFactory` passes nil: no env in scope).
Mutations (red, restored): `envFor` -> `newServerFactory(nil, ...)` -> acceptance (both assertions); `defaultEnv` -> `envFor(nil, ...)` -> `Test_defaultEnv_carries_this_binarys_build_info`.
`golangci-lint run ./...`: 0 issues. Not yet run: `verify.sh` full suite, spec tick, `spec-check.py`, STATE.md (run V). `envFor` is new: `defaultEnv` delegates to it; acceptance drives sync over it with `fixedRates()`.
