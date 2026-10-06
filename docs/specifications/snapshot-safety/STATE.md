# snapshot-safety — current state

Scenarios complete: SCENARIO-01 (folded 02, 05, 08). Last updated by SCENARIO-01.

## Binding decisions
- Lock taken in `internal/cli/sync.go` RunE via `(*snapshot.Server).LockForSync`, `defer release()`; never inside `SyncAndImport`/`ImportFrom`/`Prune`/`autoPrune` — auto-prune never re-takes it, and the lock must precede `ResolveBundle` and the `--from` branch. Order: usage (2) → config (1) → lock (1) → resolution (SCENARIO-01)
- Prune mode with the quarry folder missing returns `*lockfile.Error{Kind: KindFolderMissing}` and holds no lock; `LockForSync` returns it unchanged (not a refusal); 03's `LockForPrune` maps it to "proceed unlocked" (SCENARIO-01)
- Mode (sync creates folder 0700 / prune never creates; any non-`ModeSync` value behaves as prune) is fixed at `lockfile.New` in `cmd/quarry`; the `snapshot.Locker` port (`Acquire(ctx) (func(), error)`) carries no mode (SCENARIO-01)
- Nil locker = no lock; `internal/snapshot` and `internal/cli` tests build Servers without one; wiring pins are `cmd/quarry` tests through `run()` (SCENARIO-01)
- All lock-refusal copy lives in `internal/snapshot` (`RefusalError`); `lockfile` returns classified `*Error` kinds, no user copy (SCENARIO-01)
- Flock fault seam is `lockfile.WithFlock`, an option, never a package var — 06's L5s/L5p tests use it (SCENARIO-01)
- Lock identity is the inode (flock on `quarry.lock`, open `O_RDONLY|O_CREATE|O_NOFOLLOW|O_NONBLOCK` 0600): a hard link to a held file refuses; the kernel drops it on kill -9 (SCENARIO-01)
- Release is a bare idempotent `func()` (`sync.Once` close) (SCENARIO-01)

## Left unbuilt
- `lockfile` kinds beyond held / folder-missing (not-regular via `Lstat`, folder-create, file-create, open, lock-other) and L2/L3/L4a/L4b/L5s copy — SCENARIO-06
- `(*Server).LockForPrune`, L1p, prune RunE acquire under `!dryRun`, prune-mode wiring in `newSnapshotsFactory`, sync Long paragraphs, doc.go sentence — SCENARIO-03 (Long/doc.go: 09)

## Traps
- An unreferenced `*os.File` is closed by its GC cleanup, dropping the flock: a test holding a lock must keep the release referenced (`t.Cleanup(release)`, or `defer release()` in the kill-test child, which also calls `runtime.GC()` so a dropped release fails the test) (SCENARIO-01)
- flock is per open file description: two sequential `run()` calls in one test binary collide unless release runs when RunE returns (SCENARIO-01)
- Opening a fifo read-only blocks without `O_NONBLOCK`; Lstat-then-open races without `O_NOFOLLOW` — both flags are in; their tests are 06's (SCENARIO-01)
- `O_CREATE` on an existing `quarry.lock` needs no folder write; `run_store_faults_test.go:76-82` pre-creates it so the "cannot write to" line stays unchanged (SCENARIO-01)
- A bundle or `--from` refusal on a first-ever sync now leaves the quarry folder and `quarry.lock` (no `snapshots/`); `run_bundle_refusals_test.go:36,60` check only `snapshots/` (SCENARIO-01)
- Moving the acquire into the `Changed("from")` else-branch also reddens `Test_run_lists_mismatched_splits_in_the_failed_validation_stdout_block`: the moved `release, err :=` shadows `err` — a mutation artifact, not a lock pin (SCENARIO-01)

## Open debts
- Unclassified `lockfile` failures (mkdir, open, other flock errors) are wrapped with the path and surface as the generic error — SCENARIO-06 owns their copy and exit class
- `Acquire` ignores its `ctx` (non-blocking, so nothing to cancel) — unowned; dies unless a blocking mode is added
- Mutations verified in V (reddened test): child release discarded → `Test_acquire_succeeds_after_the_holder_process_is_killed`; `&& Kind == KindHeld` dropped (lock.go:23) → `Test_lock_for_sync_returns_a_folder_missing_lock_error_unchanged`; `ModeSync` check widened to `!= ModePrune` → `Test_acquire_with_an_unknown_mode_never_creates_the_quarry_folder`; lock taken on a per-name sibling file → `Test_acquire_refuses_through_a_hard_link_to_the_held_lock_file`
