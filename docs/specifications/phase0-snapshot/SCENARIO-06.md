---
id: SCENARIO-06
status: open
---

# SCENARIO-06: Encrypted file means Quicken does not have it open

Cadence: test-first — `internal/platform/sqlite.runBackup` (sqlite.go:198-226) treats `SQLITE_BUSY`/`SQLITE_LOCKED` from `bk.Step` as "not done, no error" (mattn's own contract) and loops with no sleep, no deadline and no `ctx` check: a lock held on the source during backup spins the goroutine forever instead of surfacing R9. Confirmed by running `Backup` under a 2s bounded `select` against a fixture holding the lock — it never returned. This is a bug fix, so it and everything that depends on it (`IsBusy`/`sourceRefusal`) go test-first.
Acceptance test: `cmd/quarry/run_test.go` `Test_run_refuses_an_encrypted_bundle`
Acceptance test (SCENARIO-07, folded): `internal/snapshot/sync_faults_test.go` `Test_sync_refuses_a_busy_bundle`
Narrow loop: `go test ./cmd/quarry/... ./internal/snapshot/... ./internal/platform/sqlite/... -run 'refuses|[Bb]usy|NotADB|wraps_an_error'`
Mutation checks: none — not a write-safety/atomicity guard (build.md's mandatory list); the reproduction test above is this scenario's red-before-green proof, and the exact-copy acceptance assertions redden on their own if classification is removed (falls back to the generic `sync <path>: ...` wrap).

Size verdict: OWNS A RUN — absorbs SCENARIO-07 (orchestrator fold, spec `:269`); one classification seam plus the `runBackup` fix it exposed, both required for R9 to be true rather than a hang.

## Verified by experiment (recorded so Build does not re-derive it)
- go-sqlite3 v1.14.52: `SQLITE_NOTADB` is `sqlite3.Error{Code: 26}`; `SQLITE_BUSY` is `Code: 5`; `SQLITE_LOCKED` is `Code: 6`. All survive `errors.As` through this codebase's `fmt.Errorf("...: %w", err)` wrapping at any depth.
- **WAL fixture does not reproduce R9.** A `mode=ro` reader against a WAL-mode db is not blocked by a second connection's `BEGIN IMMEDIATE` (or `EXCLUSIVE`) plus a write — confirmed: the read returns immediately, no error. `v9fixture` (BR-3, WAL-mode) is the wrong fixture for R9.
- **Rollback-journal (default, non-WAL) mode does reproduce it.** A second connection's `BEGIN EXCLUSIVE` plus a write, left uncommitted, blocks a `mode=ro` reader's first query for the DSN's `_busy_timeout` and then fails `Code: 5`. `platform/sqlite/sqlite_test.go`'s existing `newTestDatabase` helper (line 16) already builds this non-WAL fixture — reuse it for both the `Open`-time and `Backup`-time repros.
- `_busy_timeout` is a DSN query param the driver parses and applies via `PRAGMA busy_timeout` itself — mattn already runs it at 5000ms on every connection by default (`sqlite3.go:1665`, matches STATE.md's open debt). It has no effect on `sqlite3_backup_step`, which returns control to the caller on `BUSY`/`LOCKED` rather than waiting — the caller must retry.

## Reused / checked
`internal/platform/sqlite/sqlite.go:18-36` `OpenReadOnly` (busy-timeout variant added alongside, signature unchanged — 18 existing call sites, confirmed by grep, stay untouched); `sqlite.go:156-226` `Backup`/`runBackup` (gains a `busyTimeout` param — 6 test call sites in `sqlite_test.go` pass `0`, inert with no contention, plus 1 production call site); `internal/snapshot/bundle.go:87-99` `notABundleRefusal`/`unreadableRefusal` (the `RefusalError{msg: fmt.Sprintf("%s ...", homepath.Abbreviate(home, path))}` shape R8/R9 copy); `sync_faults_test.go`'s `fakeSource`/`assertNoPartialsLeftBehind` (reused); `cmd/quarry/run_test.go:333-346` `Test_run_reports_exit_1_when_sync_fails` (repointed per STATE.md instruction).

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `internal/snapshot/snapshot.go:22-68` — stub `WithHome(home string) Option`, `WithBusyTimeout(d time.Duration) Option` (fields only), exported `DefaultBusyTimeout` const, so both acceptance tests compile.
- [ ] Step 2: `cmd/quarry/run_test.go:333-346` — rename `Test_run_reports_exit_1_when_sync_fails` to `Test_run_refuses_an_encrypted_bundle`; assert the exact R8 stderr line (spec `:231`), empty stdout, and that the snapshots dir under `~/Library/Application Support/quarry/snapshots` does not exist.
- [ ] Step 3: `internal/snapshot/sync_faults_test.go` (new) `Test_sync_refuses_a_busy_bundle` (Server-level, this scenario's boundary choice for R9) — non-WAL `data` file, second connection holds `BEGIN EXCLUSIVE` uncommitted for the test's duration, `WithBusyTimeout(80*time.Millisecond)`, `WithHome`; asserts the exact R9 `RefusalError` message (spec `:232`) **and** `assertNoPartialsLeftBehind`/no snapshots dir.
- [ ] Step 4: `internal/platform/sqlite/sqlite_test.go` (new) `Test_backup_fails_when_the_source_is_exclusively_locked_past_the_deadline` — reuse `newTestDatabase`, hold `BEGIN EXCLUSIVE` on a second connection, run `Backup` in a goroutine, bound the wait with `select`/`time.After(2s)` so a still-broken `runBackup` fails the test (times out) instead of hanging the suite. Confirm all four new/renamed tests fail at their assertion — Steps 1-3 at the wrong message, Step 4 at the bound — not at compile.

### Build
- [ ] Step 5: `internal/platform/sqlite/sqlite.go` — add `IsNotADB(err error) bool` and `IsBusy(err error) bool` (`errors.As` into `sqlite3.Error`; `IsBusy` true for **both** `ErrBusy` and `ErrLocked`); add `OpenReadOnlyBusy(ctx, path string, busyTimeout time.Duration) (*DB, error)` appending `_busy_timeout=<ms>` to `OpenReadOnly`'s DSN. Table test covering `IsBusy`/`IsNotADB` against `Code` 5, 6, 26 and a plain `errors.New` (each combination, so dropping `ErrLocked` is caught); `Test_open_read_only_busy_succeeds_once_the_writer_releases_the_lock_before_the_timeout` (control: release before the timeout elapses — proves the DSN param is actually wired, not just that BUSY can happen).
- [ ] Step 6: `sqlite.go:198-226` `runBackup` — on `Step`'s not-done/no-error return, sleep a short retry interval and loop again, bounded by a caller-supplied deadline; past the deadline, return an `IsBusy`-recognized error; on the caller's own `ctx` cancellation (distinct from the retry budget), propagate `ctx.Err()` unchanged. `Backup(ctx, src *DB, destPath string, busyTimeout time.Duration) error` carries the budget in; update its 6 test call sites (pass `0`) and its 1 production call site. Step 4's reproduction test goes green.
- [ ] Step 7: `internal/snapshot/refusal.go` — add unexported `sourceRefusal(home, bundlePath string, err error) error`: `sqlite.IsNotADB` → R8, `sqlite.IsBusy` → R9 (both via `homepath.Abbreviate`, spec `:231-232` verbatim), else the unchanged `fmt.Errorf("sync %s: %w", bundlePath, err)`. `internal/snapshot/snapshot.go:94-115` `Sync` — route the `source.Open`, `source.Probe` and `destination.Backup` error branches through it; `NewServer` defaults `busyTimeout` to `DefaultBusyTimeout` when unset.
- [ ] Step 8: `internal/snapshot/source.go:18-30` — `newSQLiteSource(busyTimeout time.Duration) Source`; `Open` calls `OpenReadOnlyBusy`, `Backup` passes the same budget to `sqlite.Backup`. `snapshot.go`'s one call site passes `s.busyTimeout`.
- [ ] Step 9: `internal/snapshot/sync_faults_test.go` (new) `Test_sync_refuses_when_the_backup_hits_a_busy_lock` — `fakeSource{backupErr: sqlite3.Error{Code: sqlite3.ErrBusy}}`, asserts R9 — proves `Sync`'s `Backup`-failure branch is wired to `sourceRefusal` independent of real lock timing.

### Sweep
- [ ] Step 10: `cmd/quarry/run.go:32-35` — wire `snapshot.WithHome(home)` into the `NewServer` call.
- [ ] Step 11: fix what `go build ./... && golangci-lint run ./...` reports; doc comments on `WithHome`, `WithBusyTimeout`, `DefaultBusyTimeout`, `OpenReadOnlyBusy`, `IsNotADB`, `IsBusy`, `sourceRefusal`, `runBackup`'s revised contract.

### Verify
- [ ] Step 12: full verification per `.claude/rules/agent-briefs.md` → *Verification* + `.claude/scripts/spec-check.py phase0-snapshot` → tick SCENARIO-06 (SCENARIO-07 folded, naming both acceptance tests) in `specification.md`.

## Handoff

**Binding decisions:**
- `Server.home` (`WithHome`) is the seam every refusal raised *inside* `Sync` uses for `~`-abbreviation — SCENARIO-08/13 (R10-R14) reuse it, no separate seam.
- `Server.busyTimeout` (`WithBusyTimeout`, default `DefaultBusyTimeout`) governs both `Source.Open` (via the DSN's `_busy_timeout`) and `Source.Backup` (via `runBackup`'s own retry loop — the DSN param does not reach `sqlite3_backup_step`). Not a CLI flag or config key in Phase 0.
- Classification lives in `internal/snapshot/refusal.go`'s `sourceRefusal`, keyed off `sqlite.IsNotADB`/`IsBusy`. SCENARIO-08 (R10-R12) classifies failures on the already-open **snapshot copy** (a different connection, opened via plain `OpenReadOnly`) — it must not reuse or extend `sourceRefusal`, which is Source-facing only.
- R9's fixture is rollback-journal mode with a real `BEGIN EXCLUSIVE` holder, never `v9fixture` — WAL doesn't block a `mode=ro` reader the way BR-3's fixture needs elsewhere.

**Left unbuilt:**
- Busy-timeout as a config key (Phase 1 obligation, spec `:49`) — `DefaultBusyTimeout` stays a constant.
- R10-R14 classification (SCENARIO-08, 13) and M1/W1 outcome policy (SCENARIO-09/10) — untouched by `sourceRefusal`, which only covers pre-backup Source failures.

**Traps:**
- Don't add `_busy_timeout` to the plain `OpenReadOnly` used for reading the just-created snapshot copy (`buildManifest`) — no writer ever contends for that file.
- `sqlite3.Error` is a value type; `errors.As(err, &sqlite3.Error{})` needs a local `var serr sqlite3.Error`, not a pointer target.
- `bk.Step`'s not-done/no-error return means BUSY or LOCKED, not "call it again immediately" — mattn's driver deliberately gives the caller no error to retry on; treating `err == nil` as success without checking `done` is how the pre-fix code spins forever.
- A `BEGIN EXCLUSIVE` transaction must be left **uncommitted** for the whole test — committing early releases the lock before the deadline fires.
