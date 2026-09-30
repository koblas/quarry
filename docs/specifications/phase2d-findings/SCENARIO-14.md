---
id: SCENARIO-14
status: done
---

# SCENARIO-14: findings --json returns the worklist as a document

Size: LIGHT — 2 steps, cli
Cadence: code-first
Runs: L | V
Acceptance test: `cmd/quarry/run_findings_json_test.go` `Test_run_findings_json_prints_the_ruled_document_for_one_open_duplicate`
Narrow loop: `go test ./cmd/quarry/ -run 'findings_json' && go test ./internal/cli/ -run 'findings'`
Mutation checks: none (no test-first item touched)

## Implementation Plan

### Acceptance (red)
- [x] `cmd/quarry/run_findings_json_test.go`: acceptance test (sync one duplicate, pin `first_found_at` via `editStore`, `JSONEq` on the ruled document); stub `renderFindingsJSON` zero-value so red reads as assertion

### Build
- [x] 1. `internal/cli/json_findings.go`: `findingsListDocument`, entry and item documents (every item key present, null where not applicable; `jsonMoney` amounts; `fix` = `Type.Fix().Sentence`; `items` `[]` never null; counts reuse `findingsDocument`); pins: one-sided (other_account/other_account_id, split_id) and uncategorized (split_id, payee) docs with a USD amount, empty listing (`findings: []`, `warnings: []`)
- [x] 2. `internal/cli/findings.go:69-91`: take `jsonOut`, `renderResult(*jsonOut, ...)`; config warnings go to stderr (printConfigWarnings) and, unprefixed, into `warnings[]` (snapshots precedent); `root.go:32` passes `jsonOut`; pin: config warning in both places

## Handoff
`status` is `"open"` and `type` null here; 18 passes the `--status`/`--type` values and adds fixed findings (`newFindingEntryDocument` leaves `fixed_at` nil and `status` "open" for now: make both follow the finding; `items` is already `[]` for none).

## Phase report
Run V. Sweep: `go build ./...`, `golangci-lint run ./...` 0 issues. Verify: covered full suite rc=0; `uncovered-diff.py` 0 uncovered, 1 declared unreachable (`findings.go:95`); `go test -race ./internal/cli/... ./cmd/quarry/...` ok; `test-stats` cmd/quarry 305 (+4). Spec ticked, STATE.md rewritten.
