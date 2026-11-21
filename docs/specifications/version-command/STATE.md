# version-command — current state

Scenarios complete: SCENARIO-01 (folds 04, 05), SCENARIO-02, SCENARIO-03, SCENARIO-06. Last updated by SCENARIO-06.

## Binding decisions
- `internal/version.Resolve(*debug.BuildInfo) Info{Version string; Commit *string; Modified *bool; Go string}` owns the version rule; `Go` is `runtime.Version()` even for nil info. Nothing re-derives it: JSON renders straight from `Info`. (SCENARIO-01, 02)
- Input-integrity arms are ruled in spec `## Surface & Copy`: empty or conflicting-duplicate `vcs.revision` = no revision (Commit nil, text `(devel)`); malformed, empty or conflicting-duplicate `vcs.modified` = unknown (nil); `+dirty` only on the `(devel) <rev>` arm. Pinned in `Test_Resolve` and, for JSON nulls, `Test_version_json_edge_rows`. (SCENARIO-01, 02)
- `cli.Env.BuildInfo *debug.BuildInfo` is the only seam; `newVersionCommand(info, jsonOut)` never calls `debug.ReadBuildInfo()`. (SCENARIO-01)
- `cmd/quarry/run.go` `envFor(info, ...)` builds the `cli.Env` from an injected `*debug.BuildInfo` and passes that same `info` to `newServerFactory(info, ...)` and `newMCPServe(info)`; `defaultEnv` is `envFor(debug.ReadBuildInfo-result, ...)`. Tests drive `envFor` with a fixture info; only `Test_defaultEnv_carries_this_binarys_build_info` pins the `defaultEnv` -> `envFor` delegation. `valetServerFactory` in `run_sync_rates_test.go` passes nil info (no env in scope). (SCENARIO-06)
- `buildVersion` delegates to `Resolve(...).Version`, so store and MCP get `(devel)` instead of `""` for nil/empty info; store half is stamped through `newServerFactory(info, ...)` (`store_info.quarry_version`, `status --json` `.store.quarry_version`). (SCENARIO-01, 06)
- `version` emits through `emitReport(cmd, *jsonOut, nil, renderVersionJSON, text)` (`internal/cli/version.go`): one `emit`, a write failure is the existing `runtimeError`, exit 1. JSON in `internal/cli/json_version.go` (`version`, `commit`, `modified`, `go`; nulls present, not absent). `Args: noArgs`. (SCENARIO-01, 02)
- Root `--version` is a local (non-persistent) flag on root; root `RunE` calls `printVersion(cmd, env.BuildInfo, *jsonOut)` when `showVersion` is set, else `cmd.Help()`; `Args` stays nil so cobra's legacy check still refuses `quarry bogus` (exit 2). `version` and root share `printVersion`. (SCENARIO-03)
- Accepted consequence: root help Usage reads `quarry [flags]` then `quarry [command]` (cobra, once root has a `RunE`); pinned in `Test_run_help_prints_quarrys_description`; final product pass reviews it. (SCENARIO-03)

## Left unbuilt
- MCP `initialize` `serverInfo.version` row `(devel) 260d0d29f940+dirty` in `run_mcp_test.go` `startMCP`/`c.info` cases — SCENARIO-07. Its plan must drive `quarry mcp` initialize through `envFor(info, ...)` and assert `serverInfo.version`: the `ServeMCP: newMCPServe(info)` link inside `envFor` is otherwise unpinned, because the `startMCP` helpers (`run_mcp_test.go:319,414,1079`) hardcode `newMCPServe(nil, ...)`. Mutation `ServeMCP: newMCPServe(nil)` in `envFor` must redden it. (SCENARIO-06)

## Traps
- Wiring pin: under `go test` the text is `(devel)` whether or not `defaultEnv` sets `BuildInfo`; only `Test_defaultEnv_carries_this_binarys_build_info` (equal to `debug.ReadBuildInfo()`) sees a deleted line. (SCENARIO-01)
- `new(true)`/`new(x)` pointer form is used in tests (Go 1.27). (SCENARIO-01)
- `(devel)` constants in `internal/mcp/server.go:18` and `internal/store/duckstore/duckstore.go:28` stay: cleanup is out of scope per spec. (SCENARIO-01)

## Open debts
- OPEN (user, spec line 22): a breaking-change commit makes release-please cut 2.0.0, and `go install ...@v2.0.0` needs a `/v2` module path. Not needed to build this feature; unowned, dies unless re-opened.
- Mutations run (all red, restored): SCENARIO-01 `Resolve` short-revision early return dropping `+dirty` -> `Test_Resolve/a_dirty_revision_shorter_than_12_characters...`; `defaultEnv` `BuildInfo: &debug.BuildInfo{}` -> `Test_defaultEnv_carries_this_binarys_build_info`; `buildVersion` fallback to `Main.Version` -> `Test_buildVersion.../a_dirty_checkout...`. SCENARIO-02 `emitReport(cmd, false, ...)` -> acceptance + 8 edge rows; dropping `Commit`/`Modified` from `renderVersionJSON` -> acceptance + edge rows. SCENARIO-03 `if showVersion` -> `cmd.Flags().Changed("version")` -> `--version=false` row of the bare-root help pin; dropping root `RunE` -> root-help Usage pin plus `--version` tests; `if false && showVersion` -> acceptance x3 + write-failure test. Making the flag persistent reddens the refusal rows only (the help pin does NOT catch it: root lists its own persistent flags under Flags). SCENARIO-06 `envFor` -> `newServerFactory(nil, ...)` -> acceptance (both assertions); `defaultEnv` -> `envFor(nil, ...)` -> `Test_defaultEnv_carries_this_binarys_build_info`.
