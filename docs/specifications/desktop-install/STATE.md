# desktop-install — current state

Scenarios complete: SCENARIO-01, SCENARIO-02 (folds 07), SCENARIO-04 (folds 03, 05), SCENARIO-06, SCENARIO-08. Last updated by SCENARIO-08.

## Binding decisions
- Desktop runs only after a Code success, after the Code hints; Code failure and `ErrClaudeNotFound` keep their early return — continuing early breaks the `Empty(stdout)` wiring pins 11 re-points (SCENARIO-01). 11 owns continue-on-failure, 09 owns S1.
- `(*claudedesktop.Server).Install` order: empty home (`ErrNoHome`) -> folder `os.Stat` -> `Executable()` (`*ExecutableError`, D10) -> `choosePath` -> D9 `isTemporary` on the exe and the chosen path (`*TempBuildError`) -> `readConfig` (Lstat, regular check, ReadFile) -> `merge` -> (Unchanged returns here) -> backup -> config write. D9 and D10 precede the config read, so a refused build never touches or reads the config (SCENARIO-01, 02, 08).
- Path choice (`choosePath`): `os.Stat` (never `Lstat`) on the PATH quarry and the exe, `os.SameFile`; same file -> write the PATH path, else the exe plus `Result.PathQuarry` (the sole W1 signal; cli never stats). `Result.Command` is the written path for every outcome; `Unchanged` and `SymlinkError.Command` use it. Nil/erroring/relative `LookPath` or an unstat-able PATH file -> exe, no W1 (SCENARIO-08).
- `claudedesktop.LookPath` is its own type (cli converts `claudeplugin.LookPath`). `NewServer` defaults only `tempDir` to `os.TempDir()`; `WithTempDir("")` disables the root check, so a claudedesktop test with a temp-dir exe passes `WithTempDir`. cli has no temp seam: tests set `TMPDIR` (`outsideTemp` after `desktopHome`) (SCENARIO-08).
- D9 is lexical (`filepath.Rel`, no `EvalSymlinks`) plus any path element starting `go-build`; `*TempBuildError{Path}` and `*ExecutableError{Config, Err}` render in `desktopBinaryFailureLine`; every path through `claudePath` + `%q` (SCENARIO-08).
- Config path probed with `os.Lstat`, never `Stat`: a dangling symlink looks missing to `Stat` and the rename would replace the link. Symlink is tested before the non-regular arm; `fs.ErrExist` is no longer the non-regular signal and nothing may test for it (SCENARIO-01, 02, 06).
- Typed refusals carry the classification; 13 branches on type, never on message: `*SymlinkError{Path, Command}` (D5s/D5su; Install fills `Command` from `Executable()`), `*NotAFileError{Path}` (D5o), `*ReadError{Path, Err}` (D6: config Lstat error other than not-exist, and ReadFile error), `*InvalidJSONError{Path, Err}` (D1; Err is the `encoding/json` error as decoded), `*TopLevelError{Path, Kind}` (D2), `*ServersError{Path, Kind}` (D3). D2/D3/D5o/D5s are install-only wording (uninstall maps to DN/D5su); D1/D6 take `verb`. `mcpServers: null` is absent, not D3 (SCENARIO-06).
- D1 renders `, at byte N` only for `*json.SyntaxError`, which is all `Unmarshal` can return for invalid input (truncation, BOM, trailing data); valid JSON of the wrong kind is D2/D3, never D1. The no-offset arm is pinned by an internal test only (SCENARIO-06).
- D5s `<E>` is `desktopEntryJSON(link.Command)`: the ABSOLUTE chosen path (PATH path or `Executable()`), JSON-escaped with `SetEscapeHTML(false)`, not `claudePath`-abbreviated, because a pasted `~` is not expanded. (SCENARIO-06, 08).
- `reportDesktopFailure` delegates to `desktopFailureLine` -> `desktopWriteFailureLine` / `desktopConfigFailureLine` (nine arms trip the complexity lint); 13 adds D4u/D5su/DN arms there (SCENARIO-06).
- Backup is `replacefile.Write(config+".before-quarry", original, 0o600)` strictly BEFORE the config write, only for an existing regular config (empty included); config keeps the Lstat `Perm()`, a created one is `0o600` (SCENARIO-02).
- Merge decodes values as `json.RawMessage`, encodes with `SetEscapeHTML(false)`, 2-space indent, trailing newline; top-level key order may change. The `// unreachable:` on its encode error depends on that (SCENARIO-02).
- `*BackupError{Path, Config, Err}` / `*WriteError{Path, Err}` (exported, `Unwrap`) are classified in `reportDesktopFailure(cmd, verb, home, err)` into D7/D8 ahead of the `runtimeError` default arm; `BackupError` carries `Config` because D7 prints it. 04's D4 and 06's D1/D2/D3/D5/D6 add their own types the same way; 13 reuses D7/D8 via `verb` (SCENARIO-02).
- Identity of an entry of ours is `parseOurs(raw) (fields, command, ok)` in `claudedesktop.go`: JSON object, `command` a string whose text after the last `/` is exactly `quarry`, `args` exactly `["mcp"]`; name match only, no filesystem access (symlink and hard link under another name are foreign). SCENARIO-13's removal guard must call `parseOurs` so install and uninstall never disagree (SCENARIO-04).
- `*ForeignEntryError{Path}` is the D4 carrier; `reportDesktopFailure` prints it verb-parameterised, so 13 branches on `verb` for D4u rather than adding a type. `Result.Outcome` zero value is Added; `Command` is the started path for all three outcomes; `Previous` only for Updated. Updated replaces only `command` (raw map kept, `env` and unknown keys survive); Unchanged is string equality with the chosen path and still passes `readConfig`/`merge`, so D1/D2/D3 still refuse it. `renderDesktopInstalled` switches exhaustively on `Outcome` and prints the quit line for Added and Updated only; `installDesktop` appends W1 after `writeResult` (SCENARIO-04, 08).
- `osreason.Reason` has an `*os.LinkError` arm so rename failures render the bare errno (orchestrator ruling, SCENARIO-02).
- `internal/platform/replacefile` is generic: it never Lstats or refuses symlinks; `perm` is applied by explicit chmod. Temp-file faults go through the unexported `write` seam, never a package-level var (SCENARIO-01).
- `NewServer` defaults only the temp root: nil `Executable` panics once the folder exists. Every cli test that creates the Desktop folder sets `Env.Executable` (SCENARIO-01).
- Test fixtures: `Executable` is a fixed absolute string outside `$TMPDIR` (a temp-dir exe is D9); `testEnv` `Home` is a fixed absent absolute path (`testHome`). cli success-path tests use `t.TempDir()` homes. Desktop cli tests live in `internal/cli/claude_install_desktop_test.go`; printed paths go through `claudePath` + `%q` (SCENARIO-01).

