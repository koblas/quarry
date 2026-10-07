---
id: SCENARIO-12b
status: done
---

# SCENARIO-12b: Status names a store built from an .SQLITE snapshot by its id

Cadence: code-first — pure name formatting, no mandatory test-first item (no deletion, overwrite, symlink refusal, atomicity)
Acceptance test: `cmd/quarry/run_status_case_test.go` `Test_run_status_names_a_store_built_from_an_upper_case_sqlite_snapshot_by_its_id`
Narrow loop: `go test ./internal/report/... ./internal/cli/ ./cmd/quarry/ -run 'SnapshotID|refuses_with_the_store|upper_case|renderStatus'`
Mutation checks: `SnapshotID` compares `ext == snapshotExt` (case-sensitive) → `Test_SnapshotID` upper/mixed-case rows, the R2 row, `Test_NewStatus_…`, `Test_NewSummary_…`, `Test_renderStatus/upper-case_snapshot_file`, acceptance (text and json)
Runs: L | V
Size: LIGHT — 3 steps, internal/report

One rule, `report.SnapshotID` strips one trailing `.sqlite` in any letter case (`strings.EqualFold` on `filepath.Ext`, no ID pattern match), reaches four sites; each ruled output has its pin.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_status_case_test.go` (new) — sync, rename snapshot to `<id>.SQLITE` (assert ReadDir name first, macOS folds case), remove store, `sync --from <id>`, then `status` (text) and `status --json`. Red at the printed id (`…Z.SQLITE`). Text asserts `Snapshot  <id>, taken …`; `--json` asserts `snapshot.id` = `<id>`, `snapshot.path` = on-disk `.SQLITE` path.

### Build
- [x] Step 2: `report.go:61-70` `SnapshotID` (doc: "its file name without the .sqlite extension, in any letter case"). Pins in `report/status_test.go` `Test_SnapshotID`: `.SQLITE`, `latest.Sqlite` → `latest`, `.sqlite3` unchanged, `a.sqlite.SQLITE` → `a.sqlite` (one ext), directory named `.SQLITE` ignored.
- [x] Step 3: one pin per site — R2 hint `--from 20260927T143005Z` (`report/refusal_test.go` upper-case row, via `refusal.go:145`); status text line byte-identical to lowercase (`cli/render_status_internal_test.go` `upper-case snapshot file`, `render_status.go:83`); `status --json` `snapshot.id` with `snapshot.path` kept (`document/status_test.go`, `status.go:117`); `summary --json`/MCP `monthly_summary` `snapshot.id` (`document/summary_test.go`, `summary.go:78`; MCP shares `NewSummary`).

## Phase report
Run L (steps 1-3, code-first): acceptance red at the id (text: `Snapshot  <id>.SQLITE, taken`; json: expected `<id>`, actual `<id>.SQLITE`), green after step 2. Narrow loop green; `golangci-lint run ./...` 0 issues; V still owns `verify.sh`, spec tick, STATE.md, `status: done`.
- Production: `internal/report/report.go:61-70` `SnapshotID`. Tests: `cmd/quarry/run_status_case_test.go` (new), `internal/report/status_test.go` (+5 rows), `internal/report/refusal_test.go` (+1 R2 row), `internal/report/document/status_test.go` and `summary_test.go` (+1 test each), `internal/cli/render_status_internal_test.go` (+1 subtest).
- Mutation `ext == snapshotExt` (report.go:66), restored byte-identical: red in `Test_SnapshotID` (upper-case, mixed-case, one-extension-only rows), `Test_status_refuses_with_the_store_refusal_copy` upper-case row, `Test_NewStatus_names_an_upper_case…`, `Test_NewSummary_names_an_upper_case…`, `Test_renderStatus/upper-case_snapshot_file`, acceptance text and json.
- Not redone/undone by V: `import.go:152-153` `ID()` doc already says any letter case (12a). Cmd helper `snapshotID` in run_helpers_test.go is lowercase-only by design (it derives ids from lowercase fixtures).
