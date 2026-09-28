---
id: SCENARIO-04
status: done
---

# SCENARIO-04: Bundle discovery in ~/Documents

Cadence: code-first
Acceptance test: `cmd/quarry/run_test.go` `Test_run_discovers_the_bundle_from_documents_without_quicken`
Narrow loop: `go test ./cmd/quarry/... ./internal/snapshot/... ./internal/cli/... -run 'Test_run_discovers_the_bundle_from_documents_without_quicken|Test_DiscoverBundle'`
Mutation checks: (1) remove the dotfile-skip filter → `Test_DiscoverBundle_ignores_dotfiles_and_non_directories_and_returns_the_only_bundle` reddens (its `.Hidden.quicken` decoy would then count). (2) remove the `os.Stat`/`IsDir` check on each name-matched candidate → the same test's `Plain.quicken` regular-file decoy would then count too, turning its expected success into an R2 refusal. (3) replace the exactly-one branch's call to `ResolveBundlePath` with a bare `return path, nil` → `Test_DiscoverBundle_refuses_when_the_only_match_has_no_data_file` reddens (an empty `Home.quicken` directory would be accepted instead of refused with R6).

Size verdict: OWNS A RUN — one Gherkin `When` (Scenario Outline, 4 rows = one acceptance test, table-driven), one new feature-package file pair plus a two-line `RunE` change.

## Implementation Plan

Reused: `internal/snapshot/bundle.go:22-57` `ResolveBundlePath` and its `osReason` helper (`bundle.go:103-110`, unexported, same package) for R3's OS reason; `internal/snapshot/refusal.go` `RefusalError`; `internal/cli/sync.go:23-73` `newSyncCommand`/`RunE` (today always calls `ResolveBundlePath`) and its `MarkFlagRequired("quicken")` (`:71`, removed here); copy source `specification.md:223-225` (R1-R3 exact text, note R1 says "no `*.quicken` **directory**"), `:337-343` (outline), `:245` (Source line identical for discovered vs `--quicken`).

**Deviations to flag before dispatch (unruled by product-vision):**
1. **Suffix case sensitivity.** Ruled case-sensitive literal `.quicken` (unlike the explicit case-insensitive `.qdf`/`.QDF` check R5 already documents) — Windows' `.QDF` case variance is a known cross-platform fact; quarry's own bundle suffix has no such precedent in the spec. A `.QUICKEN` directory is silently invisible to discovery.
2. **Missing `~/Documents` itself** (vs. an empty one). Ruled: same outcome as R1 (`os.ReadDir` on a missing dir returns `fs.ErrNotExist`, folded into the zero-bundle branch) — spec's R1 row only names "no `*.quicken` directory", not "no `Documents`".
3. **Reusing `ResolveBundlePath` for the sole match, rather than calling `validateData` directly.** STATE.md's Left-unbuilt note names `validateData`; this plan calls the exported `ResolveBundlePath` instead — a strict superset (it also re-runs the `.qdf`-suffix and existence checks, both no-ops on a name already matched and stat'd) that reuses the R6/R7 error mapping without duplicating it. Flagged as a named departure from STATE.md, not silently substituted.
4. **R3's exact copy.** The spec table row types a literal `operation not permitted`; a `chmod 0o000` fixture actually raises `EACCES` ("permission denied") on macOS/Linux. Ruled: OS-reason-verbatim (matching every sibling unreadable-path refusal via `osReason`), so the row's *test* asserts "permission denied", not the spec table's example wording — both are the same rule, applied to different real permission errors.
5. **Flag-given detection.** Uses `cmd.Flags().Changed("quicken")`, not `quickenPath == ""`, to choose discovery — so an explicit `--quicken ""` is sent to `ResolveBundlePath` (where it fails un-ambiguously) rather than silently falling back to discovery.

