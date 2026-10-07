---
id: SCENARIO-13
status: open
---

# SCENARIO-13: Two letter cases of one id: one is listed, the user is warned

Cadence: test-first — strays never pruned or deleted (delete guard, BR-C2) and BR-C5 new-name collision check (exclusive-create naming, `destination.go:77-105`)
Acceptance test: `cmd/quarry/run_snapshots_two_case_test.go` `Test_run_snapshots_lists_one_of_two_letter_cases_and_warns_naming_both`
Narrow loop: `go test ./internal/snapshot/ ./internal/cli/ ./cmd/quarry/ -run 'letter_case|stray|variant|duplicate|backup|locate|select_folder|named_as_the_snapshot'`
Mutation checks: `selectFolder` lists each regular variant → acceptance + `Test_list_never_lists_or_counts_a_stray_letter_case_variant`; stray sizes added to `TotalBytes` → `Test_run_snapshots_json_warns_about_a_stray_letter_case_with_the_absolute_folder` (`total_bytes`); stray removed with its winner → `Test_prune_never_deletes_a_stray_letter_case_variant` + `Test_sync_and_import_never_deletes_a_stray_letter_case_variant`; D1 counts non-regular entries → `Test_list_gives_no_duplicate_warning_for_a_non_regular_variant`; three-name D1 not byte-sorted → `Test_duplicate_warning_names_every_variant` (three/four rows); `warnings[]` D1 abbreviated → the json cell above; BR-C5 exact-name compare → `Test_dir_destination_backup_skips_an_id_the_folder_uses_in_another_letter_case`; seam bypassed (`os.ReadDir` inline at `scanFolder` / `locateByID` / `locateByPath` / `Backup`) → acceptance / `Test_locate_by_id_takes_the_winner_through_the_read_dir_seam` / `Test_locate_by_path_finds_the_manifest_through_the_read_dir_seam` / `Test_sync_skips_an_id_the_folder_uses_in_another_letter_case`; `NewServer` default `readDir` not `os.ReadDir` → existing `Test_run_snapshots_json_lists_an_upper_case_sqlite_snapshot_with_its_on_disk_paths`
Runs: A (1-2) | B1 (3-5) | B2 (6-7) | V (8-9)
Size: OWNS A RUN — 5 batches, 1 feature package (`internal/snapshot`; `internal/cli` renders D1)

## Implementation Plan

