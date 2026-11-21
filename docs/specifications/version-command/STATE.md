# version-command — current state

Scenarios complete: SCENARIO-01 (folds 04, 05). Last updated by SCENARIO-01.

## Binding decisions
- `internal/version.Resolve(*debug.BuildInfo) Info{Version string; Commit *string; Modified *bool; Go string}` owns the version rule; `Go` is `runtime.Version()` even for nil info. Nothing re-derives it: 02 renders JSON straight from `Info`. (SCENARIO-01)
- Input-integrity arms are ruled in spec `## Surface & Copy`: empty or conflicting-duplicate `vcs.revision` = no revision (Commit nil, text `(devel)`); malformed, empty or conflicting-duplicate `vcs.modified` = unknown (nil); `+dirty` only on the `(devel) <rev>` arm. Pinned in `Test_Resolve` incl. first-wins/last-wins arms. (SCENARIO-01)
- `cli.Env.BuildInfo *debug.BuildInfo` is the only seam; `newVersionCommand(info)` never calls `debug.ReadBuildInfo()`. `defaultEnv` (`cmd/quarry/run.go:208`) sets it. (SCENARIO-01)
- `cmd/quarry/run.go` `buildVersion` delegates to `Resolve(...).Version`, so store and MCP get `(devel)` instead of `""` for nil/empty info. (SCENARIO-01)
- `version` emits through one `emit(cmd, out, "", nil)` (`internal/cli/version.go`); a write failure is the existing `runtimeError`, exit 1. `Args: noArgs`. (SCENARIO-01)

## Left unbuilt
- `--json` document, `json_version.go`, `jsonOut` param on `newVersionCommand`, `renderResult` arm — SCENARIO-02. Must include a `commit: null` row for an empty `vcs.revision` (spec ruling) and the `modified: null` rows.
- Root `--version` flag + root `RunE`, Flags help line in `run_status_test.go` root-help pin, refusal rows, regression pins; `root.go:13-14` "No Args or Run field" comment is rewritten there — SCENARIO-03.
- `newServerFactory(info, ...)` passing the resolved string to the store (`store_info.quarry_version`, `status --json` `.store.quarry_version`) — SCENARIO-06.
- MCP `initialize` `serverInfo.version` row `(devel) 260d0d29f940+dirty` in `run_mcp_test.go` `startMCP`/`c.info` cases — SCENARIO-07.

## Traps
- Wiring pin: under `go test` the text is `(devel)` whether or not `defaultEnv` sets `BuildInfo`; only `Test_defaultEnv_carries_this_binarys_build_info` (equal to `debug.ReadBuildInfo()`) sees a deleted line. (SCENARIO-01)
- Until 02 lands, `quarry version --json` silently prints the text line (persistent flag accepted). Do not pin that. (SCENARIO-01)
- `new(true)`/`new(x)` pointer form is used in tests (Go 1.27). (SCENARIO-01)
- `(devel)` constants in `internal/mcp/server.go:18` and `internal/store/duckstore/duckstore.go:28` stay: cleanup is out of scope per spec. (SCENARIO-01)

## Open debts
- OPEN (user, spec line 22): a breaking-change commit makes release-please cut 2.0.0, and `go install ...@v2.0.0` needs a `/v2` module path. Not needed to build this feature; unowned, dies unless re-opened.
- Mutations run in SCENARIO-01 (all red, restored): `Resolve` short-revision early return dropping `+dirty` -> `Test_Resolve/a_dirty_revision_shorter_than_12_characters...`; `defaultEnv` `BuildInfo: &debug.BuildInfo{}` -> `Test_defaultEnv_carries_this_binarys_build_info`; `buildVersion` fallback to `Main.Version` -> `Test_buildVersion.../a_dirty_checkout...`.
