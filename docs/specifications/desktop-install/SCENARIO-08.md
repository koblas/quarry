---
id: SCENARIO-08
status: open
---

# SCENARIO-08: Install picks the quarry path Claude Desktop will start

Cadence: code-first (no mandatory item: D9 refuses a path that will vanish, it guards no user file)
Acceptance test: `internal/claudedesktop/path_test.go` `Test_install_writes_the_quarry_path_that_claude_desktop_will_start`
Narrow loop: `go test ./internal/claudedesktop/ ./internal/cli/ -run 'Install|Path|Desktop|Temp'`
Mutation checks: `underTemp` root-boundary compare (string prefix, no separator) → sibling-prefix row `Test_install_refuses_a_temporary_build_...`; `os.SameFile` arm → acceptance Cellar and different-file rows; W1 gated on PATH stat → `..._warns_only_when_the_path_quarry_can_be_checked`; D9 moved after `readConfig` → D9-over-invalid-JSON row; delete `NewServer` temp default → cli `$TMPDIR` D9 row; delete `WithLookPath` line in `installDesktop` → cli W1 rows
Runs: A (1-2) | B1 (3-4) | B2 (5) | V (6-7)
Size: OWNS A RUN — 3 batches (spec sizing said 2: D9/D10 split from path choice), 1 feature package (`claudedesktop`; cli + cmd wiring do not count)

Surface surveyed (nothing to port): production calls on the old path are `s.executable()` (`claudedesktop.go:195`) and `merge(..., command)` (`:206`, `Unchanged` is string equality on it); new calls are `os.Stat` x2 (follows links, never `Lstat`), `os.SameFile`, `filepath.IsAbs`. cli passes `lookPath` already (`claude_install.go:14`); `cmd/quarry/run.go:204` already supplies `exec.LookPath`, so **no cmd/quarry edit**.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `internal/claudedesktop/path_test.go` (new) `Test_install_writes_the_quarry_path_that_claude_desktop_will_start` — four Gherkin rows over REAL files: Cellar file + symlink in PATH dir (`Result.Command` is the symlink, no `PathQuarry`); two distinct files (exe written, `PathQuarry` = PATH file); LookPath error (exe, no `PathQuarry`); exe under the `WithTempDir` root (`*TempBuildError`, folder empty). Non-temp rows use `WithTempDir(t.TempDir())` of a sibling dir; executables live in another `t.TempDir()`
- [x] Step 2: `claudedesktop.go:125-175` `LookPath` type, `WithLookPath`, `WithTempDir`, `TempBuildError{Path}`, `Result.PathQuarry` — signature-only stubs; test must fail at its assertion

