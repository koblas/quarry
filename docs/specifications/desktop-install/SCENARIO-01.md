---
id: SCENARIO-01
status: open
---

# SCENARIO-01: Install reaches both Claude Code and Claude Desktop

Cadence: test-first — the `replacefile` temp-then-rename adapter (atomicity), plus two write-safety guards in `(*claudedesktop.Server).Install`: the interim present-config refusal and the empty-home refusal (D11)
Acceptance test: `internal/cli/claude_install_desktop_test.go` `Test_claude_install_adds_quarry_to_claude_desktop_after_claude_code`
Narrow loop: `go test ./internal/platform/replacefile/ ./internal/claudedesktop/ && go test ./internal/cli/ -run 'claude' && go test ./cmd/quarry/ -run 'claude|env'`
Mutation checks: temp removal on a failed replace → `Test_write_leaves_the_old_file_and_no_temp_when_the_rename_fails`; interim present-config refusal → `Test_install_refuses_a_config_path_that_already_holds_anything`; empty-home guard → `Test_install_refuses_an_empty_home_without_writing_under_the_working_directory`; delete `Executable: os.Executable` in `defaultEnv` → `Test_default_env_reports_this_binary_as_quarrys_path`; drop the `Home` override in `testEnv` → `Test_test_env_points_claude_at_no_real_desktop`
Runs: A (1-2) | B1 (3-4) | B2 (5-7) | V (8-9)
Size: OWNS A RUN — 5 batches, 1 feature package (`internal/claudedesktop`) + 1 platform helper (`internal/platform/replacefile`)

User-visible contract: `quarry claude install` with Code present and Code succeeding prints the Code lines, then the Code stderr hints, then Desktop. Desktop folder missing → S2 on stdout, exit 0. Folder present and no config → DA + DQ on stdout, exit 0, config created 0600. `Env.Home == ""` → D11 on stderr, exit 1. Every other Desktop error goes to the unclassified `runtimeError` arm: `quarry: <err>` at process end, exit 1.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `internal/cli/claude_install_desktop_test.go` (new) `Test_claude_install_adds_quarry_to_claude_desktop_after_claude_code` — `cli.Execute`, `Home` = `t.TempDir()` holding `Library/Application Support/Claude`, `Executable` returns `/opt/homebrew/bin/quarry`, success `toolCalls`. Assert stdout is the 3 Code lines + DA + DQ, stderr empty, no error, config mode 0600, decoded content equals `{"mcpServers":{"quarry":{"command":…,"args":["mcp"]}}}`, and the folder listing is exactly `[claude_desktop_config.json]`
- [x] Step 2: signature-only stubs. New `internal/claudedesktop/doc.go`, `claudedesktop.go`: `Executable` func type, `Server`, `Option`, `WithHome`, `WithExecutable`, `NewServer`, `Result`, `(*Server).Install(ctx)`. Add `Executable claudedesktop.Executable` to `cli.Env` at `internal/cli/run.go:56-58`. Thread an `executable` param through `newClaudeCommand` at `claude.go:31,40` and `newClaudeInstallCommand` at `claude_install.go:13`, and update the call site at `root.go:45`

### Build
- [x] Step 3: `internal/platform/replacefile/{doc.go,replacefile.go,replacefile_test.go}` (new) `Write(path, data, perm)` — temp in the same folder → chmod perm → write → fsync → close → rename → `atomicfile.SyncDir` (`atomicfile.go:34`); remove the temp on any failure before rename. Test-first rows:
  - creates the file
  - replaces an existing file whole
  - `perm` 0644 gives 0644 (`CreateTemp` is already 0600, so a 0600-only pin proves nothing)
  - a non-empty folder at `path` makes the rename fail; the folder stays intact, no temp is left, and the error wraps the cause
  - temp create fails in a read-only folder
  - one fault row each for chmod, write, sync and close, through an unexported per-call temp-file seam (never a package-level var); each leaves the old file and no temp
