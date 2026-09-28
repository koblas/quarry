---
id: SCENARIO-01
status: open
---

# SCENARIO-01: Bundle only in Quicken's Documents folder is used

Cadence: code-first
Acceptance test: `cmd/quarry/run_test.go` `Test_run_discovers_the_bundle_from_documents_without_quicken`
Acceptance test (SCENARIO-02, folded): `cmd/quarry/run_test.go` `Test_run_pools_bundles_across_both_documents_folders`
Acceptance test (SCENARIO-03, folded): `cmd/quarry/run_test.go` `Test_run_counts_a_bundle_reached_two_ways_once`
Acceptance test (SCENARIO-04, folded): `cmd/quarry/run_test.go` `Test_run_refuses_when_a_discovery_location_is_unreadable`
Acceptance test (SCENARIO-05, folded): `cmd/quarry/run_test.go` `Test_run_sync_help_names_both_documents_folders`
Narrow loop: `go test ./internal/snapshot/... -run 'DiscoverBundle'` ; `go test ./cmd/quarry/... -run 'Test_run_discovers|Test_run_pools|Test_run_counts|Test_run_refuses_when_a_discovery|Test_run_sync_help|Test_run_refuses_a_bad_quicken'`
Mutation checks: location-check order (R3 before R3b) in `DiscoverBundle` → `Test_run_refuses_when_a_discovery_location_is_unreadable`; identity dedupe (`os.SameFile`) in `dedupeByIdentity` → `Test_run_counts_a_bundle_reached_two_ways_once`

## Fallible-call inventory
`os.ReadDir` (once per location — 2 call sites) · per-entry `os.Stat` (unchanged filter, now 2 call sites) · `ResolveBundlePath` on the sole survivor (unchanged, stays on concrete call). `os.SameFile` never errors (`go doc os SameFile`) — dropped from this inventory; its `FileInfo` args must come from `os.Stat` (link-following), not `DirEntry.Info()` (`Lstat`-based), or the `~/D/X.quicken`-symlink row double-counts (Handoff trap).

