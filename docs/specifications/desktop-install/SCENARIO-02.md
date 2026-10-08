---
id: SCENARIO-02
status: open
---

# SCENARIO-02: Install merges into an existing Desktop config and keeps a backup (folds SCENARIO-07)

Cadence: test-first — write-safety guards in `(*claudedesktop.Server).Install`: backup-before-config ordering, the narrowed non-regular refusal, the present-`quarry`-key refusal (never overwrite), never-write on unparseable / non-object shapes
Acceptance test: `internal/cli/claude_install_desktop_test.go` `Test_claude_install_merges_into_an_existing_desktop_config_and_keeps_a_backup`
Acceptance test (SCENARIO-07, folded): `internal/cli/claude_install_desktop_test.go` `Test_claude_install_reports_a_failed_backup_or_write_and_leaves_the_config_as_it_was`
Narrow loop: `go test ./internal/claudedesktop/ && go test ./internal/cli/ -run 'claude'`
Mutation checks: write the config before the backup → `Test_install_leaves_the_config_unchanged_when_the_backup_cannot_be_saved`; drop the non-regular check (Lstat mode) → `Test_install_refuses_a_config_path_that_is_not_a_regular_file`; allow a present `quarry` key through → `Test_install_never_overwrites_a_present_quarry_entry`; write on a parse failure → `Test_install_writes_nothing_when_the_config_is_not_a_json_object_it_can_merge_into`; backup at the original mode instead of 0600 → acceptance row `a 0644 config`; swap `float64`/default decode for `json.RawMessage` → `Test_install_keeps_other_values_byte_for_byte_in_value`
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 3 batches (merge+mode = steps 3-4, backup+typed errors = 5, cli copy = 6), 1 feature package (`internal/claudedesktop`); SCENARIO-07 folded (same code path: D7/D8 are the error arms of the backup and config writes this scenario adds)

User-visible contract (install only; uninstall verb is SCENARIO-13): present regular config → merge, backup, DA + DQ on stdout, exit 0. Backup failure → D7, config and temp untouched, exit 1. Config temp/rename failure → D8, exit 1; the backup written just before it stays. Shapes owned by SCENARIO-04/06 (`quarry` key present; unparseable; top level not object; `mcpServers` not object/null; read error) → generic error to the `runtimeError` arm (`quarry: <err>`, exit 1), never a write, no backup.

