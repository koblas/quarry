# Review Report — round 04 (re-gate)

### Target
Fix range `0b48f4a..c7f781d` (fix pass 3 for REVIEW-03). Test-only diff; coverage gate: no production lines added.

### Triggered reviewers
correctness (owner of the round-3 MAJOR).

### Round-3 status
MAJOR closed: `Test_signalContext_kills_the_process_on_a_second_sigterm` reddens when the stop-on-Done goroutine is deleted (orchestrator mutation M1: "child did not exit after repeated SIGTERM"); `-race -count=10` stable (30 PASS); no leftover child processes. REVIEW-03 test MINORs (comment budgets) closed.

### BLOCKER / MAJOR
None.

### NIT
- `signal_test.go:64` — on failure paths the killed child is not reaped until the test binary exits. Optional: start `cmd.Wait()` goroutine right after `Start`, drain in cleanup.
- `signal_test.go:54` — env guard alone could be tripped by an exported `QUARRY_SIGNALCONTEXT_SUBPROCESS=1`. Optional sentinel arg.

### Verdict: PASS WITH FOLLOW-UPS
