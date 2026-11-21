# version-command — current state

Scenarios complete: SCENARIO-01 (folds 04, 05), SCENARIO-02. Last updated by SCENARIO-02.

## Binding decisions
- `internal/version.Resolve(*debug.BuildInfo) Info{Version string; Commit *string; Modified *bool; Go string}` owns the version rule; `Go` is `runtime.Version()` even for nil info. Nothing re-derives it: JSON renders straight from `Info`. (SCENARIO-01, 02)
- Input-integrity arms are ruled in spec `## Surface & Copy`: empty or conflicting-duplicate `vcs.revision` = no revision (Commit nil, text `(devel)`); malformed, empty or conflicting-duplicate `vcs.modified` = unknown (nil); `+dirty` only on the `(devel) <rev>` arm. Pinned in `Test_Resolve` and, for JSON nulls, `Test_version_json_edge_rows`. (SCENARIO-01, 02)
- `cli.Env.BuildInfo *debug.BuildInfo` is the only seam; `newVersionCommand(info, jsonOut)` never calls `debug.ReadBuildInfo()`. `defaultEnv` (`cmd/quarry/run.go:208`) sets it. (SCENARIO-01)
- `cmd/quarry/run.go` `buildVersion` delegates to `Resolve(...).Version`, so store and MCP get `(devel)` instead of `""` for nil/empty info. (SCENARIO-01)
- `version` emits through `emitReport(cmd, *jsonOut, nil, renderVersionJSON, text)` (`internal/cli/version.go`): one `emit`, a write failure is the existing `runtimeError`, exit 1. JSON document in `internal/cli/json_version.go` (`version`, `commit`, `modified`, `go`; nulls present, not absent). `Args: noArgs`. SCENARIO-03's root `--version` reuses `newVersionCommand`'s path; do not re-derive from build info. (SCENARIO-01, 02)

## Left unbuilt
- Root `--version` flag + root `RunE`, Flags help line in `run_status_test.go` root-help pin, refusal rows, regression pins; `root.go:13-14` "No Args or Run field" comment is rewritten there — SCENARIO-03.
- `newServerFactory(info, ...)` passing the resolved string to the store (`store_info.quarry_version`, `status --json` `.store.quarry_version`) — SCENARIO-06.
- MCP `initialize` `serverInfo.version` row `(devel) 260d0d29f940+dirty` in `run_mcp_test.go` `startMCP`/`c.info` cases — SCENARIO-07.

## Traps
- Wiring pin: under `go test` the text is `(devel)` whether or not `defaultEnv` sets `BuildInfo`; only `Test_defaultEnv_carries_this_binarys_build_info` (equal to `debug.ReadBuildInfo()`) sees a deleted line. (SCENARIO-01)
- `new(true)`/`new(x)` pointer form is used in tests (Go 1.27). (SCENARIO-01)
- `(devel)` constants in `internal/mcp/server.go:18` and `internal/store/duckstore/duckstore.go:28` stay: cleanup is out of scope per spec. (SCENARIO-01)

## Open debts
- OPEN (user, spec line 22): a breaking-change commit makes release-please cut 2.0.0, and `go install ...@v2.0.0` needs a `/v2` module path. Not needed to build this feature; unowned, dies unless re-opened.
- Mutations run (all red, restored): SCENARIO-01 `Resolve` short-revision early return dropping `+dirty` -> `Test_Resolve/a_dirty_revision_shorter_than_12_characters...`; `defaultEnv` `BuildInfo: &debug.BuildInfo{}` -> `Test_defaultEnv_carries_this_binarys_build_info`; `buildVersion` fallback to `Main.Version` -> `Test_buildVersion.../a_dirty_checkout...`. SCENARIO-02 `emitReport(cmd, false, ...)` -> acceptance + 8 edge rows; dropping `Commit`/`Modified` from `renderVersionJSON` -> acceptance + edge rows.
