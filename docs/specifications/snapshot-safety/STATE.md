# snapshot-safety — current state

Scenarios complete: SCENARIO-01 (folded 02, 05, 08), SCENARIO-03 (folded 04, 07, 09), SCENARIO-06. Last updated by SCENARIO-06.

## Binding decisions
- Sync lock taken in `internal/cli/sync.go` RunE via `(*snapshot.Server).LockForSync`, `defer release()`; prune lock in `snapshots_prune.go` RunE via `LockForPrune`, only `if !dryRun`, after usage (2) → config (1) → `newSnapshots`, before `Prune`. Neither is taken inside `SyncAndImport`/`ImportFrom`/`Prune`/`PlanPrune`/`autoPrune`/`List`: auto-prune runs under sync's lock and a second in-process acquire would refuse itself (SCENARIO-01, 03)
- `LockForPrune` mirrors `LockForSync` through the shared `lock(ctx, words)` in `internal/snapshot/lock.go`; the only silent arm is `KindFolderMissing` (nil error, no-op release) and every other `lockfile` kind is a `RefusalError`; a non-lockfile error, unknown `Kind`, or classified kind with nil `Err` is returned unchanged (`osreason.Reason(nil)` panics) (SCENARIO-01, 03, 06)
- Prune-mode `snapshot.WithLocker(lockfile.New(lockPathUnder(storeDir), lockfile.ModePrune))` lives in `newSnapshotsFactory` (shared with `snapshots` and `--dry-run`); nothing there may call `LockForPrune`. Prune on an existing quarry folder leaves a 0600 `quarry.lock`; only a missing folder creates nothing (SCENARIO-03)
- Mode (sync creates folder 0700 / prune never creates; any non-`ModeSync` value acts as prune) is fixed at `lockfile.New` in `cmd/quarry`; the `snapshot.Locker` port (`Acquire(ctx) (func(), error)`) carries no mode (SCENARIO-01)
- Nil locker = no lock; `internal/snapshot` and `internal/cli` tests build Servers without one; wiring pins are `cmd/quarry` tests through `run()` (SCENARIO-01)
- All lock-refusal copy lives in `internal/snapshot` (`lock.go` held/outcome words, `lock_refusal.go` kind → L2/L3/L4a/L4b/L5s/L5p `RefusalError`); `lockfile` returns classified `*Error` kinds (held, folder-missing, not-regular, folder-create, create, open, lock), no user copy. `Error.Path` is always the lock file path (copy derives the folder with `filepath.Dir`). Flock fault seam is `lockfile.WithFlock` (cmd tests wrap `env.NewServer` / build a local `NewSnapshots` server), never a package var (SCENARIO-01, 06)
- Open failure is `KindCreate` vs `KindOpen` by whether `Lstat` saw the file, never by the error; prune-mode quarry-folder `Stat` faults other than not-exist fall through to `Lstat` and surface as L4b — no copy was ruled, judge the L4b fix text in the final product-vision pass (SCENARIO-06)
- Lock identity is the inode (flock on `quarry.lock`, `O_RDONLY|O_CREATE|O_NOFOLLOW|O_NONBLOCK` 0600): a hard link to a held file refuses; the kernel drops it on kill -9. Release is a bare idempotent `func()` (`sync.Once` close) (SCENARIO-01)
- Prune and sync `--help` Long carry the one-writer paragraph verbatim from `specification.md:87-96`; pins assert the wrap (SCENARIO-03)

## Left unbuilt
- Unreadable-recorded-path guard in prune, listing and auto-prune — SCENARIO-14 (folds 15, 16)
- `.SQLITE` case-insensitive selector, strays, `sync --from`/status any-case — SCENARIO-10 (folds 11), 12a, 12b, 13
- Post-open `fstat` on the lock file: a swap between `Lstat` and `OpenFile` (directory or fifo replacing the file) is accepted as one local-user race; `O_NOFOLLOW`/`O_NONBLOCK` keep it from hanging or following links — unowned by design (SCENARIO-06)

