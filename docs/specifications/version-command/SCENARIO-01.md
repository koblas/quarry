---
id: SCENARIO-01
status: done
---

# SCENARIO-01: quarry version prints the binary's version (folds SCENARIO-04, SCENARIO-05)

Cadence: code-first (no mandatory test-first item: no write-safety guard, atomic adapter or bug fix)
Acceptance test: `internal/cli/version_test.go` `Test_version_prints_the_binarys_version`
Acceptance test (SCENARIO-04, folded): `internal/cli/version_test.go` `Test_version_needs_no_store_config_or_home`
Acceptance test (SCENARIO-05, folded): `cmd/quarry/run_version_test.go` `Test_run_version_refuses_arguments`
Narrow loop: `go test ./internal/version/ ./internal/cli/ ./cmd/quarry/ -run 'Test_version|Test_Resolve|Test_buildVersion|Test_defaultEnv|Test_run_version|Test_run_help'`
Mutation checks: `vcs.modified` literal-`"true"` test in `Resolve` → `Test_Resolve` malformed/absent rows; `Env.BuildInfo: info` line in `defaultEnv` → `Test_defaultEnv_carries_this_binarys_build_info` (delete the line)
Runs: A (1-3) | B1 (4-6) | V (7-8)
Size: OWNS A RUN — 3 batches, 1 feature package (`internal/version`); folds 04 and 05

Placement: `internal/version` imports stdlib only (`runtime/debug`, `runtime`), no feature knowledge, so the dependency rule lets `internal/cli` and `cmd/quarry` import it. Callers of `Env` literals (grep, positive control hit known sites; all keyed literals in `internal/cli/*_test.go`, `cmd/quarry`): adding a field edits none. No new port: `Env.BuildInfo` is plain data. Survey n/a.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `internal/cli/version_test.go` `Test_version_prints_the_binarys_version` (six Outline rows through `cli.Execute`, Env carries only Stdout/Stderr/BuildInfo; asserts exact stdout `quarry <shown>\n`, empty stderr, nil error) and `Test_version_needs_no_store_config_or_home` (Env with every factory nil, `Home` ""; same stdout)
- [x] Step 2: `cmd/quarry/run_version_test.go` `Test_run_version_refuses_arguments` — `run(["version","foo"])` → exit 2, stdout empty, stderr `quarry: version takes no arguments\n` (sibling shape: `run_mcp_test.go:210`, `run_usage_test.go:422`)
- [x] Step 3: signature-only stubs — `internal/version/doc.go` + `version.go` `Resolve`/`Info` (zero Info); `internal/cli/run.go:44-58` `Env.BuildInfo *debug.BuildInfo` ("nil: no build info"); `internal/cli/version.go` `newVersionCommand(info *debug.BuildInfo)` registered in `root.go:44-45` with `Use`/`Short` only and **no `Args`** so 05 reds at its assertion. Tests 1-2 must fail at assertion before Build.

