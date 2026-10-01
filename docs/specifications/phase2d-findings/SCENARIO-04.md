---
id: SCENARIO-04
status: done
---

# SCENARIO-04: a successful sync lists one-sided transfers only as findings (folds SCENARIO-05, V1 tail copy)

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_transfers_test.go` `Test_run_lists_one_sided_transfers_only_as_findings_on_a_successful_sync`
Acceptance test (SCENARIO-05, folded): `cmd/quarry/run_validation_test.go` `Test_run_lists_mismatched_splits_in_the_failed_validation_stdout_block`
Narrow loop: `go test ./internal/snapshot/ ./internal/cli/ -run 'Outcome|Warning|History|Render|JSON|Validation|Store' && go test ./cmd/quarry/ -run 'Transfers|Json|JSON|Config|Validation|Prune'`
Mutation checks: none (code-first)
Runs: A (1-2) | B1 (3) | B2 (4-5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (`snapshot`) + `internal/cli`; folds SCENARIO-05 (one string, same test files)

Contract: success sync prints `Transfers  <count line>` then `Findings  ...` with no `?` rows; stderr has no one-sided warning;
exit 0. `sync --json`: `store.transfers.one_sided` unchanged, `warnings[]` has no one-sided entry, `store.findings` =
`{open, ignored, fixed, new, newly_fixed}` (null when `built` is false). Validation failure (exit 1) keeps its `?` rows and ends
stderr `fix them in Quicken and run quarry sync, or run quarry sync --from <id> after updating quarry`.

Surface surveyed (grep, no port is introduced): production callers of `oneSidedWarning` = `import.go:51` only; `writeTransfers`
= `render.go:95` (success) and `:387` (failure); `oneSidedRows` = `writeTransfers` only; `otherAccountLabel` = `oneSidedRows`;
`newOneSidedDocuments` = `json.go:244`. LSP `findReferences` confirms before deleting.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_transfers_test.go:188-241` rename the test to the acceptance name; expected stdout drops the three `?` rows (keeps `Transfers  1 paired, 3 one-sided` then `Findings  3 open; run quarry findings to list them`), stderr empty; keep the `transfers` table assertion. Red at the stdout assertion
- [x] Step 2: `cmd/quarry/run_validation_test.go:354` (the `assert.Contains(stderr, ...)`) — assert the whole stderr line for the splits-only failure ending `fix them in Quicken and run quarry sync, or run quarry sync --from <id> after updating quarry\n`. Red at that assertion

### Build
- [x] Step 3: drop `oneSidedWarning` and repoint its pins; change the V1 tail
  - Prod: `internal/snapshot/import.go:50-52,65-72` remove warning and helper; `:39-41` `Warnings` doc (manifest, history restart, auto-prune); `:184` V1 tail string
  - `internal/snapshot/import_test.go:52-110` the four one-sided warning tests go; replace with one pin `Test_outcome_warnings_omit_one_sided_transfers_for_a_built_store` (built store with OneSided legs + a manifest warning yields only the manifest warning); `builtWithOneSided` helper replaced by a plain built `store.Result`
  - `auto_prune_faults_test.go:62-85,91,139`: warning source = the history restart warning already set there (`HistoryFault`); drop the first expected line, keep history then prune order
  - `import_history_test.go:88-103`: source = manifest schema-extras warning (`snapshot.go:322` `schemaWarnings`, `from.go:68`) via `ImportFrom`, ordered before the history line; if no extras fixture reaches `ImportFrom`, reduce to history-only and say so in the phase report
  - V1 pins: `snapshot/sync_and_import_test.go:503`, `cmd/quarry/run_validation_test.go:175,292`, `run_json_test.go:182`, `run_transfers_test.go:275` (new tail string verbatim)
  - `cmd/quarry/run_config_test.go:242,257,277,301,317,337`: warning source = previous-store-unreadable history restart line (garbage bytes at `storePathUnder(home)` after a first sync, as `:281` already does); literal `cannot carry import history forward from the previous store (the file is not a DuckDB database); import_runs starts again with this sync` held in ONE const (08 rewrites it once); the `:281` test loses its transfer line and is renamed `..._after_the_config_warning_...`
  - `run_sync_prune_json_test.go:179`: drop the transfer element; rename `..._after_config_and_history_warnings`
  - `run_json_test.go:60-61,75`: stderr empty, `warnings` `[]`
