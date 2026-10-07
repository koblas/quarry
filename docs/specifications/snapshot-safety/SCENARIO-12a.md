---
id: SCENARIO-12a
status: open
---

# SCENARIO-12a: sync --from <id> resolves an .SQLITE snapshot

Cadence: code-first — read-only resolution; no deletion, overwrite, symlink-refusal or atomicity code (none of the `build.md` mandatory items)
Acceptance test: `cmd/quarry/run_from_case_test.go` `Test_run_sync_from_an_id_rebuilds_the_store_from_an_upper_case_sqlite_snapshot`
Narrow loop: `go test ./internal/snapshot/ ./cmd/quarry/ -run 'from|select|resolve'` (lowercase-matched: every new test name carries `from`; case rows stay lowercase, `upper_case_sqlite`)
Mutation checks: ID-form lowercase fallback when folder unreadable → F1 `0300` and `000` rows of `Test_run_sync_from_refuses_what_it_cannot_resolve`; ID-form `os.Stat` instead of the selector → lowercased-ID F3 row, directory and symlink F3 rows, and the acceptance test (recorded path); path-form manifest `strings.TrimSuffix(path, ".sqlite")` → `X.SQLITE` absolute and relative rows of `Test_run_sync_from_a_path_finds_its_snapshot_and_manifest_in_any_letter_case`; non-regular entry accepted by ID form → directory and symlink F3 rows and `Test_folder_selection_snapshot_lookup_skips_non_regular_entries`
Runs: A (1) | B1 (2-4) | V (5-6)
Size: OWNS A RUN — 3 Build batches, 1 feature package (`snapshot`; `cmd/quarry` tests only). LIGHT demoted: delivers 6 ruled lines (F1-F6), more than 3.

Surface (specification.md "`--from` resolution"): F1/F4 `quarry: cannot read <folder>: permission denied` via the shared helper; F2/F3 the existing `no snapshot <id> in …` line; F5 unchanged `cannot read <on-disk manifest>: <reason>; check the file's permissions`; F6 = F1 on a 0300 folder. All exit 1, stdout empty also under `--json`. No "changed nothing". Ruled strings are literals in tests, never read from production constants.