- [x] Step 4: `internal/claudedesktop/claudedesktop.go` `(*Server).Install` in this order: empty home → `ErrNoHome`; `os.Stat` the folder (not-exist → skipped result carrying the folder path; any other error → wrapped); `Executable()` (error → wrapped); `os.Lstat` the config (not-exist → continue; **anything else, error or not, → refusal wrapping `fs.ErrExist` or the lstat error**); encode in BR-D8 form (2-space indent, no HTML escaping, trailing newline); `replacefile.Write(…, 0o600)`. The encode error may carry `// unreachable:` per BR-D17 (every value is a quarry-built string or string slice), with that as the reason. `install_test.go` (new, real files under `t.TempDir()`), test-first for both guards:
  - `Test_install_refuses_a_config_path_that_already_holds_anything`, one row each: empty file, JSON object, folder, dangling symlink, FIFO. Each row asserts `errors.Is`, the bytes or link unchanged, the folder listing unchanged, and that `Executable` was still called (the refusal comes after it). Control: the same folder with no config gets the file created
  - `Test_install_refuses_an_empty_home_without_writing_under_the_working_directory` uses `t.Chdir` into a folder holding a relative `Library/Application Support/Claude`, so deleting the guard creates a file there
  - folder missing → skipped result, `Executable` not called (recording fake), nothing created
  - `<>&` inside the exe path stays literal in the written bytes
  - fault rows: folder stat EACCES (parent mode 000); config lstat ENOTDIR (`Claude` is a regular file); `Executable` error; write EACCES (read-only `Claude` folder). Each writes nothing
- [x] Step 5: `internal/cli/claude_install.go:33-47`, after the Code hints on the success path only, runs `claudedesktop.NewServer(WithHome(home), WithExecutable(executable)).Install`. `internal/cli/render_claude.go:16-45` gets S2/DA/DQ constants, a renderer (paths via `claudePath` at `:155-160` + `%q`), and `reportDesktopFailure` (`ErrNoHome` → D11 via `writeClaudeLine` at `:168-171` + `ReportedError`; default → `&runtimeError{}`). Tests in `claude_install_desktop_test.go`:
  - S2 row
  - D11 row (`Home` ""): Code lines on stdout, D11 on stderr, `ReportedError`
  - default-arm row (config present): stdout is only the Code lines, `errors.Is(err, fs.ErrExist)`, not `ReportedError`, refusal text not pinned
  - BR-D1 order with one buffer as both Stdout and Stderr: Code lines, turned-off hint, then DA, DQ
  - a Desktop-line stdout write failure, using a new test-local writer that fails after its first write, returns the write error
- [x] Step 6: re-point the install Code-success pins. In `claude_install_test.go:103-112`, `runClaude`/`runClaudeFinding` move from `home ""` to a `t.TempDir()` home with no Desktop folder. Every install Code-success pin gains the S2 suffix: `:147,170,184,196,289,309,321,334,570,620,632`, plus any other that the narrow loop shows failing. `:292` gets `Home: t.TempDir()`. `:646,657,668` fail at the Code stdout write and are unaffected; uninstall pins are untouched
- [x] Step 7: `cmd/quarry/run.go:191-209` `defaultEnv` gains `Executable: os.Executable`. `cmd/quarry/main_test.go:32-40` `testEnv` sets `Home` to a fixed absolute path that cannot exist (under `/`) and `Executable` to a fixed string outside `$TMPDIR`. Append to `cmd/quarry/run_claude_wiring_test.go`:
  - `Test_default_env_reports_this_binary_as_quarrys_path`: field non-nil and equal to `os.Executable()`
  - `Test_test_env_points_claude_at_no_real_desktop`: `Home` ≠ `os.UserHomeDir()`, its Desktop folder does not exist, and `Executable` returns the fixture

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; `go doc` comments on every new exported symbol in `claudedesktop` and `replacefile`; the `newClaudeInstallCommand` doc names the Desktop step

### Verify
- [ ] Step 9: `.claude/scripts/verify.sh <start> ./internal/platform/replacefile/... ./internal/claudedesktop/... ./internal/cli/... ./cmd/quarry/...`, then `spec-check.py desktop-install`, then tick SCENARIO-01 with its acceptance test, then write STATE.md

## Handoff

