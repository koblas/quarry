# Review Report — round 01

### Target
Feature `phase0-snapshot`, range `4a6d476..cd952cc` (whole feature). Coverage gate: 0 uncovered, 24 declared unreachable. spec-check: OK.

### Triggered reviewers
- arch-reviewer, correctness-reviewer, refactor-advisor: `cmd/**/*.go`, `internal/**/*.go`
- test-reviewer: `**/*_test.go`

### Skipped reviewers
- api-reviewer: no HTTP surface
- pipeline-reviewer: no `.claude/**` changes

### BLOCKER
None. (arch-reviewer raised `buildManifest` reading the snapshot without a read port as BLOCKER; orchestrator downgraded to MINOR per the severity contract — no constructible wrong result, testability only.)

### MAJOR
1. **correctness** `internal/platform/sqlite/sqlite.go:33-36` (via `source.go:27`, `snapshot.go:112`) — `mode=ro` open of a plaintext WAL `data` with no `data-wal` creates `data-wal`/`data-shm` in the live bundle and leaves them (BR-1). Fix: new refusal **R8b** (spec Surface & Copy, ruled at final gate) — read header bytes 18–19 before any SQLite open; ==2 and no `<bundle>/data-wal` → R8b. After R6/R7, before R8/R9. Test asserts bundle file set unchanged. Never `immutable=1`; never delete created files.
2. **correctness** `cmd/quarry/main.go:10` — no signal handling; SIGINT/SIGTERM leaves full-size `.partial` debris, or an orphan `<ts>.json` if between commits. Fix: `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` in `main`; in `Sync`, when `ctx.Err() != nil` at a failure site, discard partials and return interrupt refusal **I1** (`quarry: sync interrupted; nothing was kept; run quarry sync again`, exit **1**, not 130). Commit rule: check ctx once immediately before the final renames; after that, renames complete regardless and the run reports its normal outcome.
3. **correctness** `internal/cli/sync.go:72,74` — stdout write errors discarded; `--json > full-disk` exits 0 with truncated output. Fix: return `&runtimeError{err: fmt.Errorf("write output: %w", err)}` (exit 1). Stderr warning writes may stay best-effort.
4. **correctness** `internal/snapshot/discover.go:39-41` — any `Stat` error skips the entry; an unreadable `Other.quicken` symlink makes discovery silently sync `Home.quicken`. Fix: skip only `errors.Is(err, fs.ErrNotExist)` (dangling link); other errors → `unreadableRefusal(home, path, err)`.

