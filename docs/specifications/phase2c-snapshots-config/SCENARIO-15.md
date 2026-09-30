---
id: SCENARIO-15
status: open
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
- [ ] Step 3: run V (Verify)

## Handoff

`Outcome.Warnings()` order is manifest, one-sided transfers, CF2; SCENARIO-29's prune warnings append after CF2.

## Phase report

Run L done (steps 1-2); V does step 3 (Verify, tick, STATE.md, `status: done`).

Files: `internal/snapshot/import.go` (`Outcome.historyWarning`, `historyRestartWarning`, set in `importVerified`, appended last by `Warnings()`); new `internal/snapshot/import_history_test.go`; `cmd/quarry/run_import_runs_test.go` acceptance test; `cmd/quarry/run_config_test.go` `--json` order pin. No cli change: `sync.go` already prints `outcome.Warnings()` and feeds `renderJSON`.

Red: acceptance test failed at stderr assertion (expected CF2 line, actual empty). Green after step 2. Narrow loops and `golangci-lint run ./...` (0 issues) green; full suite not run.

Mutation (each reverted, diff clean): `fault.Reason` instead of `UnreadableReason(...)` reddened the NotDuckDB, Permission, Locked and path-bearing subtests plus the ImportFrom order test; `fault == nil` reddened the nil control (panic) and order test; `if false` in `Warnings()` reddened order test and repeated-id / path-bearing subtests.

Notes for V: `Outcome` keeps the CF2 text unexported so a caller-built `Outcome` never carries one; a failed validation build (`Built` false) never sets it. Not pinned, unowned: HistoryFault with Fault Missing/OtherFormat cannot occur (13's contract), so the empty-reason branch of `UnreadableReason` stays unexecuted (existing NIT).