### Build
- [x] Step 3: `claudedesktop.go:182-226` `Install` path choice, then `install_test.go`-style unit rows in `path_test.go`. Flow: `LookPath("quarry")` err/nil-func/non-absolute → exe, no W1; else `os.Stat` PATH file (fails → exe, no W1, BR-D18), `os.Stat` exe (fails → not same), `os.SameFile` → PATH path verbatim, else exe + `PathQuarry`. Chosen path feeds `merge`, `Unchanged`, `SymlinkError.Command` (`:200-202`). Rows: hard link (same file, no symlink); dangling PATH link; exe stat fails + PATH ok (W1, exe written); both fail (none); relative LookPath result; nil LookPath; Unchanged/Updated/Added each carry `PathQuarry`; Skipped folder never calls LookPath or Executable (extend `install_test.go:157-169`); symlinked config's `Command` is the chosen PATH path
- [x] Step 4: `claudedesktop.go:148-155,195-198` `NewServer` default `os.TempDir()` (the only default; nothing else reads it), `underTemp`, `*TempBuildError`, `*ExecutableError{Config, Err}` (`Unwrap`) replacing `fmt.Errorf("find the quarry binary")`. Order: folder stat -> `Executable()` (D10) -> choose -> D9 -> `readConfig`. D9 judges `Executable()` and the chosen path, naming whichever tripped. Rows: under root; `root+"x"` sibling and root-itself; trailing-slash/unclean root; empty root disables the root check; `go-build` element mid-path, `go-build-1` prefix, leaf `go-build`, `ago-build` (not); chosen PATH path under root with installed exe; D9 over invalid-JSON, symlink and unreadable config (config untouched, no backup); `ExecutableError` keeps `ErrorIs` on the injected error (re-point `install_test.go:668-676`)
- [ ] Step 5: cli — `claude_install.go:48,54-61` `installDesktop(cmd, home, executable, lookPath)` adds `WithLookPath(claudedesktop.LookPath(lookPath))` (nil converts to nil) and, after `writeResult`, W1 via `writeClaudeLine` when `res.PathQuarry != ""`; `render_claude.go:36-39` W1 const; D9/D10 consts + new `desktopBinaryFailureLine(home, err)` called from `desktopFailureLine` (`:210-220`; the existing helpers are at complexity limit), `claudePath` + `%q` on every path, `desktopEntryJSON("<the path command -v quarry prints>")` for D10. Copy verbatim from spec `## Surface & Copy` D9, D10, W1. Tests (fixed strings except as noted) in `claude_install_desktop_test.go`: D9 `$TMPDIR` row (`t.Setenv("TMPDIR", "/quarry-test-temp")`, exe `/quarry-test-temp/quarry`; the NewServer-default pin) and `go-build` row; D10 row (injected error, `desktopConfigShown`, config untouched); W1 for each of DA, DU, DK (PATH quarry = REAL file under `home/bin`, printed `~/bin/quarry`, exe fixed `/opt/homebrew/bin/quarry`); no W1 when PATH quarry does not stat (`findsAllBut` path), when LookPath errors, when folder missing, when config refused (D5s) — each asserts stderr empty/exact; exit 0 for W1, 1 + `ReportedError` for D9/D10. Json mode n/a: `--json` is refused before Desktop. Re-point the two flipping tests below

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; update `Install`/`Result`/package doc (`doc.go`), no spec ids in comments

### Verify
- [ ] Step 7: `.claude/scripts/verify.sh <start> ./internal/claudedesktop/... ./internal/cli/...` + `spec-check.py desktop-install`; tick SCENARIO-08 with its acceptance test; rewrite `STATE.md`

## Test inventory flipped by D9 (re-point in Step 5, not before)
- `claude_install_desktop_test.go:144-169` `..._prints_a_binary_under_home_as_tilde_...` and `:766-777` `..._absolute_path_in_the_symbolic_link_refusal_for_a_binary_under_home`: exe is `home/bin/quarry`, home is `t.TempDir()` (under `$TMPDIR`) -> D9. cli has no temp-root seam (an `Env.TempDir` would exceed the spec seam list), so: after `desktopHome(t)` has created the home, call a new helper that does `t.Setenv("TMPDIR", "/quarry-test-temp-that-does-not-exist")` (absolute and absent, precedent `testHome`). Order matters: the first `t.TempDir()` creates its base under the then-current `$TMPDIR`. No `t.Parallel` in `internal/cli`, so `t.Setenv` is legal.
- Not flipping (fixed exe outside `$TMPDIR`): every other desktop cli row (`desktopQuarryBinary`), every `claudedesktop` test (`quarryBinary`, `/opt/a<b>&c/quarry`; `install_test.go:377-399` `bin := t.TempDir()` is a symlink target), `cmd/quarry` (`testQuarryBinary`; its claude tests fail at the Code step).
- W1 by surprise: none today. Desktop rows pass nil LookPath; `claude_install_test.go:139-146` `findsAllBut` resolves `quarry` to `/opt/bin/quarry`, which does not exist, and those homes have no Desktop folder. Keep any fake PATH-quarry that must not warn on a path that does not exist.

## Handoff

