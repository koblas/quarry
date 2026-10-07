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

**Invariant.** Prune and auto-prune never delete a manifest whose snapshot exists under any letter case or entry type, and every path they remove or report is a name `os.ReadDir` returned, never rebuilt from the ID. The sweep has to work by name. How this plan treats each way two names can reach one file:
- Letter case is folded, for the winner and for blocking orphans. An entry of any type named `<id>.sqlite` in any case blocks orphaning but never wins.
- `.partial` stays exact lowercase, as quarry writes it and as `leftoverPartialPattern` (destination.go:21) matches it.
- A hard link under another ID is unchanged (`storeFile` SameFile in `markStoreSnapshot`). Another extension (`X.db`) leaves `X.json` an orphan, unchanged and accepted.

**Folding.** Only the extension is folded, with Unicode simple folding (the same as `strings.EqualFold`, which 12b's ruled `SnapshotID` uses). The ID part stays strict. **Coverage.** No new fallible call: `Info`, `readManifest` and `remove` are existing calls that now take on-disk names. Their fault tests stay (list_test.go:342, prune_delete_test.go:98-157); re-key any `failing(name)` to the on-disk name.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_snapshots_upper_case_test.go` (new) `Test_run_snapshots_json_lists_an_upper_case_sqlite_snapshot_with_its_on_disk_paths`
  - Setup: a newer lowercase pair, and the oldest snapshot renamed to `<id>.SQLITE` with `<id>.json`. `buildStoreFrom` the `.SQLITE` path. Assert the ReadDir names first.
  - Asserts the full document: `id` has no extension; `path` ends `.SQLITE`; `manifest` ends `.json`; `store: true`; `total_bytes` counts both; `store_snapshot` = `{id, …/<id>.SQLITE}`. Expected paths are literals.
- [x] Step 2: `internal/snapshot/prune_upper_case_test.go` (new) `Test_prune_deletes_an_upper_case_sqlite_snapshot_then_its_manifest_and_keeps_every_newer_manifest`
  - Setup: `A.SQLITE`+`A.json` (newest), `B.sqlite`+`B.json`, `C.SQLITE`+`C.json` (oldest); keep 2; `fakeRemover`.
  - Asserts `rm.calls == [C.SQLITE, C.json]`, `Deleted[0].Path` on-disk, and `A.json` still in the ReadDir names.
  - No stubs needed. Both tests are red at their assertions today: `.SQLITE` is not listed, and `A.json`/`C.json` are swept.

### Build
- [x] Step 3: `internal/snapshot/destination.go:23-27` `snapshotFilePattern`/`manifestFilePattern` fold the extension only. New `internal/snapshot/select.go` `selectFolder(dirEntries []fs.DirEntry) folderSelection` returns, per ID, the winning snapshot entry and manifest name, plus `strays` and `orphans`. `internal/snapshot/import.go:152-156` `ID` strips one `.sqlite` in any case. Test-first:
  - `select_internal_test.go` `Test_select_folder_picks_one_snapshot_and_one_manifest_per_id` (table, fake `fs.DirEntry`). Rows:
    - lone `X.SQLITE`; `X.SQLITE`+`X.sqlite` → `X.sqlite` wins, the other is a stray; `X.SQLITE`+`X.Sqlite` → `X.SQLITE`; three variants
    - a directory or symlink `X.SQLITE` beside regular `X.Sqlite` → `X.Sqlite`, no stray; non-regular only → no snapshot
    - manifests `X.json`/`X.JSON`/`X.Json` by the same rule; a symlinked manifest competes
    - lowercase-`t`/`z` ID → neither a snapshot nor a manifest; `X.ſqlite` folds; `X.db`, `X.sqlite.partial`, `notes.json` → ignored
  - `Test_select_folder_finds_orphan_manifests`. Rows:
    - `X.json` alone → orphan; `X.JSON`+`X.json` with no snapshot → both orphans; beside `X.db` → orphan
    - beside `X.SQLITE`, beside a directory or symlink `X.SQLITE`, or beside `.X.sqlite.partial` → not an orphan
    - a non-regular `X.json` alone → not an orphan
  - `import_test.go` `Test_id_strips_one_sqlite_extension_in_any_letter_case`. Rows: `.sqlite`/`.SQLITE`/`.Sqlite`; `latest.db` unchanged; `X.sqlite.SQLITE` → `X.sqlite`; `X.json` unchanged; no extension.
- [x] Step 4: Wire the selection into the listing:
  - `internal/snapshot/list.go:23-37` `Entry` gains exported `ManifestPath`: on-disk, set whenever a manifest was selected, readable or not.
  - `list.go:79-98` `listFolder` takes `Path` and `ManifestPath` from the selection and calls `readManifest(ManifestPath)`.
  - `list.go:139-176` `scanFolder` runs over `selectFolder`, calls `Info()` on the winner, keeps its refusal, and joins the orphan paths.
  - `internal/cli/json_snapshots.go:62-69` `manifest` comes from `e.ManifestPath`, still gated on `e.Manifest != nil`.
  - Tests:
    - `list_test.go` `Test_list_gives_an_upper_case_sqlite_snapshot_its_on_disk_path_and_manifest`. Rows `X.SQLITE`+`X.json`, `X.sqlite`+`X.JSON`, `X.SQLITE`+`X.JSON`; asserts `Path`, `ManifestPath`, `Manifest` and `TotalBytes`.
    - `Test_list_lists_no_directory_or_symlink_named_as_an_upper_case_snapshot`.
    - cmd text cell `Test_run_snapshots_lists_an_upper_case_sqlite_snapshot_like_a_lowercase_one` (Step 1 file): the row is byte-identical to a lowercase one.
  - The acceptance test goes green here.
- [x] Step 5: `internal/snapshot/prune.go:146-163` `deleteSnapshot` removes `entry.ManifestPath`: Lstat-regular kept, skipped when empty. Delete `manifestPath` (list.go:106-107); it has no callers left.
  - `prune_upper_case_test.go`:
    - `Test_prune_removes_the_manifest_by_its_on_disk_name`: rows `X.sqlite`+`X.JSON` and `X.SQLITE`+`X.JSON`, exact `rm.calls`.
    - `Test_prune_never_sweeps_the_manifest_of_a_directory_named_as_an_upper_case_snapshot` (on disk; control: `Test_prune_sweeps_only_orphan_manifests`).
    - `Test_prune_reports_an_upper_case_sqlite_snapshot_it_could_not_delete` (`failing("X.SQLITE", EACCES)`): `Failed[0].Entry.Path` on-disk, `X.json` kept.
    - `Test_plan_prune_would_delete_an_upper_case_sqlite_snapshot_by_its_on_disk_path`.
  - `auto_prune_test.go` `Test_sync_and_import_deletes_an_upper_case_sqlite_snapshot_then_its_manifest`: checks `rm.calls`; the newer `.SQLITE` snapshot's manifest is not swept.
  - cmd, Step 1 file: `Test_run_snapshots_prune_deletes_an_upper_case_sqlite_snapshot_and_its_manifest`, a table over text and `--json`. The text row shows the ID; `deleted[].path` ends `.SQLITE`; the ReadDir names left over equal a literal list.
  - n/a: sync `--json` `pruned.deleted[]` uses `newPrunedEntryDocuments` (json.go:220); `failed[].path` is `Entry.Path` (json_prune.go:43), pinned by the Server fault row.

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`. Doc comments on `ID` ("its extension, in any letter case", Surface change 3), `Entry`/`ManifestPath` (on-disk), the two patterns, and `selectFolder`.

### Verify
- [ ] Step 7: full verification + `spec-check.py snapshot-safety`. Tick SCENARIO-10 with its acceptance test, and SCENARIO-11 "delivered by SCENARIO-10" with its folded test. Rewrite STATE.md.

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `selectFolder` (select.go) is the one name decision for snapshot, manifest, stray and orphan. 12a's BR-C6 `--from <id>` and 13's D1/BR-C5 call it and never re-derive from the patterns. Otherwise `snapshots`, prune and `--from` disagree about which file is `<id>`.
- Only the extension is folded, with Unicode simple folding (agrees with 12b's `strings.EqualFold`). The ID stays strict: a whole-pattern fold makes a lowercase-`t`/`z` name prunable.
- Snapshot winners come from regular entries only. Every entry type competes for the manifest, because `readManifest` follows symlinks (today's listing behaviour, and BR-C7's rule for 12a). Orphan candidates are regular manifests only. An orphan is blocked by any-type, any-case `<id>.sqlite` or by exact `.<id>.sqlite.partial`.
- `Entry.Path` and `Entry.ManifestPath` are ReadDir names, used by every remove and every `--json` path. `manifestPath(id)` is gone. Entries are still marked through `markStoreSnapshot` (S14 binding).

**Left unbuilt:**
- `folderSelection.strays` is computed but has no consumer. D1, the `WithReadDir` seam and BR-C5 (destination.go:93) belong to SCENARIO-13, which also rules whether D1 counts non-regular manifest variants.
- `from.go:76-89` `resolveFrom` belongs to 12a; `report.SnapshotID` belongs to 12b.

**Traps:**
- macOS temp dirs are case-insensitive. `FileExists`, `os.Remove` and `readManifest` on a rebuilt lowercase name all reach the `.SQLITE`/`.JSON` file. Name mutations redden only on `rm.calls`, `Entry` fields, `--json` strings or ReadDir names.
- `writeSnapshot`/`writeManifest` write lowercase names. Rename to the upper-case name, then assert the ReadDir name, or the fixture proves nothing.
- 12a's, not S10's:
  - `run_case_variant_test.go:25-38` (lowercased-ID `--from`) likely becomes F3 under BR-C6's strict ID.
  - `run_store_identity_test.go:247-256` hard-links `X.SQLITE.json`, which pins from.go:89.
- Green, no edit, keep the skip: `run_case_variant_test.go:40-75` and `list_store_identity_test.go:84-92,131-139` pin identity through a differently-cased name via `markStoreSnapshot`, which S10 leaves alone. The same holds for `Test_prune_leaves_a_manifest_that_is_not_a_regular_file`, `Test_prune_sweeps_only_orphan_manifests` and `Test_list_ignores_everything_that_is_not_a_snapshot_file`.
- Test names stay lowercase (`upper_case_sqlite`), or the narrow loop misses them.

## Orchestrator rulings (2026-10-06)

- Accepted: every entry type competes for the manifest winner (BR-C7 mid-scope ruling); orphan candidates regular manifests only; deleteSnapshot keeps its regular-file check.
- Accepted: Unicode simple folding (agrees with (?i) and strings.EqualFold, 12b); pin X.ſqlite.
- Accepted: .partial guard stays exact lowercase.
- Accepted: the Triage Brief re-points are 12a's or unaffected (listed in Handoff Traps).

## Phase report

Run B1 (Steps 3-5) done; both acceptance tests green. Commit 98fc956c plus one follow-up (orphan lower-case t/z rows, `partials`→`names` rename). Run A: start 2bd1adfb, acceptance 2aa5a7f5.

Production:
- `internal/snapshot/select.go` (new): `selectFolder`, `folderSelection{snapshots, strays, orphans}`, `selectedSnapshot{entry, id, stamp, suffix, manifest}`, `preferred`/`rankName`. Names only; `strays` has no consumer (13's).
- `destination.go:23-27` patterns fold the extension via `(?i:...)`; `import.go` `ID` uses `sqliteExtension` regexp.
- `list.go`: `Entry.ManifestPath`; `snapshotFile` gains `name`, `manifest`; `scanFolder` runs over `selectFolder`; `listFolder` reads `ManifestPath`; `manifestPath()` deleted.
- `prune.go` `deleteSnapshot` removes `entry.ManifestPath` (Lstat-regular kept, skipped when empty). `internal/cli/json_snapshots.go` `manifest` = `e.ManifestPath`.

Tests (new): `select_internal_test.go` (white-box, fake DirEntry; picks 17 rows, orphans 16), `import_test.go` ID table, `list_upper_case_test.go` (+ helper `renamedExtension`), `prune_upper_case_test.go` (4 tests), `auto_prune_test.go` 1 test, cmd text cell and prune text/json table in the Step 1 file.

Deviations: list tests are in `list_upper_case_test.go`, not `list_test.go` (file size). Step 5's unit tests were written after its production edit, not before; seen red only through the plan's `deleteSnapshot` mutation.
Green on arrival: `Test_list_lists_no_directory_or_symlink_named_as_an_upper_case_snapshot` (the pattern fold alone already skips non-regular entries; it pins the type guard, which the select tests also redden).

Mutations (all restored, byte-identical): see report; all six plan mutations went red, whole-manifest-`(?i)` first SURVIVED, fixed by two orphan rows.

Next (V): sweep lint, doc comments (`selectFolder` etc. already carry docs; check budget), `verify.sh 2bd1adfb ./internal/snapshot/... ./internal/cli/... ./cmd/quarry/...`, spec tick, STATE.md rewrite.
