---
id: SCENARIO-03
status: open
---

# SCENARIO-03: Prune refuses while locked and deletes nothing (folds 04, 07, 09)

Cadence: test-first — write-safety guard: the delete refusal (`!dryRun` gate, lock before `planPrune`/`sweepOrphans`) and the never-create-folder claim
Acceptance test: `cmd/quarry/run_prune_lock_test.go` `Test_run_snapshots_prune_refuses_while_another_writer_holds_the_lock`
Acceptance test (SCENARIO-04, folded): `cmd/quarry/run_prune_lock_test.go` `Test_run_snapshots_prune_dry_run_runs_while_a_writer_holds_the_lock`
Acceptance test (SCENARIO-07, folded): `cmd/quarry/run_prune_lock_test.go` `Test_run_snapshots_prune_refuses_usage_and_config_before_the_lock`
Acceptance test (SCENARIO-09, folded): `cmd/quarry/run_usage_test.go` `Test_run_help_says_only_one_writer_runs_at_a_time`
Narrow loop: `go test ./internal/snapshot/ -run 'Lock' && go test ./internal/cli/ -run 'Prune' && go test ./cmd/quarry/ -run 'Prune|Help|Sync_lock|Lock'`
Mutation checks (each in the B run that builds the guard):
- drop the `!dryRun` gate (`snapshots_prune.go` RunE) → `Test_run_snapshots_prune_dry_run_runs_while_a_writer_holds_the_lock`, cli `Test_prune_dry_run_takes_no_lock`
- lock taken in the shared factory or in `List`/`PlanPrune` → same dry-run test plus `Test_run_snapshots_lists_while_a_writer_holds_the_lock`
- acquire moved after `srv.Prune` (plan + sweep run first) → acceptance (snapshots and orphan manifest gone) and cli `Test_prune_takes_the_lock_before_it_deletes`
- `KindFolderMissing` mapped to a refusal → `Test_lock_for_prune_proceeds_unlocked_when_the_quarry_folder_is_missing`, `Test_run_snapshots_prune_says_nothing_to_delete_with_no_quarry_folder_and_creates_nothing`
- factory locker built `ModeSync` instead of `ModePrune` → the creates-nothing test above (home holds no `Library` after)
Runs: A (1-2) | B1 (3) | B2 (4) | B3 (5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (`internal/snapshot`; plus `internal/cli` and `cmd/quarry` wiring)

Surface survey (nothing new to port): `snapshot.Locker` (`ports.go:76-82`, `Acquire(ctx)`) already covers prune — no mode, no new method. `WithLocker` (`snapshot.go:100-104`) reused. No port change, so no adapter work.

Contract: `quarry snapshots prune` (no `--dry-run`) acquires after usage (2) → config (1), before `Prune`. Held → stderr = L1p verbatim (`specification.md:76`), stdout empty (also `--json`), exit 1, nothing deleted, no orphan sweep. `--dry-run` and `snapshots` never lock. Quarry folder missing → no lock, `Nothing to delete: no snapshots in …`, exit 0, creates nothing. Other lock failures stay generic (SCENARIO-06).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_prune_lock_test.go` (new) `Test_run_snapshots_prune_refuses_while_another_writer_holds_the_lock` — through `run()`, real factory: reuse `holdLockedStore` (`run_sync_lock_test.go:30-52`) for the lock, a real store via `buildStoreFrom` (`run_prune_test.go:79`), `writeSnapshots` (`run_snapshots_test.go:54`) with MORE snapshots than `--keep` and one orphan manifest. Cells text + `--json`; assert L1p, stdout empty, exit 1, every snapshot and the orphan still present. Control arm: same fixture after `release()` deletes (differs in one variable). Must fail at the assertion (exit 0 today)
- [x] Step 2: `internal/snapshot/lock.go:18` — signature-only stub `(*Server).LockForPrune(ctx) (func(), error)` so the tests compile

### Build
- [x] Step 3 (B1): `internal/snapshot/lock.go:10-27`, `internal/snapshot/doc.go:9-11` — `LockForPrune` + L1p const beside `lockHeldSyncMsg` (share the held-check, do not copy it) + doc.go sentence (sync's store build and `Prune` each run under quarry's lock file). Maps `*lockfile.Error` `KindHeld` → `RefusalError{L1p}`; `KindFolderMissing` → no-op release, nil; anything else returned as given; nil locker → no-op. Tests in `lock_test.go` (extend `newLockedServer*` at :18-34 with a prune-mode pair): `Test_lock_for_prune_refuses_with_the_prune_lock_held_line` (holder is sync mode, contender prune mode, L1p text asserted verbatim, differs from L1s), `Test_lock_for_prune_proceeds_unlocked_when_the_quarry_folder_is_missing` (asserts folder still absent; release callable), `Test_lock_for_prune_returns_other_lock_errors_unchanged` (fake Locker returning an error shaped as the adapter's `*lockfile.Error` of a non-held kind, plus a plain error), nil-locker no-op, lock-after-release row. Mutation: `KindFolderMissing` → refusal
- [x] Step 4 (B2): `internal/cli/snapshots_prune.go:30-35,58-64` RunE, `internal/cli/sync.go:66-69` Long — acquire `srv.LockForPrune` only `if !dryRun`, after `newSnapshots` and before `prune(...)`, `defer release()` (do not shadow `err`; error → `&runtimeError{err}`, so exit 1 and nothing on stdout). Long: prune gets the one-writer paragraph after the first paragraph and the replaced `--dry-run` line, sync gets its paragraph right after the auto-prune paragraph — all verbatim from `specification.md:87-96`. cli tests (fake `SnapshotsFactory` whose Server uses `snapshot.WithLocker(recordingLocker)` + `WithRemove` recorder, as `cmd/quarry/run_sync_lock_test.go:140-` does for sync): `Test_prune_takes_the_lock_before_it_deletes` (event order acquire → remove → release; refusing locker → remove never called, L1p exit class), `Test_prune_dry_run_takes_no_lock` (acquired == 0), `Test_prune_releases_the_lock_when_it_returns` (success and a refusal arm), acquire-error row (nothing on stdout), usage/config before lock (`--keep 0`, malformed config: acquired == 0). Re-point the Long pin `snapshots_prune_test.go:8-21` (keep the dry-run-line pin; add the paragraph). Mutations: drop `!dryRun`; acquire after `Prune`
- [ ] Step 5 (B3): `cmd/quarry/run.go:109-124` `newSnapshotsFactory` — add `snapshot.WithLocker(lockfile.New(lockPathUnder(storeDir), lockfile.ModePrune))` (the factory is shared with `snapshots` and `--dry-run`: harmless only because neither calls `LockForPrune`; never lock in the factory). Tests in `run_prune_lock_test.go` through `run()`: the folded acceptances (04 dry-run while locked, exit 0, lists what it would delete — control: same listing as `run_prune_dryrun_test.go:17`; 07 table: `--keep 0` → usage line exit 2, malformed config `quicken.path = 12\n` → `configShown`/`configFix` C1 line exit 1, both while locked, stdout empty); `Test_run_snapshots_prune_refuses_with_nothing_beyond_the_cap_while_locked` (count ≤ N → L1p); `Test_run_snapshots_lists_while_a_writer_holds_the_lock` (`snapshots` exit 0); `Test_run_snapshots_prune_says_nothing_to_delete_with_no_quarry_folder_and_creates_nothing` (HOME empty: stdout `Nothing to delete: no snapshots in ~/Library/Application Support/quarry/snapshots`, exit 0, `os.ReadDir(home)` empty); `Test_run_snapshots_prune_proceeds_past_a_lock_left_by_an_earlier_run` (lock file exists, nobody holds it: deletes as normal). Add `Test_run_help_says_only_one_writer_runs_at_a_time` to `run_usage_test.go` pinning both `sync --help` and `snapshots prune --help` Long text at wrap width (sync paragraph follows the auto-prune paragraph; prune paragraph, replaced dry-run line); re-point `run_usage_test.go:44-70` only if it conflicts. Mutations: `ModeSync` in the factory; factory/`List` locking

### Sweep
- [ ] Step 6 (V): fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `LockForPrune`; `Locker` doc (`ports.go:76`) already says prune — leave

### Verify
- [ ] Step 7 (V): `.claude/scripts/verify.sh <start> ./internal/snapshot/... ./internal/cli/... ./cmd/quarry/...` + `spec-check.py snapshot-safety`; tick SCENARIO-03 with its acceptance test and 04, 07, 09 with "delivered by SCENARIO-03" lines naming their tests; rewrite STATE.md

## Handoff

**Binding decisions:**
- `LockForPrune` mirrors `LockForSync` in `internal/snapshot/lock.go`; folder-missing is a silent no-lock there, never at `lockfile` — SCENARIO-06 adds more `lockfile` kinds and their copy beside it, and the L3/L4a/L4b/L5p lines must reach prune through the same method
- Prune acquires in RunE only under `!dryRun`, after config, before `Prune` — `Server.Prune`/`PlanPrune`/`autoPrune` never lock (auto-prune runs under sync's lock; a second acquire in-process would refuse itself)
- Prune-mode locker lives in `newSnapshotsFactory` (shared with `snapshots` and `--dry-run`); nothing there may call `LockForPrune`
- Prune on an existing quarry folder now creates a 0600 `quarry.lock` there; only a missing folder creates nothing

**Left unbuilt:** L2/L3/L4a/L4b/L5s/L5p copy and their `lockfile` kinds (SCENARIO-06); unreadable-recorded-path guard in prune (SCENARIO-14)

**Traps:**
- Moving the acquire above `newSnapshots` or into `Server.Prune` still passes unit tests but breaks the config-before-lock order or auto-prune
- A prune test that holds a lock with a sentinel store refuses at the cannot-tell step if the lock arm is mutated away — use a real store (`buildStoreFrom`) so the unlocked arm actually deletes, else the "deletes nothing" claim has no control
- `defer release()` inside the `if !dryRun` block is function-scoped (fine), but `release, err :=` shadows `err`
- The Long text is wrapped in the source; pins assert the exact line breaks of `specification.md:88-96`

## Phase report

Run B2 done (step 4), `<start>` d223b623, B1 f3f9976f. Narrow loop green; cmd acceptance still red only because B3 has not wired the prune-mode locker in `newSnapshotsFactory` (`run_prune_test`-style help pins in `cmd/quarry` did not conflict with the new Long text).

- `internal/cli/snapshots_prune.go:30-39,67-75`: Long has the one-writer paragraph and the replaced `--dry-run` paragraph (verbatim, `specification.md:87-96`); RunE picks `PlanPrune`, and under `!dryRun` calls `srv.LockForPrune` after `newSnapshots`, `defer release()`, then `prune = srv.Prune`. Lock error -> `&runtimeError{lockErr}` (`lockErr`, `err` not shadowed).
- `internal/cli/sync.go:70-72`: sync Long paragraph right after the auto-prune paragraph (ends "Run quarry snapshots to list them."), verbatim.
- `internal/cli/snapshots_prune_test.go`: Long pin re-pointed (paragraph + replaced dry-run line).
- `internal/cli/snapshots_prune_lock_test.go` (new): `eventLog`/`logLocker`/`runPrune` (real Server over temp folder, `WithRemove` logs without deleting); tests `Test_prune_takes_the_lock_before_it_deletes`, `..._refused_by_a_held_lock_deletes_nothing_and_prints_nothing` (text/json), `..._returns_a_lock_failure_that_is_not_a_held_lock_with_nothing_on_stdout`, `Test_prune_dry_run_takes_no_lock`, `..._releases_the_lock_when_it_returns`, `..._releases_the_lock_when_it_refuses_after_taking_it`, `..._refused_before_the_lock_never_takes_it` (keep 0, config error). Red before code: first, refused, lock-failure and both release tests (assertions); dry-run and before-lock tests green on arrival (they pin absence; the mutation below makes dry-run red).
- Mutations (backup `$TMPDIR/mut-b2-snapshots_prune.go.bak`, restored, diff clean): gate dropped (`if !dryRun || dryRun`) -> `Test_prune_dry_run_takes_no_lock` red (`[acquire remove remove release]`); acquire after `prune(...)` -> `Test_prune_takes_the_lock_before_it_deletes` (`[remove remove acquire release]`) and the refusal test red.
- B3 must still: factory locker, `cmd/quarry` tests, `Test_run_help_says_only_one_writer_runs_at_a_time`.