## Left unbuilt
- D4u, DR, DN, ours-only removal — SCENARIO-13.
- W1 under uninstall: none (install-only).
- Folder stat error (D12) and `Claude` is a file (S2b), S1 — SCENARIO-09.
- Continue-on-Code-failure — SCENARIO-11. Uninstall's Desktop step — SCENARIO-13; the dead not-found copy (`render_claude.go:28,42,50,199-202`) stays until 09.

## Traps
- `json.Unmarshal` of `null` into `map[string]json.RawMessage` returns nil without error: top-level `null` must be detected as non-object (`decodeObject`) (SCENARIO-02).
- `syscall.Chflags` exists only on darwin: the D8 tests compile on macOS only (no GOOS branches, repo precedent `cmd/quarry/run.go:162`); clear the flag in `t.Cleanup` or `t.TempDir` removal fails (SCENARIO-02).
- A read-only folder fails at `CreateTemp`: it proves D7 (backup) but not temp cleanup; D8 tests must fault after the temp exists (SCENARIO-02).
- A 0600 pin on a replaced config proves nothing (`CreateTemp` is 0600); use 0644 (SCENARIO-02).
- `failingWriter` (`fakes_test.go:96`) fails the first Code write and never reaches the Desktop line; use `failsAfterFirstWrite` in `claude_install_desktop_test.go`. A cli test home like `/home/ada` reaching Code success would stat a real path; use `t.TempDir()` homes (SCENARIO-01).

- Claude folder mode `0o100` does not fault the config Lstat (search permission is granted); use `0o000`. A FIFO config row hangs the suite if the non-regular guard is missing: run mutations and the narrow loop with `-timeout` (SCENARIO-06).
- The BOM row's D1 text is the stdlib's escaped `'\ufeff'` (literal backslash-u), not the rune. `notObjectError.Error` is declared `// unreachable:` (merge converts it, `parseOurs` drops it) (SCENARIO-06).
- The `runtimeError` arm of `reportDesktopFailure` is covered only by `claude_desktop_internal_test.go`; D12 (SCENARIO-09) makes a Library `0o000` a classified failure, so re-check which test reaches the arm then (SCENARIO-06).
- `json.Unmarshal` of `null` into a `string` succeeds and leaves `""`: `command: null` is foreign only because `""` has no `quarry` element; the row is pinned, do not rely on the decode error. `filepath.Base("/opt/x/quarry/")` is `quarry`, so the name rule splits on the last `/` instead (SCENARIO-04).

## Open debts
- `internal/claudedesktop/claudedesktop.go` Lstat-then-ReadFile-then-rename window: a config swapped or rewritten between Lstat and rename (Desktop running) is read or replaced via the path; spec mitigation is "quit Desktop first". Unowned beyond 02 acknowledging it — dies unless re-opened.
- The chmod-0o000 / 0o555 fault tests (`install_test.go`, `claude_install_desktop_test.go`, including the D6 unreadable-file and Claude-folder-0o000 rows and the config-cannot-be-checked test) assume a non-root runner; root ignores mode bits and they would fail. Unowned — dies unless re-opened.
- D9's temp-root check is lexical, so macOS `/var` vs `/private/var` is not unified: accepted design residual. Unowned — dies unless re-opened (SCENARIO-08).
- `if s.tempDir == ""` in `isTemporary` is behaviour-equivalent to the `Rel` path (NIT, mutation cannot redden it). Unowned (SCENARIO-08).
