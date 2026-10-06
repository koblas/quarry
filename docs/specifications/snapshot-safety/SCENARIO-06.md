---
id: SCENARIO-06
status: open
---

# SCENARIO-06: An unusable lock file refuses with a fix

Cadence: test-first — write-safety guard: the symlink refusal (`Lstat`, BR-L5), and prune's delete refusal when the lock is unusable (`LockForPrune` must not swallow the new kinds)
Acceptance test: `cmd/quarry/run_sync_lock_file_test.go` `Test_run_sync_refuses_an_unusable_lock_file_with_a_fix` (outline rows: directory, symlink, mode 0000, read-only folder; each text + `--json`)
Narrow loop: `go test ./internal/platform/lockfile/ && go test ./internal/snapshot/ -run 'lock' && go test ./cmd/quarry/ -run 'lock'` (lowercase patterns)
Mutation checks (each in the B run that builds the guard; one at a time):
- `Lstat` → `Stat` in `Acquire` → symlink rows (`Test_acquire_refuses_a_lock_file_that_is_not_a_regular_file`, cmd L3 symlink cells): symlink-to-regular then fails ELOOP as `KindOpen`, dangling as `KindCreate`
- drop the not-regular branch → directory and fifo rows red at their assertion (a lock "succeeds" on both); then also drop `O_NONBLOCK` → fifo row red by `acquireNow`'s deadline, not by go test timeout. `O_NONBLOCK` alone is race-only defence in depth (Lstat refuses the fifo first): no filesystem state reddens it alone — say so in the report
- open failure classified by error instead of by whether `Lstat` found the file (cannot-open → cannot-create) → mode-0000 (L4b) and read-only-folder (L4a) cells
- `LockForPrune` swallows any `*lockfile.Error`, not only `KindFolderMissing` → prune L3/L4/L5p cmd cells (five snapshots + orphan still present; the 0400 control arm shows prune deletes when the lock is usable)
- L5 sync/prune "changed nothing"/"deleted nothing" swapped → `Test_lock_for_sync_*`/`Test_lock_for_prune_*` cannot-lock rows and cmd L5s/L5p cells
- `KindFolderCreate` arm dropped from the copy switch → L2 Server and cmd rows
Runs: A (1) | B1 (2) | B2 (3) | B3 (4-5) | V (6-7)
Size: OWNS A RUN — 3 Build batches, 1 feature package (`internal/snapshot`) + `internal/platform/lockfile`; `internal/cli` unchanged (a `RefusalError` already prints `quarry: <msg>`, exit 1)