### Build
- [x] Step 4 (batch 1): `internal/version/version.go` `Resolve` + `version_test.go` `Test_Resolve` table — one row per `## Surface & Copy` edge row (tag, no vcs; pseudo-version clean/dirty with 40-char commit and modified false/true; `(devel)`+rev modified; rev shorter than 12 and exactly 12/13 as bound rows; modified absent; modified malformed `"True"`/`"1"`/`""` → null; empty `Main.Version`; `(devel)` no vcs; nil info) asserting all four `Info` fields, `Go == runtime.Version()` every row. Fallible arms: none (pure). Info shape fixed: `Commit *string`, `Modified *bool` (BR-V3 null).
- [x] Step 5 (batch 2): `internal/cli/version.go` `newVersionCommand` — `Use: version`, `Args: noArgs`, Short/Long verbatim from `## Surface & Copy` (no Example); `RunE` = `version.Resolve(info)` → `"quarry "+Version+"\n"` via `emit(cmd, out, "", nil)` (`output.go:44-58`, keep one `emit` call so 02 adds only a `renderResult` JSON arm; 02 adds the `jsonOut` parameter, do not add it unused now). `root.go:7-11` doc, `root.go:13-14` comment ("No Args or Run field" stays true in 01: root `RunE` is 03's), `root.go:29-45` register with `env.BuildInfo`. Tests in `version_test.go`: `Test_version_help_prints_the_ruled_long_text` (Long verbatim, every line ≤ wrap width, as `mcp_test.go` `mcpHelpLong` pattern), `Test_version_reports_a_failed_stdout_write` (`failingWriter` → error text `cannot write the result to stdout: <err>`), stdout-not-a-TTY same bytes is covered by the Buffer writer. `--json` is accepted-and-ignored until 02 (persistent flag): not pinned here.
- [x] Step 6 (batch 3): `cmd/quarry/run.go:88-93` `buildVersion` → `return version.Resolve(info).Version`; `run.go:192-209` `defaultEnv` sets `BuildInfo: info` (`info` already read :193); `main_process_test.go:164-179` nil/empty rows `""` → `(devel)` (tagged row unchanged); `run_status_test.go:126-143` insert `"  version     Show quarry's version\n"` after the `sync` line (Flags `--version` line is 03's). `run_version_test.go`: `Test_defaultEnv_carries_this_binarys_build_info` (`env.BuildInfo` non-nil; the end-to-end text is `(devel)` either way so only this pin can see a deleted wiring line), `Test_run_version_prints_without_a_home` (`t.Setenv("HOME","")`, `run(["version"])` → `quarry (devel)\n`, exit 0). Re-run `Test_newMCPServe_reports_the_build_infos_module_version` (`run_mcp_test.go:109-135`: nil row stays `(devel)` via `mcpTestServerVersion`).

### Sweep
- [x] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `Resolve`, `Info`, `Env.BuildInfo`, `newVersionCommand`; `go doc ./internal/version` reads as the contract; confirm `Test_every_quarry_name_the_skill_uses_exists` (`run_skill_drift_test.go`) still green with no skill edit.

### Verify
- [x] Step 8: `.claude/scripts/verify.sh <start> ./internal/version/... ./internal/cli/... ./cmd/quarry/...`, `spec-check.py version-command`; tick SCENARIO-01 with its acceptance test, and SCENARIO-04 and SCENARIO-05 each with "delivered by SCENARIO-01" before their test reference (test ref last on the line); write `STATE.md`.

## Handoff

**Binding decisions:**
- `internal/version.Resolve(*debug.BuildInfo) Info{Version string; Commit *string; Modified *bool; Go string}` is the single rule owner (BR-V1); `Go` is `runtime.Version()` even for nil info — 02 renders JSON straight from `Info`, nothing re-derives.
- `cli.Env.BuildInfo *debug.BuildInfo` is the only seam; `newVersionCommand` takes the info, never reads `debug.ReadBuildInfo()` itself — tests inject rows without a binary.
- `buildVersion` delegates to `Resolve(...).Version`, so the store and MCP now get `(devel)` instead of `""` for nil/empty info; 06/07 only change what they pass in.
- `version` emits through one `emit(cmd, out, "", nil)`; write failure is the existing `runtimeError` (exit 1).

**Left unbuilt:**
- `--json` document/`json_version.go` and `jsonOut` parameter on `newVersionCommand` — SCENARIO-02.
- Root `--version` flag + root `RunE`, Flags help line, refusal rows, regression pins — SCENARIO-03 (`root.go:13-14` comment is rewritten there, not here).
- `newServerFactory(info, …)` — 06; MCP `(devel) 260d0d29f940+dirty` row — 07. `(devel)` constants in `internal/mcp/server.go:18`, `duckstore.go:28` stay.

**Traps:**
- Wiring pin: under `go test` the text is `(devel)` whether or not `defaultEnv` sets `BuildInfo`; only the `env.BuildInfo != nil` assertion catches a deleted line.
- Until 02 lands, `quarry version --json` silently prints the text line (persistent flag accepted). Do not pin that behaviour.
- Cobra sorts Available Commands: registration position is irrelevant to the help pin; `version` sorts after `sync`.
- Stub without `Args` is deliberate so 05 reds; add `Args: noArgs` in Step 5 only.
- UNRULED input-integrity arms of the rewritten rule, to rule before run A (orchestrator → product-vision copy ruling): `vcs.revision` present but empty; duplicate `vcs.revision`/`vcs.modified` settings keys. Spec rules only "absent" and `vcs.modified` malformed. Plan assumes empty revision = absent (Commit nil, text `(devel)`) and does not test duplicates; replace once ruled.

## Phase report

Run V done. Pin-only checkpoint findings applied; Sweep (`go build`, `golangci-lint` 0 issues) and Verify green; spec ticked (01, 04, 05), `spec-check.py` OK, `STATE.md` written, status done.

Files: `internal/version/version_test.go` (dirty short-revision row), `internal/version/version.go` (Resolve/setting docs shortened), `cmd/quarry/run_version_test.go` (`Test_defaultEnv_...` asserts equal to `debug.ReadBuildInfo()`), `cmd/quarry/main_process_test.go` (dirty `(devel)` row).

Mutations (each restored, diff-clean): see STATE.md `## Open debts`. Nothing for later runs; next scenario is SCENARIO-02.
