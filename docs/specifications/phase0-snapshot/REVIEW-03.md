# Review Report — round 03 (re-gate)

### Target
Fix range `dabd08d..0d5c6f7` (fix pass 2 for REVIEW-02). Coverage gate: 0 uncovered, 1 declared unreachable (main.go:10).

### Triggered reviewers
correctness, test (the concerns fix pass 2 touched).

### Round-2 status
First-signal leg of the signal MAJOR closed (tests send real SIGINT/SIGTERM). Dir fsync, FailureOutcome export, doc budgets closed.

### MAJOR
1. **correctness** `cmd/quarry/run.go:23-26` — stop-on-Done goroutine untested; deleting it leaves the suite green (mutation M1). Without it every Ctrl-C after the first is swallowed during the uninterruptible backup copy — the accepted Step(-1) debt assumes the second signal kills. Fix: subprocess test in `cmd/quarry/signal_test.go`: re-exec `os.Args[0]` with `-test.run=^<name>$` + env guard; child calls `signalContext`, prints "ready", waits `ctx.Done()`, prints "done", sleeps; parent waits "ready", sends SIGTERM, waits "done", re-sends SIGTERM in a bounded loop, asserts child exited `Signaled()`. Use SIGTERM (not SIGINT, which may be inherited as ignored). Confirm M1 reddens it. (Same finding from test-reviewer as MINOR comment-overclaim — closed by the test.)

### MINOR
- **test** `sqlite_test.go:296-297` in-body comment 2 lines (budget 1).
- **test** `atomicfile_test.go:79-81` comment 3 lines (budget 2).
- **test** `sync_faults_test.go:673-675` comment on `cancelAndFailWriteManifestDestination` 3 lines (budget 1-2).

### NIT
- **test** two signal tests could be a table.

### Verdict: FAIL