Survey of what `Install` calls (no new port; `replacefile.Write` is the only writer): `os.Stat(folder)` and `os.Lstat(config)` stay (`claudedesktop.go:75,87`); `os.ReadFile(config)` is new (after the regular-file check); `replacefile.Write` is called twice (backup `0o600`, then config at the Lstat mode's `Perm()`, `claudedesktop.go:96`); `encode` (`:111-123`) is replaced by one merge func used by both create and replace.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `internal/cli/claude_install_desktop_test.go:67-190` `Test_claude_install_merges_into_an_existing_desktop_config_and_keeps_a_backup` — `cli.Execute`, `desktopHome`, one row per Examples shape (empty, whitespace, no `mcpServers`, `mcpServers: null`, other servers with env secret + `12345678901234567890` + `<>&`), each config pre-written `0o644`. Assert stdout = 3 Code lines + DA + DQ, stderr empty; `.before-quarry` bytes equal the original, mode `0o600`; config mode stays `0o644`; decode both with `UseNumber`, drop `mcpServers.quarry`, equal; raw text holds `<>&` literally and the big number unchanged; folder listing is exactly config + backup
- [x] Step 2: same file `Test_claude_install_reports_a_failed_backup_or_write_and_leaves_the_config_as_it_was` — two rows. D7: non-empty folder at `…json.before-quarry`. D8: config `0o644` made user-immutable with `syscall.Chflags(config, syscall.UF_IMMUTABLE)`, cleared in `t.Cleanup` so `t.TempDir` can delete it. Both faults fire AFTER the temp exists (folder is writable), so "no temp" is not vacuous. Assert exact D7/D8 line (`~/…` form, `osreason` words read from the run, not hard-coded to a guess), `ReportedError`, stdout = Code lines only, config bytes/mode unchanged, no `.replace-*` entry. No stubs needed: both tests fail at their assertions today (present config returns `fs.ErrExist`)

### Build
- [ ] Step 3: `internal/claudedesktop/claudedesktop.go:65-123` `(*Server).Install`, merge func replacing `encode` — Lstat result: missing → create as today; non-regular (symlink, folder, FIFO, `Mode().IsRegular()` false) → still `fs.ErrExist`-wrapped refusal (NARROWED from "anything"; do not delete); regular → `ReadFile`, empty/whitespace-only = `{}`, decode top level to `map[string]json.RawMessage` (key order may change; duplicate keys last wins), `mcpServers` absent/null → new, else `map[string]json.RawMessage`; a present `quarry` key → exported sentinel refusal wrapping nothing in `fs`, no write, no backup (replaced by SCENARIO-04); encode with `SetEscapeHTML(false)`, 2-space indent, trailing newline. `// unreachable:` on the merge's encode error only with BR-D17's reason (every value a `json.RawMessage` from a successful decode, or a string / `[]string` built here); the existing string-only mark at `:94`/`:120` is rewritten to that reason. Tests in `internal/claudedesktop/install_test.go`: `Test_install_keeps_other_values_byte_for_byte_in_value` (big number, `<>&`, nested env, other servers), `Test_install_merges_into_an_empty_or_whitespace_config`, `…_mcpservers_null_or_absent`, `Test_install_never_overwrites_a_present_quarry_entry` (ours and foreign rows; listing unchanged), `Test_install_writes_nothing_when_the_config_is_not_a_json_object_it_can_merge_into` (invalid JSON, top-level array/null/string, `mcpServers` array/string; control: a valid object merges), `Test_install_refuses_a_config_path_that_is_not_a_regular_file` (rewrite `:127-155`: drop the empty-file and JSON-object rows, they now merge), `Test_install_fails_without_writing_when_the_config_cannot_be_read` (mode `0o000`, unwritten listing)
- [ ] Step 4: same file — mode keep: replace passes the Lstat `Perm()`; create stays `configMode` `0o600`. Tests: `Test_install_keeps_the_mode_of_the_config_it_replaces` (0644 and 0600 rows), `Test_install_creates_the_config_with_mode_0600_and_no_backup` stays (`:75-88`). Update `Result` doc if it gains `Backup string` (optional; cli prints no backup line)
- [ ] Step 5: `claudedesktop.go` typed errors `BackupError{Path string; Err error}` and `WriteError{Path string; Err error}` (`Unwrap`), exported for the cli's `errors.AsType`; backup via `replacefile.Write(config+".before-quarry", original, 0o600)` strictly BEFORE the config write, only when a regular config existed (including empty); failure → `*BackupError`, config untouched. Config `replacefile.Write` failure → `*WriteError`. Tests (`install_test.go`): `Test_install_leaves_the_config_unchanged_when_the_backup_cannot_be_saved` (non-empty folder at backup name; config bytes + listing unchanged), `Test_install_reports_a_failed_config_write_and_leaves_no_temp` (Chflags immutable; backup present and equal to the original; no `.replace-*`), `Test_install_replaces_a_symlink_at_the_backup_name_without_following_it` (target file unchanged, backup is a regular 0600 file), `Test_install_overwrites_the_backup_of_an_earlier_write`, `Test_install_writes_a_backup_of_an_empty_config`. Fallible-call coverage: ReadFile (step 3), both `Write`s (here)
- [ ] Step 6: `internal/cli/render_claude.go:168-176` `reportDesktopFailure(cmd, verb, home, err)` — add `home` (caller `claude_install.go:58`); two arms before the default: `*claudedesktop.BackupError` -> D7, `*claudedesktop.WriteError` -> D8, each `writeClaudeLine` + `ReportedError{}`; paths through `claudePath(home, p)` then `%q`, `<D>` = `filepath.Dir(Path)`, `<os reason>` = `osreason.Reason(err.Err)`, verb in the "run quarry claude <verb> again" clause. Copy as consts beside `desktopNoHome` (`:33-35`), verbatim from Surface & Copy D7/D8. Re-point `claude_install_desktop_test.go:153-165` (`…when_a_config_is_already_present`) to a non-regular config (symlink) — a regular config now merges. Confirm Step 1-2 go green

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; `go doc ./internal/claudedesktop` — `Install` doc says: merges and backs up, refuses non-regular files and a present `quarry` key, names `BackupError`/`WriteError`; no spec ids

### Verify
- [ ] Step 8: `.claude/scripts/verify.sh <start> ./internal/claudedesktop/... ./internal/cli/... ./cmd/quarry/...`; `spec-check.py desktop-install`; tick SCENARIO-02 with its acceptance test and SCENARIO-07 with a line naming SCENARIO-02's folded test (`Test_claude_install_reports_a_failed_backup_or_write_and_leaves_the_config_as_it_was`); STATE.md rewrite

## Handoff

**Binding decisions:**
- Backup is `replacefile.Write(config+".before-quarry", originalBytes, 0o600)` BEFORE the config write, only for an existing regular config — D7 must leave the config untouched and BR-D10's symlink-at-backup-name replacement comes free from rename.
- Merge decodes values as `json.RawMessage` and encodes with `SetEscapeHTML(false)` — BR-D8 (no `float64`, `<>&` literal) and the BR-D17 `// unreachable:` reason both depend on it. Create path and replace path share one merge func.
- `*BackupError` / `*WriteError` (exported, `Path`, `Err`) are what the cli classifies into D7/D8; 06's D1/D2/D3/D5/D6 and 04's D4 add their own types the same way, ahead of the `runtimeError` default arm.
- The `reportDesktopFailure` signature gains `home`; SCENARIO-13 reuses D7/D8 under the uninstall verb through the `verb` argument.

**Left unbuilt:**
- "Ours" check, DU/DK, D4, quit-line suppression — SCENARIO-04 (replaces the interim present-`quarry`-key refusal sentinel).
- D1/D2/D3/D5s/D5o/D6 copy and typed errors — SCENARIO-06; this scenario's generic errors for those shapes reach the user as `quarry: <err>`.
- Folder/Claude-is-a-file (S2b, D12) — SCENARIO-09. W1/D9/D10 — SCENARIO-08.

**Traps:**
- `json.Unmarshal` of `null` into `map[string]json.RawMessage` returns nil without error — a top-level `null` must be detected as non-object, not merged.
- The Lstat-then-ReadFile-then-rename window (STATE.md Open debts) is inherited; the merge path reads via the path, so a swap to a symlink between Lstat and read is not closed here.
- `syscall.Chflags` exists only on darwin: the D8 tests compile on macOS only (no GOOS branches, repo precedent `cmd/quarry/run.go:162`). Clear the flag in `t.Cleanup` or `t.TempDir` removal fails.
- A read-only folder fails at `CreateTemp` (before any temp exists) and proves nothing about temp cleanup — do not use it for D7/D8.
- A 0600 pin on a replaced config proves nothing (`CreateTemp` is 0600); use 0644.
- `Test_install_refuses_a_config_path_that_already_holds_anything` rows "an empty file" and "a JSON object" now merge; leaving them fails the build of the narrowing.

## Phase report

Run A done (start `5f54fa31`). Files: `internal/cli/claude_install_desktop_test.go` only (+consts `userImmutableFlag`, `desktopBackupName`, `desktop*Shown`, `otherServersConfig`; helpers `decodeConfig`, `splitQuarryEntry`, `writeDesktopConfig`, `entryNames`; tests at the end of the file). `golangci-lint run ./internal/cli/...` = 0 issues; no production code touched.

Red now (both at assertions, present config returns `fs.ErrExist`):
- merges test, 5 rows: `Received unexpected error: …/claude_desktop_config.json already exists: file already exists` (`require.NoError`, line 292).
- failed-backup-or-write test, 2 rows: `Target error should be in err chain: expected: "already reported" in chain: …already exists: file already exists` (`require.ErrorIs ReportedError`, line 357).

D8 fault proven before writing: `Chflags(0x2)` on the config, then `CreateTemp` + chmod + write + sync + close all succeed and `os.Rename(temp, config)` fails `operation not permitted` (EPERM). D7 fault proven: rename of a temp file over a non-empty folder fails `file exists`.

Next runs must know:
- `syscall.UF_IMMUTABLE` does not exist; the test uses const `userImmutableFlag = 0x2`.
- Both tests assert the clean errno reason: `(file exists)` for D7, `(operation not permitted)` for D8. `replacefile.Write` returns `*os.LinkError` from the rename, and `osreason.Reason` (`internal/platform/osreason/osreason.go`) handles only `*fs.PathError` and `*os.SyscallError`, so today it would return the whole `replace <path>: rename <tmp> <path>: file exists` message with a random temp name. B2 (step 6) must make `<os reason>` the bare errno: add an `*os.LinkError` arm to `osreason.Reason` (with a test in that package) or unwrap it where `BackupError`/`WriteError` are built. Not decided here.
- D8 listing after failure is config + backup (the backup is written first and stays); D7 listing is config + the folder planted at the backup name.
- The test file now needs darwin to compile (`syscall.Chflags`).
- Step 6 must still re-point `Test_claude_install_returns_the_desktop_error_unreported_when_a_config_is_already_present` (currently green, a regular config will merge).

