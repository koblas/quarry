# snapshot-safety — current state

Scenarios complete: SCENARIO-01 (folded 02, 05, 08), SCENARIO-03 (folded 04, 07, 09), SCENARIO-06, SCENARIO-14 (folded 15, 16). Last updated by SCENARIO-14.

## Binding decisions
- Sync lock taken in `internal/cli/sync.go` RunE via `(*snapshot.Server).LockForSync`, `defer release()`; prune lock in `snapshots_prune.go` RunE via `LockForPrune`, only `if !dryRun`, after usage (2) → config (1) → `newSnapshots`, before `Prune`. Neither is taken inside `SyncAndImport`/`ImportFrom`/`Prune`/`PlanPrune`/`autoPrune`/`List`: auto-prune runs under sync's lock and a second in-process acquire would refuse itself (SCENARIO-01, 03)
- `LockForPrune` mirrors `LockForSync` through the shared `lock(ctx, words)` in `internal/snapshot/lock.go`; the only silent arm is `KindFolderMissing` (nil error, no-op release) and every other `lockfile` kind is a `RefusalError`; a non-lockfile error, unknown `Kind`, or classified kind with nil `Err` is returned unchanged (`osreason.Reason(nil)` panics) (SCENARIO-01, 03, 06)
- Prune-mode `snapshot.WithLocker(lockfile.New(lockPathUnder(storeDir), lockfile.ModePrune))` lives in `newSnapshotsFactory` (shared with `snapshots` and `--dry-run`); nothing there may call `LockForPrune`. Prune on an existing quarry folder leaves a 0600 `quarry.lock`; only a missing folder creates nothing (SCENARIO-03)
- Mode (sync creates folder 0700 / prune never creates; any non-`ModeSync` value acts as prune) is fixed at `lockfile.New` in `cmd/quarry`; the `snapshot.Locker` port (`Acquire(ctx) (func(), error)`) carries no mode (SCENARIO-01)
- Nil locker = no lock; `internal/snapshot` and `internal/cli` tests build Servers without one; wiring pins are `cmd/quarry` tests through `run()` (SCENARIO-01)
- All lock-refusal copy lives in `internal/snapshot` (`lock.go`, `lock_refusal.go`: L2/L3/L4a/L4b/L5s/L5p `RefusalError`); `lockfile` returns classified `*Error` kinds, no user copy. `Error.Path` is always the lock file path. Flock fault seam is `lockfile.WithFlock`, never a package var (SCENARIO-01, 06)
- Open failure is `KindCreate` vs `KindOpen` by whether `Lstat` saw the file, never by the error; prune-mode quarry-folder `Stat` faults other than not-exist fall through to `Lstat` and surface as L4b — no copy ruled, judge the L4b fix text in the final product-vision pass (SCENARIO-06)
- Lock identity is the inode (flock on `quarry.lock`, `O_RDONLY|O_CREATE|O_NOFOLLOW|O_NONBLOCK` 0600): a hard link to a held file refuses; kill -9 drops it. Release is a bare idempotent `func()` (SCENARIO-01)
- Prune and sync `--help` Long carry the one-writer paragraph verbatim from `specification.md:87-96`; pins assert the wrap (SCENARIO-03)
- Recorded-path guard: ONE stat of the recorded path, inside `markStoreSnapshot` (list.go), returning every error but not-exist; both callers (`markStore`, `autoPrune`) branch on it. A selector change (SCENARIO-10/13) may change how entries are built but must keep going through `markStoreSnapshot`, never `entriesAt`/`storeEntryIndex` directly, or auto-prune loses the guard (SCENARIO-14)
- Unreadable arm (EACCES, ELOOP, ENOTDIR, ENAMETOOLONG): nothing marked, no ID fallback (`storeEntryIndex` runs only on stat-ok/not-exist); `StorePath` = recorded, `StoreUnreadable` = `cannot read <~path>: <reason>`; prune's refusal is the existing `planPrune` cannot-tell check, no new refusal type (SCENARIO-14)
- `Listing.StoreWarningAbsolute` is what `snapshots --json` `warnings[]` carries (stderr keeps abbreviated `StoreWarning`); the R3 arm sets it equal to `StoreWarning`. SCENARIO-13's warning order (config, no-snapshots, D1, store warning) appends this field (SCENARIO-14)
- Auto-prune cannot-tell: `Pruned` non-nil with `Keep` and `Snapshots` set (set before the mark), nil `Deleted`/`Failed`, no warning, no orphan sweep, exit 0 (SCENARIO-14)

## Left unbuilt
- `.SQLITE` case-insensitive selector, strays, `sync --from`/status any-case — SCENARIO-10 (folds 11), 12a, 12b, 13
- Entry-side `os.Stat` in `entriesAt` (list.go) stays swallowed: `scanFolder` already refuses an unstattable snapshot, only a scan-to-mark race reaches it — unowned by design (SCENARIO-14)
- Auto-prune EACCES/ENOTDIR on the recorded path: unreachable without the folder failing to list first (`cannotListWarning`) — unowned by design (SCENARIO-14)
- Post-open `fstat` on the lock file: a swap between `Lstat` and `OpenFile` is accepted as one local-user race — unowned by design (SCENARIO-06)

