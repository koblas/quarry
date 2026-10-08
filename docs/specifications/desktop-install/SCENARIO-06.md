---
id: SCENARIO-06
status: done
---

# SCENARIO-06: Install refuses a config it cannot safely change

Cadence: test-first — write-safety guard: symlink and non-regular-path refusals (`build.md` → Build cadence). Batch 1 only; batches 2-3 are code-first.
Acceptance test: `internal/cli/claude_install_desktop_test.go` `Test_claude_install_refuses_a_desktop_config_it_cannot_safely_change`
Narrow loop: `go test -timeout 60s ./internal/claudedesktop/ ./internal/cli/ -run 'Install|Desktop'` (timeout: a FIFO row hangs on `ReadFile` if the guard is mutated away)
Mutation checks: `os.Lstat` → `os.Stat` in `readConfig` → symlink rows (dangling one writes through the link); drop the `IsRegular` refusal → folder and FIFO rows; test the non-regular arm before the symlink arm → symlink row's `*SymlinkError` assertion; treat any Lstat error as "missing" → `Test_install_fails_without_writing_when_the_config_cannot_be_checked`; backup write moved ahead of `merge` → D1/D2/D3 rows with a seeded backup; `SetEscapeHTML(true)` on the `<E>` encoder → the `<>&` row; `claudePath` applied to `<E>`'s command → the under-home row
Runs: A (1) | B1 (2-3) | B2 (4) | V (5-6)
Size: OWNS A RUN — 3 batches, 1 feature package (`internal/claudedesktop`; `internal/cli` copy)

Surface surveyed (no new port or adapter): `reportDesktopFailure` is the only consumer of `Install`'s error types (`render_claude.go:189`, one call at `claude_install.go:58`, grep); `fs.ErrExist` as the non-regular signal lives only in `claudedesktop.go:123,180,191` and tests `install_test.go:199`, `claude_install_desktop_test.go:196`. Sibling precedent: `*BackupError`/`*WriteError`/`*ForeignEntryError` (`claudedesktop.go:40-67`), classified ahead of the `runtimeError` arm.

