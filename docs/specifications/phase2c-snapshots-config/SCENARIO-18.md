---
id: SCENARIO-18
status: done
---

# SCENARIO-18: snapshots --json lists every snapshot with its store flag

Cadence: code-first — a read-only document over `Listing`; no write-safety guard, no atomic adapter, no bug fix
Acceptance test: `cmd/quarry/run_snapshots_json_test.go` `Test_run_snapshots_json_prints_the_ruled_document_for_two_snapshots_and_a_store`
Narrow loop: `go test ./internal/cli/ ./cmd/quarry/ -run '(?i)snapshot'`
Mutation checks: none (code-first)
Runs: L | V
Size: LIGHT — 2 steps, cli (one-token export `snapshot.ID` so cli never copies the `.sqlite` suffix rule)

Contract (specification.md `### quarry snapshots` `--json`): keys in order `directory, keep, store_snapshot, snapshots, total_bytes, warnings`; each snapshot `id, path, manifest, taken_at, bytes, source, sha256, schema_verified, store`. `manifest` is the `.json` beside `path` (null with no manifest); `taken_at` RFC3339 UTC; `bytes` is the size on disk; a missing manifest nulls `manifest, taken_at, source, sha256, schema_verified`; an unparsable `taken_at` or empty `source` nulls only that field. `store_snapshot` is `{id, path}` from `Listing.StorePath` (unresolved) or null. `keep` = `cfg.Keep`. `warnings` = config C3, no-snapshots line, store warning, no prefixes, never nil. stderr keeps the prefixed lines.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: new `cmd/quarry/run_snapshots_json_test.go` acceptance test — exact bytes, two snapshots (newer marked), default keep 12; `snapshotFixture` (`run_snapshots_test.go:42`) gains `sha256`. Red: `--json` still prints the table. No stubs needed (nothing new to compile)

### Build
- [x] Step 2: new `internal/cli/json_snapshots.go` (`snapshotsDocument`, `renderSnapshotsJSON(listing, keep, warnings)`, `snapshotsWarnings`); `internal/cli/snapshots.go` takes `jsonOut` (`root.go:33`) and renders through `renderResult`, stderr lines unchanged; `internal/snapshot/import.go:82` `snapshotID` → exported `ID`. Tests, each its own function: no manifest (five nulls, keys present), unparsable `taken_at`, empty `source`, empty folder (`snapshots: []`, `total_bytes: 0`), config + no-snapshots + store warning order with stderr, `store_snapshot` null (no store / unreadable / zero runs / R2 without path, table), `store_snapshot` names the recorded path outside the folder and for a hand-deleted snapshot (table), `keep` from config; `render_snapshots_internal_test.go` non-ASCII Source pin

### Sweep / Verify
- [x] Run V: full verification, tick SCENARIO-18 in specification.md, rewrite STATE.md

## Phase report

Run L done (steps 1-2 green; narrow loop `go test ./cmd/quarry/ ./internal/cli/ ./internal/snapshot/` ok, `golangci-lint run ./...` 0 issues). Red quoted: acceptance failed at `assert.Equal` in `run_snapshots_json_test.go:68` (`--json` printed the table); the other JSON tests failed on JSON decode of the table.
Files: new `internal/cli/json_snapshots.go` (document types, `renderSnapshotsJSON`, `snapshotsWarnings`); `internal/cli/snapshots.go` (`jsonOut` param, `renderResult` branch with one `// unreachable:` on its error return) and `root.go`; `internal/snapshot/import.go:82` `snapshotID` -> exported `ID` (callers in list/import/outcome/from); new `cmd/quarry/run_snapshots_json_test.go` (9 tests); `run_snapshots_test.go` `snapshotFixture.sha256`; `render_snapshots_internal_test.go` non-ASCII Source case (green on arrival: the padding already counts runes; folds the SCENARIO-16 debt).
Decisions for V's STATE.md: `manifest` is derived (`<path minus .sqlite>.json`), not the manifest's recorded `snapshot.manifest`, so a relocated folder still reports a real file; `bytes` is the size on disk; `sha256` is passed through as recorded (empty string stays `""`, only source/taken_at null on their own faults); `taken_at` is re-formatted RFC3339 UTC from `Entry.TakenAt`. `warnings` built by `snapshotsWarnings`, stderr lines unchanged. V: drop the SCENARIO-16 `--json` binding line and Left-unbuilt `renderSnapshotsJSON` row; close the non-ASCII debt; run `uncovered-diff.py`.
