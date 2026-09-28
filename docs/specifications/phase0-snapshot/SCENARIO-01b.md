---
id: SCENARIO-01b
status: done
---

# SCENARIO-01b: quarry sync reports a verified snapshot

Cadence: test-first — Step 4 (the manifest self-path bug fix) is a defect in already-shipped SCENARIO-01a behaviour; its test must go red before the fix.
Acceptance test: `cmd/quarry/run_test.go` `Test_run_writes_a_verified_snapshot_and_reports_success`
Acceptance test (SCENARIO-02, folded): `cmd/quarry/run_test.go` `Test_run_prints_the_manifest_as_json_with_the_json_flag`
Acceptance test (SCENARIO-14, folded): `cmd/quarry/run_test.go` `Test_run_rejects_usage_errors`
Narrow loop: `go test ./cmd/quarry/... ./internal/cli/... ./internal/snapshot/... -run 'Test_run_|render|abbreviate|format|own_path|scoped_reference'`
Mutation checks: `run()`'s exit-code check (a declared `var ue cli.UsageError; errors.As(err, &ue)`) → `Test_run_rejects_usage_errors` must redden if inverted or removed

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_test.go` (new) — three tests, all red against the stub: `Test_run_writes_a_verified_snapshot_and_reports_success` (real `v9fixture.OpenBundle`, `t.Setenv("HOME", tmp)`, `run(ctx, []string{"sync","--quicken",bundle.Dir}, &stdout, &stderr)`; asserts exit 0, stderr empty, and stdout built from the **committed file itself** — `os.Stat`/`sha256.Sum256` on the actual snapshot file, a directory listing for its name, not from re-deriving expected values out of the manifest — including the pinned `82 tables, 1,835 columns`); `Test_run_prints_the_manifest_as_json_with_the_json_flag` (SCENARIO-02: stdout bytes equal the manifest file's bytes, exit 0); `Test_run_rejects_usage_errors` (SCENARIO-14, table-driven over its four rows plus a near-miss `synk` row, asserting no `"Did you mean"` text — exact stderr line + exit 2 + empty stdout each)
- [x] Step 2: `cmd/quarry/run.go`, `internal/cli/run.go` (new) — signature-only stubs `run(ctx context.Context, args []string, stdout, stderr io.Writer) int` and `cli.Execute(ctx context.Context, args []string, stdout, stderr io.Writer, srv *snapshot.Server, home string) error`, enough to compile and fail Step 1 at its assertions

### Build
- [x] Step 3: `internal/snapshot/manifest.go:31-40` `SchemaInfo` — add `ReferenceTables`, `ReferenceColumns int`, both tagged `json:"-"`; `snapshot.go:195-207` `schemaInfoFromDiff` — compute both from the one `scopeSchema(reference)` call already needed for `ReferenceFingerprint`; new `internal/snapshot/sync_test.go` `Test_sync_reports_the_scoped_reference_table_and_column_counts` pins `82`/`1835` (same numbers `scope_internal_test.go` already pins) and asserts `Manifest.Encode()` bytes are unchanged by the new fields
- [x] Step 4 (bug fix, test-first): `internal/snapshot/sync_test.go` `Test_sync_writes_a_manifest_whose_own_path_fields_match_where_it_is_committed` — reads the `.json` file back off disk and asserts its `snapshot.path`/`snapshot.manifest` equal the paths `Sync` returned (today they are `""` on disk: `snapshot.go:121` encodes `buildManifest`'s result before `Snapshot.Path`/`Manifest` are set at lines 143-144). See it red, then fix: add `FinalPaths(name string) (snapshotPath, manifestPath string)` to `Destination` (`ports.go:25-41`), pure name→path prediction; implement in `dirDestination` (`destination.go:17-99`, reusing its existing join convention) and in both test fakes (`sync_faults_test.go:136-152` `fixedPathDestination`, `:261-287` `partialFaultDestination`, which delegates to `real`); in `Sync` (`snapshot.go:108-145`), call it once right after computing `name`, set `manifest.Snapshot.Path`/`.Manifest` from it before `Encode`, and return exactly the manifest that was encoded — do not overwrite its path fields with `CommitManifest`/`CommitSnapshot`'s own return values afterward
- [x] Step 5: `internal/cli/render.go`, `render_test.go` (new) — unexported pure funcs `abbreviateHome(path, home string) string`, `formatMB(bytes int64) string`, `formatThousands(n int) string`, `renderSuccess(m snapshot.Manifest, home string) string` (reads `m.Schema.ReferenceTables`/`ReferenceColumns` directly, no separate count params; six-line block; Schema line handles the exact-match case only: `matches reference <label> (T tables, C columns)`); bound tests: 1 vs 2 accounts, 999 vs 1,000 (comma), MB rounding/grouping, `~` boundary (`/Users/dave` must not abbreviate `/Users/davex/…`)
- [x] Step 6: `internal/cli/errors.go` (new) — `UsageError struct{ msg string }` (exported, `Error() string`) for exit-2 messages that are already final and must not get the cobra-native hint appended
- [x] Step 7: `internal/cli/root.go`, `internal/cli/sync.go` (new) — `newRootCommand(srv *snapshot.Server, home string, jsonOut *bool) *cobra.Command`: root (`Use`/`Short`/`Long` verbatim, persistent `--json` bool, `SilenceUsage`/`SilenceErrors`/`DisableSuggestions` true, `CompletionOptions.DisableDefaultCmd = true`, cobra's default `help` subcommand left as-is); `sync` (`Short`/`Long`/`Example` verbatim, `--quicken` string with pflag backtick placeholder, `MarkFlagRequired("quicken")`, `Args` returns `UsageError{"sync takes no arguments; pass the file with --quicken <path>"}` for any positional arg — U1 verbatim); `RunE` calls `srv.Sync`, wraps any error in the unexported `runtimeError` (`Error()`/`Unwrap()`) so `Execute` recognizes it as already-classified; on success writes either `manifest.Encode()` bytes (`--json`) or `renderSuccess(...)` to stdout
- [x] Step 8: `internal/cli/run.go` — real `Execute`: builds a fresh command tree every call (no package-level `*cobra.Command`), `SetArgs`/`SetOut`/`SetErr`, `ExecuteContext`; classifies the returned error — a `UsageError` passes through unchanged; a `runtimeError` (`errors.As`) returns the original `err` unchanged (exit-1 default in `cmd/quarry`); anything else (cobra-native: unknown flag, missing value, required flag, unknown command) becomes `UsageError{err.Error() + "; Run 'quarry sync --help' for usage."}`
- [x] Step 9: `cmd/quarry/run.go` — real `run`: `os.UserHomeDir()` failure → `quarry: %s\n` + exit 1 directly, never through the `UsageError` check; `v9.Reference(ctx)` failure marked `// unreachable: Reference always executes the fixed, valid ReferenceDDL (see v9.Reference)`; builds `snapshot.NewServer(WithSnapshotDir(<home>/Library/Application Support/quarry/snapshots), WithReference(v9.ReferenceLabel, ref))`; calls `cli.Execute(ctx, args, stdout, stderr, srv, home)`; on its error, `fmt.Fprintf(stderr, "quarry: %s\n", err)` then a declared `var ue cli.UsageError; errors.As(err, &ue)` → exit 2, else exit 1
- [x] Step 10: `cmd/quarry/main.go` (new) — one line, `os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))`, marked `// unreachable: main is the process entrypoint; run is exercised directly by its own tests.`
- [x] Step 11: `cmd/quarry/run_test.go` — `Test_run_reports_exit_1_when_home_directory_cannot_be_resolved` (`t.Setenv("HOME", "")`); `Test_run_reports_exit_1_when_sync_fails` (nonexistent `--quicken` path — exercises the `runtimeError`/exit-1 default arm without inventing refusal copy)