- [x] Step 4: drop the success `?` rows — `internal/cli/render.go:109-116` `writeTransfers` (success prints only the count line; `renderStoreFailure` `:387` keeps count + `oneSidedRows`: split into two small funcs, do not delete `oneSidedRows`/`otherAccountLabel` `:314-351`); `:86-98` `renderStore` doc. Tests: `internal/cli/render_internal_test.go:241-255` becomes `Test_renderStore_prints_the_transfers_count_line_without_one_sided_rows` (legs present, no `?` row, Findings line follows Transfers); `:257-275` unchanged text re-checked; `:430-560` (`oneSidedRows`, `renderStoreFailure`) stay as the control
- [x] Step 5: `sync --json` `store.findings` — `internal/cli/json.go:34-42` `storeDocument.Findings *findingsDocument` (`json:"findings"`), new `findingsDocument{Open,Ignored,Fixed,New,NewlyFixed}` (tags `open ignored fixed new newly_fixed`), `:157-170` `newStoreDocument` fills it from `result.Findings` when `result.Built`, else nil. Tests: `internal/cli/json_internal_test.go:40-90` cases built (all five keys) / unbuilt (null) / nil result; `cmd/quarry/run_json_test.go:79-101` `wantStore` gains `findings` (counts derived from the fixture; `one_sided` still 3 entries beside it); `:108-188` unbuilt test asserts `findings` is null

### Sweep
- [x] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues` (unused imports `humanize`/`slices` if they go dead); doc comment on `findingsDocument`

### Verify
- [x] Step 7: full verification + `spec-check.py phase2d-findings`; tick SCENARIO-04 and SCENARIO-05 in `specification.md` (05 line: `— delivered by SCENARIO-04 —` before its test reference); strike the V1-tail debt in `docs/specifications/phase2a-read-foundation/STATE.md:74`; rewrite STATE.md; `status: done`

## Handoff

**Binding decisions**
- `Outcome.Warnings()` order is now manifest, history restart, auto-prune; no one-sided entry in text or `warnings[]` — the count survives only as the `Transfers` line, `store.transfers.one_sided` and the Findings count (P2d-11)
- `store.findings` is nil iff the build was not reached (`Built` false); `findingsDocument` holds plain ints; 19's `status --json` needs `ignored: null`, so it must not reuse this type as is
- `writeTransfers` is shared by the success and failure blocks; only the failure block prints `?` rows (spec P2d-11); `oneSidedRows` and `otherAccountLabel` stay live there
- V1 tail is `fix them in Quicken and run quarry sync, or run quarry sync --from <id> after updating quarry`

**Left unbuilt**
- `(M new)` / `K fixed since the last sync` / `J ignored` clauses and `ignored` from a config (still 0) — 06 / 16
- Text rows for one-sided legs (`one-sided-transfer:xfer-N  date  account  payee  amount  other account: ...`) — 11 reuses `oneSidedRows` columns minus the `?`

**Traps**
- The six `run_config_test.go` tests, `run_sync_prune_json_test.go:179` and snapshot `historyRestartLine` all pin the CF2 history line; 08 replaces it for the store-level case (combined line), so those pins change again there
- Deleting `writeTransfers`'s rows outright breaks `renderStoreFailure`'s `?` rows (pinned `render_internal_test.go:491-560`, `run_transfers_test.go:271`)
- `Counts.New == Counts.Open` today, so a first-sync JSON shows `new` = `open`; correct until 06 carries history

## Phase report

Run V done; all steps ticked, `status: done`. Sweep: `go build ./...` and `golangci-lint run ./...` 0 issues.

Verify (base 8e7351fc): full covered suite `go test rc=0`; `uncovered-diff.py`: 0 uncovered added lines; `go test -race ./internal/cli/... ./internal/snapshot/...` ok.
test-stats: cmd/quarry 283 (+0), internal/cli 162 (+3), internal/snapshot 253 (-4), TOTAL 698 (-1); tempdir/disk +0.

Docs touched: `specification.md` ticks for 04 and 05 (05 "delivered by SCENARIO-04"); `phase1-import-store/specification.md:528` and `SCENARIO-08.md` repointed to the renamed acceptance test (`spec-check.py phase1-import-store` OK); `phase2a-read-foundation/STATE.md:74` V1-tail debt struck; `STATE.md` rewritten.