Surface survey (what `Acquire` calls on the OS; none goes behind a port, `snapshot.Locker` stays `Acquire(ctx) (func(), error)`): `os.MkdirAll` (sync folder), `os.Stat` (prune folder), `os.OpenFile(openFlags, 0600)`, `syscall.Flock` via `WithFlock`; new: `os.Lstat`. Callers of lockfile errors: `lock.go:42` `errors.AsType[*lockfile.Error]` (LSP findReferences not needed; grep `lockfile.Error` hits only `lock.go`, `lock_test.go`, `lockfile_test.go`).

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_sync_lock_file_test.go` (new) `Test_run_sync_refuses_an_unusable_lock_file_with_a_fix` — through `run()`, HOME = temp dir, quarry folder arranged per row (directory / symlink to a regular file / 0000 file / folder 0500 with no lock file; `skipAsRoot` on the mode rows, `t.Cleanup` chmod back); asserts exit 1, empty stdout, L3 / L3 / L4b / L4a verbatim with `~/Library/Application Support/quarry/quarry.lock`, and no `snapshots/` folder, store sentinel untouched. No stubs: compiles on today's API; red at the stderr assertion (directory row today proceeds past the lock)

### Build
- [ ] Step 2 (B1): `internal/platform/lockfile/lockfile.go:34-57,90-136` + `lockfile_test.go:191-233,275-285,287-304` — new `Kind`s `KindNotRegular`, `KindFolderCreate`, `KindCreate`, `KindOpen`, `KindLock` (all `Path` = lock file path, `Err` = raw cause) with `Error()` strings. `Acquire`: `ensureFolder` (sync `MkdirAll` fault → `KindFolderCreate`; prune `Stat` fault other than not-exist is ignored so the `Lstat` below classifies it); then `os.Lstat`: not-exist = missing, other fault → `KindOpen`, not `IsRegular()` → `KindNotRegular`; `OpenFile` fault → `KindOpen` if `Lstat` found the file else `KindCreate`; `lock` non-would-block fault → `KindLock`. Re-point the four existing "unclassified" tests (`NotErrorAs` flips to `Kind` asserts; reason still reachable by `errors.Is`). Tests (real fs, both modes unless noted): not-regular rows directory / symlink→regular / dangling symlink / fifo (`syscall.Mkfifo`, via `acquireNow`); `KindFolderCreate` (parent is a file, ENOTDIR; sync only); `KindOpen` mode 0000 + prune-mode folder-stat ENOTDIR; `KindCreate` folder 0500 no lock file; `KindLock` ENOTSUP and ENOLCK injected through `WithFlock` (file closed, as the existing test); controls: 0400 file locks fine, would-block still `KindHeld`, prune + missing folder still `KindFolderMissing`; `Error()` string per new kind. Socket row n/a: same `IsRegular()` branch as fifo, unix socket path limit
- [ ] Step 3 (B2): `internal/snapshot/lock.go:10-15,25-46` + new `internal/snapshot/lock_refusal.go` + `lock_test.go:93-114,156-178` — `lock(ctx, heldMsg)` takes per-command copy (held line + `changed nothing`/`deleted nothing`); a `lockRefusal(home, *lockfile.Error, copy)` maps kinds to L2, L3, L4a, L4b, L5s/L5p as `RefusalError`; abbreviate with `homepath.Abbreviate(s.home, …)`, folder = `filepath.Dir(lockErr.Path)`, L2's "make … writable" names `filepath.Dir` of that folder, reasons from `osreason.Reason(lockErr.Err)`. `LockForPrune` keeps only `KindFolderMissing` silent. Fallbacks returned unchanged: non-lockfile error, unknown `Kind`, and a classified kind with nil `Err` (`osreason.Reason(nil)` panics). Server-method tests (real adapter in temp dir with `WithHome`, `fakeLocker` for unreachable shapes): L2 sync; L3, L4a, L4b via both `LockForSync` and `LockForPrune`; L5s via sync and L5p via prune with `WithFlock(ENOTSUP)` and `ENOLCK` rows; fallback rows (plain error, unknown kind, nil `Err`); path outside home prints absolute. Re-point the two "other lock errors unchanged" tests to the fallback rows
- [ ] Step 4 (B3a): `cmd/quarry/run_sync_lock_file_test.go` — rest of the sync matrix, text and `--json` each (stdout empty both): L2 (HOME/Library 0500, no Application Support), L5s via `env.NewServer` wrapper appending `snapshot.WithLocker(lockfile.New(lockPathUnder(…), ModeSync, WithFlock(ENOTSUP)))` (precedent `run_sync_lock_test.go:174`; copy + exit only, wiring is pinned by the real-fs rows), fifo row bounded by a goroutine deadline, 0400 control (sync succeeds, exit 0, lock file stays 0400)
- [ ] Step 5 (B3b): `cmd/quarry/run_prune_lock_file_test.go` (new) — prune matrix over `newPrunableStore` (`run_prune_lock_test.go:40-50`), `prune --keep 3`, text and `--json`: L3 directory + symlink, L4b 0000, L4a folder 0500 (no lock file), L5p via a locally built `snapshot.NewServer(…WithLocker(ModePrune+WithFlock))` in `env.NewSnapshots`; each asserts exit 1, empty stdout, verbatim line, five snapshots + orphan kept (`requireSnapshotsKept`); 0400 control: prune exits 0 and deletes. L2 for prune is n/a: prune never creates the folder (existing `..._with_no_quarry_folder_and_creates_nothing`)

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on the new kinds and `Acquire` (kinds it returns, per the `go doc` budget), `LockForSync`/`LockForPrune` docs updated for the new refusals

### Verify
- [ ] Step 7: `.claude/scripts/verify.sh <start> ./cmd/... ./internal/snapshot/... ./internal/platform/lockfile/...`; `spec-check.py snapshot-safety`; tick SCENARIO-06 with its acceptance test; rewrite `STATE.md` (close the Open debt "unclassified `lockfile` failures", move the Left unbuilt entry)

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `lockfile.Error.Path` is always the lock file path; the snapshot copy derives the folder with `filepath.Dir` — L2/L4a/L5 name the folder and L2's fix names its parent, so no second path field
- Open failure is `KindCreate` vs `KindOpen` by whether `Lstat` saw the file, never by the error — L4a/L4b copy differ on exactly that
- Prune-mode folder-`Stat` faults other than not-exist (ENOTDIR, EACCES) are not classified there: they fall through to `Lstat` and surface as L4b ("cannot open … : not a directory"). No copy was ruled for this; it reuses L4b. Orchestrator: flag if product-vision should rule it
- `LockForPrune` stays the only place `KindFolderMissing` is swallowed; every other kind is a refusal (SCENARIO-03 binding decision, widened not changed)
- L5 flock fault seam stays `lockfile.WithFlock` through `snapshot.WithLocker`; cmd tests inject it by wrapping the factory, production wiring passes no option

**Left unbuilt**
- No post-open `fstat`: a swap between `Lstat` and `OpenFile` (directory or fifo replacing the file) is accepted — one local-user race, and `O_NOFOLLOW`/`O_NONBLOCK` keep it from hanging or following links. SCENARIO-14 onward does not own it
- Unreadable-recorded-path guard — SCENARIO-14; `.SQLITE` selector — SCENARIO-10

**Traps**
- `Lstat` makes `O_NONBLOCK` unreachable by any filesystem state (a fifo is refused before open); do not write a test claiming it hangs without dropping the not-regular branch too
- A directory opens `O_RDONLY` and `flock`s fine: without the `Lstat` branch a directory "locks" silently, which is why the directory row is the first red
- Mode rows (0000, 0500) need `skipAsRoot`; a 0500 folder needs a `t.Cleanup` chmod to 0700 or `t.TempDir` removal fails
- `osreason.Reason(nil)` panics; flock errors are bare `syscall.Errno`, not `*fs.PathError`, so Reason returns `err.Error()` ("operation not supported", "no locks available")
- Existing tests asserting "unclassified" (`lockfile_test.go:191-233,275`, `lock_test.go:106-114,156-178`) must be re-pointed, not deleted

## Orchestrator rulings (2026-10-06)

1. O_NONBLOCK double mutation accepted as planned; report that no filesystem state reddens O_NONBLOCK alone.
2. Prune-mode quarry-folder stat faults (ENOTDIR, EACCES) surface as the ruled L4b line via Lstat — no new copy (near-impossible states: quarry folder replaced by a file or unsearchable). Pin one row. Recorded for the final product-vision pass to judge the L4b fix text there.
3. No post-open fstat; Lstat-to-OpenFile race accepted (BR-L5 literal), stays in Left unbuilt.
