---
id: SCENARIO-04
status: open
---

# SCENARIO-04: Install repoints a stale quarry entry (folds SCENARIO-03 DK no-op, SCENARIO-05 D4 refusal)

Cadence: test-first — write-safety guard: the "ours" identity check is the clobber/overwrite refusal for `mcpServers.quarry` (`build.md` → Build cadence). Batch 1 only; batches 2-3 are code-first.
Acceptance test: `internal/cli/claude_install_desktop_test.go` `Test_claude_install_repoints_a_stale_quarry_entry_and_keeps_its_env`
Acceptance test (SCENARIO-03, folded): `internal/cli/claude_install_desktop_test.go` `Test_claude_install_leaves_the_desktop_config_alone_when_it_already_starts_this_quarry`
Acceptance test (SCENARIO-05, folded): `internal/cli/claude_install_desktop_test.go` `Test_claude_install_refuses_a_quarry_entry_that_does_not_run_quarry_mcp`
Narrow loop: `go test ./internal/claudedesktop/ ./internal/cli/ -run 'Install|Desktop'`
Mutation checks: args-exactly-`["mcp"]` check in `isOurs` → `Test_install_refuses_a_quarry_entry_that_does_not_start_quarry_mcp` (extra-arg row); name rule `==` → `strings.EqualFold` → same test (`Quarry` row); last-element split → `filepath.Base` → same test (trailing-slash row); non-object entry treated as absent (overwritten) → same test (`null` row); drop the Unchanged early return in `Install` → `Test_install_changes_nothing_when_the_entry_already_starts_this_quarry`; rebuild the entry from `{command,args}` instead of replacing only `command` → `Test_install_repoints_only_the_command_of_an_entry_that_starts_quarry_mcp` (env row); print quit line for Unchanged → `Test_claude_install_leaves_the_desktop_config_alone_when_it_already_starts_this_quarry`
Runs: A (1) | B1 (2) | B2 (3-4) | V (5-6)
Size: OWNS A RUN — 3 batches, 1 feature package (`internal/claudedesktop`; `internal/cli` copy/render)

Surface surveyed (existing, from `go doc` + grep): `claudedesktop.Result` is read only at `render_claude.go:167-172`; `ErrEntryPresent` only in `claudedesktop.go:34-35,100,195` and `install_test.go:305,333`; `BackupError`/`WriteError` classified at `render_claude.go:181-190`. No new port or adapter. Callers by grep (no LSP row needed: all in-module, one render site).

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `internal/cli/claude_install_desktop_test.go` (append after `:376`, reuse `desktopHome`/`writeDesktopConfig`/`entryNames`/`splitQuarryEntry`) — the three acceptance tests above through `cli.Execute`. Repoint: rows "other absolute path" and bare `"quarry"`, entry carries `env` and one extra entry-level key beside a sibling server; asserts DU, DQ, backup = original, config mode kept, only `command` changed. DK: compact non-canonical config, DK line and no quit line, folder listing and bytes unchanged. D4: foreign entry, D4 line verbatim on stderr with `ReportedError`, Code lines on stdout, no Desktop stdout line, config unchanged, no backup. No stubs needed (black-box); each fails at its assertion on the interim `ErrEntryPresent` runtime error — quote that in the report.

### Build
- [ ] Step 2 (batch 1, test-first): `internal/claudedesktop/claudedesktop.go:34-35,181-216` `merge`, new unexported `isOurs(raw json.RawMessage) bool`, new exported `*ForeignEntryError{Path string}` (typed like `BackupError`; carries the config path because D4 prints it); delete `ErrEntryPresent`; `install_test.go:288-309` replaced by `Test_install_refuses_a_quarry_entry_that_does_not_start_quarry_mcp`, `:333` becomes `NotErrorAs` the new type.
  - Identity rule (BR-D4), no filesystem access, no symlink/hard-link resolution: ours = JSON object, `command` a JSON string whose text after the last `/` is exactly `quarry`, `args` a JSON array of exactly one string `mcp`. Anything else is foreign and refused — the safe direction for every ambiguity.
  - Rows, each a way a non-ours value reaches the key: entry non-object (null, array, string, number, bool); `args` missing, null, `[]`, `["mcp","x"]`, `["x"]`, `["MCP"]`, `["mcp",1]`, the string `"mcp"`; `command` missing, null, number, array, object, `""`; name differs: `quarry.sh`, `Quarry`, `QUARRY`, `xquarry`, trailing slash `/opt/x/quarry/`. Each asserts `*ForeignEntryError` and the folder snapshot unchanged (no write, no backup). Controls differing in one variable: `/usr/local/bin/quarry`, bare `quarry`, `./quarry` are not refused and the config is rewritten.
  - Rulings: other-name symlink to the binary, hard link, `Quarry` on a case-insensitive volume, `.sh` wrapper are all foreign (name match only; the guard never resolves). Only key `quarry` is judged.
