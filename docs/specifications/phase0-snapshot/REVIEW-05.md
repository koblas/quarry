# Review Report — round 05 (re-gate after final product-vision SHIP WITH CHANGES)

### Target
Range `84a2fdf..b63b48b`. Coverage gate: 0 uncovered, 1 declared unreachable.

### Triggered reviewers
correctness, test, arch (wiring seam).

### BLOCKER
1. **test** `cmd/quarry/run_test.go:378-391` — H1 test derives its expectation from `os.UserHomeDir()` (same call as production), so it cannot see that production prints `($HOME is not defined)` while the spec rules the fixed literal `($HOME is not set)`. **Orchestrator ruling:** H1 is a fixed literal — `homeDirectoryRefusal` must print exactly `cannot find your home directory ($HOME is not set); set HOME, then run quarry sync again` (no OS-string interpolation); test asserts the literal.

### MAJOR
1. **correctness** `internal/snapshot/destination_refusal.go` (`backupFailureRefusal`, `writeFaultRefusal`) — a real disk-full during backup surfaces as `sqlite3.Error{Code: SQLITE_FULL}` with no errno, so `errors.Is(ENOSPC)` misses and it gets R14c ("cannot copy…; run quarry sync again") instead of R14 ("free disk space"). Regression from this range. Fix: `sqlite.IsFull(err)` (mask like IsBusy/IsNotADB, `ErrFull`) in both disk-full cases; change the backup disk-full test to inject the real chain `fmt.Errorf("backup: %w", fmt.Errorf("backup to %s: %w", p, sqlite3.Error{Code: sqlite3.ErrFull}))` (keep PathError/ENOSPC as a second row).
2. **test** `destination_refusal.go:39,55` — EDQUOT arm untested at both sites. Add EDQUOT rows (PathError) asserting R14; state the mutant each kills.

### MINOR
- **correctness** `snapshot.go:152` — Encode-failure site returns R15 ("nothing was kept") without `Discard(snapshotPartial)` or `FailureOutcome`. Add both.
- **correctness** `cli/sync.go:95-104` — mismatch + stdout failure: O1 replaces M1, warnings skipped. Spec O1 doesn't rule it; record for product-vision recheck (STATE.md Open debts).
- **test** comment budgets: `run_test.go:95-97, 298-299, 375-377, 393-394`; `sync_faults_test.go:79-82, 104-106, 281-283`.
- **test** `Test_run_help_and_usage_errors_do_not_need_home` — restore Given/When/Then blank lines.
- **test** Collapse three `Test_sync_refuses_when_the_backup_fails_*` tests into one table (fits MAJOR 1/2 rows).
- **test** Rename subtest `"backup fails with a generic write fault"` → `"backup fails with an unclassified cause"` (sync_faults_test.go:717).
- **test** O1 test: also assert the `.json` manifest exists.
- **test** R15 schema test uses prefix/contains/suffix — keep unless exact text derivable.
- **arch** `ServerFactory` returns `home` separately from `Server`'s own — add `Server.Home()` and drop the second return value.

### Deferred (STATE.md Open debts)
- EDQUOT during SQLite backup arrives as SQLITE_IOERR_WRITE with errno dropped by the driver — reads as R14c; needs driver-level errno capture or a product ruling. Phase 1.

### NIT
- **arch** `homeDirectoryRefusal` copy/comment encode "sync" — accept.

### Verdict: FAIL