Repo-wide grep for `"in ~/Documents"` / `"default: the only"` over `*_test.go` and `testdata/` (positive control: known hit at `run_test.go:237` found) turns up only `cmd/quarry/run_test.go` and `internal/snapshot/discover_test.go` — no other test file or fixture pins this copy.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_test.go:228-296` `Test_run_discovers_the_bundle_from_documents_without_quicken` — drop the refusal-table cases (moved to Step 2/4), keep "exactly one bundle in ~/Documents"; add "~/Documents missing, Quicken Documents folder has one" (SCENARIO-01)
- [ ] Step 2: `cmd/quarry/run_test.go` — new `Test_run_pools_bundles_across_both_documents_folders` (SCENARIO-02, subsumes the moved no-bundle/three-bundles cases): table over both-missing→R1, both-exist-both-empty→R1, bundles-only-under-`.../Quicken/Backups`→R1, one-each→R2 (`~/Documents` path first), two-in-`~/Documents`→R2, two-in-Quicken-folder→R2 — all `wantStderr` exact, full `~`-paths
- [ ] Step 3: `internal/snapshot/discover_test.go:116-150` `Test_DiscoverBundle_refuses_when_multiple_bundles_exist`, `:14-40` `Test_DiscoverBundle_refuses_when_no_bundle_is_found` — update `wantStderr`/`re.Error()` to the new copy (kept as package-level regressions distinct from the cmd-level table)
- [ ] Step 4: `cmd/quarry/run_test.go` — new `Test_run_counts_a_bundle_reached_two_ways_once` (SCENARIO-03): table over "`~/D/X.quicken` symlinks to the only bundle in `~/L`" and "`~/L` symlinked to `~/D`", both exit 0. **Both rows pass unmodified today** (Quicken folder isn't scanned yet, so the sole `~/Documents` candidate already resolves) — say so on arrival per `agent-briefs.md` → *Reporting*; they go genuinely red after Step 7 (pooling, no dedupe yet) and green again after Step 8 (dedupe)
- [ ] Step 5: `cmd/quarry/run_test.go` — new `Test_run_refuses_when_a_discovery_location_is_unreadable` (SCENARIO-04, moves the "unreadable Documents" case out of Step 1's table): 3 rows, exact stderr, exit 1, snapshots dir absent (`:279` pattern) — `~/D` unreadable/`~/L` has one → exact R3 text; `~/D` has one/`~/L` unreadable → exact R3b text; both unreadable → exact R3 text (proves order, not `Contains` — R3 and R3b share "permission denied")
- [ ] Step 6: `cmd/quarry/run_test.go` — new `Test_run_sync_help_names_both_documents_folders` (SCENARIO-05): `sync --help`, assert stdout contains both folder paths verbatim in the Long help and the `--quicken` flag help
- [ ] Step 7: `internal/cli/sync.go:137-227` `Test_run_refuses_a_bad_quicken_path` — add case: bundles exist in both folders, `--quicken` names one → exit 0 (DQ-6: proves discovery never runs; would be R2 if it did)
- [ ] Step 8: `internal/snapshot/discover_test.go:102-114` `Test_DiscoverBundle_refuses_when_a_candidate_cannot_be_statted_...` — tighten `Contains(re.Error(), "cannot read")` to the exact `unreadableRefusal` text; add a second row for the same fault under the Quicken folder

### Build
- [ ] Step 9: `internal/snapshot/discover.go:17-61` `DiscoverBundle` — scan `~/Documents` then the Quicken Documents folder in order (new `quickenDocumentsDir` beside existing pattern), pool candidates, refuse on the first location-level unreadable met (R3/R3b) without reading the second location. No dedupe yet.
- [ ] Step 10: `internal/snapshot/discover.go` `dedupeByIdentity` (new) — first-found-wins by `os.SameFile`; wire into `DiscoverBundle` before the 0/1/2+ switch
- [ ] Step 11: `internal/snapshot/discover.go:65-67` `noBundleFoundRefusal`, `:71-75` `multipleQuickenBundlesRefusal`, `:79-84` `documentsUnreadableRefusal` — update copy/signatures for two locations (full abbreviated + bytewise-sorted paths for R2); new `quickenDocumentsUnreadableRefusal` (R3b)
- [ ] Step 12: `internal/cli/sync.go:42-50,110-111` — replace `Long`'s last paragraph and the `--quicken` flag help with the verbatim copy from `specification.md` → *Surface & Copy*

### Sweep
- [ ] Step 13: fix what `go build ./... && golangci-lint run ./...` reports; `go doc ./internal/snapshot DiscoverBundle` reads as a two-location contract, no history

### Verify
- [ ] Step 14: full verification per `.claude/rules/agent-briefs.md` → *Verification*, `.claude/scripts/spec-check.py discovery-quicken-library`, tick SCENARIO-01..05 (SCENARIO-02..05 lines note "delivered by SCENARIO-01" before the test reference)

## Handoff

**Binding decisions**:
- `DiscoverBundle(home string) (string, error)` signature is unchanged — `internal/cli/sync.go:72`'s call site needs no edit beyond the help-copy strings.
- R2's message format is full `~`-abbreviated paths, bytewise-sorted, no bare filenames and no "in ~/Documents" — a future third location keeps this shape.
- Per-entry `os.Stat` faults keep the existing generic `unreadableRefusal(home, path, cause)` regardless of folder; only the location-level `ReadDir` fault is folder-specific (R3 vs R3b).

**Left unbuilt**: none — DQ-6 now has Step 7's explicit test instead of being asserted only structurally.

**Traps**:
- `os.SameFile` never errors; the `FileInfo` it compares must come from `os.Stat` (follows symlinks), not `DirEntry.Info()` (`Lstat`) — the wrong source silently defeats identity dedupe for the file-symlink row.
- Bytewise sort of full `~`-paths puts `~/Documents` before `~/Library/...` because `'D' < 'L'` — the plain sort already matches the spec's "`~/D` first" edge row, no special-casing needed.
- `~/Library/Application Support/Quicken/Backups` is never read because `quickenDocumentsDir` joins `.../Quicken/Documents`, not `.../Quicken` — do not generalize to a `filepath.Glob` over `Quicken/*`.