- [ ] Step 3 (batch 2): `claudedesktop.go:90-96,98-138,175-179` `Result` gains `Outcome` (Added = zero value, Updated, Unchanged; keep `install_test.go:114,166` equality pins valid) and `Previous` (the stored command, set when Updated); `Install` returns Unchanged right after `merge` decides, before the backup and write; Updated replaces only `command` (entry kept as `map[string]json.RawMessage`; `args`, `env`, other keys untouched). `Result.Command` is set for all three outcomes (the path Desktop starts, for W1). Tests in `install_test.go`: `Test_install_changes_nothing_when_the_entry_already_starts_this_quarry` (string-equal `command`; non-canonical bytes so an accidental rewrite is visible; snapshot unchanged, no backup, Outcome Unchanged, Command set; control row: same path differing by one character is Updated), `Test_install_repoints_only_the_command_of_an_entry_that_starts_quarry_mcp` (env, extra key, `<>&` and large number inside the entry survive; backup = original; mode kept; Previous holds the stored value, bare `quarry` included). Unchanged still goes through `readConfig`, so D1/D2/D3 and non-regular refusals still apply to it. Keep the encode-error `// unreachable:` true (`:213`): no decoded value may become `any`/`float64`.
- [ ] Step 4 (batch 3): `internal/cli/render_claude.go:34,39-41,165-192` and `claude_install.go:54-61` — D4 copy const plus `*ForeignEntryError` arm in `reportDesktopFailure` ahead of the `runtimeError` default (install wording, `claudePath` + `%q` of `Path`, then `ReportedError`); `renderDesktopInstalled` switches on `res.Outcome` (exhaustive): DA, DU (`claudePath(home, res.Command)` and `claudePath(home, res.Previous)`, both `%q`), DK; `desktopQuitLine` only for Added and Updated. Copy verbatim from spec §C DU/DK and §D D4. `installDesktop` keeps `writeResult` as its last action so SCENARIO-08 can append W1 after DA/DU/DK; do not emit W1. cli pins beyond the acceptance tests: old command under home prints `~/...`; DK, DU and D4 each in the same test file, DU's stderr empty.

### Sweep
- [ ] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; refresh doc comments on `Install` (`claudedesktop.go:98-100`, drops the `ErrEntryPresent` sentence), `Result`, `ForeignEntryError`.

### Verify
- [ ] Step 6: full verification (`.claude/scripts/verify.sh <start> ./internal/claudedesktop/... ./internal/cli/...`) + `.claude/scripts/spec-check.py desktop-install`; tick SCENARIO-04 in `specification.md` with its acceptance test, and record 03 and 05 as folded with their tests; rewrite `STATE.md`.

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `isOurs` is a name-and-shape rule with no filesystem access — SCENARIO-13's removal guard must call this same function, so uninstall and install never disagree on what is ours.
- `*ForeignEntryError{Path}` is the D4/D4u carrier; 04's `reportDesktopFailure` arm prints install wording. SCENARIO-13 branches on `verb` there for D4u rather than adding a second type.
- `Result.Outcome` zero value is Added; `Result.Command` is the started path for Added, Updated and Unchanged (W1 in 08 reads it); `Previous` only for Updated.
- Unchanged is decided after `readConfig`/`merge`, so an unparseable or non-regular config still refuses even when it would have been a no-op candidate.
- "Same path" is `command == Executable()` string equality; the written path stays `Executable()` verbatim until 08.

**Left unbuilt** — named so nobody assumes it exists:
- W1 warning after DA/DU/DK — SCENARIO-08. D4u, DR, DN, ours-only removal — SCENARIO-13.
- D1/D2/D3/D5/D6 copy (still `quarry: <err>`) — SCENARIO-06.

**Traps** — things that look right and are not:
- `json.Unmarshal` of `null` into a `string` succeeds and leaves `""`: `command: null` is foreign only because `""` has no `quarry` element — pin the row, do not rely on the decode error.
- `filepath.Base("/opt/x/quarry/")` is `quarry`: using it would call a directory path ours.
- Repacking the entry as a Go struct drops `env` and unknown keys; keep the raw map. Entry key order may change, values may not.
- Removing `ErrEntryPresent` breaks `install_test.go:305,333` at compile time; the "our own entry" row of the old test is now DK, not a refusal.
