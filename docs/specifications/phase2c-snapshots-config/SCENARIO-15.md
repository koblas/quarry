---
id: SCENARIO-15
status: done
---

# SCENARIO-15: sync warns and restarts history when the previous store's history cannot be read

Cadence: code-first (no bug fix, write-safety guard or atomic adapter: 13 owns the carry; this only renders its fault)
Acceptance test: `cmd/quarry/run_import_runs_test.go` `Test_run_sync_from_warns_and_restarts_history_when_the_previous_store_is_not_a_duckdb_database`
Narrow loop: `go test ./internal/snapshot/ -run 'history|outcome_warnings' && go test ./cmd/quarry/ -run 'import_runs|history'`
Mutation checks: none mandatory; load-bearing: CF2 line built from `Error()` or `Reason` instead of `UnreadableReason(at)` -> the path-bearing row of `Test_sync_and_import_names_the_history_fault_reason_in_the_warning`
Runs: L | V
Size: LIGHT — 2 steps, snapshot (+ one cmd test)

Contract (specification.md `## Surface & Copy` CF2, verbatim): `quarry: warning: cannot carry import history forward from the previous store (<reason>); import_runs starts again with this sync`; exit 0, build proceeds; `warnings[]` carries it without the prefix. Order: config C3 warnings, then Outcome.Warnings() = manifest, one-sided transfers, CF2.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `run_import_runs_test.go` acceptance test: sync a bundle, overwrite the store file with non-DuckDB bytes, `sync --from <id>`; stderr is exactly the CF2 line, `import_runs` holds 1 row, exit 0. Red today: stderr empty

### Build
- [x] Step 2: `snapshot/import.go` `Outcome` gets an unexported `historyWarning`, set in `importVerified` from `result.HistoryFault.UnreadableReason(homepath.Abbreviate(s.home, storeProbe.Path()))` on a built store; `Warnings()` appends it last. Tests in `snapshot/import_history_test.go`: one pin per ruled reason string (NotDuckDB, Permission, Locked, the three import_runs phrases, a Path-bearing Other), nil-fault control, unbuilt store carries none, order (manifest, one-sided, CF2); `cmd/quarry` `--json` `warnings[]` pin (config warning first, CF2 without prefix)

### Verify
- [x] Step 3: run V (Verify)

## Handoff

`Outcome.Warnings()` order is manifest, one-sided transfers, CF2; SCENARIO-29's prune warnings append after CF2.

## Phase report

Run V done: scenario complete, `status: done`.

Verify: `go build ./...` ok; `golangci-lint run ./...` 0 issues; covered full suite rc=0; `uncovered-diff.py` 0 uncovered added lines; `-race` green on `internal/snapshot` and `cmd/quarry`. Tick added to `specification.md` (spec-check OK); `STATE.md` rewritten (CF2 moved from Left unbuilt into Binding decisions).

Files from run L unchanged: `internal/snapshot/import.go`, `internal/snapshot/import_history_test.go`, `cmd/quarry/run_import_runs_test.go`, `cmd/quarry/run_config_test.go`.
