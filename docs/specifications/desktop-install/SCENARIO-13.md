---
id: SCENARIO-13
status: open
---

# SCENARIO-13: Uninstall removes quarry's Desktop entry (folds 14, 15)

Cadence: test-first — the ours-only removal guard (destructive identity guard) and backup-before-write in `Uninstall` are on the mandatory set
Acceptance test: `internal/cli/claude_uninstall_desktop_test.go` `Test_claude_uninstall_removes_quarry_from_claude_desktop_after_claude_code`
Acceptance test (SCENARIO-14, folded): `internal/cli/claude_uninstall_desktop_test.go` `Test_claude_uninstall_leaves_claude_desktop_alone_when_its_config_has_no_quarry_entry`
Acceptance test (SCENARIO-15, folded): `internal/cli/claude_uninstall_desktop_test.go` `Test_claude_uninstall_refuses_a_desktop_config_it_cannot_judge`
Narrow loop: `go test -timeout 60s ./internal/claudedesktop/ ./internal/cli/ -run 'Test_uninstall|Test_claude_uninstall|Test_install|Test_claude_install|reportDesktop'` (install filters included: Step 3 changes helpers `Install` shares)
Mutation checks: `!ours` refusal in `(*Server).Uninstall` (remove regardless) → `Test_uninstall_refuses_a_quarry_entry_that_does_not_start_quarry_mcp` | backup before config write in `Uninstall` (drop the backup's error return / swap order) → `Test_uninstall_leaves_the_config_unchanged_when_the_backup_cannot_be_saved` | `readConfig` symlink arm as reached from `Uninstall` → `Test_uninstall_refuses_a_config_path_that_is_a_symlink`
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches, 1 feature package (`internal/claudedesktop`; cli wiring does not count)

User contract: `quarry claude uninstall` — Code lines and hints unchanged, then Desktop: stdout S2 | DR+DQu | DN (exit 0); stderr D11/D1/D4u/D5su/D6/D7/D8 with prefix `quarry: claude uninstall: ` (exit 1, `ReportedError`). Copy verbatim from spec §C/§D; edge rows from §E "Desktop, uninstall". Code failure keeps its early return (SCENARIO-11). No new port or adapter: `Uninstall` reuses `readConfig`, `decodeObject`, `parseOurs`, `replacefile.Write`, so the port survey is n/a.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `internal/cli/claude_uninstall_desktop_test.go` (new) `Test_claude_uninstall_removes_quarry_from_claude_desktop_after_claude_code` — `cli.Execute` `claude uninstall`, Code lists `ourMarketplace`/`userPluginOn`, `desktopHome` (`claude_install_desktop_test.go:101-107`) holding `otherServersConfig`-style servers + an ours entry; rows: beside other servers, and the only server (`mcpServers` becomes `{}`); asserts Code lines + DR + DQu, empty stderr, backup == original bytes, only `quarry` gone. `Env` carries **no** `Executable`.
- [x] Step 2: `internal/claudedesktop/claudedesktop.go:192-211` `UninstallResult` (`Folder`, `Config`, `Skipped`, `Removed`) + `(*Server).Uninstall(ctx) (UninstallResult, error)` — signature-only stubs. Acceptance must fail on the missing DR line.

### Build
- [x] Step 3: `claudedesktop.go:217-229` (extract folder resolve+stat shared with `Install`), new `Uninstall`, `:363-422` (`encodeConfig` takes the final servers map so removal reuses the BR-D8 encoder; `// unreachable:` reason stays true) + `internal/claudedesktop/uninstall_test.go` (new) — test-first. Order: `ErrNoHome` → folder stat (not-exist → `Skipped`) → `readConfig` (`*NotAFileError` → not-in; `*SymlinkError` returned as is, `Command` empty) → classify → backup (`backupSuffix`, `backupMode`) **before** write with `current.mode`. Never calls `executable`/`lookPath`/`isTemporary`. Tests: `Test_uninstall_refuses_a_quarry_entry_that_does_not_start_quarry_mcp` (rows copied from `install_test.go:328-406`, including link-under-another-name `:377`, `quarry: null`, plus an ours control row that differs in one field, e.g. `args ["mcp"]` vs `["mcp","x"]`; foreign → `*ForeignEntryError`, bytes identical, no backup); `Test_uninstall_removes_an_entry_of_ours_at_any_path` (bare `quarry`, `…/quarry` element per `install_test.go:408`, a `$TMPDIR`/`go-build` path, Server built with `WithHome` only); `Test_uninstall_keeps_an_empty_mcpservers_after_removing_the_only_entry`; `Test_uninstall_keeps_other_values_byte_for_byte_in_value` (mirror `install_test.go:241`); `Test_uninstall_changes_nothing_when_there_is_no_quarry_entry` (rows: missing, empty, whitespace, top level null/array/string/number/boolean, `mcpServers` absent/null/array/string/number/boolean, object without `quarry`, folder at config path, FIFO; each: `Removed` false, folder listing (`snapshot`, `install_test.go:80`) unchanged, nothing created); `Test_uninstall_refuses_a_config_path_that_is_a_symlink` (link + target unchanged); `Test_uninstall_refuses_a_config_that_is_not_valid_json` (`*InvalidJSONError`); `Test_uninstall_skips_a_missing_desktop_folder`; `Test_uninstall_refuses_an_empty_home`; `Test_uninstall_leaves_the_config_unchanged_when_the_backup_cannot_be_saved` (`*BackupError`; folder-at-backup-name fault per `install_test.go:702-736`) — seen red before the backup call exists.
- [x] Step 4: `Uninstall` fault and mode pins + `uninstall_test.go` (may arrive green; say so): `Test_uninstall_reports_a_failed_config_write_and_leaves_no_temp` (`*WriteError`; `Chflags` per `install_test.go:738`), `Test_uninstall_keeps_the_mode_of_the_config_it_replaces` (0644, not 0600), backup mode 0600 + overwrite of an earlier backup, `Test_uninstall_fails_without_writing_when_the_config_cannot_be_read` (ReadFile and Lstat `*ReadError` rows, `0o000`), folder-stat error row (wrapped, not classified — D12 is SCENARIO-09).
- [x] Step 5: `internal/cli/claude_uninstall.go:31-43` call new `uninstallDesktop(cmd, home)` after the hints (`:39-41`); `render_claude.go:36-41` constants DR/DN/DQu, shared S2 format with `renderDesktopInstalled` (`:192-207`), new `renderDesktopUninstalled`; `desktopNoHome` (`:40-41`) takes the verb (`desktopFailureLine` `:219-222`). Folded-14 test `Test_claude_uninstall_leaves_claude_desktop_alone_when_its_config_has_no_quarry_entry` (one row per 14 Example, mcpServers-without-quarry included; DN, exit 0, folder listing unchanged). Siblings in the new file: `Test_claude_uninstall_reports_a_missing_home_after_the_claude_code_lines` (mirror `claude_install_desktop_test.go:186`), `Test_claude_uninstall_prints_the_claude_code_hint_before_the_claude_desktop_lines` (one shared writer, mirror `:213`), `Test_claude_uninstall_returns_the_error_of_a_failed_claude_desktop_line_write` (`failsAfterFirstWrite`, mirror `:228`). Re-point every uninstall Code-success pin in `claude_uninstall_test.go` in this batch: `:25-36`, `:214-223`, `:276-308` gain `desktopSkippedLine` (`claude_install_test.go:41`); `:38-48` and the `/home/ada` rows of `:60-212` move to a `t.TempDir()` home with project paths built from it, gaining S2; `:50-58` and the unset-home row (`:157-164`) gain the D11 uninstall line + `ReportedError` (a `wantErr` field asserted with `assert.ErrorIs` in the table loop `:202-211` — no `if` in the loop). Code-failure tests (`:225-236`, `:310-416`) stay as they are.
- [x] Step 6: `render_claude.go:258-265` D4u in the `ForeignEntryError` arm and D5su in the `SymlinkError` arm, both branching on `verb` (never on `Command == ""`); folded-15 test `Test_claude_uninstall_refuses_a_desktop_config_it_cannot_judge` (rows D4u, D5su, D1 with offset, D6, D7, D8; config untouched via `desktopRefusalSnapshot` `claude_install_desktop_test.go:498`). Delete `claude_desktop_internal_test.go:30-48` (now reachable through `Execute`) and rewrite its header comment `:1-2`.

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues` (complexity on `desktopConfigFailureLine` likely); doc comments: `claudedesktop/doc.go` (adds and removes), `Server` and `Uninstall` (name its refusals), `newClaudeUninstallCommand` (`claude_uninstall.go:11-12`). Expected test-count drop: the two deleted white-box tests.

### Verify
- [ ] Step 8: `verify.sh <start> ./internal/claudedesktop/... ./internal/cli/...` + `spec-check.py desktop-install` → tick SCENARIO-13 (with 14 and 15 fold notes) and rewrite STATE.md.

## Handoff

**Binding decisions:**
- `Uninstall` returns only `ErrNoHome`, a wrapped folder-stat error, `*SymlinkError` (`Command` empty), `*ReadError`, `*InvalidJSONError`, `*ForeignEntryError`, `*BackupError`, `*WriteError`. Every shape that cannot hold our entry, `*NotAFileError` included, becomes `Removed: false` inside the package. cli never sees D2/D3/D5o types under uninstall (BR-D7 inverts install's refusals).
- The removal guard is `parseOurs` and only that: uninstall removes `mcpServers.quarry` only when `parseOurs` accepts it, whatever the path. It is judged per decoded value. Duplicate `quarry` keys collapse to the last on decode (BR-D8: "no row"), so an earlier duplicate is not a gate finding.
- Uninstall takes only `home`: no `Executable`, `LookPath` or temp check, and no signature change at `claude.go:42` or in `cmd/quarry`. The uninstall cli tests omit `Env.Executable` on purpose; this departs from STATE's fixture rule for Desktop-folder tests.
- Folder resolve+stat is one helper shared by `Install` and `Uninstall`, so SCENARIO-09 adds S2b/D12 once for both verbs.
- `UninstallResult` mirrors `claudeplugin.UninstallResult` naming; `renderDesktopUninstalled` prints DQu only when `Removed`.
- `desktopNoHome` is verb-parameterised. D4u/D5su are verb branches in the existing arms; no new error types.

**Left unbuilt:**
- D12 under uninstall: the folder-stat error stays a `runtimeError` (SCENARIO-09). S2b under uninstall: when `Claude` is a regular file, the config Lstat fails with ENOTDIR and prints D6, the same as install today (SCENARIO-09). S1/N1u/N1bu: SCENARIO-09. Continue after a Code failure and D13 for uninstall: SCENARIO-11.

**Traps:**
- `readConfig` maps a folder or FIFO to `*NotAFileError`, which uninstall must turn into DN. It also maps a symlink first, which must stay D5su. A FIFO row hangs without `-timeout`.
- `encodeConfig` (`:402-411`) always inserts the entry today. Removal must not reuse it unchanged.
- An empty config is "present" for install (backup) but DN for uninstall (no write, no backup).
- `/home/ada` homes on Code-success uninstall tests now stat a real path. Use `t.TempDir()`.

## Phase report

Run B2 (steps 5-6) done; start commit bd145daf. `go test ./internal/cli/ ./internal/claudedesktop/` green (cli 712 (+4), claudedesktop 63 (+17) vs start), `golangci-lint run ./internal/cli/...` = 0 issues. All three acceptance tests green (`Test_claude_uninstall_removes_quarry_from_claude_desktop_after_claude_code`, `..._leaves_claude_desktop_alone_when_its_config_has_no_quarry_entry`, `..._refuses_a_desktop_config_it_cannot_judge`).

- `internal/cli/claude_uninstall.go`: `uninstallDesktop(cmd, home)` called after the hints (no Executable/LookPath); doc comment of `newClaudeUninstallCommand` and Long help NOT touched (V step 7 doc comment; Long is SCENARIO-16).
- `internal/cli/render_claude.go`: `desktopSkippedFmt` (shared S2), `desktopNoHome` now `%s`-verb, DR/DN/DQu constants, `renderDesktopUninstalled`, `desktopForeignLine`/`desktopSymlinkLine` branch on `verb == uninstallCommand` (D4u/D5su), never on `Command == ""`.
- Tests: `claude_uninstall_desktop_test.go` (folded 14/15 + 3 siblings; `uninstallDesktop`, `folderState` helpers); `claude_uninstall_test.go` re-pointed (Code-success pins gain S2; hints table uses `tool func(home)`, `home func(t)`, `wantErr`; unset-home rows gain D11 + `ReportedError`); `claude_install_test.go` `projectPlugin` const became `projectPluginUnder(home)`; `claude_desktop_internal_test.go` lost its two uninstall-verb tests, header rewritten.
- Step 6 production arms were written in the same batch as step 5 copy (acceptance went green first), so D4u/D5su tests were not seen red before code; proved by mutation instead: foreign verb branch off -> `.../a_foreign_quarry_entry`; symlink verb branch off -> `.../a_symbolic_link`; `desktopNoHome` verb ignored -> missing-home, outside-home and unset-home rows; DQu dropped / Removed negated -> acceptance rows. All restored.
- V must: sweep (`go build ./... && golangci-lint run ./...`), doc comments (`claudedesktop/doc.go`, `Server`, `Uninstall`, `newClaudeUninstallCommand`), `verify.sh`, spec tick (13 + folds 14, 15), `spec-check.py`, STATE.md rewrite, `status: done`. Expected test-count change: the two deleted white-box tests (already counted in cli +4).