**Binding decisions:**
- Desktop runs **only after a Code success**, after the Code hints. A Code failure and `ErrClaudeNotFound` keep their early return. SCENARIO-11 owns continue-on-failure and SCENARIO-09 owns S1; continuing early breaks the `Empty(stdout)` wiring pins that 11 re-points.
- `Install` order is home → folder stat → `Executable()` → config Lstat → write. D9 (08) must run before the config is read, so `Executable()` already sits ahead of the Lstat.
- The config path is probed with `os.Lstat`, never `Stat`. A dangling symlink looks missing to `Stat`, and the rename would then replace the link (BR-D9).
- `replacefile` is generic: it never Lstats or refuses symlinks. That guard belongs to `claudedesktop`. `perm` is applied by explicit chmod, so 02 can pass the original mode.
- `claudedesktop` never reads `os.TempDir()`. 08 adds `WithTempDir` as the temp root.
- Departure from BR-D15's literal "t.TempDir()-based": `testEnv` takes no `*testing.T` and has about 28 callers, so it uses a fixed, absent, absolute `Home`. This takes the spec's "fake Desktop locator" option. The only `runProcess` tests that reach `claude install` (`run_claude_wiring_test.go:15,35`) already set `HOME` to `t.TempDir()`.
- Departure from claudeplugin precedent: Desktop tests live in a new file, `claude_install_desktop_test.go`. `claude_install_test.go` is already over 700 lines.
- As in `claudeplugin`, `NewServer` has no defaults: a nil `Executable` panics once the folder exists. Every cli test that creates the folder sets `Env.Executable`.

**Left unbuilt:**
- Interim refusal of any present config path (unruled text, goes to the `runtimeError` arm). SCENARIO-02 narrows it to the non-regular shapes (symlink, folder, FIFO) and merges regular files. SCENARIO-06 turns those into D5s/D5o.
- Merge, backup, original-mode keep, and D7/D8 copy → 02. "Ours", DU/DK and D4 → 04. Executable path choice, W1, D9 (`WithTempDir`) and D10 → 08.
- Unclassified in 01, landing in the `runtimeError` arm: folder stat error (D12, 09); `Claude` is a file (S2b, 09); `Executable()` error (D10, 08).
- Uninstall's Desktop step → 13. The dead not-found copy (`render_claude.go:28,42,50,199-202`) stays until 09.

**Traps:**
- If 02 deletes the present-config guard wholesale, a symlinked config becomes a merge target until 06 lands. Narrow it; do not remove it.
- `failingWriter` (`fakes_test.go:96`) fails the first Code write, so it can never reach the Desktop line. Use the new fails-after-first writer.
- A cli test home like `/home/ada` reaching Code success would stat a real path. Use `t.TempDir()` homes for success paths.

## Phase report

Run B2 (done): Steps 5-7 green, ticked. `golangci-lint run ./...` `0 issues`; narrow loops green on replacefile, claudedesktop, cli, cmd/quarry. The acceptance test `Test_claude_install_adds_quarry_to_claude_desktop_after_claude_code` is green.
- `internal/cli/claude_install.go`: Desktop step after the Code hints on the success path only (`installDesktop`); `render_claude.go`: `renderDesktopInstalled` (S2 / DA+DQ, paths through `claudePath` + `%q`), `reportDesktopFailure` (`ErrNoHome` -> D11 + `ReportedError`; default -> `&runtimeError{}`), `desktopQuitLine`, `desktopNoHome` consts.
- `internal/cli/claude_install_desktop_test.go`: S2, D11, default arm (config present), single-buffer order, fails-after-first-write rows (`failsAfterFirstWrite`, `installDesktop` helper).
- `internal/cli/claude_install_test.go`: `runClaudeFinding` now passes `t.TempDir()` home; `desktopSkippedLine` const; 10 Code-success pins gained the S2 suffix (the plan listed 11 lines; the narrow loop showed exactly these 10 failing); the `runs_both_steps` Execute test got `Home: t.TempDir()`.
- `cmd/quarry/run.go`: `defaultEnv` gains `Executable: os.Executable`. `main_test.go`: `testEnv` sets `Home = testHome` (absent absolute path) and `Executable` fixture; both are consts. Two tests appended to `run_claude_wiring_test.go`.
- Mutations (all red, restored): delete `Executable: os.Executable` -> `Test_default_env_reports_this_binary_as_quarrys_path` (`Expected value not to be nil`); drop the `Home` override -> `Test_test_env_points_claude_at_no_real_desktop` (home equals real `/Users/koblas`, and the real Desktop folder exists so the not-exist assertion also fails); `ErrNoHome` arm off -> `Test_claude_install_reports_a_missing_home_after_the_claude_code_lines`; `installDesktop` call removed -> the acceptance test.
- Not done (V): Step 8 doc comments check, Step 9 `verify.sh`, spec tick, `spec-check.py`, STATE.md, `status: done`.
- Must not redo: the interim present-config refusal stays; `NewServer` has no defaults (nil `Executable` panics once the folder exists).