ReadDir survey (`grep -n 'os.ReadDir' internal/snapshot/*.go`, non-test): `list.go:155` `scanFolder` (List, PlanPrune, Prune, autoPrune) → seam · `from.go:87` `locateByID` → seam · `from.go:114` `locateByPath` (any `--from` parent) → seam · new listing in `dirDestination.Backup` → seam · `destination.go:57` `sweepLeftovers` → stays `os.ReadDir` · `discover.go:81` (Quicken Documents) → stays. Also `chosen.entry.Info()` (`list.go:165`): fakes must return a real `FileInfo` with nonzero size. No `cmd/quarry` wiring line: production takes the `NewServer` default.

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_snapshots_two_case_test.go` `Test_run_snapshots_lists_one_of_two_letter_cases_and_warns_naming_both` — `runWith` with `env.NewSnapshots` (precedent `run_prune_test.go:171-178`) building a Server `WithReadDir(<real os.ReadDir + fake regular <id>.SQLITE wrapping the real entry>)`; stdout one row for `<id>`, stderr the two-name D1 line verbatim (`specification.md:106`), exit 0
- [x] Step 2: `internal/snapshot/snapshot.go:28-44,106-110` `Server.readDir` + `WithReadDir(readDir func(dir string) ([]fs.DirEntry, error)) Option` — stub sets the field only; red at the D1 assertion

### Build
- [x] Step 3: `snapshot.go:131-137` default `readDir: os.ReadDir` beside `remove`; `list.go:154-160` `scanFolder` via `s.readDir`. Tests (seam-injected strays, assert `rm.calls` / `would_delete[]` never file existence — macOS `os.Remove("X.SQLITE")` hits `X.sqlite`): `Test_list_never_lists_or_counts_a_stray_letter_case_variant` (entries, `TotalBytes`); `Test_prune_never_deletes_a_stray_letter_case_variant` (winner beyond keep: `rm.calls` = winner only, manifest kept per `manifestShared`); `Test_plan_prune_never_would_delete_a_stray_letter_case_variant`; `Test_sync_and_import_never_deletes_a_stray_letter_case_variant` (auto-prune, `WithAutoPrune`, no prune warning); fault: seam returns `*fs.PathError{EACCES}` → `List` gives `folderUnreadableRefusal` copy; rewrite `prune_upper_case_test.go:169-207` `Test_prune_keeps_the_manifest_while_another_entry_is_named_as_the_snapshot` — all three rows (regular, directory, symlink) injected through the seam with their modes, skip removed; `newPruneServer` (`prune_delete_test.go:44-53`) takes extra opts
- [x] Step 4: `from.go:22-26,77-119` `locateFrom`/`locateByID`/`locateByPath` take the Server's `readDir` (update `from_internal_test.go:72,88`). Tests: `Test_locate_by_id_takes_the_winner_through_the_read_dir_seam` (`ImportFrom <id>` where the seam lists `X.SQLITE` + `X.Sqlite`: recorded path is `X.SQLITE`); `Test_locate_by_path_finds_the_manifest_through_the_read_dir_seam` (seam lists only `X.JSON`: manifest path `X.JSON`); seam error rows reach F1 (ID form) and F4 (path form)
- [x] Step 5: `select.go:11-97` replace `folderSelection.strays` with per-`selectedSnapshot` regular snapshot names and regular manifest names; `list.go:43-66,83-105` `Listing.Duplicates` / `DuplicatesAbsolute` built in listing order by a `duplicateWarning(folder, names, winner)` beside `noSnapshotsNote` (`list.go:107-110`). Re-point `select_internal_test.go:62-150` strays column. Tests: `Test_duplicate_warning_names_every_variant` (2 names: `both <winner> and <other>` … `the other`; 3 and 4 names: byte order, `A, B and C`, `the others`; readDir order ≠ byte order); manifest-variant row (`.json` names); `Test_list_gives_no_duplicate_warning_for_a_non_regular_variant` (directory and symlink `X.SQLITE` beside `X.sqlite`, and non-regular manifest variant); ID with snapshot and manifest variants → snapshot line then manifest line; two affected IDs → newest first; unlisted ID (only non-regular snapshot, orphan manifests) → no D1
- [x] Step 6: `internal/cli/snapshots.go:70-75` print each D1 as `quarry: warning: <line>` after `NoSnapshots`, before `StoreWarning`; `json_snapshots.go:78-89` `snapshotsWarnings` appends `DuplicatesAbsolute` between them (doc updated). cmd cells (`run_snapshots_two_case_test.go`, same seam helper): `Test_run_snapshots_json_warns_about_a_stray_letter_case_with_the_absolute_folder` (`snapshots[]` one entry, `total_bytes` winner only, `warnings[]` absolute D1 without prefix, `json.Unmarshal` read-back); order cell config warning → D1 → store warning, stderr and `warnings[]` (no-snapshots + D1: n/a — D1 implies a listed entry); `Test_run_snapshots_prune_is_silent_about_a_stray_letter_case` — text, `--json` (`deleted[]`, `warnings[]`), `--dry-run` (`would_delete[]`): stray never in `rm.calls` or output, stderr empty
- [x] Step 7: `select.go` add `folderSelection` query "some entry of any type names `id` as snapshot or manifest, any case" (over every group key, not only `snapshots`); `destination.go:33-43,77-105` `dirDestination.readDir`, `newDirDestination(dir, readDir)` with `export_test.go:11` wrapping `os.ReadDir` (27 test sites unchanged), `snapshot.go:162` passes `s.readDir`; collision check at `:92-97` = `fileExists` OR that query. Listing fault falls back to `fileExists` only. Tests: `Test_dir_destination_backup_skips_an_id_the_folder_uses_in_another_letter_case` (seam lists `<name>.SQLITE`, `<name>_2.Sqlite` → `_3`; `<name>.JSON` row; directory-typed `<name>.SQLITE` row; other-ID control → `<name>`; seam error → `<name>`); wiring pin `Test_sync_skips_an_id_the_folder_uses_in_another_letter_case` (Server `Sync`, seam reports `<n>.SQLITE` for each `.<n>.sqlite.partial` it sees → snapshot `<n>_2.sqlite`)

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `WithReadDir`, `Listing.Duplicates*`, `newDirDestination`; `grep -n 'os.ReadDir' internal/snapshot/*.go` non-test hits = `snapshot.go` default, `destination.go:57`, `discover.go:81` only

### Verify
- [ ] Step 9: full verification + `spec-check.py snapshot-safety` → tick SCENARIO-13 with its acceptance test; STATE.md drops the `strays` and BR-C8-skip entries

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `WithReadDir` is the one listing seam for every folder the Server lists (snapshots folder, `--from` parent, `Backup`); default `os.ReadDir` in `NewServer` — two-case rows exist only through it on macOS, and `snapshots`, prune, `--from` and naming must see the same listing
- `sweepLeftovers` and `discover.go` stay on `os.ReadDir` — not snapshot naming; the seam grep in Step 8 names them as the only exceptions
- D1 counts regular entries only, snapshots and manifests alike; it is built in `internal/snapshot` as `Listing.Duplicates`/`DuplicatesAbsolute`; prune, `--dry-run` and auto-prune never read it
- BR-C5 asks `selectFolder`'s query, never `snapshotFilePattern` directly; a listing fault keeps the stat check alone (no refusal, no copy) — a missed collision only leaves a never-deleted stray
- Copy as provisionally planned, pending ruling: two names = `both <winner> and <other>` (spec:106 puts the lowercase winner first, not byte order); three or more = byte order; one ID with snapshot and manifest variants = two lines, snapshot first

**Left unbuilt** — named so nobody assumes it exists:
- D1 for orphan or unlisted IDs — not listed, so no warning (by design)
- A non-regular manifest that wins (every type competes) while two regular variants are counted: "only" names a file outside the counted list — unruled, no cell

**Traps** — things that look right and are not:
- Fake strays must wrap a real entry (`Info()` nonzero size) or the `TotalBytes` mutation reddens by nil-panic, not by value
- On macOS a fake `X.SQLITE` path removes or reads the real `X.sqlite`: assert `rm.calls`, `would_delete[]`, `Entry` fields, never `FileExists`
- `fakeDirEntry` (`select_internal_test.go:17-25`) is internal-package only; `snapshot_test` and `cmd/quarry` need their own wrapper
- Test names stay lowercase (`letter_case`, `stray`) or the narrow loop misses them

## Orchestrator rulings (2026-10-06)

1. D1 two names: `both <winner> and <other>` (winner first, as the ruled example at specification.md:106); three or more: byte order (:107).
2. An ID with both snapshot and manifest variants: two D1 lines, the snapshot line first.
3. A non-regular manifest that wins: no D1 cell; stays in Left unbuilt (final product-vision pass may rule).
4. BR-C5 fail-open on a listing fault (falls back to the fileExists stat, no refusal, no new copy) accepted: worst case is a stray that is never deleted, never data loss.

## Phase report

Runs A, B1 and B2 (steps 1-7) done; V (8-9) next. Acceptance green, narrow loop green, `golangci-lint run` on snapshot/cli/cmd = 0 issues (not the full V run).

B2 files:
- `internal/cli/snapshots.go`: each `Listing.Duplicates` line printed `quarry: warning: <line>` after NoSnapshots, before StoreWarning. `json_snapshots.go`: `snapshotsWarnings` appends `DuplicatesAbsolute` between no-snapshots and store warning (doc updated).
- `internal/snapshot/select.go`: `folderSelection.used` + `uses(id)` (every ID any entry of any type is named after). `destination.go`: `dirDestination.readDir`, `newDirDestination(dir, readDir)`, `folderUses(id)` ORed with the stat check in `Backup`; listing fault = false. `snapshot.go`: `Sync` passes `s.readDir`. `export_test.go`: `NewDirDestination(dir)` wraps `os.ReadDir`; `NewDirDestinationReading` is the raw constructor.
- Tests: `cmd/quarry/run_snapshots_two_case_test.go` (json absolute D1, order cell, prune-silent table text/json/dry-run json; `runSnapshotsWithReadDir` takes `opts []snapshot.Option`), `internal/snapshot/destination_letter_case_test.go` (Backup table, Sync wiring pin), `sync_test.go` `newServer` takes trailing options.

B2 deviations: prune-silent cells expect only `<id>.sqlite` removed, not the manifest (a stray named as the snapshot makes `manifestShared` keep it, BR-C8); they were green on arrival (B1 built that). The Sync wiring pin's seam reports `<n>.SQLITE` only for the un-suffixed partial, else every candidate is refused forever. Order cell uses a corrupt store file for the store warning.

B1 files:
- `internal/snapshot/snapshot.go`: `NewServer` defaults `readDir: os.ReadDir`.
- `internal/snapshot/list.go`: `scanFolder` lists through `s.readDir`; `Listing.Duplicates`/`DuplicatesAbsolute` built in `listFolder` (abbreviated and absolute folder, listing order); `duplicateWarning(folder, names, winner)` and `selectedSnapshot.duplicates(folder)` (snapshot line, then manifest line) beside `markStore`.
- `internal/snapshot/select.go`: `folderSelection.strays` gone; `selectedSnapshot.names` (regular snapshot names, winner included, directory order) and `.manifestNames` (regular manifest names). `idGroup.manifestNames`, `entryNames` helper. A D1 manifest line is skipped when the chosen manifest is not among the regular names (ruling 3).
- `internal/snapshot/from.go`: `locateFrom`/`locateByID`/`locateByPath` are now `*Server` methods reading through `s.readDir`.
- Tests: new `read_dir_seam_test.go` (helpers `variant`, `regularVariant`, `readDirWith(t, ...variant)`; step-3 tests), `list_duplicates_test.go` (D1 text, order, non-regular, unlisted), `from_internal_test.go` (listing seam + three locate tests), `select_internal_test.go` (strays column re-pointed via `strayNames`; names table), `prune_upper_case_test.go` BR-C8 test now seam-injected for all three rows, no skip. `newListServer` and `newPruneServer` take trailing `...snapshot.Option`.

Notes: `readDirWith`/`variant` are `snapshot_test` helpers; `cmd/quarry` has its own `readDirWithVariant`. V: Step 8 (doc comments, `os.ReadDir` grep), Step 9.