**Binding decisions:**
- Path choice is `os.Stat` (follows links) + `os.SameFile`; `Lstat` makes the Homebrew symlink look different and writes the Cellar path (BR-D5).
- `Result.PathQuarry` (non-empty only when PATH quarry is absolute, stat-able, and not the written file) is the sole W1 signal; cli never stats. `Result.Command` stays the written path for all outcomes.
- `claudedesktop.LookPath` is its own type; cli converts `claudeplugin.LookPath`. Nil LookPath = no PATH quarry. Empty `WithTempDir` root disables the root test; `NewServer` defaults it to `os.TempDir()`, so a `claudedesktop` test with a temp-dir exe must pass `WithTempDir`.
- Typed carriers for 13/09: `*TempBuildError{Path}` (D9, install-only), `*ExecutableError{Config, Err}` (D10, install-only). `desktopBinaryFailureLine` is where they render.
- D9 is lexical (`filepath.Rel` against the root; no `EvalSymlinks`), so macOS `/var` vs `/private/var` is not unified; the `go-build` element rule covers `go run`.
- D5s `<E>` (SymlinkError.Command) becomes the chosen path, not always `Executable()`; STATE decision 11 is amended.

**Left unbuilt:** W1 under uninstall (none: install-only); S1/S2 skips — 09; continue-on-Code-failure — 11; D4u/DR/DN — 13.

**Traps:** `desktopHome` before `t.Setenv("TMPDIR", ...)` or `t.TempDir` fails; a test that sets TMPDIR for a D9 row must also use a home that is not under it; D9 judged on the chosen path as well as `Executable()` is a deliberate widening of the spec's `<exe>` wording (BR-D5 says "chosen path"); orchestrator may veto.

## Phase report

Run B1 (steps 3-4) done; `internal/claudedesktop` green and lint-clean (`0 issues`); cli not touched.

- `internal/claudedesktop/claudedesktop.go`: `ExecutableError{Config, Err}` (`Error` = `find the quarry binary: <err>`, `Unwrap`); `Server.lookPath`/`tempDir` (`WithLookPath`, `WithTempDir` store; `NewServer` defaults `tempDir` to `os.TempDir()`); `Install` order folder stat -> `Executable()` (`*ExecutableError`) -> `choosePath` -> D9 loop over `{exe, command}` (`*TempBuildError{Path}` names whichever tripped, exe first) -> `readConfig`; `PathQuarry` set on Added/Updated/Unchanged only; `SymlinkError.Command` is the chosen path. `isTemporary`: any `go-build*` path element (leaf included), else lexical `filepath.Rel` against the cleaned root (root itself and `root+"x"` are not under; empty root skips only the root test).
- `internal/claudedesktop/path_test.go`: unit rows for steps 3-4 (helpers from run A reused); `install_test.go` ExecutableError row extended (type, `Config`, message).
- Mutations (each reverted, file diffed byte-identical): prefix `HasPrefix` compare -> reddens `..._refuses_a_temporary_build_and_names_it/the_root_is_not_clean` and `..._accepts_a_binary_outside_the_temporary_directory/{sibling prefix, the root itself}`; `SameFile && false` -> acceptance PATH-link row, hard-link, symlink-refusal, path-under-temp rows; `SameFile || true` -> acceptance differing-files row plus Updated/Unchanged/exe-unstattable rows; D9 moved after `merge` -> `..._before_reading_a_config_it_would_refuse/{not valid JSON, symbolic link}` and `..._cannot_read`. (D9 moved only after `readConfig` leaves the invalid-JSON row green; the invalid-JSON arm needs it after `merge`.)
- RED ON PURPOSE, for B2 step 5: `internal/cli` `Test_claude_install_prints_a_binary_under_home_as_tilde_and_writes_its_absolute_path` and `..._prints_the_absolute_path_in_the_symbolic_link_refusal_for_a_binary_under_home` now fail (D9: exe under `$TMPDIR` home, cli D9 copy not built). Re-point them per `## Test inventory flipped by D9`. Other cli tests green.
- Narrow loop `-run 'Install|Path|Desktop|Temp'` is case-sensitive and matches no `claudedesktop` test (they are `Test_install_...`); run `go test ./internal/claudedesktop/` whole instead.
- Not done: cli (step 5), mutations owned by B2 (W1 gated on PATH stat, `NewServer` temp default via cli `$TMPDIR` row, delete `WithLookPath` line), `doc.go`, `verify.sh`.
