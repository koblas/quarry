---
id: SCENARIO-05
status: done
---

# SCENARIO-05: Bad --quicken path refused

Cadence: code-first
Acceptance test: `cmd/quarry/run_test.go` `Test_run_refuses_a_bad_quicken_path`
Narrow loop: `go test ./cmd/quarry/... ./internal/snapshot/... ./internal/platform/homepath/... ./internal/cli/... -run 'Test_run_refuses_a_bad_quicken_path|Test_run_reports_exit_1_when_sync_fails|Test_ResolveBundlePath|Test_Abbreviate|Test_Expand|Test_renderSuccess'`
Mutation checks: (1) delete the R5 suffix guard → the R5 row's stderr assertion AND its "nothing written under snapshots dir" assertion both redden — the R5 fixture is a symlink to a real, syncable bundle, so removing the guard lets `Sync` run to completion and write. (2) make the R5 suffix compare case-sensitive → only the uppercase `.QDF` subtest reddens. (3) move the R5 check to run after the `IsDir`/R6 check → `Test_ResolveBundlePath_refuses_a_qdf_suffix`'s plain-file fixture reddens (would get R6's copy instead of R5's).

Size verdict: OWNS A RUN — one Gherkin `When` (Scenario Outline, 6 rows = one acceptance test, table-driven), bounded to one new platform package plus one new feature-package file pair.

## Implementation Plan

Reused: `internal/cli/sync.go:23-69` `newSyncCommand`/`RunE` (calls `srv.Sync` with the raw `--quicken` value today); `internal/cli/render.go:12-22` `abbreviateHome` (moves out); `internal/cli/render_internal_test.go:1-28` (its 3 tests move); `cmd/quarry/run_test.go:154-163` `Test_run_reports_exit_1_when_sync_fails` (fixture changes, see Sweep); copy source `specification.md:89` (`~/` rule), `:226-229` (R4-R7 exact text), `:314-328` (outline). Checked `internal/cli/sync_internal_test.go` — no `--quicken` paths there, unaffected.

