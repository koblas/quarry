---
id: SCENARIO-28
status: done
---

# SCENARIO-28: prune --json reports deleted, would_delete and failed in one shape

Cadence: code-first — renders `snapshot.Pruned` facts only; no bug fix, write-safety guard or atomicity adapter is touched (deletion logic is SCENARIO-21/24's)
Acceptance test: `cmd/quarry/run_prune_json_test.go` `Test_run_snapshots_prune_json_prints_the_ruled_document_for_a_real_run`
Narrow loop: `go test ./internal/cli/ -run 'Prune|prune' && go test ./cmd/quarry/ -run 'prune_json|snapshots_json'`
Mutation checks: none (code-first; each ruled bullet is its own cmd case)
Runs: L | V
Size: LIGHT — 2 steps, cli (cmd tests only beyond it)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_prune_json_test.go` (new) — exact-bytes acceptance test (five snapshots, store built from newest, `--keep 3 --json`: `deleted` 2 entries, `would_delete` `[]`, `failed` `[]`, `store_snapshot` set, `warnings` `[]`, stderr empty). Red: prune prints the text block, not JSON. Then one case per ruling: dry run, nothing to delete with store readable, store unreadable within the cap, partial failure (JSON, exit 1, stderr line), interrupt mid-delete (JSON, exit 1), refusal (stdout empty), config warning in `warnings[]` and on stderr, store's snapshot kept outside newest N, `keep` from config

### Build
- [x] Step 2: `internal/cli/json_prune.go` (new) `renderPrunedJSON(p snapshot.Pruned, warnings []string)` with keys `dry_run, keep, store_snapshot, deleted, would_delete, failed, warnings` (lists never nil, `store_snapshot` via `newStoreSnapshotDocument(p.StorePath)`); `snapshots_prune.go` takes `jsonOut *bool` (from `snapshots.go:79`) and `reportPruned` renders through `renderResult`. Fold SCENARIO-18's checkpoint MINORs into `run_snapshots_json_test.go` (stat size vs manifest `Bytes`, `-04:00` `taken_at` to UTC, `sha256: ""`)

### Sweep and Verify
- [x] Run V

## Handoff

None beyond STATE.md: `Pruned` facts are rendered as they stand; no new port, sentinel or copy.

## Phase report

Run V done. Full suite rc=0; `-race` on `internal/cli`, `cmd/quarry` green; `golangci-lint` 0 issues; uncovered-diff 0 added lines (1 declared unreachable: `snapshots_prune.go:88` `return err`); test-stats `cmd/quarry 240 (+14)`. SCENARIO-28 ticked, STATE.md rewritten, `status: done`.
