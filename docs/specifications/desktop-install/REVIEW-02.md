# Review Report — desktop-install, round 02 (re-gate)

### Target
Fix pass 1 only: `d24a598f..c4e46c58`. Coverage gate: 0 uncovered added lines. Mutation sample: 13 of 13 — 0 killed, 0 survived, 13 timed out (120s cap); 1829s; test-reviewer judged each by reading as killed by named tests.

### Triggered reviewers (narrow re-gate by concern)
- test-reviewer: blocked in round 01; tests changed
- correctness-reviewer: production logic changed (nil-executable guard, saveReplacing, readConfig signature, decodeServers)
- refactor-advisor: had findings the fix claimed to close

### Skipped reviewers
- arch-reviewer: no import, placement or wiring change
- api-reviewer, pipeline-reviewer: no matching files

### Closed from REVIEW-01
- [test-reviewer] MAJOR `exe` half of D9 temp check — pinned by `Test_install_refuses_a_temporary_build_that_a_path_link_outside_the_temporary_directory_points_to` (mutation reddens it).
- [correctness-reviewer] MINOR nil `s.executable` — now `*ExecutableError`; test-first.
- [refactor-advisor] doc comments, `saveReplacing`, `SymlinkError` construction, `decodeServers`, verb on TopLevel/Servers arms, `replacefile.write` doc — all closed.
- [test-reviewer] MINORs: `if` in test bodies/setup, `d13` names, duplicate help pins, inline `cli.Execute`, `rel != ".."` NIT — all closed.

### BLOCKER
none

### MAJOR
none

### MINOR
none

### NIT
- [refactor-advisor] `internal/claudedesktop/claudedesktop.go` — "Install and Uninstall" repeated on seven error-type docs; could be stated once in the package doc.
- [refactor-advisor][test-reviewer] `internal/cli/render_claude.go:352,355` — TopLevel/Servers arms now pass `verb`; unreachable for uninstall (equivalent mutant); copy says "cannot add quarry".
- [test-reviewer] `internal/cli/claude_install_desktop_path_test.go:74` — "a new entry" row now seeds `{}` instead of no config file; no production branch depends on it.
- [test-reviewer] nil-executable test asserts `require.Error(execErr.Err)` (sentinel unexported).

### Verdict: PASS WITH FOLLOW-UPS
