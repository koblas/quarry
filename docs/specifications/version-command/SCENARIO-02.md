---
id: SCENARIO-02
status: done
---

# SCENARIO-02: --json reports version, commit, modified and Go version

Cadence: code-first (no mandatory test-first item: no write-safety guard, atomic adapter or bug fix)
Acceptance test: `internal/cli/version_test.go` `Test_version_json_reports_version_commit_modified_and_go`
Narrow loop: `go test ./internal/cli/ -run 'Test_version'`
Mutation checks: none (code-first)
Runs: L | V
Size: LIGHT — 3 steps, internal/cli

Surface & Copy delivered: the `--json` document (key order `version`, `commit`, `modified`, `go`, nulls per edge rows) asserted verbatim in `Test_version_json_edge_rows`; stdout write failure `cannot write the result to stdout: <err>` in the JSON arm. Long text already pinned (SCENARIO-01). No root `--version` (SCENARIO-03).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `internal/cli/version_test.go` `Test_version_json_reports_version_commit_modified_and_go` (dirty checkout, `version --json` through `cli.Execute`, exact 2-space-indented bytes, `go` = `runtime.Version()`, empty stderr). Red already at assertion: `--json` prints the text line; no stubs needed.

### Build
- [x] Step 2 (batch 1): `internal/cli/json_version.go` `renderVersionJSON(version.Info)` via `marshalDocument`; `version.go` takes `jsonOut *bool` and goes through `renderResult` then the single `emit`; `root.go:46` passes `jsonOut`. Tests: `Test_version_json_edge_rows` (tagged/no vcs, (devel)+rev modified absent / malformed / conflicting duplicate, empty revision, conflicting duplicate revision, tagged + empty vcs keys, nil info; all four fields incl. null vs value), `Test_version_json_reads_back_as_the_four_keys_in_order` (stdlib decode: key order via token stream, nulls present as null not absent), `Test_version_json_reports_a_failed_stdout_write`.

### Sweep
- [x] Step 3: `go build ./... && golangci-lint run ./...` to `0 issues`; doc comments. (Lint already clean after L; V re-runs it with verify.sh.)

## Handoff
- `--json` precedes root `--version` work: 03 reuses `renderResult`/`emit` through `newVersionCommand`'s `emitReport` call; do not re-derive from build info.

## Phase report
Run L done (start eac74f14). Files:
- `internal/cli/json_version.go` (new): `versionDocument`, `renderVersionJSON(version.Info)` via `marshalDocument`.
- `internal/cli/version.go:12-30`: `newVersionCommand(info, jsonOut *bool)` now goes through `emitReport(cmd, *jsonOut, nil, ...)` (renderResult + the single emit; no duplicate unreachable arm).
- `internal/cli/root.go:46`: passes `jsonOut`. No other callers.
- `internal/cli/version_test.go`: +4 tests: `Test_version_json_reports_version_commit_modified_and_go` (acceptance), `Test_version_json_edge_rows` (9 rows incl. empty/duplicate revision, malformed/duplicate modified, tagged + empty vcs, nil info), `Test_version_json_reads_back_as_the_four_keys_in_order`, `Test_version_json_reports_a_failed_stdout_write`.
Red (before code): acceptance failed at assertion, expected the JSON document, actual `quarry v0.0.0-20261121151146-260d0d29f940+dirty\n`. Now green: `go test ./internal/cli/ -run Test_version`; lint `0 issues`.
`Test_version_json_reports_a_failed_stdout_write` was green on arrival: the write goes through the shared `emit`/`writeResult`, already pinned for text; it pins the JSON arm's error text and survives the one-emit shape.
Extra mutations (beyond plan, all red, restored): `emitReport(cmd, false, ...)` in version.go -> acceptance + 8 edge rows; drop `Commit`/`Modified` from `renderVersionJSON` -> acceptance + edge rows.
V must still: `verify.sh eac74f14... ./internal/cli/...`, tick spec SCENARIO-02 (acceptance test last on line), spec-check, STATE.md (remove 02 from Left unbuilt and the "--json silently prints text" trap), status: done. Do not add root `--version` (03).
