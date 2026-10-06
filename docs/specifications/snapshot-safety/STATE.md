# snapshot-safety — current state

Scenarios complete: SCENARIO-01 (folded 02, 05, 08), SCENARIO-03 (folded 04, 07, 09). Last updated by SCENARIO-03.

## Binding decisions
- Sync lock taken in `internal/cli/sync.go` RunE via `(*snapshot.Server).LockForSync`, `defer release()`; prune lock in `snapshots_prune.go` RunE via `LockForPrune`, only `if !dryRun`, after usage (2) → config (1) → `newSnapshots`, before `Prune`. Neither is taken inside `SyncAndImport`/`ImportFrom`/`Prune`/`PlanPrune`/`autoPrune`/`List`: auto-prune runs under sync's lock and a second in-process acquire would refuse itself (SCENARIO-01, 03)
- `LockForPrune` mirrors `LockForSync` through the shared `lock(ctx, heldMsg)` in `internal/snapshot/lock.go`; held → `RefusalError` (L1s / L1p copy there); folder-missing → silent no-lock, nil error, no-op release. Other `lockfile` kinds are returned unchanged (SCENARIO-01, 03)
- Prune-mode `snapshot.WithLocker(lockfile.New(lockPathUnder(storeDir), lockfile.ModePrune))` lives in `newSnapshotsFactory` (shared with `snapshots` and `--dry-run`); nothing there may call `LockForPrune`. Prune on an existing quarry folder now leaves a 0600 `quarry.lock`; only a missing folder creates nothing (SCENARIO-03)
- Mode (sync creates folder 0700 / prune never creates; any non-`ModeSync` value acts as prune) is fixed at `lockfile.New` in `cmd/quarry`; the `snapshot.Locker` port (`Acquire(ctx) (func(), error)`) carries no mode (SCENARIO-01)
- Nil locker = no lock; `internal/snapshot` and `internal/cli` tests build Servers without one; wiring pins are `cmd/quarry` tests through `run()` (SCENARIO-01)
- All lock-refusal copy lives in `internal/snapshot` (`RefusalError`); `lockfile` returns classified `*Error` kinds, no user copy. Flock fault seam is `lockfile.WithFlock`, an option, never a package var (SCENARIO-01)
- Lock identity is the inode (flock on `quarry.lock`, `O_RDONLY|O_CREATE|O_NOFOLLOW|O_NONBLOCK` 0600): a hard link to a held file refuses; the kernel drops it on kill -9. Release is a bare idempotent `func()` (`sync.Once` close) (SCENARIO-01)
- Prune and sync `--help` Long carry the one-writer paragraph verbatim from `specification.md:87-96`; pins assert the wrap (SCENARIO-03)

## Left unbuilt
- `lockfile` kinds beyond held / folder-missing (not-regular via `Lstat`, folder-create, file-create, open, lock-other) and L2/L3/L4a/L4b/L5s/L5p copy; the prune variants must reach `LockForPrune`'s method, beside the folder-missing arm — SCENARIO-06
- Unreadable-recorded-path guard in prune, listing and auto-prune — SCENARIO-14 (folds 15, 16)
- `.SQLITE` case-insensitive selector, strays, `sync --from`/status any-case — SCENARIO-10 (folds 11), 12a, 12b, 13

## Traps
- Test `-run` patterns are lowercase-matched against names like `Test_run_snapshots_prune_...`: use `-run 'lock|prune|help'`, not `Prune|Lock` (SCENARIO-03)
- A lock test with a sentinel store (not `buildStoreFrom`) refuses at the cannot-tell step if the lock arm is mutated away, so its "deletes nothing" claim has no delete control; use `newPrunableStore` (real store + orphan manifest) (SCENARIO-03)
- `release, err :=` inside the `if !dryRun` block shadows `err`; `defer release()` there is function-scoped, which is what is wanted (SCENARIO-03)
- An unreferenced `*os.File` is closed by its GC cleanup, dropping the flock: a test holding a lock keeps the release referenced (`t.Cleanup(release)`); flock is per open file description, so two sequential `run()` calls in one test binary collide unless release runs when RunE returns (SCENARIO-01)
- Opening a fifo read-only blocks without `O_NONBLOCK`; Lstat-then-open races without `O_NOFOLLOW` — both flags are in; their tests are 06's (SCENARIO-01)
- `O_CREATE` on an existing `quarry.lock` needs no folder write; `run_store_faults_test.go:76-82` pre-creates it so the "cannot write to" line stays unchanged. A bundle or `--from` refusal on a first-ever sync leaves the quarry folder and `quarry.lock` (no `snapshots/`) (SCENARIO-01)
- Moving the sync acquire into the `Changed("from")` else-branch also reddens `Test_run_lists_mismatched_splits_in_the_failed_validation_stdout_block` via the shadowed `err` — a mutation artifact, not a lock pin (SCENARIO-01)

## Open debts
- Unclassified `lockfile` failures (mkdir, open, other flock errors) surface as the generic error — SCENARIO-06 owns their copy and exit class
- `Acquire` ignores its `ctx` (non-blocking, nothing to cancel) — unowned; dies unless a blocking mode is added
- Mutations verified (reddened test): sync — child release discarded → `Test_acquire_succeeds_after_the_holder_process_is_killed`; `Kind == KindHeld` dropped → `Test_lock_for_sync_returns_a_folder_missing_lock_error_unchanged`; `ModeSync` check widened → `Test_acquire_with_an_unknown_mode_never_creates_the_quarry_folder`; per-name sibling lock → `Test_acquire_refuses_through_a_hard_link_to_the_held_lock_file`. Prune — factory `ModeSync` → creates-nothing test (`Library/` left in home); `List`/`PlanPrune` take the lock → `Test_run_snapshots_lists_while_a_writer_holds_the_lock` / dry-run json cell; `!dryRun || *jsonOut` → dry-run json cell; `KindFolderMissing` mapping dropped → creates-nothing text and json cells; lock only when a plan has work → `..._with_nothing_beyond_the_cap_while_locked` text and json