## Traps
- Test `-run` patterns are lowercase-matched against names like `Test_run_snapshots_prune_...`: use `-run 'lock|prune|help'`, not `Prune|Lock`; a cmd test must carry `lock` in its name to join the lock narrow loop (SCENARIO-03, 06)
- A lock test with a sentinel store (not `buildStoreFrom`) refuses at the cannot-tell step if the lock arm is mutated away; use `newPrunableStore` (real store + orphan manifest) (SCENARIO-03)
- `release, err :=` inside the `if !dryRun` block shadows `err`; `defer release()` there is function-scoped, which is wanted (SCENARIO-03)
- An unreferenced `*os.File` is closed by its GC cleanup, dropping the flock: tests holding a lock keep the release referenced (`t.Cleanup(release)`); two sequential `run()` calls in one test binary collide unless release runs when RunE returns (SCENARIO-01)
- Opening a fifo read-only blocks without `O_NONBLOCK`; Lstat-then-open races without `O_NOFOLLOW`. A mutation of `openFlags` SURVIVING is expected; fifo cells go red only with the not-regular branch dropped too, by their 10s deadline. A directory `flock`s fine: without the `Lstat` not-regular branch it "locks" silently (SCENARIO-01, 06)
- Mode rows (0000, 0500) need `skipAsRoot` and a `t.Cleanup` chmod to 0700 or `t.TempDir` removal fails (SCENARIO-06, 14)
- Flock errors are bare `syscall.Errno`: `osreason.Reason` yields `err.Error()` ("operation not supported") (SCENARIO-06)
- Never derive an expected ruled string from a production constant; expected reasons are literals (`permission denied`, `too many levels of symbolic links`, `not a directory`, `file name too long`) (SCENARIO-06, 14)
- `O_CREATE` on an existing `quarry.lock` needs no folder write; `run_store_faults_test.go:76-82` pre-creates it. A bundle or `--from` refusal on a first-ever sync leaves the quarry folder and `quarry.lock` (SCENARIO-01)
- A recorded path outside HOME (`/Volumes/...`) abbreviates to itself, so the warnings[]-abbreviated mutation survives: cmd cells use `<HOME>/Backup/…` (SCENARIO-14)
- The auto-prune self-symlink fixture needs more than `autoKeep` other regular snapshots (symlink is skipped by `scanFolder`) or today's code deletes nothing and the test is green on arrival; `requireNothingDeleted` asserts `Pruned` nil, wrong for cannot-tell (`Pruned` non-nil). `deleteSnapshot` decrements `Pruned.Snapshots`, so the ENOENT control reads 1 (3 listed - 2 deleted) (SCENARIO-14)
- Prune >N refuses before `selectPrune`, so prune cells cannot catch the ID-fallback mutation; only listing cells with a same-ID regular entry can (SCENARIO-14)

## Open debts
- `Acquire` ignores its `ctx` (non-blocking, nothing to cancel) — unowned; dies unless a blocking mode is added
- Final product-vision pass to judge the L4b fix text for the prune-mode quarry-folder-is-a-file / unsearchable state (SCENARIO-06 ruling 2) — owned by the final pass
- Mutations verified, sync lock: child release discarded → `Test_acquire_succeeds_after_the_holder_process_is_killed`; `KindHeld` dropped → `Test_lock_for_sync_returns_a_folder_missing_lock_error_unchanged`; `ModeSync` widened → `Test_acquire_with_an_unknown_mode_never_creates_the_quarry_folder`; per-name sibling lock → `Test_acquire_refuses_through_a_hard_link_to_the_held_lock_file`. Prune lock: factory `ModeSync` → creates-nothing test; `List`/`PlanPrune` lock → `Test_run_snapshots_lists_while_a_writer_holds_the_lock`; `KindFolderMissing` mapping dropped → creates-nothing cells; lock only when a plan has work → `..._with_nothing_beyond_the_cap_while_locked`. Unusable lock file: `Lstat`→`Stat` → symlink cells; not-regular branch dropped → directory/symlink/fifo cells; open kind forced → L4b/L4a cells; outcomes swapped → L5s/L5p cells; `KindFolderCreate` arm dropped → L2 cells
- Mutations verified, recorded path (SCENARIO-14): `pruned.Snapshots` moved below the `markStoreSnapshot` return in `autoPrune` → `Test_sync_and_import_prunes_nothing_when_the_recorded_snapshot_cannot_be_read` (`expected: 3, actual: 0`); the classifier, ID-fallback and `StoreWarningAbsolute` mutations were run in B1 per the plan line
