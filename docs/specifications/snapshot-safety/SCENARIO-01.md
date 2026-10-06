---
id: SCENARIO-01
status: open
---

# SCENARIO-01: A sync refuses while another sync or prune is running

Cadence: test-first — exclusive-claim adapter (`flock(LOCK_EX|LOCK_NB)` on `quarry.lock`)
Acceptance test: `cmd/quarry/run_sync_lock_test.go` `Test_run_sync_refuses_while_another_writer_holds_the_lock`
Acceptance test (SCENARIO-02, folded): `cmd/quarry/run_sync_lock_test.go` `Test_run_sync_from_refuses_on_the_lock_before_resolving_the_snapshot`
Acceptance test (SCENARIO-05, folded): `cmd/quarry/run_sync_lock_test.go` `Test_run_sync_proceeds_past_a_lock_left_by_an_earlier_run`
Acceptance test (SCENARIO-08, folded): `cmd/quarry/run_sync_lock_test.go` `Test_run_status_reports_while_a_sync_holds_the_lock`
Narrow loop: `go test ./internal/platform/lockfile/ ./internal/snapshot/ -run 'lock|acquire' && go test ./cmd/quarry/ -run 'lock|Test_run_never_replaces_the_store_when_the_build_fails|Test_run_refuses_an_unmappable_value_and_keeps_the_snapshot'`
Mutation checks: `LOCK_NB` dropped from the flock call → `Test_acquire_refuses_at_once_while_another_open_holds_it`; prune mode's no-create branch replaced by sync mode's mkdir → `Test_acquire_in_prune_mode_never_creates_the_quarry_folder`; `snapshot.WithLocker(...)` wiring line deleted from `newServerFactory` → `Test_run_sync_refuses_while_another_writer_holds_the_lock`; acquire moved below `ResolveBundle` (sync.go:118) → `Test_run_sync_refuses_on_the_lock_before_looking_for_the_quicken_file`; acquire moved into the non-`--from` branch (below sync.go:114) → `Test_run_sync_from_refuses_on_the_lock_before_resolving_the_snapshot`; `defer` release replaced by `_ = release` → `Test_run_sync_releases_the_lock_when_it_returns`
Runs: A (1-2) | B1 (3-5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (`internal/snapshot`) + new `internal/platform/lockfile`

User-visible contract (this scenario): `quarry sync [--json] [--quicken P | --from X]` with the lock held → stderr exactly `quarry: another quarry sync or quarry snapshots prune is running, so this sync changed nothing; run the command again once that one finishes\n` (L1s), stdout empty (text and `--json`), exit 1; no `snapshots/` dir created, store bytes unchanged. Order: usage (2) → config (1) → lock (1) → bundle / `--from` resolution. Lock free → sync as today; first-ever sync leaves the quarry folder 0700 and `quarry.lock` 0600, empty.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_sync_lock_test.go` (new) `Test_run_sync_refuses_while_another_writer_holds_the_lock` + `Test_run_sync_from_refuses_on_the_lock_before_resolving_the_snapshot` — through `run()` (testEnv → `newServerFactory`, so the shipped wiring line is under test); v9fixture bundle in `<home>/Documents`, sentinel `quarry.duckdb` as in `run_import_test.go:166-172`; hold a real lock via `lockfile` in-process on `<home>/Library/Application Support/quarry/quarry.lock`; cells: text and `--json` (stdout empty both); assert L1s verbatim, exit 1, no `snapshots` dir, sentinel bytes unchanged. 02: `--from 20990101T000000Z` → L1s, not the from.go:118-121 unknown-snapshot line
- [ ] Step 2: `internal/platform/lockfile/` (new: `doc.go`, `lockfile.go`) signature-only `Locker`, `Mode` (sync / prune), `New`, `Acquire`, `Release`, typed `*Error` with `Kind`; `internal/snapshot/ports.go:74` `Locker` port; `internal/snapshot/snapshot.go:28-43,95-122` `locker` field + `WithLocker`; `(*Server).LockForSync` stub in new `internal/snapshot/lock.go`. Red = exit 0 / unknown-snapshot line where L1s expected

### Build
- [ ] Step 3: `internal/platform/lockfile/lockfile.go` `Acquire`/`Release` + `lockfile_test.go` — sync mode `MkdirAll` quarry folder 0700; prune mode never creates it and returns `*Error` kind folder-missing (no lock held); open `O_RDONLY|O_CREATE|O_NOFOLLOW|O_NONBLOCK` 0600; stdlib `syscall.Flock(LOCK_EX|LOCK_NB)`, `EWOULDBLOCK` → `Kind` held; every other failure wrapped with the path, unclassified (06 classifies); fd closed on every failure after open; flock func injected through a `New` option (never a package var). Tests: `Test_acquire_refuses_at_once_while_another_open_holds_it` (second `Acquire` run in a goroutine, bounded by `select` with a deadline so dropping `LOCK_NB` fails at an assertion, not the `go test` timeout); `Test_acquire_in_sync_mode_creates_the_folder_0700_and_the_lock_file_0600` (mask `Perm()`); `Test_acquire_in_prune_mode_never_creates_the_quarry_folder` (control: same call in sync mode creates it); `Test_acquire_after_release_succeeds`; fault rows: mkdir fails (`Application Support` a regular file → ENOTDIR), open fails, injected flock error other than would-block (fd closed, error wraps it)
- [ ] Step 4: `internal/snapshot/lock.go` `(*Server).LockForSync(ctx)` + `lock_test.go` — returns a release func; nil locker → no-op release, nil error; `*lockfile.Error` held → `RefusalError` L1s verbatim; any other error returned unchanged (06 owns its copy). Tests against the real `lockfile` adapter in `t.TempDir()`: `Test_lock_for_sync_refuses_with_the_lock_held_line`; `Test_lock_for_sync_after_release_locks_again`; `Test_lock_for_sync_without_a_locker_is_a_no_op`; `Test_lock_for_sync_returns_other_lock_errors_unchanged` (via lockfile's injected flock option)
- [ ] Step 5: `internal/cli/sync.go:110-113` acquire right after `newServer`, before the `Changed("from")` branch, `defer` release so it drops when RunE returns; `cmd/quarry/run.go:26-34` port guard, `:64-75` `snapshot.WithLocker(lockfile.New(<storeDir>/quarry.lock, sync mode))`. Re-point `run_import_test.go:187-189` and `run_store_faults_test.go:163-165` to `{"quarry.duckdb","quarry.lock","snapshots"}`; `run_store_faults_test.go:76-82` pre-create `quarry.lock` 0600 before the chmod 0500 (expected "cannot write to" line unchanged). Tests in `run_sync_lock_test.go`: `Test_run_sync_refuses_on_the_lock_before_looking_for_the_quicken_file` (lock held, no bundle under `$HOME` → L1s; cells: malformed config while held → C1 exit 1; extra arg while held → usage exit 2); `Test_run_sync_releases_the_lock_when_it_returns` (`runWith` with `NewServer` wrapping `newServerFactory(fixedRates())` and appending `snapshot.WithLocker(<recording fake>)`; release called exactly once after return — cells: successful sync, refusal after the lock is taken (no bundle)); `Test_run_sync_proceeds_past_a_lock_left_by_an_earlier_run` (two `run()` syncs in one process, both exit 0 — the control arm for Step 1's negative assertions; plus a `quarry.lock` left by an acquire+release); `Test_run_sync_creates_the_quarry_folder_0700_and_its_lock_file_0600` (first-ever sync); `Test_run_status_reports_while_a_sync_holds_the_lock` (store built by a sync, lock held; table over `status`, `sql`, `snapshots` — each stdout equals its unlocked run's, exit 0; the `snapshots` cell guards 03's prune-mode wiring in the shared `newSnapshotsFactory`; MCP n/a: same `newReportFactory` as `status`, no lock call reachable)

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `lockfile` package/`Acquire`/`Error`/`Kind`, `Locker`, `WithLocker`, `LockForSync` (state: taken once per command, held until release; auto-prune never re-takes it). Do NOT touch sync Long or `doc.go` (SCENARIO-03/09)

### Verify
- [ ] Step 7: `.claude/scripts/verify.sh <start> ./internal/platform/lockfile/... ./internal/snapshot/... ./internal/cli/... ./cmd/quarry/...` + `spec-check.py snapshot-safety` → tick SCENARIO-01 with its acceptance test, and 02/05/08 each "delivered by SCENARIO-01" with its folded test

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- The lock is taken in `internal/cli` RunE through a `Server` method (`LockForSync`; 03 adds `LockForPrune` for L1p), never inside `SyncAndImport`/`ImportFrom`/`Prune`/`autoPrune` — BR-L4 "auto-prune never re-takes it" and BR-L3's before-`ResolveBundle` ordering (a package func called in RunE) both depend on it.
- Prune mode with the quarry folder missing returns `*lockfile.Error` with `Kind` folder-missing and holds no lock; 03's `LockForPrune` maps it to "proceed unlocked" (existing no-snapshots line), never to a refusal.
- Mode (sync creates the folder 0700 / prune never creates it) is fixed when `lockfile.New` is built in `cmd/quarry`; the `Locker` port carries no mode — 03 wires prune mode in `newSnapshotsFactory` (run.go:106-120) without touching the port.
- Nil locker = no lock — `internal/snapshot` and `internal/cli` tests build Servers without one; the wiring pins are the `cmd/quarry` tests through `run()`.
- Copy for every lock refusal lives in `internal/snapshot` (`RefusalError`); `lockfile` returns only classified `*Error` kinds, no user copy.
- Flock fault seam is a `lockfile.New` option, not a package var — 06's L5s/L5p tests use it.

**Left unbuilt** — named so nobody assumes it exists:
- `lockfile` kinds beyond held / folder-missing (not-regular via `Lstat`, folder-create, file-create, open, lock-other) and L2/L3/L4a/L4b/L5s copy — SCENARIO-06.
- `(*Server).LockForPrune`, L1p, prune RunE acquire under `!dryRun`, prune-mode wiring, Long paragraphs, doc.go sentence — SCENARIO-03.

**Traps** — things that look right and are not:
- flock is per open file description: two sequential `run()` calls in one test binary collide unless release runs when RunE returns.
- Opening a fifo read-only blocks without `O_NONBLOCK`; Lstat-then-open races without `O_NOFOLLOW` — both flags go in now even though their tests are 06's.
- An unreferenced `*os.File` is closed by its GC cleanup, dropping the flock: a test holding the lock keeps it referenced (`t.Cleanup(lock.Release)`), and a leaked release cannot be pinned by two sequential runs alone — hence the recording-fake release test.
- `O_CREATE` on an existing `quarry.lock` needs no folder write — that is why pre-creating it keeps `run_store_faults_test.go:80` on its original line.
- A bundle or `--from` refusal on a first-ever sync now leaves the quarry folder and `quarry.lock` behind (no `snapshots/`); `run_bundle_refusals_test.go:36,60` check only `snapshots/` and stay green.

## Orchestrator rulings (2026-10-06)

- Kill -9 edge row gets a pin in B1: `internal/platform/lockfile` test that re-execs the test binary (`os.Args[0]`, `-test.run` helper-process pattern, env flag) to hold the lock in a child, SIGKILLs it, waits, then acquires successfully in the parent. Control: acquire while the child is alive refuses held.
