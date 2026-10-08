# Review Report — desktop-install, round 01

### Target
Changed files, range `0b7108ab0c69..6605279b` (merge-base with origin/main to HEAD at gate start).

Coverage gate: 0 uncovered added lines; 4 declared unreachable (all upheld by correctness-reviewer and test-reviewer).
`spec-check.py --run desktop-install`: OK.
Mutation sample: 20 of 114 — 2 killed, 0 survived, 18 timed out (120s cap vs ~150s `cmd/quarry` suite); 2589s. test-reviewer re-ran 29 mutants on an export: 28 killed, 1 survived (M1 below).

### Triggered reviewers
- arch-reviewer, correctness-reviewer, refactor-advisor: `cmd/**/*.go`, `internal/**/*.go`
- test-reviewer: `**/*_test.go`

### Skipped reviewers
- api-reviewer: no HTTP surface
- pipeline-reviewer: no `.claude/**` change

### BLOCKER
none

### MAJOR
- [test-reviewer] `internal/claudedesktop/path_test.go` (missing test for `claudedesktop.go:263`) — the `exe` half of the D9 temp check is unpinned. Failure: mutant `[]string{exe, command}` → `[]string{command}` survives the whole package; real case `go build -o /tmp/x/quarry` with `~/bin/quarry` symlinked to it on PATH → install writes an entry that dies when temp is cleaned. Fix (pin-only): running binary under the `WithTempDir` root, PATH symlink to it in another `t.TempDir()`; assert `*TempBuildError{Path: built}` and empty folder snapshot. Mutation: delete `exe` from the slice.

### MINOR
- [correctness-reviewer] `internal/claudedesktop/claudedesktop.go:258` — `s.executable()` called with no nil check while `choosePath` checks nil `lookPath`; a `cli.Env` without `Executable` panics once the Desktop folder exists. No production path (only setter is `defaultEnv`, pinned). Fix: return `&ExecutableError{...}` on nil, like `choosePath`.
- [refactor-advisor] `claudedesktop.go:29-134` — doc comments say "returned by Install" on error types Uninstall also returns (ErrNoHome, ForeignEntryError, InvalidJSONError, BackupError, WriteError, SymlinkError, ReadError); `SymlinkError.Command` is always empty from Uninstall. Fix: "Install and Uninstall"; "empty from Uninstall".
- [refactor-advisor] `claudedesktop.go:248-301, 343-394` — Install and Uninstall duplicate the backup-then-write tail (and its comment). Fix: extract `saveReplacing(ctx, config, current, doc) error` (stopped → backup → write).
- [refactor-advisor] `claudedesktop.go:271-277` — Install patches `SymlinkError.Command` after `readConfig` via `errors.AsType` + mutation. Fix: build the error where the command is known, or drop the field.
- [refactor-advisor] `claudedesktop.go:492-528, 369` — servers decode open-coded in `merge` and `Uninstall`. Fix: extract `decodeServers(path, top)`.
- [refactor-advisor] `claudedesktop.go:231-241, 330-337` — `Skipped` + `NotAFolder` booleans allow an invalid combination. Fix: one skip value, or keep the fields adjacent.
- [refactor-advisor] `internal/cli/claude.go:32`, `claude_install.go`, `run.go:56` — five positional params threaded through constructors (data clump; existing open NIT from mcp-install). Fix later: pass a small env struct.
- [refactor-advisor] `internal/cli/render_claude.go:351-356` — TopLevelError/ServersError arms pass constant `installCommand` where neighbours pass `verb`. Fix: pass `verb`.
- [arch-reviewer] `claudedesktop.go:213` — `NewServer` defaults `tempDir` to `os.TempDir()` (feature package reads env). Documented; no failure.
- [arch-reviewer] `claudedesktop.go:318,447,460` — direct `os.Stat/Lstat/ReadFile`, not a port. Matches `snapshot`/`config` idiom.
- [test-reviewer] `internal/cli/claude_install_desktop_path_test.go:90` — `if c.config != ""` in a test body; same smell as `if err := os.WriteFile` in setup closures at `internal/claudedesktop/install_test.go:294`, `uninstall_test.go:218`.
- [test-reviewer] `internal/cli/claude_continue_test.go:94,108` — test names carry a spec id (`d13`). Rename to plain language.
- [test-reviewer] duplicate help pins: `mcp --help` in `mcp_test.go:34` and `claude_help_test.go:94`; `claude --help` in `claude_test.go:64` and `claude_help_test.go:91`. Drop the duplicate rows.
- [test-reviewer] table shape: `claude_absent_target_test.go:84` (name with "and", five behaviours); `claude_install_desktop_test.go:338,505` per-case seed/verify closures.
- [test-reviewer] file size: `internal/claudedesktop/install_test.go` 906, `internal/cli/claude_install_desktop_test.go` 810, `claude_install_test.go` 743 lines.
- [test-reviewer] `internal/cli/claude_install_desktop_test.go:114,150,249` — inline `cli.Execute(... cli.Env{...})` instead of the existing helpers.
- [test-reviewer] `README.md` — Desktop paragraph omits that uninstall also backs up and also needs Desktop quit. Omission, pinned byte-equal; needs a copy ruling to change.

### NIT
- [arch-reviewer] `internal/cli/claude_install.go:80` — duplicate `LookPath` func types across features (deliberate decoupling).
- [arch-reviewer] handlers build `claudedesktop.NewServer` inline (compliant).
- [refactor-advisor] `claudedesktop.go:422-433` — `isTemporary` treats any `go-build*` element as temporary (`~/tools/go-buildkit/quarry` refused).
- [refactor-advisor] `internal/platform/replacefile/replacefile.go:41` — unexported `write` has no doc line.
- [refactor-advisor] `render_claude.go:20-101` — ~70 copy consts interleaved; optional `render_claude_desktop.go`.
- [test-reviewer] `isTemporary` `rel != ".."` clause unpinned (M2; practically equivalent).
- [test-reviewer] `Test_test_env_points_claude_at_no_real_desktop` asserts the fixture against its own constants.

### Strengths
- Fault tests inject real shapes: one-call-at-a-time temp failures, `Chflags`, unsearchable folders, FIFOs.
- Destructive guards covered across case, extension, trailing slash, symlink and hard-link aliases for both verbs.
- Cancellation tests carry control arms (live ctx writes; unchanged/foreign return before the interrupt check).
- All Desktop copy in `render_claude.go`; `claudedesktop` returns typed errors; dependency rule clean.

### Verdict: BLOCKED
One MAJOR (pin-only): the `exe` half of the D9 temp check.