### MINOR
- **correctness** `sqlite.go:192` `tableColumns` returns `rows.Err()` unwrapped; unreachable markers at `sqlite.go:168` and `snapshot.go:226` are false (FTS5 virtual table reaches them). Wrap and remove both markers; add a test if constructible (FTS5 module absent → `no such module`).
- **correctness** `destination.go:116`/`atomicfile.go:24` — manifest not fsynced; `Close` error discarded. `f.Sync()` then `f.Close()` explicitly, return errors.
- **correctness** `sqlite.go:64-67,269-271` — `IsBusy` compares `Code` exactly; extended busy codes would miss. Compare `serr.Code & sqlite3.ErrNoMask` (or ExtendedCode).
- **correctness** `atomicfile.go:27-29` + `snapshot.go:170-176` — Link succeeds then Remove fails → committed `.sqlite` with no manifest. Treat commit as done if Link succeeded (or discard final too).
- **correctness** unreachable audit: `destination.go:120-121` and `sqlite.go:238` are production-reachable (ENOSPC) though handled; `snapshot.go:202,211` become reachable with signal handling → classify as I1, not unreachable.
- **refactor** Spec/finding IDs (R*, BR-*, M1, W1, SCENARIO-*) in production doc comments: `snapshot/{manifest,bundle,destination_refusal,outcome,snapshot,ports,discover,scope}.go`, `cli/render.go`. Replace each with the rule; state the Z-scope rule once in `scope.go`.
- **refactor** `snapshot.go:130-131` duplicates `sourceRefusal`'s predicate. Give `sourceRefusal` an `(error, bool)` return; ask once.
- **refactor** Merge `osReason` (bundle.go) into `writeReason` (destination_refusal.go), rename call-site-neutral (e.g. `causeText`); drop osReason's unreachable fallback.
- **refactor** `SchemaInfo.HasExtras()` method; use in `outcome.go` and `cli/render.go`.
- **refactor** `Sync` (86 lines): extract the commit sequence into a named helper (fits MAJOR 2's commit rule).
- **refactor/arch** Doc-comment budget overruns: `Sync`, `cli.Execute`, `Destination.Backup`, `sqlite.Backup`, `ReferenceDDL`, `ReferenceLabel`, `snapshotNameLayout`, `errNoReference`, `leftoverMaxAge`, `leftoverPartialPattern`, `newRootCommand`, `runtimeError`, `newSyncCommand`, `escapePath`, `runBackup`, `validateData`.
- **arch** `buildManifest` reads snapshot via concrete `sqlite`/`os` without a read port (downgraded from BLOCKER). Deferred — note in STATE.md Open debts for Phase 1.
- **arch** `ResolveBundlePath`/`DiscoverBundle` touch the filesystem without a port; `home` threaded two ways. Deferred, note only.
- **arch** `Destination` port 7 methods. No action.
- **test** `cmd/quarry/run_test.go:269-281` empty `--quicken=` test depends on ambient cwd; `t.Chdir(t.TempDir())`; leave stderr unpinned (copy unruled) and add Open-debt note for product-vision.
- **test** Delete `noopBackupSource` (destination_test.go:19-24); use `&fakeSource{}`.
- **test** Rename five `Test_sync_wraps_an_error_when_*` tests in `sync_faults_test.go` (81,132,367,393,422) to `Test_sync_refuses_when_*`.
- **test** Rename `Test_DiscoverBundle_follows_a_symlinked_bundle_and_skips_a_dangling_one` → `..._skips_a_dangling_symlink_and_finds_the_real_bundle`.
- **test** Delete `internal/cli/errors_internal_test.go` and `sync_internal_test.go` (trivial accessors covered end-to-end).
- **test** Test comment budget/spec-ID violations: `sync_faults_test.go:46-47,79-80,129-131,419-421,474-476,494-495,591-593`; `sqlite_test.go:272-275,405-407`; `sync_test.go:252-253`; `destination_test.go:113-114`; `source_test.go:18-19`; `manifest_test.go:42-43`.
- **test** Drop dead `assertNoPartialsLeftBehind` in `Test_sync_refuses_a_busy_bundle` (sync_faults_test.go:585-588).
- **test** File size: split `cmd/quarry/run_test.go` (649 lines) and `sync_faults_test.go` (628) — fix-if-cheap.
- **test** Real-time margins in two sqlite concurrency tests — accepted as-is; widen if flaky.

### NIT
- **refactor** `prepareRefusal` naming → content-based (`unwritableDirRefusal`, `writeFaultRefusal`).
- **refactor** Duplicate fact "excludes the `quarry: ` prefix" on RefusalError and MismatchError; `Manifest` vs `Encode` repeat.
- **refactor** `atomicfile.Commit`, `v9fixture` helper doc lengths.
- **test** `os.Geteuid()==0` skip guards (~9) — accepted.
- **test** Spec ID in test name `Test_sync_still_returns_the_R14_refusal_...`.
- **test** "and" names in two tests — accepted.

### Also applied from final-gate copy ruling (product-vision)
- W1/M1/Schema-clause singular/plural forms — spec `### Singular / plural`. Add count-1 test rows.

### Strengths
- Independently computed pins (fingerprint via `shasum`, R10 text via separate connection, second snapshot located by directory listing).
- One configurable fake per port wrapping the real adapter (`partialFaultDestination`).
- Age-gate boundary tests (59/61 min).
- Clean dependency graph; thin main; compile-time interface guards.

### Verdict: FAIL