**Deviations to flag before dispatch (unruled by product-vision):** the `homepath` extraction and its `Abbreviate` argument order; a top-level `os.Stat` failure other than not-exist (e.g. an unreadable `~/Documents`-adjacent parent) has no Surface & Copy row — Step 6 gives it R7's template naming the bundle path itself, pending a ruling; the repointed `Test_run_reports_exit_1_when_sync_fails` fixture.

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_test.go` `Test_run_refuses_a_bad_quicken_path` — table-driven over the 6 rows: missing/R4; `.QDF` suffix/R5 (a **symlink** named `…/Home.QDF` pointing at a real `v9fixture.OpenBundle` bundle — see Mutation checks); plain file at the path/R6; a directory without `data`/R6; `data` unreadable (`chmod 0o000` on `data`, skip if `os.Geteuid()==0`)/R7; valid bundle passed as `~/Documents/Home.quicken`/success. Each refusal row asserts exact stderr (quote from spec `:226-229`), empty stdout, exit 1, and `os.Stat` on the snapshots dir returns not-exist. Success row asserts exit 0 as in `Test_run_writes_a_verified_snapshot_and_reports_success` (`cmd/quarry/run_test.go:23-52`).
- [x] Step 2: `internal/snapshot/bundle.go` (new) — stub `func ResolveBundlePath(home, path string) (string, error)` returning `(path, nil)`; `internal/cli/sync.go:45-49` `RunE` — call it before `srv.Sync`, error wrapped `&runtimeError{}` same as `srv.Sync`'s own error. This wiring does not change again — only `ResolveBundlePath`'s body does, in Build. Confirm the acceptance test fails at its stderr-text assertions (passthrough yields generic `Sync` errors, not R4-R7 text), not at compile or panic.

### Build
- [x] Step 3: `internal/platform/homepath/` (new: `doc.go`, `homepath.go`) — `Expand(home, path string) string` (expands a leading `~/` only, per `:89`; anything else unchanged) and `Abbreviate(home, path string) string` (move body from `render.go:12-22`, same argument order as `Expand`). `homepath_test.go`: move the 3 `Test_abbreviateHome_*` cases from `render_internal_test.go:13-28` as `Test_Abbreviate_*`; add `Test_Expand_*` (expands a leading `~/`; leaves absolute/relative-non-`~`/bare-`~` paths unchanged).
- [x] Step 4: `internal/cli/render.go:12-22` — delete `abbreviateHome`, call `homepath.Abbreviate` from `renderSuccess`; `render_internal_test.go` — delete the 3 moved tests, update its white-box doc comment.
- [x] Step 5: `internal/snapshot/refusal.go` (new) — `RefusalError` (value type, `msg` field, `Error() string` returns `msg` verbatim, no `quarry: ` prefix — `run.go` adds that). Doc it as the shared one-line-copy contract a refusal returns; no spec IDs in the comment.
- [x] Step 6: `internal/snapshot/bundle.go` — real `ResolveBundlePath`, in this order: `homepath.Expand`, then `filepath.Abs` unconditionally (it Cleans an already-absolute path too — a trailing slash from tab-completion would otherwise defeat the R5 suffix check; its only error is `os.Getwd` failing on a deleted cwd, which is reachable, not `// unreachable` — see Step 7). One `os.Stat` on the result: error → `fs.ErrNotExist` gives R4, any other error (e.g. `EACCES` on a parent directory — do **not** call `.IsDir()` on a nil `FileInfo` here) gives the flagged-deviation refusal naming the bundle path. Stat succeeded → case-insensitive `.qdf` suffix (`strings.EqualFold(filepath.Ext(path), ".qdf")`, checked on the path string itself so it fires for the R5 symlink regardless of its target) gives R5 **before** the directory check. Not a `.qdf` path and `!info.IsDir()` gives R6. Otherwise unexported `validateData(bundlePath string) error` — `os.Stat` `filepath.Join(bundlePath, "data")`: `fs.ErrNotExist` or stat-succeeds-but-not-`IsRegular` gives R6; any other stat error (e.g. `EACCES` walking into an unreadable bundle dir) gives R7; stat succeeds and is regular → `os.Open` it and close immediately without reading (a plain FS-level check, not through `platform/sqlite` — keeps BR-1's "one probe read plus the backup" true since no bytes are read and nothing is opened through SQLite here), error gives R7 using the innermost `*fs.PathError.Err.Error()` as the OS reason (not the full path-error string). Refusal text abbreviates the relevant path with `homepath.Abbreviate(home, _)` before formatting into `RefusalError`.
- [x] Step 7: `internal/snapshot/bundle_test.go` (new) — `Test_ResolveBundlePath_refuses_a_missing_path` (R4); `_refuses_when_the_top_level_path_cannot_be_statted` (parent dir `chmod 0o000`, skip-if-root, asserts an error and **no panic**, not `fs.ErrNotExist`); `_refuses_a_qdf_suffix` (plain files, both `.qdf` and `.QDF` — this fixture is what discriminates the R5-before-R6 ordering, see mutation check 3); `_refuses_a_plain_file` and `_refuses_a_bundle_without_data` (R6); `_refuses_unreadable_data` (`chmod 0o000` on `data`, skip-if-root, asserts the OS-reason substring, R7); `_refuses_an_unreadable_bundle_directory` (`chmod 0o000` on the bundle dir itself, skip-if-root, R7); `_accepts_a_symlinked_data_file` (data is a symlink to a regular file — success, guards a future accidental `Lstat`); `_resolves_a_relative_path_to_absolute` (`t.Chdir` into a valid bundle's parent, derive the expected path from `os.Getwd()` rather than the `TempDir` string — macOS's `Getwd` can return the `/private/var/...` form); `_expands_a_tilde_path_under_home`; `_returns_an_error_when_the_working_directory_no_longer_exists` (`t.Chdir` into a temp dir, `os.Remove` it, pass a relative path — if `Abs`/`Getwd` unexpectedly succeeds on this platform, report that rather than assume the branch is unreachable). The two `chmod 0o000` directory tests (`t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })` before removal) — `t.TempDir()` cannot clean up a `0o000` directory otherwise.