Rulings this plan makes (departures from precedent or silent spec):
- **What reaches D1:** only `*json.SyntaxError` (`json.Unmarshal` pre-validates: truncation, BOM, trailing data `{} x`, bad nesting). Valid JSON of the wrong kind is `*json.UnmarshalTypeError`/nil map and is D2/D3, never D1. So D1's no-offset arm is unreachable through `cli.Execute`; it stays in the renderer (spec) and is pinned by one direct internal test of the extracted pure func (justified: unreachable via the command).
- **`<E>` carries the absolute `Executable()` path, not `claudePath`-abbreviated:** the user pastes it into JSON, and `~` is not expanded there (BR-D14's "every path" read for display paths only). Pinned by an under-home row; SCENARIO-08 re-points that row with `Test_claude_install_prints_a_binary_under_home_as_tilde...` (`:137`) when D9 lands. Orchestrator: overrule before dispatch if BR-D14 is meant literally.
- D2, D3, D5s, D5o are install-only wording (uninstall maps them to DN/D5su in 13): they take no verb. D1 and D6 take `verb` (13 reuses them).
- Two kind types (`TopLevelError`, `ServersError`), not one: 13 maps both to DN, and D2/D3 have different copy.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `internal/cli/claude_install_desktop_test.go` (append after `:475`; reuse `desktopHome`/`writeDesktopConfig`/`entryNames`/`installDesktop`, add `desktopConfigShown` rows) — table `Test_claude_install_refuses_a_desktop_config_it_cannot_safely_change`, one row each D1, D2, D3, D5s, D5o (FIFO), D6 (config `chmod 0o000`, restored in `t.Cleanup`): `ReportedError`, Code lines on stdout and nothing from Desktop, the ruled stderr line verbatim (spec §D), config bytes/link/FIFO and folder listing unchanged, D1 row seeds a `.before-quarry` with distinct bytes and asserts it unchanged. No stubs; each fails at its assertion today (`ErrorIs ReportedError`: generic runtime error). Quote that.

### Build
- [x] Step 2 (batch 1, test-first): `claudedesktop.go:171-198` `readConfig`, `:138-145` `Install` — new exported `*SymlinkError{Path, Command string}` (Install fills `Command` from the already-called `Executable()`), `*NotAFileError{Path}`, `*ReadError{Path, Err}` (`Unwrap`; BR-D17: Lstat error other than not-exist AND `ReadFile` error). Lstat mode symlink → SymlinkError; other non-regular → NotAFileError; drop the `fs.ErrExist` wrap and update `Install`'s doc (`:121-124`). Tests in `install_test.go`: `:171-204` re-pointed (folder, FIFO → NotAFile; dangling symlink, symlink to a regular file → Symlink with `Command`; snapshot unchanged, `exe.calls == 1`); `:485-498` also `AsType *ReadError` with `Path`; `:554-565` re-pointed to the config-Lstat fault by Claude folder mode `0o000` (NOT `0o100`: that grants search and Lstat then reports not-exist; Stat of the folder needs search on its parent only), restore before `snapshot`; asserts `ReadError` wrapping `fs.ErrPermission`. Red first: new assertions fail before `readConfig` changes.
- [x] Step 3 (batch 2): `claudedesktop.go:32-33,225-235,282-294` `decodeObject`/`merge` — new exported `*InvalidJSONError{Path, Err}` (Err is the `encoding/json` error as decoded), `*TopLevelError{Path, Kind}`, `*ServersError{Path, Kind}`; unexported `kindOf(raw)` from the first non-space byte (null, array, string, boolean, else number — after validation nothing else can start there). `mcpServers: null` stays absent (not D3). Entry decode in `parseOurs` stays "not ours". Unchanged still passes these checks. Tests `install_test.go:456-483` re-pointed to the types: invalid JSON, `[]`, `null`, `"s"`, `5`, `true`; `mcpServers` array/string/number/boolean; control `mcpServers: null` is Added; every row snapshot-unchanged, no backup.
- [x] Step 4 (batch 3): `internal/cli/render_claude.go:34-46,187-209` — copy consts for D1, D2, D3, D5s, D5o, D6 (spec §D verbatim, `claudePath` + `%q` for P; D1/D6 with `%s` verb); extract the `errors.AsType` chain of `reportDesktopFailure` into `desktopFailureLine(verb, home, err) (string, bool)` (nine arms would trip the complexity lint; 13 adds D4u/D5su there); `invalidJSONDetail(err) string` (`, at byte N` only for `*json.SyntaxError`); `entryJSON(command)` (JSON string via `SetEscapeHTML(false)`; encode error `// unreachable:` — a string always encodes). Tests, all through `cli.Execute` in `claude_install_desktop_test.go` unless noted: D1 rows truncated, BOM, trailing data, garbage, each with literal offset taken from running `encoding/json` on the fixture; D2 one row per kind (null, array, string, number, true, false); D3 rows array/string/number/boolean; D5s symlink to a file and dangling, `<E>` for exe `/opt/a<b>&c"d\e/quarry` (literal `<>&`, `\"`, `\\`) and one under-home exe (absolute in `<E>`, `~` in nothing); D5o folder and FIFO; D6 unreadable file and Claude folder `0o000`; re-point `:188-200` to assert the D5s line. New `internal/cli/claude_desktop_internal_test.go` (`package cli`, precedent `fx_warning_internal_test.go`): `invalidJSONDetail` with a non-SyntaxError, and `reportDesktopFailure` under verb `uninstall` for `*InvalidJSONError` and `*ReadError` (13 wires uninstall; not reachable via `Execute` yet). Cross edge rows × output: stdout carries the Code lines only, stderr exactly one line, in every row.

### Sweep
- [x] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on the six new types (short, no spec ids).

### Verify
- [x] Step 6: `.claude/scripts/verify.sh <start> ./internal/claudedesktop/... ./internal/cli/...` + `.claude/scripts/spec-check.py desktop-install`; tick SCENARIO-06 with its acceptance test; rewrite `STATE.md` (drop the S2b re-point debt, add the new types and the D1 ruling); `status: done`.

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Typed carriers: `*SymlinkError{Path, Command}` (D5s/D5su), `*NotAFileError{Path}` (D5o; uninstall → DN), `*ReadError{Path, Err}` (D6, Lstat and ReadFile), `*InvalidJSONError{Path, Err}` (D1), `*TopLevelError{Path, Kind}` (D2; uninstall → DN), `*ServersError{Path, Kind}` (D3; uninstall → DN) — 13 branches on type, never on message.
- `fs.ErrExist` is no longer the non-regular signal; nothing may test for it.
- D1/D6 render with `verb`; 13 reaches them under `uninstall` and the internal test already pins that wording.
- `<E>`'s command is absolute (not `claudePath`) — a pasted `~` would not expand.

**Left unbuilt** — named so nobody assumes it exists:
- D4u, D5su, DN, DR, ours-only removal, Desktop in `claude_uninstall.go` — SCENARIO-13. Folder Stat error D12, S2b — SCENARIO-09. D9/D10/W1/`WithTempDir` — SCENARIO-08.

**Traps** — things that look right and are not:
- Claude folder mode `0o100` does not fault the config Lstat (search permission is granted); use `0o000`. chmod-based rows assume a non-root runner (existing debt).
- A FIFO row hangs the suite when the non-regular guard is missing: run mutations and the narrow loop with `-timeout`.
- Trailing data and truncation are `*json.SyntaxError` (Unmarshal pre-validates), so "non-SyntaxError" cannot be produced from a fixture file.
- `UnmarshalTypeError` for a top-level array/string/number/bool is valid JSON: it must become D2, not D1. `null` decodes with no error (nil map).

## Phase report

Run V done; scenario complete. `verify.sh` rc=0 throughout (go build, go test, -race, golangci-lint 0 issues); uncovered-diff 0 uncovered, 2 declared unreachable; test-stats: claudedesktop 30 (+3), cli 697 (+13), total 727 (+16).

- `internal/claudedesktop/claudedesktop.go`: `notObjectError.Error` marked `// unreachable:` (merge converts it, `parseOurs` drops it); `merge` and `readConfig` docs cut to 2 lines.
- `internal/cli/render_claude.go`: `desktopEntryJSON` unreachable marker moved onto the fallback body (line above `quoted.Reset()`), reason names both halves.
- `internal/claudedesktop/install_test.go`: `err.Error()` asserted on all six exported types (symlink, not-a-file, invalid JSON, top level, servers, read x2).
- `internal/cli/claude_install_desktop_test.go`: new control-character `<E>` row (`\u0001`, `\t`, round-tripped through `json.Unmarshal`); stdout == Code lines on the html-char and under-home rows.
- Mutations (restored, diffs clean): each Error() text change / dropped Path reddened its install_test row; `strconv.Quote` in place of the encoder reddened `Test_claude_install_escapes_control_characters_in_the_symbolic_link_refusal_as_json` (`\u0001` vs `\x01`).
- Nothing left for a later run.
