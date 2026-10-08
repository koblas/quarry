# desktop-install — current state

Scenarios complete: SCENARIO-01. Last updated by SCENARIO-01.

## Binding decisions
- Desktop runs only after a Code success, after the Code hints; Code failure and `ErrClaudeNotFound` keep their early return — continuing early breaks the `Empty(stdout)` wiring pins 11 re-points (SCENARIO-01). 11 owns continue-on-failure, 09 owns S1.
- `(*claudedesktop.Server).Install` order: empty home (`ErrNoHome`) -> folder `os.Stat` -> `Executable()` -> config `os.Lstat` -> encode -> `replacefile.Write(…, 0o600)`. D9 (08) must run before the config is read, so `Executable()` stays ahead of the Lstat (SCENARIO-01).
- Config path probed with `os.Lstat`, never `Stat`: a dangling symlink looks missing to `Stat` and the rename would replace the link (SCENARIO-01).
- `internal/platform/replacefile` is generic: it never Lstats or refuses symlinks (that guard is `claudedesktop`'s); `perm` is applied by explicit chmod, so 02 can pass the original mode. Temp-file faults are driven through the unexported `write` seam, never a package-level var (SCENARIO-01).
- `claudedesktop` never reads `os.TempDir()`; 08 adds `WithTempDir` as the temp root (SCENARIO-01).
- `NewServer` has no defaults: nil `Executable` panics once the folder exists. Every cli test that creates the Desktop folder sets `Env.Executable` (SCENARIO-01).
- Test fixtures: `Executable` is a fixed absolute string outside `$TMPDIR`; `testEnv` `Home` is a fixed absent absolute path (`testHome`), because `testEnv` takes no `*testing.T` (SCENARIO-01). cli success-path tests use `t.TempDir()` homes.
- Desktop tests live in `internal/cli/claude_install_desktop_test.go` (`claude_install_test.go` is >700 lines). Printed paths go through `claudePath` + `%q`; the config on disk holds the absolute path (SCENARIO-01).
- Unclassified Desktop errors land in the `runtimeError` arm (`reportDesktopFailure` default); only `ErrNoHome` -> D11 + `ReportedError` is classified (SCENARIO-01).

## Left unbuilt
- Merge, backup, original-mode keep, D7/D8 copy — SCENARIO-02. "Ours", DU/DK, D4 — SCENARIO-04. Symlink/folder/FIFO refusals D5s/D5o — SCENARIO-06.
- Executable path choice, W1, D9 (`WithTempDir`), D10 (`Executable()` error classification) — SCENARIO-08.
- Folder stat error (D12) and `Claude` is a file (S2b), S1 — SCENARIO-09.
- Continue-on-Code-failure — SCENARIO-11. Uninstall's Desktop step — SCENARIO-13; the dead not-found copy (`render_claude.go:28,42,50,199-202`) stays until 09.

## Traps
- Interim present-config refusal (`fs.ErrExist`, unruled copy, goes to the `runtimeError` arm): SCENARIO-02 must NARROW it to non-regular shapes, not delete it, or a symlinked config becomes a merge target until 06 lands (SCENARIO-01).
- `failingWriter` (`fakes_test.go:96`) fails the first Code write and never reaches the Desktop line; use `failsAfterFirstWrite` in `claude_install_desktop_test.go` (SCENARIO-01).
- A cli test home like `/home/ada` reaching Code success would stat a real path; use `t.TempDir()` homes (SCENARIO-01).

## Open debts
- `internal/claudedesktop/claudedesktop.go:87-96` Lstat-then-rename window: a config that appears between Lstat and rename (Desktop rewriting while running) is replaced; spec mitigation is "quit Desktop first". SCENARIO-02's merge path inherits the window. Unowned beyond 02 acknowledging it.
- `internal/claudedesktop/install_test.go` `Test_install_fails_without_writing_when_the_config_cannot_be_checked` reaches the config-Lstat fault via "Claude is a regular file", which SCENARIO-09 turns into S2b. 09 must re-point it (e.g. Claude folder mode 0o100 so folder Stat passes and config Lstat gets EACCES).
- The interim present-config refusal (above) is unruled copy; SCENARIO-02 narrows it.
