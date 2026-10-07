---
id: SCENARIO-14
status: open
---

# SCENARIO-14: Prune refuses when the store's recorded snapshot cannot be read

Cadence: test-first — destructive guard (what prune and auto-prune may delete)
Acceptance test: `cmd/quarry/run_prune_recorded_test.go` `Test_run_snapshots_prune_refuses_when_the_recorded_snapshot_cannot_be_read`
Acceptance test (SCENARIO-15, folded): `cmd/quarry/run_snapshots_recorded_test.go` `Test_run_snapshots_warns_and_marks_nothing_when_the_recorded_snapshot_cannot_be_read`
Acceptance test (SCENARIO-16, folded): `internal/snapshot/auto_prune_recorded_test.go` `Test_sync_and_import_prunes_nothing_when_the_recorded_snapshot_cannot_be_read`
Narrow loop: `go test ./internal/snapshot/ ./internal/cli/ ./cmd/quarry/ -run 'recorded|prune|list_marks|snapshots_json'`
Mutation checks: not-exist test widened to `err != nil` in the recorded-path classifier → acceptance prune test + EACCES row of `Test_list_marks_nothing_and_warns_when_the_recorded_snapshot_cannot_be_read`; not-exist arm dropped (ENOENT treated as cannot-tell) → ENOENT rows `list_store_identity_test.go:184-217` + ENOENT control of `Test_prune_deletes_nothing_when_the_recorded_snapshot_cannot_be_read`; ID fallback (`storeEntryIndex`, list.go:243) still run on the unreadable arm → EACCES row of the listing test + `snapshots --json` EACCES cell (`store:false`); `autoPrune` ignores the classifier's result → `Test_sync_and_import_prunes_nothing_when_the_recorded_snapshot_cannot_be_read`; `snapshotsWarnings` appends `StoreWarning` instead of `StoreWarningAbsolute` → `snapshots --json` EACCES cell
Runs: A (1-3) | B1 (4-6) | V (7-8)
Size: OWNS A RUN — 3 batches, 1 feature package (`internal/snapshot`; plus `internal/cli` warnings[] and `cmd/quarry` cells)

**Invariant** (BR-U1): prune and auto-prune delete nothing unless the store's recorded path either stats (identity known: every entry that is that file is spared by `os.SameFile`) or is reported not-exist (no file there for any entry to be). Every other stat outcome — EACCES, ELOOP, ENOTDIR, ENAMETOOLONG — is cannot-tell: nothing marked, nothing deleted, no ID fallback. Name matching (ID fallback, `storeEntryIndex`) only picks the entry output names, and runs only on the stat-ok / not-exist arms.