## Traps
- Test `-run` patterns are lowercase-matched against names like `Test_run_snapshots_prune_...`: use `-run 'lock|prune|help'`, not `Prune|Lock`; a cmd test must carry `lock` in its name to join the lock narrow loop (SCENARIO-03, 06)
- A lock test with a sentinel store (not `buildStoreFrom`) refuses at the cannot-tell step if the lock arm is mutated away, so its "deletes nothing" claim has no delete control; use `newPrunableStore` (real store + orphan manifest) (SCENARIO-03)
- `release, err :=` inside the `if !dryRun` block shadows `err`; `defer release()` there is function-scoped, which is what is wanted (SCENARIO-03)
- An unreferenced `*os.File` is closed by its GC cleanup, dropping the flock: a test holding a lock keeps the release referenced (`t.Cleanup(release)`); flock is per open file description, so two sequential `run()` calls in one test binary collide unless release runs when RunE returns (SCENARIO-01)
- Opening a fifo read-only blocks without `O_NONBLOCK`; Lstat-then-open races without `O_NOFOLLOW`. Neither flag has a filesystem state that reddens it alone (`Lstat` refuses the fifo/symlink first): a mutation sample of `openFlags` SURVIVING is expected; the fifo cells go red only with the not-regular branch dropped too, by their 10s goroutine deadline (SCENARIO-01, 06)
- A directory opens `O_RDONLY` and `flock`s fine: without the `Lstat` not-regular branch a directory "locks" silently (SCENARIO-06)
- Mode rows (0000, 0500) need `skipAsRoot`; a 0500 folder needs a `t.Cleanup` chmod to 0700 or `t.TempDir` removal fails (SCENARIO-06)
- Flock errors are bare `syscall.Errno`, not `*fs.PathError`: `osreason.Reason` yields `err.Error()` ("operation not supported") (SCENARIO-06)
- Never derive an expected ruled string from a production constant: B2's Server test copied the `KindLock` outcome from production and missed the missing `so` until a cmd cell asserted the verbatim line (SCENARIO-06)
- `LockForPrune` swallowing any `*lockfile.Error` reddens no cmd cell (`lock()` already phrased those kinds); only the `internal/snapshot` prune fallback rows pin it (SCENARIO-06)
- `O_CREATE` on an existing `quarry.lock` needs no folder write; `run_store_faults_test.go:76-82` pre-creates it so the "cannot write to" line stays unchanged. A bundle or `--from` refusal on a first-ever sync leaves the quarry folder and `quarry.lock` (no `snapshots/`) (SCENARIO-01)
- Moving the sync acquire into the `Changed("from")` else-branch also reddens `Test_run_lists_mismatched_splits_in_the_failed_validation_stdout_block` via the shadowed `err` — a mutation artifact, not a lock pin (SCENARIO-01)

## Open debts
- `Acquire` ignores its `ctx` (non-blocking, nothing to cancel) — unowned; dies unless a blocking mode is added
- Final product-vision pass to judge the L4b fix text for the prune-mode quarry-folder-is-a-file / unsearchable state (SCENARIO-06 ruling 2) — owned by the final pass
- Mutations verified (reddened test): sync — child release discarded → `Test_acquire_succeeds_after_the_holder_process_is_killed`; `Kind == KindHeld` dropped → `Test_lock_for_sync_returns_a_folder_missing_lock_error_unchanged`; `ModeSync` check widened → `Test_acquire_with_an_unknown_mode_never_creates_the_quarry_folder`; per-name sibling lock → `Test_acquire_refuses_through_a_hard_link_to_the_held_lock_file`. Prune — factory `ModeSync` → creates-nothing test; `List`/`PlanPrune` take the lock → `Test_run_snapshots_lists_while_a_writer_holds_the_lock` / dry-run json cell; `!dryRun || *jsonOut` → dry-run json cell; `KindFolderMissing` mapping dropped → creates-nothing text and json cells; lock only when a plan has work → `..._with_nothing_beyond_the_cap_while_locked`. Unusable lock file — `Lstat`→`Stat` → symlink cells (sync, prune); not-regular branch dropped → directory, symlink, fifo cells (fifo by 10s deadline, sync and prune); open kind forced Create/Open → L4b / L4a cells; outcomes swapped → L5s/L5p cells; `KindFolderCreate` arm dropped → L2 cells