Surveyed surface (nothing new to port): `resolveFrom` has one caller, `ImportFrom` (from.go:26); the only fs calls are `os.Stat` (from.go:95), `os.ReadFile` (:137), `os.ReadDir` (new). `folderUnreadableRefusal` callers: list.go:160, :167 (+ doc mention auto_prune.go:49). No `WithReadDir` seam (13 owns it): faults are real chmod.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_from_case_test.go` (new) `Test_run_sync_from_an_id_rebuilds_the_store_from_an_upper_case_sqlite_snapshot` — `run` slice (model: run_from_test.go:18-46): sync, remove the store, `os.Rename` `<id>.sqlite` → `<id>.SQLITE`, assert the ReadDir name first (macOS folds case), `sync --from <id>` exit 0, `import_runs.snapshot_path` (via `importRunQuery`, run_import_runs_test.go:127) and `status --json` `snapshot.path` equal the on-disk `.SQLITE` path. No stubs; red at the path assertion (macOS) / exit code (Linux).

### Build
- [ ] Step 2 (batch 1, ID form, F1/F2/F3, changes 8-9): `list.go:191-197` `folderUnreadableRefusal` → free func over a folder argument (callers :160, :167; fix mention at `auto_prune.go:49`); `select.go:98` add `(folderSelection) snapshot(id)` beside `preferred`; `from.go:22-39,77-90,94-110` ID form calls `os.ReadDir(snapshotDir)` once → not-exist = F2, other error = F1, no `selectFolder` winner for the value = F3 (the value verbatim in the line, `idNotFoundRefusal` :118); manifest = the winner's selected manifest, a lowercase `<id>.json` path when none (reaches today's no-manifest line); NO `os.Stat` and no lowercase fallback on this form. Tests: `import_from_test.go:360-370` re-point (path form keeps `cannot read <snapshot>: permission denied; check the file's permissions` on a 000 parent) + ID-form F1 rows (000, 0300, snapshotDir is a file → `not a directory`); `:383-396` re-point to the path form (keeps `is not a snapshot file`) + ID-form F3 cases (directory, symlink, lowercased ID, folder missing = F2); new ID-form success naming `.SQLITE`; `select_internal_test.go` `Test_folder_selection_snapshot_lookup_skips_non_regular_entries` (regular winner, a non-regular sibling of the ID, absent ID).
- [ ] Step 3 (batch 2, path form, BR-C7, F4/F5): `select.go` add `selectManifest(dirEntries, stem)` — every entry type competes, ext folds (`strings.EqualFold`), stem exact, `preferred(…, stem+".json")` so lowercase wins then byte order; `from.go:77-90` path test = `.sqlite` in any case or a `/`, strip one such ext, `Stat` first (:94 order kept: not-exist / bundle / not-a-file refusals precede any listing), then list `filepath.Dir`, any error = F4 via the helper with that parent, no match = the `…/<stem>.json` path so `manifestReadRefusal` (:171-182) gives the existing no-manifest line. Tests: `from_internal_test.go:14-57` rewrite for the split (path shapes incl. `.SQLITE`, `/a/b` no-ext, `~`, relative; ID shape moves to Step 2's tests); `select_internal_test.go` `selectManifest` table (lowercase beats `.JSON`; byte order between `.Json`/`.JSON`; directory and symlink competing; `X.SQLITE.json` and `X.json.bak` do not match; stem prefix `X2.json` does not match); `import_from_test.go` F4 (parent 0300, snapshot present), F5 (upper-case manifest, mode 0, on-disk name in the line), no-manifest in any case. Skip as root; chmod back to 0700 in `t.Cleanup`.
- [ ] Step 4 (batch 3, command cells + re-points): `cmd/quarry/run_from_refusals_test.go` (after :182) `Test_run_sync_from_refuses_what_it_cannot_resolve` — rows F1 (0300, 000), F3 (directory, symlink, lowercased ID), F4 (parent `~/Backups` 0300), F2 (folder missing), each as text and `--json` (stdout empty, exact stderr, exit 1, no store file); `cmd/quarry/run_from_case_test.go` `Test_run_sync_from_a_path_finds_its_snapshot_and_manifest_in_any_letter_case` — `X.SQLITE` absolute and relative (`t.Chdir`), `X.sqlite` + `X.JSON`, `X.SQLITE` + `X.JSON`, a symlink named `<id>.sqlite` reached by path form still succeeds (change 9), text and `--json`; `run_case_variant_test.go:25-38` → replace by the F3 lowercased-ID pin (strict ID; drop the skip); `run_store_identity_test.go:247-256` drop the `X.SQLITE.json` hard link (:253), manifest is now `X.json`.

### Sweep
- [ ] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `ImportFrom` (:17-21: ID resolves the way `snapshots` lists it) and the new helpers, short and id-free.

### Verify
- [ ] Step 6: `.claude/scripts/verify.sh <start> ./internal/snapshot/... ./cmd/quarry/...`, `.claude/scripts/spec-check.py snapshot-safety`; tick SCENARIO-12a in `specification.md` with its acceptance test; rewrite STATE.md; `status: done`.

## Handoff

**Binding decisions:**
- ID-form `--from` takes its file and manifest from `selectFolder` (via `folderSelection.snapshot`), lists once, does no `os.Stat` — `snapshots` and `--from` must agree on which file is `<id>`, and `os.Stat` follows symlinks and folds case on macOS.
- Path-form manifest comes from `selectManifest` (select.go, shares `preferred`), not `selectFolder`: the stem need not match the ID pattern (`latest.sqlite`, `~/Backups/x.sqlite`). Name decisions stay in select.go; from.go only calls them.
- `folderUnreadableRefusal` becomes a free function taking the folder; F1 and F4 differ only by that argument. 13's D1/`WithReadDir` work builds on the same helper.

**Left unbuilt:**
- `report.SnapshotID` any-case, status line, `summary --json` id — 12b.
- D1 text, `folderSelection.strays` consumer, `WithReadDir`, BR-C5 — 13.

**Traps:**
- macOS folds case: a lowercase-built name still stats and reads as `.SQLITE`/`.JSON`. `X.JSON` rows cannot go red by name on macOS; the `selectManifest` table, F5's on-disk-name line and Linux carry them. Fixtures rename, then assert ReadDir names before acting.
- Path-form `Stat` must stay before the parent listing, or a missing path reports F4 instead of "does not exist" (`run_from_refusals_test.go:29-39`).
- Lowercased ID is F3 now, so `run_case_variant_test.go:25-38` is a rewrite, not a rename; its keep-beyond-the-newest-12 assertions go away.
- A hard link named `X.SQLITE.json` no longer pins anything; the `.SQLITE` extension strip is what the re-pointed identity row proves.
