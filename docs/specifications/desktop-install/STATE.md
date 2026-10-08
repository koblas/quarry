# desktop-install — current state

Scenarios complete: SCENARIO-01, SCENARIO-02 (folds 07). Last updated by SCENARIO-02.

## Binding decisions
- Desktop runs only after a Code success, after the Code hints; Code failure and `ErrClaudeNotFound` keep their early return — continuing early breaks the `Empty(stdout)` wiring pins 11 re-points (SCENARIO-01). 11 owns continue-on-failure, 09 owns S1.
- `(*claudedesktop.Server).Install` order: empty home (`ErrNoHome`) -> folder `os.Stat` -> `Executable()` -> `readConfig` (Lstat, regular check, ReadFile) -> `merge` -> backup `replacefile.Write` -> config `replacefile.Write`. D9 (08) must run before the config is read, so `Executable()` stays ahead of the Lstat (SCENARIO-01, 02).
- Config path probed with `os.Lstat`, never `Stat`: a dangling symlink looks missing to `Stat` and the rename would replace the link. A non-regular path keeps the `fs.ErrExist` refusal until 06 rules its copy (SCENARIO-01, 02).
- Backup is `replacefile.Write(config+".before-quarry", original, 0o600)` strictly BEFORE the config write, only for an existing regular config (empty included); config keeps the Lstat `Perm()`, a created one is `0o600` (SCENARIO-02).
- Merge decodes values as `json.RawMessage`, encodes with `SetEscapeHTML(false)`, 2-space indent, trailing newline; top-level key order may change. The `// unreachable:` on its encode error depends on that (SCENARIO-02).
- `*BackupError{Path, Config, Err}` / `*WriteError{Path, Err}` (exported, `Unwrap`) are classified in `reportDesktopFailure(cmd, verb, home, err)` into D7/D8 ahead of the `runtimeError` default arm; `BackupError` carries `Config` because D7 prints it. 04's D4 and 06's D1/D2/D3/D5/D6 add their own types the same way; 13 reuses D7/D8 via `verb` (SCENARIO-02).
- `osreason.Reason` has an `*os.LinkError` arm so rename failures render the bare errno (orchestrator ruling, SCENARIO-02).
- `internal/platform/replacefile` is generic: it never Lstats or refuses symlinks; `perm` is applied by explicit chmod. Temp-file faults go through the unexported `write` seam, never a package-level var (SCENARIO-01).
- `claudedesktop` never reads `os.TempDir()`; 08 adds `WithTempDir` as the temp root (SCENARIO-01).
- `NewServer` has no defaults: nil `Executable` panics once the folder exists. Every cli test that creates the Desktop folder sets `Env.Executable` (SCENARIO-01).
- Test fixtures: `Executable` is a fixed absolute string outside `$TMPDIR`; `testEnv` `Home` is a fixed absent absolute path (`testHome`). cli success-path tests use `t.TempDir()` homes. Desktop cli tests live in `internal/cli/claude_install_desktop_test.go`; printed paths go through `claudePath` + `%q` (SCENARIO-01).

## Left unbuilt
- "Ours" check, DU/DK, D4, quit-line suppression — SCENARIO-04 (replaces the interim `ErrEntryPresent` refusal, which reaches the user as `quarry: <err>`).
- D1/D2/D3/D5s/D5o/D6 copy and typed errors (unparseable, non-object, `mcpServers` shape, non-regular path, read error) — SCENARIO-06; until then they reach the user as `quarry: <err>`.
- Executable path choice, W1, D9 (`WithTempDir`), D10 (`Executable()` error classification) — SCENARIO-08.
- Folder stat error (D12) and `Claude` is a file (S2b), S1 — SCENARIO-09.
- Continue-on-Code-failure — SCENARIO-11. Uninstall's Desktop step — SCENARIO-13; the dead not-found copy (`render_claude.go:28,42,50,199-202`) stays until 09.

## Traps
- `json.Unmarshal` of `null` into `map[string]json.RawMessage` returns nil without error: top-level `null` must be detected as non-object (`decodeObject`) (SCENARIO-02).
- `syscall.Chflags` exists only on darwin: the D8 tests compile on macOS only (no GOOS branches, repo precedent `cmd/quarry/run.go:162`); clear the flag in `t.Cleanup` or `t.TempDir` removal fails (SCENARIO-02).
- A read-only folder fails at `CreateTemp`: it proves D7 (backup) but not temp cleanup; D8 tests must fault after the temp exists (SCENARIO-02).
- A 0600 pin on a replaced config proves nothing (`CreateTemp` is 0600); use 0644 (SCENARIO-02).
- `failingWriter` (`fakes_test.go:96`) fails the first Code write and never reaches the Desktop line; use `failsAfterFirstWrite` in `claude_install_desktop_test.go`. A cli test home like `/home/ada` reaching Code success would stat a real path; use `t.TempDir()` homes (SCENARIO-01).

## Open debts
- `internal/claudedesktop/claudedesktop.go` Lstat-then-ReadFile-then-rename window: a config swapped or rewritten between Lstat and rename (Desktop running) is read or replaced via the path; spec mitigation is "quit Desktop first". Unowned beyond 02 acknowledging it — dies unless re-opened.
- `internal/claudedesktop/install_test.go` `Test_install_fails_without_writing_when_the_config_cannot_be_checked` reaches the config-Lstat fault via "Claude is a regular file", which SCENARIO-09 turns into S2b. 09 must re-point it (e.g. Claude folder mode 0o100 so folder Stat passes and config Lstat gets EACCES).
- The chmod-0o000 / 0o555 fault tests (`install_test.go`, `claude_install_desktop_test.go`) assume a non-root runner; root ignores mode bits and they would fail. Unowned — dies unless re-opened.