### Sweep
- [x] Step 12: fix what `go build ./... && golangci-lint run ./...` reports; add `internal/cli/doc.go`; doc comments on exported `cli.Execute`, `cli.UsageError`, `snapshot.Destination.FinalPaths`, and `snapshot.SchemaInfo`'s two new fields

### Verify
- [x] Step 13: full verification (`go build ./...`, `go test ./...`, `go test -race ./cmd/quarry/... ./internal/cli/... ./internal/snapshot/...`, `golangci-lint run ./...`, `uncovered-diff.py`, `test-stats.py`) + `.claude/scripts/spec-check.py phase0-snapshot` → tick SCENARIO-01b, and SCENARIO-02/SCENARIO-14 ("delivered by SCENARIO-01b") each with their acceptance test

## Handoff

**Orchestrator rulings (all ACCEPTED before implementation):**
1. `Destination.FinalPaths` port amendment fixing the 01a manifest-path bug — accepted.
2. Interim `MarkFlagRequired` on `--quicken` until SCENARIO-04 — accepted.
3. U2 hint joined as `` quarry: <cobra message>; Run 'quarry sync --help' for usage. `` — accepted.
4. Keep `help` subcommand; disable completion command and suggestions — accepted.

**Binding decisions:**
- `Destination` gained `FinalPaths(name) (snapshotPath, manifestPath string)` — amends STATE.md's binding decision naming `Source`/`Destination`'s methods; STATE.md's line now includes it. It is a pure prediction assuming no collision suffix yet, and `Sync` now returns exactly the manifest it encoded rather than overwriting its path fields with `Commit*`'s results. **SCENARIO-11 must revisit both**: once `_2`/`_3` exist, the manifest's name has to reuse whatever suffix the snapshot's `Backup` actually picked, so `FinalPaths`'s call site likely moves to after collision is resolved.
- `cli.Execute` takes `home string` as a parameter (resolved once in `cmd/quarry/run.go` via `os.UserHomeDir()`); it never reads the environment itself. SCENARIO-04/05 reuse this same value for discovery and `~` expansion — do not add a second resolution point.
- `--quicken` is `MarkFlagRequired` in 01b (no discovery yet, ruling 2 above); bare `quarry sync` exits 2 (missing required flag) until SCENARIO-04 adds `~/Documents` discovery and removes the requirement, making it exit 1 via R1–R3 instead. No 01b test exercises bare `sync`.
- Exit-code classification: `UsageError` → 2, everything else → 1 default. Cobra's `help` subcommand is kept; `DisableSuggestions` and `CompletionOptions.DisableDefaultCmd` are both set (ruling 4 above).
- U2's hint join, `"; Run 'quarry sync --help' for usage."`, is inferred from the vocabulary, not literal spec text (ruling 3 above); confirm final copy at the last product-vision pass.

