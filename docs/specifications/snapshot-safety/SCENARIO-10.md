---
id: SCENARIO-10
status: open
---

# SCENARIO-10: An upper-case .SQLITE snapshot is listed with its on-disk paths

Cadence: test-first — destructive guard: the selector decides which manifests the orphan sweep deletes and which files prune removes (BR-C2, BR-C3, BR-C4)
Acceptance test: `cmd/quarry/run_snapshots_upper_case_test.go` `Test_run_snapshots_json_lists_an_upper_case_sqlite_snapshot_with_its_on_disk_paths`
Acceptance test (SCENARIO-11, folded): `internal/snapshot/prune_upper_case_test.go` `Test_prune_deletes_an_upper_case_sqlite_snapshot_then_its_manifest_and_keeps_every_newer_manifest`
Narrow loop: `go test ./internal/snapshot/ ./internal/cli/ -run 'select_folder|upper_case|id_strips|prune|list'` then `go test ./cmd/quarry/ -run 'upper_case'`
Mutation checks: orphan check back to exact `id+".sqlite"` in `selectFolder` → `Test_select_folder_finds_orphan_manifests` + S11 acceptance; entry path rebuilt as `id+".sqlite"` (list.go:86) → acceptance (`path`) + S11 acceptance (`rm.calls`); winner = byte-order first over exact lowercase → `Test_select_folder_picks_one_snapshot_and_one_manifest_per_id` (`X.SQLITE`+`X.sqlite` row); non-regular `<id>.SQLITE` no longer blocks orphaning → `Test_select_folder_finds_orphan_manifests` + `Test_prune_never_sweeps_the_manifest_of_a_directory_named_as_an_upper_case_snapshot`; whole-pattern `(?i)` instead of extension-only → selector lowercase-`t`/`z` ID row; `deleteSnapshot` manifest rebuilt as `id+".json"` → `Test_prune_removes_the_manifest_by_its_on_disk_name`
Runs: A (1-2) | B1 (3-5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (`internal/snapshot`; cli/cmd cells only)

**Invariant.** Prune and auto-prune never delete a manifest whose snapshot exists under any letter case or entry type, and every path they remove or report is a name `os.ReadDir` returned, never rebuilt from the ID. The sweep has to work by name. Each way two names can reach one file, and how this plan treats it:
- **Letter case.** Folded, both for which ID wins and for blocking the orphan sweep.
- **Entry type.** A directory, symlink or fifo named `<id>.sqlite` (any case) blocks orphaning but never wins.
- **`.partial`.** Matched in exact lowercase, as quarry writes it and as `leftoverPartialPattern` (destination.go:21) matches it.
- **Hard link under another ID.** Unchanged. It is guarded by `storeFile` SameFile in `markStoreSnapshot`.
- **Other extension (`X.db`, `.sqlite3`).** `X.json` stays an orphan. That is unchanged and accepted.

**Folding.** Only the extension is folded, with Unicode simple folding. That matches `strings.EqualFold`, which is what 12b's ruled `SnapshotID` uses. The ID part stays `\d{8}T\d{6}Z(_\d+)?`.

**Coverage.** The batches add no new fallible call. `Info`, `readManifest` and `remove` are existing calls that now take on-disk names. Their fault tests stay in place: list_test.go:342 and prune_delete_test.go:98-157. Re-key any `failing(name)` to the on-disk name.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_snapshots_upper_case_test.go` (new) `Test_run_snapshots_json_lists_an_upper_case_sqlite_snapshot_with_its_on_disk_paths` — setup:
  - a newer lowercase pair, plus the oldest snapshot renamed to `<id>.SQLITE` with `<id>.json`;
  - `buildStoreFrom` the `.SQLITE` path;
  - assert the fixture's `os.ReadDir` names first.

  Asserts the full document:
  - entry `id` has no extension;
  - `path` ends `.SQLITE` and `manifest` ends `.json`;
  - `store: true`;
  - `total_bytes` counts both;
  - `store_snapshot` = `{id: <id>, path: …/<id>.SQLITE}`.

  Expected paths are literals.
- [ ] Step 2: `internal/snapshot/prune_upper_case_test.go` (new) `Test_prune_deletes_an_upper_case_sqlite_snapshot_then_its_manifest_and_keeps_every_newer_manifest`:
  - setup: newest `A.SQLITE`+`A.json`, `B.sqlite`+`B.json`, oldest `C.SQLITE`+`C.json`, keep 2, `fakeRemover`;
  - asserts `rm.calls == [C.SQLITE, C.json]`, `Deleted[0].Path` is the on-disk name, and `A.json` is still among the ReadDir names.

  Both tests need no stubs. Both must be red at their assertions: today the `.SQLITE` files are not listed and `A.json`/`C.json` are swept as orphans.

### Build
- [ ] Step 3: `internal/snapshot/destination.go:23-27` `snapshotFilePattern`/`manifestFilePattern` (fold the extension only), plus a new `internal/snapshot/select.go` with `selectFolder(dirEntries []fs.DirEntry) folderSelection`. `selectFolder` returns:
  - per ID: the winning snapshot entry and manifest name;
  - `strays`;
  - `orphans`.

  Also `internal/snapshot/import.go:152-156` `ID`, which strips one `.sqlite` extension in any case.

  Tests are test-first:
  - **`select_internal_test.go` `Test_select_folder_picks_one_snapshot_and_one_manifest_per_id`** — table, with a fake `fs.DirEntry`. Rows:
    - lone `X.SQLITE`;
    - `X.SQLITE`+`X.sqlite` → `X.sqlite`, other is a stray;
    - `X.SQLITE`+`X.Sqlite` → `X.SQLITE`;
    - three variants;
    - directory and symlink `X.SQLITE` beside regular `X.Sqlite` → `X.Sqlite`, no stray;
    - non-regular only → no snapshot;
    - manifest rows `X.json`/`X.JSON`/`X.Json` by the same rule;
    - a symlinked manifest competes;
    - ID with lowercase `t`/`z` → neither a snapshot nor a manifest;
    - `X.ſqlite` (simple fold);
    - `X.db`, `X.sqlite.partial`, `notes.json` → ignored.
  - **`Test_select_folder_finds_orphan_manifests`** — rows:
    - `X.json` alone → orphan;
    - beside `X.SQLITE` → not an orphan;
    - beside a directory or symlink `X.SQLITE` → not an orphan;
    - beside `.X.sqlite.partial` → not an orphan;
    - `X.JSON` and `X.json`, no snapshot → both orphans;
    - non-regular `X.json` alone → not an orphan;
    - beside `X.db` → orphan.
  - **`import_test.go` `Test_id_strips_one_sqlite_extension_in_any_letter_case`** — rows:
    - `.sqlite`, `.SQLITE`, `.Sqlite`;
    - `latest.db` unchanged;
    - `X.sqlite.SQLITE` → `X.sqlite`;
    - `X.json` unchanged;
    - no extension.
- [ ] Step 4: Put the selection into the listing:
  - `internal/snapshot/list.go:23-37` `Entry`: new exported `ManifestPath`. It is the on-disk manifest path, set whenever a manifest was selected, readable or not.
  - `list.go:79-98` `listFolder`: `Path` and `ManifestPath` come from the selection, and `readManifest(ManifestPath)` reads it.
  - `list.go:139-176` `scanFolder` runs over `selectFolder`. It calls `Info()` on the winner, keeps the refusal unchanged and joins orphan paths.
  - `internal/cli/json_snapshots.go:62-69`: `manifest` comes from `e.ManifestPath`, still gated on `e.Manifest != nil`.

  Tests:
  - **`list_test.go` `Test_list_gives_an_upper_case_sqlite_snapshot_its_on_disk_path_and_manifest`** — rows `X.SQLITE`+`X.json`, `X.sqlite`+`X.JSON`, `X.SQLITE`+`X.JSON`. Asserts `Path`, `ManifestPath`, `Manifest` non-nil and `TotalBytes`.
  - **`Test_list_lists_no_directory_or_symlink_named_as_an_upper_case_snapshot`**.
  - **cmd text cell `Test_run_snapshots_lists_an_upper_case_sqlite_snapshot_like_a_lowercase_one`**, in the Step 1 file. The row is byte-identical to a lowercase one.

  The acceptance test goes green here.
- [ ] Step 5: `internal/snapshot/prune.go:146-163` `deleteSnapshot` removes `entry.ManifestPath`. It keeps the Lstat-regular check and skips when the path is empty. Delete `manifestPath` (list.go:106-107), which then has no callers.

  Tests in `prune_upper_case_test.go`:
  - **`Test_prune_removes_the_manifest_by_its_on_disk_name`** — rows `X.sqlite`+`X.JSON` and `X.SQLITE`+`X.JSON`. Asserts exact `rm.calls`.
  - **`Test_prune_never_sweeps_the_manifest_of_a_directory_named_as_an_upper_case_snapshot`** — on disk. The control is `Test_prune_sweeps_only_orphan_manifests`.
  - **`Test_prune_reports_an_upper_case_sqlite_snapshot_it_could_not_delete`** — `failing("X.SQLITE", EACCES)`. Asserts `Failed[0].Entry.Path` is the on-disk name and `X.json` is kept.
  - **`Test_plan_prune_would_delete_an_upper_case_sqlite_snapshot_by_its_on_disk_path`**.

  Test in `auto_prune_test.go`:
  - **`Test_sync_and_import_deletes_an_upper_case_sqlite_snapshot_then_its_manifest`** — uses `rm.calls`. The newer `.SQLITE` manifest is not swept.

  cmd cells, in the Step 1 file:
  - **`Test_run_snapshots_prune_deletes_an_upper_case_sqlite_snapshot_and_its_manifest`** — table over text and `--json`. The text row shows the ID. `--json` `deleted[].path` ends `.SQLITE`. The ReadDir names left over equal a literal list.
  - **Not pinned (n/a):**
    - sync `--json` `pruned.deleted[]`: it goes through `newPrunedEntryDocuments`, json.go:220.
    - `failed[].path`: json_prune.go:43 renders `Entry.Path`, which the Server fault row pins.

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`. Doc comments:
  - `ID` — "its extension, in any letter case" (Surface change 3);
  - `Entry` — on-disk paths;
  - `ManifestPath`;
  - the two patterns;
  - `selectFolder`.

### Verify
- [ ] Step 7: run the full verification and `spec-check.py snapshot-safety`. Then:
  - tick SCENARIO-10 with its acceptance test;
  - tick SCENARIO-11 "delivered by SCENARIO-10" with its folded test;
  - rewrite STATE.md.

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- **`selectFolder` (select.go) is the one name decision** for snapshot, manifest, stray and orphan. 12a's BR-C6 `--from <id>` and 13's D1/BR-C5 call it and never re-derive from the patterns. Otherwise `snapshots`, prune and `--from` disagree about which file is `<id>`.
- **Only the extension is folded, with Unicode simple folding.** The ID stays strict. 12b's `strings.EqualFold` agrees with it. A whole-pattern fold would make a lowercase-`t`/`z` name prunable.
- **Which entry types compete:**
  - snapshots: regular entries only;
  - manifests: every entry type, because `readManifest` follows symlinks (today's listing behaviour, and BR-C7's rule for 12a);
  - orphan candidates: regular manifests only.

  An orphan is blocked by an `<id>.sqlite` of any type in any case, or by an exact-lowercase `.<id>.sqlite.partial`.
- **On-disk names.** `Entry.Path` and `Entry.ManifestPath` are names from ReadDir. Every remove and every `--json` path uses them. `manifestPath(id)` is gone. Entries are still marked through `markStoreSnapshot` (S14 binding).

**Left unbuilt:**
- `folderSelection.strays` is computed and has no consumer yet. The D1 warning, the `WithReadDir` seam and BR-C5 at destination.go:93 belong to SCENARIO-13. 13 also rules whether D1 counts non-regular manifest variants.
- `from.go:76-89` `resolveFrom` belongs to 12a. `report.SnapshotID` belongs to 12b.

**Traps:**
- **macOS temp dirs are case-insensitive.** `FileExists`, `NoFileExists`, `os.Remove` and `readManifest` on a rebuilt lowercase name all reach the `.SQLITE`/`.JSON` file. Name mutations only redden on `rm.calls`, `Entry` fields, `--json` strings or ReadDir names.
- **Fixture names.** `writeSnapshot`/`writeManifest` write lowercase names. Rename to the upper-case name, then assert the ReadDir name, or the fixture proves nothing.
- **Owned by 12a.** `run_case_variant_test.go:25-38`, a lowercased-ID `--from`, will probably turn into F3 under BR-C6's strict ID. `run_store_identity_test.go:247-256` hard-links `X.SQLITE.json`, which pins from.go:89. Neither changes under S10.
- **Green, no edit, keep the skip.** `run_case_variant_test.go:40-75` and `list_store_identity_test.go:84-92,131-139` pin store identity through a differently-cased name via `markStoreSnapshot`, which S10 does not touch. The same holds for `Test_prune_leaves_a_manifest_that_is_not_a_regular_file`, `Test_prune_sweeps_only_orphan_manifests` and `Test_list_ignores_everything_that_is_not_a_snapshot_file`.
- **Test names stay lowercase** (`upper_case_sqlite`, never `SQLITE`), or the narrow loop misses them.
