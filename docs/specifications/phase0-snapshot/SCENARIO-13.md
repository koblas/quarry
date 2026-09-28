---
id: SCENARIO-13
status: done
---

# SCENARIO-13: Snapshots directory not writable (R13) + BR-11's R14 unit-level requirement

Size verdict: OWNS A RUN. One `When` (R13's). R14 is not a Gherkin scenario — BR-11 names
SCENARIO-13 as its owner, a unit-level obligation riding the same Destination-refusal
machinery R13 needs, not a second acceptance behaviour.

Cadence: test-first — write-safety guard: BR-9's "nothing left on disk" now covers
`Backup`/`WriteManifest`/`CommitManifest`/`CommitSnapshot` failure via `Discard`, the same
partial-lifecycle guard SCENARIO-08 built test-first for the content-check branch
(`build.md` → *Build cadence*, "write-safety guards").
Acceptance test: `cmd/quarry/run_test.go` `Test_run_refuses_a_snapshots_directory_that_is_not_writable`
Narrow loop: `go test ./cmd/quarry/... ./internal/snapshot/... -run '(?i)writable|prepared|backup_fails|manifest_fails|committing|classified_error|discard'`
Mutation checks: the `errors.Is(err, fs.ErrPermission)` branch at the `Backup` call site →
delete it and the acceptance test's exact-R13-copy assertion goes red (falls to R14
instead); the `Discard` call added at the `WriteManifest`/`CommitManifest`/`CommitSnapshot`
failure branches → delete one and its test's on-disk-empty assertion goes red.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_test.go:363-380` (append after `Test_run_refuses_an_encrypted_bundle`)
  `Test_run_refuses_a_snapshots_directory_that_is_not_writable` — real bundle
  (`v9fixture.OpenBundle`), pre-create `.../quarry/snapshots` itself (`os.MkdirAll` `0o700`),
  `os.Chmod` it `0o500` (`t.Cleanup` restores `0o700` first, before `t.TempDir()` cleanup —
  the dir must already exist so `Prepare`'s `MkdirAll` is a no-op and the real failure
  happens later, at `Backup`'s `atomicfile.Create`). Assert exit 1, stdout empty, stderr is
  R13's exact line, `os.ReadDir` on the snapshots dir is empty. No stub needed — `run()`'s
  signature is unchanged; today's generic wrap already returns exit 1, so this reddens at
  the stderr-content assertion.

### Build
- [x] Step 2: `internal/snapshot/destination_refusal.go` (new) — `writeReason(err) string`
  (unwraps `*fs.PathError` (`Create`/`Remove`) and `*os.LinkError` (`Commit`'s `os.Link`) to
  the inner errno text; falls back to `err.Error()` for a real sqlite disk-full — a
  reachable default, not `// unreachable`, unlike `bundle.go`'s `osReason`, which only ever
  sees `*fs.PathError`). Two classifiers built on it: one that always returns R13 (for
  `Prepare`, whose row is "cannot create/write the dir" regardless of OS reason), one that
  returns R13 when `errors.Is(err, fs.ErrPermission)` and R14 otherwise (for every
  post-`Prepare` write). Both format `homepath.Abbreviate(home, s.snapshotDir)`.
- [x] Step 3: `internal/snapshot/snapshot.go:121-123` (`Prepare` call site) — always-R13
  classifier. Strengthen `internal/snapshot/sync_faults_test.go:109-123`
  `Test_sync_wraps_an_error_when_the_snapshots_directory_cannot_be_prepared` (add
  `WithSnapshotDir`/`WithHome` to the `Server`) to assert `errors.As(err,
  &snapshot.RefusalError{})` and the exact R13 copy.
- [x] Step 4: `internal/snapshot/snapshot.go:128-131` (`Backup` call site) — keep
  `sqlite.IsNotADB`/`IsBusy` routing to `sourceRefusal` first; every other error goes
  through the permission-or-not classifier. Update
  `internal/snapshot/sync_faults_test.go:76-90`
  `Test_sync_wraps_an_error_when_the_backup_fails` (`WithSnapshotDir`/`WithHome` added) to
  expect the R14 refusal for its generic `errBoom` — a deliberate reclassification, flagged
  below, not a regression. `Test_sync_refuses_when_the_source_reports_a_classified_error`'s
  busy/encrypted backup rows are unaffected.
- [x] Step 5: `internal/snapshot/snapshot.go:153-166`
  (`WriteManifest`/`CommitManifest`/`CommitSnapshot` call sites) — before each now-classified
  return: `WriteManifest` discards `snapshotPartial`; `CommitManifest` discards
  `snapshotPartial` then `manifestPartial`; `CommitSnapshot` discards the **manifest
  final** (`manifestPath`, from the `FinalPaths` call already made at line 132 — it just
  committed) before `snapshotPartial` — final first so the reservation stays held until
  last. Widen `Destination.Discard`'s doc (`ports.go`) — it now removes any path Sync hands
  it, partial or committed final, not only "a snapshot partial `Backup` created". Update the
  three existing `sync_faults_test.go:280-376` fault tests (add `WithSnapshotDir`/
  `WithHome`; inject a `*fs.PathError{Err: syscall.ENOSPC}`-flavored error in place of
  `errBoom`) to assert the R14 copy and that `os.ReadDir` on the real destination dir is
  **empty** afterward — not `assertNoPartialsLeftBehind`, which only matches the `.partial`
  suffix and would miss a leftover committed manifest final on the `CommitSnapshot` case.
- [x] Step 6: one table test, mirroring `sync_faults_test.go:378-402`'s
  discard-fails-but-refusal-still-returned pattern, covering
  `WriteManifest`/`CommitManifest`/`CommitSnapshot`: `Discard` itself fails but the R14
  refusal returned is unchanged (best-effort, SCENARIO-08's rule).

### Sweep
- [x] Step 7: fix what `go build ./... && golangci-lint run ./...` reports; doc comments on
  `destination_refusal.go`'s symbols and the widened `Discard` doc.

### Verify
- [x] Step 8: full verification (`go build`, full suite + coverage, `-race` on
  `internal/snapshot`/`cmd/quarry`, `golangci-lint`) + `.claude/scripts/spec-check.py
  phase0-snapshot` → tick SCENARIO-13 with its acceptance test.

## Handoff

**Binding decisions:**
- Classification is **content-based after `Prepare`, call-site-forced for `Prepare`
  itself**: `Prepare` failure is always R13 whatever the OS reason (its row names the
  directory, not a permission split); every later `Backup`/`WriteManifest`/
  `CommitManifest`/`CommitSnapshot` failure is R13 when `errors.Is(err, fs.ErrPermission)`,
  else R14. Orchestrator ruling this scenario adds, flagged as a **deviation** for the final
  product-vision pass: R13's row text ("cannot write to … dir") did not previously say it
  covers a permission failure discovered during a *later* write, only at creation time.
- A non-permission, non-sqlite-classified `Backup` error now returns R14 instead of the old
  generic `sync <bundlePath>: %w` wrap — changes
  `Test_sync_wraps_an_error_when_the_backup_fails`'s expected message; deliberate, not a
  regression.
- `Destination.Discard` (best-effort, never overrides the classified refusal on its own
  failure — SCENARIO-08's rule, unchanged) now also runs on `WriteManifest`/
  `CommitManifest`/`CommitSnapshot` failure and on a committed manifest final when
  `CommitSnapshot` fails — closes STATE.md's "still leak a partial … unowned until
  SCENARIO-13" debt with no leftover of any kind, partial or final.

**Left unbuilt:** nothing — the `CommitSnapshot`-after-`CommitManifest` orphan STATE.md
flagged is closed by Step 5's final-then-partial discard order.

**Traps:**
- `os.MkdirAll` returns `nil` for an already-existing directory regardless of its
  permission bits — the acceptance test must pre-create the snapshots dir itself and chmod
  *that*, or `Prepare` never even reaches the real bug (a permission failure surfacing at
  `Backup`, not at creation).
- `bundle.go`'s `osReason` only unwraps `*fs.PathError`; `atomicfile.Commit`'s `os.Link`
  failure is `*os.LinkError` — reusing `osReason` verbatim silently prints a wrong or empty
  reason for a `CommitManifest`/`CommitSnapshot` fault. Use the new `writeReason` for every
  destination-side refusal, never `osReason`.

**Implementation notes (this run):**
- `writeReason` is a generic `errors.Unwrap` loop to the innermost cause, not a type switch
  on `*fs.PathError`/`*os.LinkError` as the plan's Step 2 sketch described — both types
  implement `Unwrap`, so one loop covers both with no separate branch to leave uncovered
  (the plan's own concern about `bundle.go`'s `osReason` needing a `// unreachable` default
  does not apply here: the loop's fallback is reached for real by any error with no further
  `Unwrap`, e.g. a raw `sqlite3.Error`).
- Orchestrator constraint (this run): a `Backup` failure must classify through
  `sourceRefusal` first — `sqlite.IsNotADB`/`IsBusy` stay the *first* check at the `Backup`
  call site, `writeRefusal` only sees what neither matched. Added
  `Test_sync_reports_the_source_refusal_when_a_backup_error_is_also_a_permission_error`
  (a `fakeSource.backupErr` wrapping both a busy `sqlite3.Error` and `fs.ErrPermission`) to
  prove the ordering; mutation-verified by swapping the two checks, which reddens exactly
  that test.
- Step 6's table test also mutation-verifies best-effort `Discard`-fails-but-refusal-
  unchanged across all three of `WriteManifest`/`CommitManifest`/`CommitSnapshot`, using
  `*os.LinkError`-flavored faults for the two `Commit*` cases (realistic: `Commit` really
  fails via `os.Link`) and `*fs.PathError` for `WriteManifest` (`Create`'s real failure
  shape) — the plan's Step 5 sketch used `*fs.PathError` for all three; this run varied the
  shape per call site instead, which exercises `writeReason`'s unwrap loop against both
  types across the fault tests as a whole.
- Deviation flagged for the final product-vision pass, per orchestrator ruling: any
  unclassified `Backup`/write error — including a context cancellation mid-backup, or a
  source-side SQLite error `sourceRefusal` does not name — now renders as R14 ("free disk
  space, then run quarry sync again"), which may not fit every such cause. No branch was
  added for this; it rides the existing default. Recorded in STATE.md `## Open debts`
  alongside the R13-covers-a-later-write deviation above.