**Left unbuilt:**
- A relative or `~`-prefixed `--quicken` value is passed to `Sync` unresolved, so `Snapshot.Source` can end up non-absolute — SCENARIO-05 owns resolving it.
- `~/` expansion and R4–R7: SCENARIO-05. Discovery when `--quicken` is absent, R1–R3: SCENARIO-04.
- Warning line (W1), DIFFERS block, `.partial` sweep: SCENARIO-09/10/12. Refusal copy R8–R14, M1: SCENARIO-06 onward.

**Traps:**
- `os.UserHomeDir()`/`v9.Reference(ctx)` failures happen in `run()` before `cli.Execute` runs — routing them through the `UsageError` check would wrongly exit 2 on an unset `HOME`.
- Cobra's own `c.Find` failure (unknown command) returns a plain error straight from `ExecuteContext` — `Execute`'s default branch, not a named check, must catch it. Without `DisableSuggestions`, a near-miss like `synk` appends `"\n\nDid you mean this?\n\tsync"`, breaking the one-stderr-line rule.
- `SilenceErrors`/`SilenceUsage` on the root command alone suppress printing for every subcommand (cobra checks the root's flags, not the found command's).
- If a future scenario ever makes `FinalPaths`'s prediction diverge from what `Commit*` actually returns, `--json`'s re-encoded stdout will silently stop matching the file on disk (BR-10) — keep them equal by construction, not by coincidence.
