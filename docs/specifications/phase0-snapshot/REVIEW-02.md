# Review Report — round 02 (re-gate)

### Target
Fix range `63b621f..dc0f776` (fix pass 1 for REVIEW-01). Coverage gate: 0 uncovered, 4 declared unreachable.

### Triggered reviewers
correctness, test, refactor-advisor, arch (each on the concern the fix touched).

### Round-1 status
All four REVIEW-01 MAJORs closed in behaviour (correctness verified empirically: real SIGINT → I1, exit 1, no files; 600 MB closed-WAL fixture → R8b, bundle unchanged). Round-1 MINORs closed except those listed below.

### BLOCKER
None.

### MAJOR
1. **correctness** `cmd/quarry/main.go:11-19` — `signal.NotifyContext` + stop-on-Done wiring is untested and hidden behind the entrypoint `// unreachable` marker. Mutation `run(context.Background(), ...)` leaves the suite green. Fix: extract `signalContext(parent) (context.Context, context.CancelFunc)` into `run.go`; test in-process with `syscall.Kill(os.Getpid(), SIGINT)` and SIGTERM asserting `ctx.Done()` closes (and that after Done a second signal is no longer captured, if testable); `main` becomes one line so its marker is accurate.

### MINOR
- **correctness** `sqlite.go:306` `bk.Step(-1)` uninterruptible — ctx honoured only after the whole copy (~2 s / 600 MB); second Ctrl-C in that window leaves a partial until the 1-hour sweep. Orchestrator ruling: accept bounded latency; record in STATE.md Open debts (chunked step needs restart guard — Phase 1).
- **correctness** `atomicfile.go:25` no directory fsync after `os.Link`. Fix if cheap: open dir, `Sync()`.
- **test** `snapshot.go:216,225` still carry `unreachable:` prefix while describing a reachable ctx-cancel path. Drop the prefix (plain why-comment) or mark consistently; no seam to test (read-port debt).
- **test** `sqlite_test.go:296-299` 4-line in-body comment (budget 1).
- **test** `sync_faults_test.go:615-618, 657-659`; `run_test.go:404-406` comments over 2-line budget.
- **test** File size: `run_test.go` 705 lines, `sync_faults_test.go` 884 — record in STATE.md Open debts (split before next feature adds to them).
- **refactor** Doc budgets on new symbols: `commit` (snapshot.go:170-174), `errNotOpenInQuicken`/`walFormatByte` (bundle.go:19-27), `primaryErrNo` (sqlite.go:54-57), `sourceRefusal` (refusal.go:19-23), `causeText` (destination_refusal.go:11-14).
- **arch** `cmd/quarry/run.go:28-30` duplicates `failureOutcome`'s ctx rule; export one decision point (e.g. `snapshot.FailureOutcome`) and call it.

### NIT
- **arch** `InterruptedRefusal` doc names its cmd caller — drop.
- **correctness** `bundle.go:184` `n, _ := io.ReadFull` — one-line why-comment.
- **test** `closeWALFormattedDatabase` duplicates `v9fixture.ClosedWALBundle` — leave.

### Verdict: FAIL
