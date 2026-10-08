---
id: SCENARIO-09
status: open
---

# SCENARIO-09: An absent target is skipped and named (folds 10, 12)

Cadence: code-first — no mandatory test-first item (no overwrite/delete guard, no atomic adapter touched; `locate` only decides skip vs refuse, writes stay behind `readConfig`)
Acceptance test: `internal/cli/claude_absent_target_test.go` `Test_claude_skips_the_absent_target_and_handles_the_present_one`
Acceptance test (SCENARIO-10, folded): `internal/cli/claude_absent_target_test.go` `Test_claude_refuses_when_neither_claude_code_nor_claude_desktop_is_present`
Acceptance test (SCENARIO-12, folded): `internal/cli/claude_absent_target_test.go` `Test_claude_refuses_a_claude_desktop_it_cannot_look_for`
Narrow loop: `go test -timeout 60s ./internal/claudedesktop/ ./internal/cli/ -run 'Claude|Install|Uninstall|reportDesktop'`
Mutation checks: `IsDir` arm in `(*Server).locate` (treat a file as a folder) → `Test_install_skips_a_claude_that_is_a_file` | cli `codeAbsent && Skipped` refusal gate (refuse on any Skipped) → `Test_claude_install_skips_claude_desktop_when_its_folder_does_not_exist` | S1 written before the Desktop error line (drop it on the error path) → acceptance rows "Code absent + D11/D12"
Runs: A (1-2) | B1 (3-4) | B2 (5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (`internal/claudedesktop`; cli wiring does not count)

User contract (copy verbatim from spec §C S1/S2b, §D N1/N1u/N1b/N1bu/D12; rows from §E and *Code × Desktop*):
- `claude` lookup fails (any `LookPath` error, incl. `ErrDot`/non-executable) = Code absent. Desktop then runs: stdout `S1` then Desktop lines (success) / `S1` then D-line on stderr (D11, D12, D4…, exit 1).
- Code absent and Desktop skipped: stdout EMPTY, stderr `N1`/`N1u` (folder missing) or `N1b`/`N1bu` (Claude is a file), `ReportedError`, exit 1. Code present: Desktop skipped stays `S2`/`S2b` on stdout, exit 0.
- D12 (folder stat error other than not-exist) stderr, exit 1, both verbs, any Code outcome that reaches Desktop.

Survey (step 5): no new port. Callers of `ErrClaudeNotFound` in production: `render_claude.go:394` only (LSP `findReferences` not needed; `claudeplugin.go:159` produces it). `claudeRefusalCopy` reads: `render_claude.go:391,395`. cmd/quarry inventory: no test reaches the old refusal — `run_claude_wiring_test.go:15,40` run with a real `claude` file on PATH and `HOME=t.TempDir()` (Code present, unchanged); every other `cmd/quarry` `claude install|uninstall` test is `--help`, arg or `--json` refusal that returns before the lookup (`run_usage_test.go:224-229,340-344,424-434`, `run_read_usage_test.go:60-65`, `run_claude_drift_test.go:52`). `main_test.go:44` testEnv (claude absent, `testHome` absent) now prints N1/N1u for a completed run, but none completes one. Nothing to re-point there.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `internal/cli/claude_absent_target_test.go` (new) — three table tests over `cli.Execute` + `Env.Home`/`Executable` fakes (`runClaudeAt`, `desktopHome`). 09: S1 install, S1 uninstall (Desktop folder present, entry DA/DN), S2b install (Claude is a file, Code present), S2 install/uninstall as green-on-arrival control rows. 10: N1, N1u, N1b, N1bu (stdout empty, exact stderr, no tool calls, `ReportedError`). 12: D11 install (green on arrival) and D12 uninstall, plus Code-absent rows (S1 then D11, S1 then D12) and D12 install; D12 fixture is `Library/Application Support` chmod `0o000` with `t.Cleanup` restoring (non-root runner, existing debt)
- [x] Step 2: run the three; they must fail at their assertions (old refusal text, D6 for a file, `runtimeError` for D12). No stubs needed: tests use existing `cli.Execute` API

### Build
- [x] Step 3 (B1, batch 1): `internal/claudedesktop/claudedesktop.go:277-292` `locate` + `:203-214` `Result` + `:294-300` `UninstallResult` + new `FolderError{Path, Err}` beside `:100-125` — `locate` stats the folder, a non-directory returns not-found-with-reason (`Skipped` true, new `NotAFolder` true on both results, `Install`/`Uninstall` early-return as for a missing folder, no `Executable()` call), any other stat error returns `*FolderError` (Unwrap; replaces the `fmt.Errorf("check %s")` wrap; `ErrorIs fs.ErrPermission` pins at `install_test.go:635`, `uninstall_test.go:439` must stay green). Tests in `install_test.go` + `uninstall_test.go` for BOTH verbs: `Test_install_skips_a_claude_that_is_a_file` (+ uninstall twin), symlink to a folder is a folder, symlink to a file is `NotAFolder`, dangling symlink is missing (`NotAFolder` false), `Library` a file -> `*FolderError` (ENOTDIR is not not-exist), `*FolderError.Path` is the folder; `fakeExecutable.calls == 0` on the file row
- [x] Step 4 (B1, batch 2): `internal/cli/render_claude.go:40,203-228,241-253` — S2b skip line (second format beside `desktopSkippedFmt`, selected by `NotAFolder`, in `renderDesktopInstalled` and `renderDesktopUninstalled`); D12 arm: fold `ErrNoHome` and `*FolderError` into one `desktopLocateFailureLine(verb, home, err)` helper called from `desktopFailureLine` (nine-arm complexity lint); D12 text `cannot check for Claude Desktop at %q (%s); check its permissions, then run quarry claude %s again` with `osreason.Reason`. Tests (Code present, via `claude_install_desktop_test.go` / `claude_uninstall_desktop_test.go` helpers): S2b and D12 for install AND uninstall (stdout = Code lines only on D12). `claude_desktop_internal_test.go:30` still reaches the `runtimeError` arm with `errBoom` (D12 no longer does)
- [ ] Step 5 (B2): `internal/cli/claude_install.go:34-37,50-60`, `claude_uninstall.go:32-35,41-48`, `render_claude.go:20-34,89-104,394-397` — in each `RunE`, `errors.Is(err, claudeplugin.ErrClaudeNotFound)` is intercepted BEFORE `reportClaudeFailure` and runs Desktop with `codeAbsent`: `installDesktop`/`uninstallDesktop` gain that parameter; on `err == nil && res.Skipped && codeAbsent` return the N1/N1b (N1u/N1bu) refusal with nothing written to stdout; otherwise, when `codeAbsent`, write S1 first, then the usual success lines or `reportDesktopFailure`. New consts: `S1` line, four N1* lines built from one shared head + verb tail + reason (`does not exist` / `is not a folder`), all via `claudePath` + `%q`. Delete `installClaudeNotFoundRefusal` (`:32`), `uninstallClaudeNotFoundRefusal` (`:89`), `claudeRefusals.notFound` (`:97`; collapse the one-field struct to `map[string]string` if it leaves one), the `ErrClaudeNotFound` arm (`:394-397`). Re-point `claude_install_test.go:224-250` `Test_claude_install_refuses_when_claude_is_not_on_the_path` (3 LookPath-error rows -> N1 on a `t.TempDir()` home; keep `tool.argv` empty, stdout empty) and `claude_uninstall_test.go:278-290` -> N1u. Add: Code-absent × {DA, DN/DR, DK, D4 foreign} S1 + Desktop lines, both verbs; fault test: failing stdout on the S1 write returns that error, both verbs (precedent `Test_claude_install_returns_the_error_of_a_failed_claude_desktop_line_write`, `claude_install_desktop_test.go:228`); `Skipped` + Code present still S2/S2b exit 0 (control for the gate)

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `FolderError`, `NotAFolder`, updated `Install`/`Uninstall`/`locate` docs (state the skip rule once; no spec ids)

### Verify
- [ ] Step 7: `.claude/scripts/verify.sh <start> ./internal/claudedesktop/... ./internal/cli/...`; `spec-check.py desktop-install`; tick SCENARIO-09 with its acceptance test and SCENARIO-10, 12 as folded lines naming `SCENARIO-09`'s delivery and their acceptance tests (test reference last on the line); rewrite STATE.md

## Handoff

**Binding decisions:**
- `locate` is the one place that classifies the Desktop folder: missing and not-a-folder are both `Skipped`, told apart by `NotAFolder`; any other stat error is `*FolderError{Path, Err}` for both verbs — cli never stats, branches on type (SCENARIO-11, 16 reuse the shapes).
- `ErrClaudeNotFound` is intercepted in the two `RunE` funcs, never in `reportClaudeFailure`; S1 is stdout and printed only when Desktop is not the N1 refusal, so N1/N1b leave stdout empty. Code-absent + D4/D7/D8… prints S1 then the stderr line, exit 1 (implied by *Code × Desktop*, unruled per row).
- Code failure early return is unchanged; SCENARIO-11 adds continue-on-failure and must keep S1/N1 off the failure path (S1 is a skip, not a failure).

**Left unbuilt:** continue-on-Code-failure, D13, C1 in-place line (SCENARIO-11); help Long/README/PRD (SCENARIO-16).

**Traps:**
- `os.Stat` on `Claude` succeeds for a regular file, so without `IsDir` the file reaches `readConfig` and prints D6 (ENOTDIR) — the S2b gap.
- D12 fixture must chmod the PARENT (`Application Support`), not `Claude` (`0o000` on `Claude` only fails the config Lstat, D6, `claude_install_desktop_test.go:798`).
- The mcp-install spec (`mcp-install/specification.md:135-136`) still quotes the deleted not-found copy; it is history, do not edit.

## Phase report

Run B1 done (steps 3-4 ticked). Narrow loop `go test ./internal/claudedesktop/` green; `internal/cli` has only the B2 reds below.

Changed:
- `internal/claudedesktop/claudedesktop.go`: `locate() (location, error)` (new unexported `location{folder, config, found, notAFolder}`), `FolderError{Path, Err}` (Unwrap), `NotAFolder` on `Result` and `UninstallResult`, `Install`/`Uninstall` docs updated.
- `internal/cli/render_claude.go`: `desktopNotFolder` (S2b), `desktopFolderRefusal` (D12), `desktopSkipLine`, `desktopLocateFailureLine` (ErrNoHome + `*FolderError`, called first from `desktopFailureLine`).
- Tests: `install_test.go` (`claudeNotAFolderRows`, `homeWithSupport`, skip table over file / symlink-to-file / dangling, symlink-to-folder, `*FolderError` ENOTDIR, Path pin on the permission test), `uninstall_test.go` twins, `claude_uninstall_desktop_test.go` (S2b uninstall, Code present).

Mutation (plan line, B1 part): `locate` `loc.found, loc.notAFolder = info.IsDir(), !info.IsDir()` -> `true, false` reddened `Test_install_skips_a_claude_that_is_not_a_folder/{a_regular_file,a_symlink_to_a_file}` and the uninstall twin (test is named `..._not_a_folder`, not the plan's `..._is_a_file`). The `desktopSkipLine` `NotAFolder` selector, dropped, reddened `Test_claude_skips_the_absent_target..._install_skips_Claude_Desktop_when_Claude_is_a_file` and `Test_claude_uninstall_skips_claude_desktop_when_its_path_is_a_file`. The cli `codeAbsent && Skipped` gate and S1 mutations belong to B2.

Still red (all B2): `Test_claude_skips_the_absent_target...` rows install/uninstall without claude; all four rows of `Test_claude_refuses_when_neither...`; the three Code-absent rows of `Test_claude_refuses_a_claude_desktop_it_cannot_look_for`. Green now: S2b install/uninstall, D12 install/uninstall with Code present.

Left for V: `golangci-lint` reports 3 `thelper` in `internal/cli/claude_absent_target_test.go:81,96,102` (row `home` closures lack `t.Helper()`); everything else in the two packages lints clean.
