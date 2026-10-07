# Review Report — round 02 (re-gate after fix pass 1)

### Target
Fix range `d8a9a171..964f5ddd` (tree clean). Re-gate narrowed by concern: correctness (production logic changed), test (tests changed). arch and refactor not re-run (imports unchanged; refactor findings deferred to STATE).

### Triggered reviewers
- correctness-reviewer: fix diff
- test-reviewer: fix diff

### Gate inputs
- uncovered-diff since d8a9a171: 0 uncovered; 1 declared unreachable (`toolrun.go:100`).
- test-stats: cmd/quarry 1023 (+1) | claudeplugin 41 (+2) | cli 672 (+0) | toolrun 16 (+4) | TOTAL 1752 (+7).
- correctness ran `go test -race -count=5` on toolrun + claudeplugin: green.

### Prior findings
- REVIEW-01 correctness MAJOR (lists decoded from combined): **CLOSED** — `run` returns stdout on success, `ExitError.Output` carries combined, `list` decodes stdout; wiring test with `#!/bin/sh` fake claude proves it end-to-end.
- REVIEW-01 test MAJOR (toolrun output unpinned): **CLOSED for the non-zero-exit return only** — see MAJOR below.

### BLOCKER
none

### MAJOR
- **test-reviewer** `internal/platform/toolrun/toolrun_test.go:268,241` (helpers `helperSelfKill` ~L72, `helperHang` ~L64) — signal and cancel pins write only stdout, so they cannot tell `combined` from `stdout`. Mutating `toolrun.go:87` (cancel) and `:95` (signal) to `return stdout.Bytes(), stdout.Bytes(), …` leaves the package green; stderr written just before a kill would be lost from the replay. Fix (pin-only): `helperSelfKill` and `helperHang` write a stderr line, sleep `interleaveGap`, then the stdout line (for `helperHang`, before the ready file); assert `combined == "err\nout\n"` in `Test_run_returns_the_output_of_a_command_ended_by_a_signal` and `..._cancelled_mid_run`. Mutation: `combined` → `stdout.Bytes()` at `:87` and `:95` must redden both.

### MINOR
- **test-reviewer** `toolrun_test.go:241,268` — stdout return unpinned on signal/cancel paths (same edit as the MAJOR closes it).
- **test-reviewer** `internal/cli/claude_install_test.go:47-48` — `toolReply.stdout` and its `toolCalls.run` branch are dead (no cli test sets `stdout:`); delete, or note the cli layer never pins stdout ≠ combined.
- **correctness-reviewer** `toolrun.go:71-72` — combined replay is in pipe-read order, not child write order; no decision depends on order, doc states it. Accepted (STATE Traps).

### NIT
- **test-reviewer** `toolrun_test.go:120` — `Test_run_returns_stdout_without_stderr` reuses `helperInterleave` and pays two needless 50ms sleeps.
- **test-reviewer / correctness-reviewer** `toolrun_test.go:113` — `Test_run_keeps_stdout_and_stderr_in_arrival_order` asserts an order production no longer promises; passes on the 50ms gap (5/5 under `-race`). Rename to say "read order with a gap", or accept.

### Verdict: BLOCKED
- correctness-reviewer: PASS WITH FOLLOW-UPS · test-reviewer: BLOCKED (1 MAJOR, pin-only).
