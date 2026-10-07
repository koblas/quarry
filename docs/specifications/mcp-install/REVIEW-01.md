# Review Report — round 01

### Target
Changed files, `979e648e..9b8f039c` (first gate; tree clean). Pre-gate: `uncovered-diff.py` found one uncovered added line (`state.go:51` `ExitError.Error` status branch); pinned in 9b8f039c before the round.

### Triggered reviewers
- arch-reviewer: `cmd/**/*.go`, `internal/**/*.go`
- correctness-reviewer: same
- test-reviewer: `**/*_test.go`
- refactor-advisor: `cmd/**/*.go`, `internal/**/*.go`

### Skipped reviewers
- api-reviewer: no `internal/http*` files
- pipeline-reviewer: no `.claude/**` files

### Gate inputs
- uncovered-diff: 0 uncovered added lines; 1 declared unreachable (`toolrun.go:77`), claim judged to hold by correctness and test reviewers.
- test-stats: cmd/quarry 1022 (+3) | claudeplugin 38 (+38) | cli 672 (+51) | toolrun 12 (+12) | TOTAL 1744 (+104).
- mutation-sample: 20 of 75 — 7 killed, 0 survived, 13 timed out (120s budget vs ~150s cmd/quarry suite), 2249s. test-reviewer judged every timed-out mutant killed by a fast-package test by reading; none a gap.

### BLOCKER
none

### MAJOR
- **correctness-reviewer** `internal/claudeplugin/state.go:396-398` — `list` decodes JSON from the Runner's combined stdout+stderr buffer. Failure: `claude ... --json` prints `[]` on stdout plus one stderr line (node deprecation warning, update notice) and exits 0 → `json.Unmarshal` fails → every `install`/`uninstall` refuses with the R4 "update Claude Code" line, which is false advice, and replays nothing so the cause is hidden. Fix (orchestrator ruling): Runner returns stdout and combined separately — `Runner func(ctx, name, args...) (stdout, combined []byte, exitCode int, err error)`; `toolrun.run` captures stdout via `io.MultiWriter(&stdoutBuf, &combined)` and stderr into `combined` only; `list` decodes `stdout`; `ExitError.Output` and every replay stay on `combined`. Spec BR-4 and `### Seams` amended accordingly. (Observed: `claude` 2.1.285 writes 0 bytes to stderr on a successful list today; failure remains constructible.)
- **test-reviewer** `internal/platform/toolrun/toolrun_test.go:106` — `toolrun.Run`'s returned output is unpinned on the non-zero-exit, signal and cancelled returns (`_, got, err :=`). Mutating those returns to `nil` output leaves every test green; `ExitError.Output` and the "see its message above" replay would be empty in production. Fix (pin-only): helper mode writing stderr then stdout then exiting N; assert output + status through `Run`; self-kill helper prints before killing itself, assert output on the `*SignalError` path. Mutation: drop `output.Bytes()` from the non-zero-exit return.

### MINOR
- **correctness-reviewer** `toolrun.go:61`, `claudeplugin.go:430` — terminal Ctrl-C reaches the child first; if it dies/exits 130 before `NotifyContext` cancels ctx, user sees "stopped by signal interrupt" / "exited with status 130" instead of R5. Exit 1 either way; no wrong result. Optional: treat SIGINT/SIGTERM or 130/143 as interrupt.
- **correctness-reviewer** `toolrun.go:46` — `CommandContext` kills the direct child only; grandchildren (e.g. a `git clone`) survive on SIGTERM to quarry. Not constructible as a wrong result. Optional: `Setpgid` + kill `-pgid`.
- **test-reviewer** `claude_install_test.go:51` — `toolCalls` doc 3 lines (budget 2).
- **test-reviewer** `claude_install_test.go` (732 lines) — mixes shared fake/runners, install tests, failure tables; move helpers to `claude_fake_test.go`, failure/interrupt tests to `claude_install_failure_test.go`.
- **test-reviewer** `claude_install_test.go:347,359,494,506,516,609` — standalone tests duplicate table rows; fold distinct cells into the tables, delete the rest.
- **test-reviewer** `claude_install_test.go:709`, `claude_uninstall_test.go:416` — help tests have two Whens (child `--help` + group `--help`); split, or keep group pins only in `claude_test.go`.
- **refactor-advisor** `state.go:~78-122` — `readState` two phases + foreign check in one body (Compose method): extract `classifyMarketplaces`, `classifyPlugins`.
- **refactor-advisor** `claude.go:12-24`, `claude_install.go:~38`, `claude_uninstall.go:~34` — `runTool, lookPath, home, jsonOut` clump; build `Server` once in `newClaudeCommand`, pass `srv, home, jsonOut`.
- **refactor-advisor** `claude_install.go` / `claude_uninstall.go` — `Args` closure and `--json` guard duplicated verbatim; `noArgs(verb)`, `refuseJSON(verb, jsonOut)`.
- **refactor-advisor** `render_claude.go:~100-125,150-165` — `installDoneLead`/`renderInstallDone` derive the same fact twice; pass one `claudeProgress{line, lead}` into `reportClaudeFailure`.

### NIT
- **test-reviewer** — no test composes real `toolrun.Run` with claudeplugin parsing; a `#!/bin/sh` fake `claude` printing a stderr warning then JSON for both lists would catch the MAJOR class. (Fold into the MAJOR fix as its end-to-end pin.)
- **refactor-advisor** `render_claude.go` `reportClaudeFailure` — six `if errors.As/Is` blocks each returning `ReportedError{}`; a `switch` with one return.
- **refactor-advisor** `claudeplugin.go` Install/Uninstall docs — identical Errors sentence twice; state once on `Server`.
- **refactor-advisor** `state.go` `list` — `&ListUnreadableError{}` allocated eagerly on every call.
- **refactor-advisor** `toolrun.go` `run` — no comment explaining the injectable `delay` parameter.

### Strengths
- `runClaudeAt` marker pin on the command ctx + fakes answering from the parameter ctx make propagation unfakeable.
- Identity-based deletion guards (name+source+repo; id+scope), both orderings pinned.
- Drift test derives expected values from manifests and `go.mod` instead of constants.

### Verdict: BLOCKED
- arch-reviewer: PASS · correctness-reviewer: BLOCKED (1 MAJOR) · test-reviewer: BLOCKED (1 MAJOR) · refactor-advisor: PASS WITH FOLLOW-UPS.