**Ruled (spec-explicit, not a deviation):** discovery counts only top-level **directories** named `*.quicken` (R1's own wording), symlinks followed — each name-matched entry is `os.Stat`'d (not `DirEntry.IsDir()`, which does not follow symlinks) and skipped if the stat fails or it is not a directory. This mirrors R6's already-established "symlinks followed" behavior and keeps a stray `*.quicken` regular file from displacing a real bundle sitting beside it.

---

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_test.go` `Test_run_discovers_the_bundle_from_documents_without_quicken` — table-driven over the outline's 4 rows, run with `quarry sync` (no `--quicken`), `HOME` set to a temp dir: no bundle in `~/Documents` (dir absent, or empty)/R1; exactly one valid bundle (`v9fixture.OpenBundle`)/exit 0, and its stdout `Source` line matches the same `~`-abbreviated bundle path `Test_run_writes_a_verified_snapshot_and_reports_success` (`run_test.go:24-54`) produces for an explicit `--quicken`; three bundle directories named `Business.quicken`, `Home.quicken` (the real `v9fixture` bundle), `Old.quicken` (empty dirs suffice for the other two), created in a non-alphabetical order, asserting R2's exact spec text (names sorted bytewise, not by creation order); an unreadable `~/Documents` (`chmod 0o000`, `t.Cleanup` restores it, skip if `os.Geteuid()==0`)/R3 asserting the "permission denied" OS reason. Each refusal row asserts exact stderr, empty stdout, exit 1, and `os.Stat` on the snapshots dir returns not-exist (same shape as `Test_run_refuses_a_bad_quicken_path`, `run_test.go:77-153`).
- [x] Step 2: `internal/snapshot/discover.go` (new) — stub `func DiscoverBundle(home string) (string, error)` returning a placeholder `RefusalError`; `internal/cli/sync.go:45-49` `RunE` — call `DiscoverBundle(home)` when `!cmd.Flags().Changed("quicken")` (deviation 5), else `ResolveBundlePath` as today; remove `cmd.MarkFlagRequired("quicken")` (`:71`). Confirm the acceptance test's exactly-one and multi-bundle rows fail at their assertions (placeholder refusal), not at compile or a required-flag usage error.

### Build
- [x] Step 3: `internal/snapshot/discover.go` — real `DiscoverBundle`: `os.ReadDir(filepath.Join(home, "Documents"))`; `fs.ErrNotExist` on that call and a zero-length filtered result both give R1 (deviation 2); any other `ReadDir` error gives R3 via `documentsUnreadableRefusal(home, err)` (deviation 4, uses `osReason`); filter entries skipping a leading `.` in the name, requiring a `.quicken` suffix **case-insensitively** (orchestrator override of deviation 1: `strings.EqualFold(filepath.Ext(name), ".quicken")`, matching R5's `.qdf`/`.QDF` handling), then `os.Stat`ing the joined path and skipping any entry whose stat fails or is not a directory (Ruled note above) — `os.ReadDir` already returns entries sorted by name, so the filtered slice needs no further sort; exactly one surviving match calls `ResolveBundlePath(home, filepath.Join(documentsDir, name))` unchanged (deviation 3); more than one gives R2 via `multipleQuickenBundlesRefusal(names)` (count + comma-joined names, "choose one with --quicken <path>").
- [x] Step 4: `internal/snapshot/discover_test.go` (new, `snapshot_test` package, mirrors `bundle_test.go`'s shape) — `Test_DiscoverBundle_refuses_when_no_bundle_is_found` (table: `Documents` absent, and `Documents` empty — both assert R1's exact text); `Test_DiscoverBundle_ignores_dotfiles_and_non_directories_and_returns_the_only_bundle` (plants a `.Hidden.quicken` dir, a `Notes.txt` file, and a `Plain.quicken` regular file alongside the one real `v9fixture` bundle — success, returns the bundle's path; doubles as mutation checks 1-2's fixture); `Test_DiscoverBundle_finds_a_bundle_whose_suffix_case_differs` (a lone `Home.QUICKEN` directory with a plain `data` file — success; orchestrator-added row/mutation check 4 for the case-insensitive suffix match); `Test_DiscoverBundle_follows_a_symlinked_bundle_and_skips_a_dangling_one` (a `Broken.quicken` symlink to a nonexistent target, plus the real bundle — success on the real one, proving the stat-based skip is a handled fault, not a panic); `Test_DiscoverBundle_refuses_when_multiple_bundles_exist` (table: two arbitrary-named directories for the exactly-one/more-than-one boundary, then the spec's three names `Business.quicken`/`Home.quicken`/`Old.quicken` created out of alphabetical order — both rows assert R2's exact text with names sorted bytewise); `Test_DiscoverBundle_refuses_when_the_only_match_has_no_data_file` (a lone empty `Home.quicken` directory — asserts R6's exact text, proving the `ResolveBundlePath` passthrough; mutation check 3's fixture); `Test_DiscoverBundle_refuses_when_documents_is_unreadable` (`chmod 0o000` on `Documents`, `t.Cleanup` restores it, skip if root, R3, asserts "permission denied").

### Sweep
- [x] Step 5: `internal/cli/sync.go:20-22` — update `newSyncCommand`'s doc comment (currently says "requires --quicken") to state the Documents fallback; fix what `go build ./... && golangci-lint run ./...` reports; doc comment on `DiscoverBundle`.

### Verify
- [x] Step 6: full verification per `.claude/rules/agent-briefs.md` → *Verification* + `.claude/scripts/spec-check.py phase0-snapshot` → tick SCENARIO-04 with its acceptance test.

## Handoff

**Orchestrator rulings applied (deviations from this plan, before dispatch):**
1. `.quicken` suffix match is **case-insensitive** (`strings.EqualFold(filepath.Ext(name), ".quicken")`), overriding deviation 1's case-sensitive `strings.HasSuffix` — consistent with R5's `.qdf`/`.QDF` handling. Added `Test_DiscoverBundle_finds_a_bundle_whose_suffix_case_differs` and a 4th mutation check (below).
2. Missing `~/Documents` itself folds into R1 (deviation 2): accepted as planned.
3. Reusing `ResolveBundlePath` for the sole match (deviation 3): accepted as planned.
4. R3 uses the OS reason verbatim (deviation 4, like R7): accepted as planned.
5. `cmd.Flags().Changed("quicken")` detection (deviation 5): accepted as planned.

**Binding decisions:**
- `DiscoverBundle(home string) (string, error)` lives in `internal/snapshot/discover.go`, mirrors `ResolveBundlePath`'s signature shape, and delegates its sole-match case to `ResolveBundlePath` rather than re-implementing R6/R7 (deviation 3) — any later change to `ResolveBundlePath`'s validation order automatically covers discovery too.
- `--quicken` is no longer `MarkFlagRequired`; `internal/cli/sync.go`'s `RunE` branches on `cmd.Flags().Changed("quicken")` (deviation 5, not an empty-string check) to choose `DiscoverBundle` vs `ResolveBundlePath`. This closes STATE.md's interim note that bare `quarry sync` exited 2 (usage) rather than reaching R1-R3.
- Discovery counts only top-level **directories** named `*.quicken` **case-insensitively** (dotfiles skipped), stat'd (not `DirEntry.IsDir()`) so a symlinked bundle is followed and a dangling one or a same-named regular file is silently skipped rather than counted.

**Left unbuilt:** nothing new — SCENARIO-06 through 13 (R8-R14, collision suffix, `.partial` sweep) are unaffected by this scenario and remain as STATE.md already records.

**Traps:**
- `os.ReadDir` already returns entries sorted by filename — do not add a second sort call, and do not assume creation order in a test without a deliberate out-of-order fixture (Step 1/4 both plant one to catch a regression to unsorted output).
- `DirEntry.IsDir()` from `os.ReadDir` reflects the entry's own `Lstat`, so it reports `false` for a symlink even when its target is a directory — checking it instead of `os.Stat`ing the joined path would silently stop following symlinked bundles.
- A `*.quicken` regular file at the top level must not be filtered by name alone — the stat/`IsDir` check is what excludes it from the count; removing that check (mutation check 2) lets it collide with a real bundle and wrongly produce R2.
- The suffix match is case-insensitive, so `filepath.Ext(name) != ".quicken"` (a case-sensitive rewrite) silently drops any `.QUICKEN`/`.Quicken` bundle back to zero matches (mutation check 4).

**Mutation results (all four `Mutation checks:` guards, individually):**
1. Removed the dotfile-skip filter → `Test_DiscoverBundle_ignores_dotfiles_and_non_directories_and_returns_the_only_bundle` reddened (`.Hidden.quicken` counted, produced R2 instead of success).
2. Removed the `os.Stat`/`IsDir` check → the same test reddened (`Plain.quicken` counted, produced R2 instead of success).
3. Replaced the exactly-one branch's `ResolveBundlePath` call with `return filepath.Join(documentsDir, names[0]), nil` → `Test_DiscoverBundle_refuses_when_the_only_match_has_no_data_file` reddened (accepted an empty bundle instead of R6).
4. Replaced `strings.EqualFold(filepath.Ext(name), ".quicken")` with `filepath.Ext(name) != ".quicken"` (case-sensitive) → `Test_DiscoverBundle_finds_a_bundle_whose_suffix_case_differs` reddened (R1 instead of success).
Each mutation was applied to a backed-up copy of `internal/snapshot/discover.go`, run with `-run` against only its named test, then restored and diffed byte-identical against the backup.