### Sweep
- [x] Step 8: `cmd/quarry/run_test.go:154-163` `Test_run_reports_exit_1_when_sync_fails` — repoint its fixture to a bundle that passes R4-R7 (valid shape, readable `data`) but holds non-SQLite bytes in `data`, so it still exercises `srv.Sync`'s own error path in `RunE`, not `ResolveBundlePath`'s.
- [x] Step 9: fix what `go build ./... && golangci-lint run ./...` reports (stale imports in `render.go`/`render_internal_test.go` after the move); doc comments on `ResolveBundlePath`, `RefusalError`, `homepath.Expand`/`Abbreviate`.

### Verify
- [x] Step 10: full verification per `.claude/rules/agent-briefs.md` → *Verification* + `.claude/scripts/spec-check.py phase0-snapshot` → tick SCENARIO-05 with its acceptance test.

## Handoff

**Binding decisions:**
- `ResolveBundlePath` (exported) and its unexported `validateData` helper live in `internal/snapshot` — SCENARIO-04's discovery calls `validateData` once it has picked a candidate from `~/Documents`, instead of re-implementing R5-R7.
- `internal/platform/homepath` (`Expand`/`Abbreviate`, both `(home, path string)`) is the one seam for home-relative path text — `internal/cli` no longer owns a local `abbreviateHome`. Every later refusal scenario needing a `~`-abbreviated path (R8-R14) imports `homepath`.
- `RefusalError` (`internal/snapshot`, value type, `Error()` excludes the `quarry: ` prefix) is the established shape for a final, exact-one-line refusal.
- Resolution happens in `internal/cli/sync.go`'s `RunE`, before `srv.Sync` — `Sync`'s own signature and BR-8 step numbering are untouched; a refusal from `ResolveBundlePath` never reaches `Destination.Prepare`. Check order inside `ResolveBundlePath` is Stat → R4 → (other-stat-error) → R5 → R6 → `validateData` (R6/R7) — R5 must stay ahead of the directory check or a real `.QDF` file (not a directory) gets R6's copy instead.

**Left unbuilt:**
- `DiscoverBundle`/R1-R3 (`~/Documents` scan) — SCENARIO-04.
- `Server` has no `home` field — the first refusal raised *inside* `Sync` (SCENARIO-06, R8) must add a `WithHome` option, wired from `cmd/quarry/run.go`'s existing `home`, or it cannot build `~`-abbreviated copy there.

**Orchestrator rulings on plan deviations (accepted, applied as built):**
1. A top-level `os.Stat` failure other than not-exist is R7b, now ruled and in `specification.md` → *Surface & Copy* (same wording as R7, naming the bundle path itself instead of `<path>/data`). Implemented verbatim via `unreadableRefusal` in `bundle.go`.
2. `internal/platform/homepath` extraction with `Abbreviate(home, path)` — accepted as planned.
3. `Test_run_reports_exit_1_when_sync_fails` repointed to a valid-shape bundle (passes R4-R7) with non-SQLite bytes in `data` — accepted; it now exercises `srv.Sync`'s own error path, not `ResolveBundlePath`'s.

**Traps:**
- `Sync` still wraps every error it returns as `fmt.Errorf("sync %s: %w", bundlePath, err)`. SCENARIO-05's own refusals never pass through `Sync` so this doesn't bite here — but R8-R14 (later scenarios) raise refusals *inside* `Sync`. Whoever builds those must return the `RefusalError` unwrapped from `Sync` (or unwrap via `errors.As` before printing), or the `sync <path>: ` prefix corrupts the exact-copy contract.
- `os.Stat` already follows symlinks — do not add a separate `Lstat` check for "symlinks followed"; `_accepts_a_symlinked_data_file` guards this.
- The R7 readability check is a plain `os.Open`+immediate-`Close`, never a read. A successor who moves this into `Sync` as literal "BR-8 step 1" must keep it that way — closing any fd drops every fcntl lock the process holds on that file, so doing this after `Source.Open` would drop SQLite's lock mid-backup.
- `t.TempDir()` resolves through `/private/var` on macOS; do not resolve symlinks in `ResolveBundlePath` (no `filepath.EvalSymlinks`) or `~`-abbreviation against an unresolved injected `HOME` breaks in tests.