**Grid** — recorded-path class {nil, ENOENT, EACCES, ELOOP, ENOTDIR, ENAMETOOLONG} × {`List`, `Prune` >N, `PlanPrune` >N, `Prune` ≤N, auto-prune}, decision pinned at Server level (step 4, 5); text/`--json` cells per command at cmd level with {nil, ENOENT, EACCES, ELOOP, ENOTDIR} (step 6; copy varies by command, not class; ENAMETOOLONG n/a at cmd: same arm, its reason pinned at Server level). Every unreadable row of `List`/`snapshots` has a regular entry carrying the recorded ID in the folder, so the ID-fallback mutation can redden.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_prune_recorded_test.go` (new) `Test_run_snapshots_prune_refuses_when_the_recorded_snapshot_cannot_be_read` — `writeSnapshots` five (`run_prune_test.go:39`), `buildStoreFrom` (`run_snapshots_test.go:77`) a path `<HOME>/Backup/<id>.sqlite` with `<HOME>/Backup` 0000 (`skipAsRoot`, `t.Cleanup` chmod 0700), `<id>` also a regular entry in the folder; `--keep 1`: stderr the ruled refusal verbatim with `(cannot read ~/Backup/<id>.sqlite: permission denied)`, stdout empty, exit 1, every snapshot kept (`requireSnapshotsKept`)
- [x] Step 2: `cmd/quarry/run_snapshots_recorded_test.go` (new) `Test_run_snapshots_warns_and_marks_nothing_when_the_recorded_snapshot_cannot_be_read` — same fixture; text: no row carries the store status, stderr `quarry: warning: cannot tell which snapshot the store was built from: cannot read ~/Backup/<id>.sqlite: permission denied`, exit 0
- [x] Step 3: `internal/snapshot/auto_prune_recorded_test.go` (new) `Test_sync_and_import_prunes_nothing_when_the_recorded_snapshot_cannot_be_read` — `syncBesideOldSnapshots` (`auto_prune_test.go:219-230`: three old + orphan, keep 1) through a new fake importer that replaces the just-written snapshot at its ref path with a self-symlink (ELOOP); `err` nil, `Pruned` non-nil with `Keep` 1 and empty `Deleted`/`Failed`, `rm.calls` empty, old pairs and orphan on disk, `Warnings()` and `WarningsAbsolute()` equal to the step-5 ENOENT control's (no new warning). Ref path = `Manifest.Snapshot.Path` (import.go:178-200; nothing re-stats it before `autoPrune`). Run all three; each must fail at its assertion (deletes / no warning / deletes) before Build

### Build
- [x] Step 4: `internal/snapshot/list.go:214-224` `markStoreSnapshot` — stats `recorded` once and returns that error when it is not `fs.ErrNotExist`, marking nothing; `list.go:252-264` `entriesAt` takes the stat'd `fs.FileInfo` (no second, swallowed stat of `recorded`); `list.go:107-120` `markStore` on that error sets `StorePath` = recorded, `StoreUnreadable` = `cannot read <~-abbreviated recorded>: <osreason.Reason>`, `StoreWarning`, new `Listing.StoreWarningAbsolute` (`list.go:52-57`, absolute path; R3 arm sets it equal to `StoreWarning`); `recorded == ""` stays no-store. Tests: `list_recorded_test.go` (new) `Test_list_marks_nothing_and_warns_when_the_recorded_snapshot_cannot_be_read` (rows EACCES, ELOOP, ENOTDIR, ENAMETOOLONG + nil and ENOENT controls; asserts `markedIDs`, `StorePath`, `StoreUnreadable`, both warnings as literals built from fixture paths); `prune_recorded_test.go` (new) `Test_prune_deletes_nothing_when_the_recorded_snapshot_cannot_be_read` (`Prune` and `PlanPrune` >N per unreadable class: refusal literal per `prune_store_test.go:193-215` / `prune_dryrun_test.go:186`, `rm.calls` empty; ENOENT control deletes beyond N) and `Test_prune_reports_the_recorded_snapshot_within_the_newest_n_when_it_cannot_be_read` (≤N: no error, `StorePath` recorded, nothing deleted). ENOENT controls `list_store_identity_test.go:184-217`, `list_test.go:486`, `:512` stay green unedited
- [x] Step 5: `internal/snapshot/auto_prune.go:26-37` `autoPrune` — on `markStoreSnapshot`'s error return nil before `selectPrune` and `sweepOrphans` (no deletes, no orphan sweep, no `pruneWarning`). Tests in `auto_prune_recorded_test.go`: ENOENT control row through the same fake (importer removes the snapshot instead: old beyond keep deleted, orphan swept — differs from step 3 in one variable). EACCES/ENOTDIR rows n/a: the recorded path is inside the snapshots folder, so any parent fault fails `listFolder` first (`cannotListWarning`, pinned `auto_prune_faults_test.go:160`). Sync text/json n/a: empty `Pruned` rendering already pinned (`internal/cli/render_prune_internal_test.go:258` no Pruned line; `json_internal_test.go:85` `{"keep":12,"deleted":[],"failed":[]}`)
- [x] Step 6: `internal/cli/json_snapshots.go:80-91` `snapshotsWarnings` — appends `StoreWarningAbsolute`; stderr (`snapshots.go:73-75`) keeps `StoreWarning`. Cmd cells, recorded path always under HOME: `run_snapshots_recorded_test.go` `Test_run_snapshots_recorded_snapshot_cells` text + `--json` × {nil, ENOENT, EACCES, ELOOP, ENOTDIR} (json: `store_snapshot` = recorded `{id,path}`, every `store:false` on unreadable rows, `warnings[]` absolute line); `run_prune_recorded_test.go` `Test_run_snapshots_prune_recorded_snapshot_cells` — prune and `--dry-run` >N text + `--json` × same classes (unreadable: refusal, stdout empty, exit 1, nothing deleted; nil/ENOENT: deletes or would-delete), prune ≤N text + `--json` × unreadable classes (`Nothing to delete: …` line, stderr empty, `store_snapshot` recorded, `warnings[]` empty — arms of `run_prune_refusals_test.go:114`). R3 control `run_snapshots_json_test.go:253` stays byte-identical

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments truer and no longer: `List` (list.go:62-64), `Listing.StorePath`/`StoreUnreadable`/`StoreWarningAbsolute`, `markStore`, `markStoreSnapshot`, `entriesAt`, `autoPrune`

### Verify
- [ ] Step 8: full verification + `spec-check.py snapshot-safety` → tick SCENARIO-14 with its acceptance test, SCENARIO-15 and -16 as "delivered by SCENARIO-14" with theirs

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- One stat of the recorded path, inside `markStoreSnapshot`, returning every error but not-exist; both callers (`markStore`, `autoPrune`) branch on it — SCENARIO-10/13's selector may change how entries are built but must keep going through `markStoreSnapshot`, never `entriesAt` directly, or auto-prune loses the guard
- Unreadable arm: `StorePath` = recorded, `StoreUnreadable` = `cannot read <~path>: <reason>`, no ID fallback; prune's refusal is the existing `planPrune` check (prune.go:96) — no new refusal type
- `Listing.StoreWarningAbsolute` is what `warnings[]` carries; the R3 arm sets it equal to `StoreWarning` (R3's abbreviated store path is out of scope) — SCENARIO-13's order (config, no-snapshots, D1, store warning) appends this field
- Auto-prune cannot-tell: `Pruned` non-nil, `Keep` set, nil `Deleted`/`Failed`, no warning, no orphan sweep, exit 0

**Left unbuilt** — named so nobody assumes it exists:
- Entry-side `os.Stat` in `entriesAt` (list.go:259) stays swallowed — `scanFolder` already refuses an unstattable snapshot (list.go:157-160); only a scan-to-mark race reaches it
- Auto-prune EACCES/ENOTDIR on the recorded path — unreachable without the folder failing to list first

**Traps** — things that look right and are not:
- A recorded path outside HOME (`/Volumes/...`) abbreviates to itself: the warnings[]-abbreviated mutation survives. Cmd cells use `<HOME>/Backup/…`
- The swapped snapshot is a symlink, skipped by `scanFolder`'s type check (list.go:154) with no stat; chmod on the snapshots folder instead hits `cannotListWarning`. The fixture needs more than `autoKeep` other regular snapshots or today's code deletes nothing and step 3 is green on arrival
- `requireNothingDeleted` (auto_prune_test.go:233) asserts `Pruned` nil — wrong here, `Pruned` is non-nil
- Prune >N refuses before `selectPrune`, so prune cells cannot catch the ID-fallback mutation; only listing cells with a same-ID entry can
- Expected reasons are literals (`permission denied`, `too many levels of symbolic links`, `not a directory`, `file name too long`), never `osreason.Reason` output; EACCES rows need `skipAsRoot` and a `t.Cleanup` chmod

## Orchestrator rulings (2026-10-06)

- Accepted: in the cannot-tell case auto-prune also skips sweepOrphans (BR-U4 "deletes nothing"; matches prune cannot-tell).
- Accepted: per-entry stat in entriesAt (list.go:259) stays swallowed (scan/mark race only); stays in Left unbuilt.


## Phase report

Run B1 (steps 4-6) done; all three acceptance tests green, narrow loop green (`internal/snapshot`, `internal/cli`, `cmd/quarry` -run 'recorded|prune|list_marks|snapshots_json'). Sweep and Verify (V) not run.

Production:
- `internal/snapshot/list.go`: `markStoreSnapshot` stats `recorded` once and returns every error but not-exist; `entriesAt` takes the `fs.FileInfo`; `markStore` sets `StorePath`, `StoreUnreadable`, `StoreWarning`, new `StoreWarningAbsolute` (R3 arm: equal to `StoreWarning`); new const `cannotTellWarning`.
- `internal/snapshot/auto_prune.go` `autoPrune`: returns nil on that error before `selectPrune`/`sweepOrphans`; `pruned.Snapshots` now set before the mark.
- `internal/cli/json_snapshots.go` `snapshotsWarnings` appends `StoreWarningAbsolute`.

Tests (new): `internal/snapshot/list_recorded_test.go` (grid helper `unreadableRecordedPaths`, list rows, controls, R3 both-forms), `prune_recorded_test.go` (Prune, PlanPrune, ENOENT control, <=N), `auto_prune_recorded_test.go` (+ ENOENT control); `cmd/quarry/run_prune_recorded_test.go` (class grid `recordedClasses`, `buildStoreFromClass`, `pruneRecordedCell`), `run_prune_recorded_cells_test.go` (prune/dry-run/json refusal cells, readable controls, <=N text and json), `run_snapshots_recorded_test.go` (text and json cells).

Mutation results are in the run report. Not redone in V; V adds none.
Deviations: cmd cells split into text/json test functions per assertion shape; prune cells live in `run_prune_recorded_cells_test.go`, not `run_prune_recorded_test.go`. Server <=N and ENOENT controls were green on arrival (current behaviour is already right there); only the unreadable rows were red.
Do not redo: tests assert expected strings as literals; `StoreWarningAbsolute` is what `warnings[]` carries, stderr keeps `StoreWarning`.
