---
id: SCENARIO-12
status: open
---

# SCENARIO-12: Uninstall keeps the marketplace for a project copy (folds SCENARIO-26)

Cadence: test-first — the keep rule is a deletion guard: it decides whether `claude plugin marketplace remove` (a delete of a marketplace some project copy still resolves against) runs (`build.md` → *Build cadence*, write-safety guards). Whole scenario stays test-first.
Acceptance test: `internal/cli/claude_uninstall_test.go` `Test_claude_uninstall_keeps_the_marketplace_for_a_project_copy`
Acceptance test (SCENARIO-26, folded): `internal/cli/claude_uninstall_test.go` `Test_claude_uninstall_shows_a_project_path_outside_home_unabbreviated`
Narrow loop: `go test ./internal/claudeplugin/ ./internal/cli/ -run 'uninstall|read_state'`
Mutation checks (invariant: **`marketplace remove` runs only when the plugin list just read holds no `quarry@quarry` entry whose scope is anything other than exactly `user`** — decided by id equality and scope inequality, no name matching; scope strings `project`, `local`, `managed`, any unknown string all keep; a missing/non-string/empty `projectPath` never changes the decision, only the hint variant):
- drop the keep guard (`st.marketplaceOurs` alone) → `Test_uninstall_keeps_the_marketplace_while_a_non_user_copy_remains` + acceptance
- scope test `!= userScope` → `== "project"` → `local`/`managed` rows of the same test
- count a user-scope entry as remaining (drop scope filter) → `Test_uninstall_runs_only_the_steps_that_are_present` both-present row
- count `quarry@other` project entry as remaining → "another plugin's project copy is not ours" row
- hint order reversed → `Test_claude_uninstall_hints_each_remaining_copy` three-entry row
- `projectPath` branch swapped (named ↔ did-not-name) → both variant rows (absent key, non-string, `""`)
- `Kept` counted in `Ran()` → project-only row (no restart line); user step ran + Kept still prints the restart line (acceptance)
- hint path via `homepath.Abbreviate` directly (not `claudePath`) → fold-26 test
Runs: A (1-2) | B1 (3) | B2 (4) | V (5-6)
Size: OWNS A RUN — 2 batches, 1 feature package (`internal/claudeplugin`). Sizing row said 3; the home batch is dead: `Env.Home` (`run.go:57`), `claudePath` (`render_claude.go:126-132`), `defaultEnv` wiring (`cmd/quarry/run.go:193-205`) and its pin `Test_the_shipped_claude_install_reports_a_claude_it_cannot_run` already exist. No new wiring.

Surveyed surface (no new port): `readState` is the only list reader; `UninstallResult` has 2 bools + `Ran()`; callers of `Uninstall`/`UninstallResult` = `claude_uninstall.go:37-41`, `render_claude.go:94-124` (LSP findReferences, developer re-confirms).

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `claude_uninstall_test.go` (after `:57`) `Test_claude_uninstall_keeps_the_marketplace_for_a_project_copy` — `runClaudeAt` home `/home/ada`, our marketplace + user copy + project copy `projectPath:/home/ada/repos/foo`; asserts argv has no remove, stdout Uninstalled/Kept/restart, stderr = the one R6 hint with `"~/repos/foo"`, nil err. Needs a `projectPlugin` fixture const beside `userPluginOn` (`claude_install_test.go:30-33`)
- [ ] Step 2: same file `Test_claude_uninstall_shows_a_project_path_outside_home_unabbreviated` — home `""`, `projectPath:/srv/x` → `"/srv/x"` in hint. Both red at the stdout/stderr assertion, no stubs needed (cli compiles today)

### Build
- [ ] Step 3 (B1, test-first): `state.go:78-83,112-118` `state` + `readState` collect each non-user `quarry@quarry` entry in list order as exported `Copy{Scope, ProjectPath string}` (nil when none, never `[]Copy{}`: `assert.Equal` tells them apart; a non-string/missing `projectPath` is `""`); `claudeplugin.go:96-107` `UninstallResult` gains `MarketplaceKept bool` + `Remaining []Copy` (`Ran()` unchanged: Kept does not count); `claudeplugin.go:123-135` `Uninstall` sets `res.Remaining = st.others` after the plugin step, then runs the marketplace step iff `st.marketplaceOurs && len(st.others)==0`, else `MarketplaceKept = st.marketplaceOurs` (guard stays a one-line condition on state, no name compare). Tests in `uninstall_test.go` (reuse `newFakeClaude`): `Test_uninstall_keeps_the_marketplace_while_a_non_user_copy_remains` rows — project / local / `managed` scope each keeps and records `Copy`; no user copy + project copy → no plugin step, no remove, Kept; marketplace absent + project copy → no remove, `MarketplaceKept` false, `Remaining` set; `quarry@other` project and a user-scope-only list do not keep; `projectPath` absent / number / `""` → `ProjectPath ""`; two copies keep list order; plugin step exits non-zero with a project copy → error path leaves `Kept`/`Remaining` zero and no remove (fault); Ran false with only Kept. Existing `Test_uninstall_runs_only_the_steps_that_are_present` rows stay as they are (`Remaining` nil)
- [ ] Step 4 (B2): `render_claude.go:32-36,92-107` `marketplaceKeptLine` const (ruled copy, spec `## Surface & Copy`) chosen over Removed/Absent in `renderUninstalled` (Kept first); `render_claude.go` add `uninstallRemainingHints(home, res) []string` composing the two R6 variants (`%q` over `claudePath(home, ProjectPath)`, `<scope>` verbatim) in `Remaining` order; `claude_uninstall.go:41` after `writeResult`, loop `writeClaudeLine(cmd, uninstallCommand, hint)` (stdout first, as install's R2/PATH lines `claude_install.go:41-46`). Tests in `claude_uninstall_test.go`: `Test_claude_uninstall_hints_each_remaining_copy` table (stdout, stderr, argv): R8 project-only no user copy → `not installed`/Kept/no restart/no uninstall child; user+project (acceptance row, restart present); marketplace absent + project → `not in Claude Code`, hint, no Kept; two project copies + one `local` without `projectPath` in list order; `local` scope verbatim in `--scope local`; path == home → `~`; outside home with home set → raw; home `""` → raw; path with a `"` and space (`%q` escaping, abbreviated first). `--json`/args n/a: refusals precede the Server (`claude_uninstall.go:26-35`). Existing `Test_claude_uninstall_reports_each_outcome` / `_help_prints_the_ruled_text` rows stay byte-identical (uninstall Long's "names each one" is now true; do not reword)

### Sweep
- [ ] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `Copy`, `Remaining`, `MarketplaceKept`; update `Uninstall` doc comment (keep rule, new fields)

### Verify
- [ ] Step 6: `.claude/scripts/verify.sh <start> ./internal/claudeplugin/... ./internal/cli/...`; `spec-check.py mcp-install`; tick SCENARIO-12 (and its fold) in `specification.md` with both test names, ref last; rewrite `STATE.md` (drop the BR-6 / R6 Left-unbuilt entries and the "Until SCENARIO-12" trap)

## Handoff

**Binding decisions**
- `Copy{Scope, ProjectPath}` + `UninstallResult{MarketplaceKept, Remaining}` carry non-user copies out of `claudeplugin`; cli composes all hint/Kept copy (`render_claude.go`) — copy stays out of the feature package (SCENARIO-01 rule). `Remaining` is every non-user `quarry@quarry` entry whether or not the marketplace is ours; `MarketplaceKept` is only "ours and not removed"
- Keep invariant above is the whole BR-6 guard; scope is compared against `user` only, so an unknown scope keeps (fails safe toward not deleting)
- Derived, not newly worded: marketplace absent + project copy → the existing "not in Claude Code" line plus the hints, no Kept line (Kept would claim a marketplace that is not there). Orchestrator: confirm, else rule copy before B2
- Kept never counts toward the restart line (BR-7); hints print only on success, after stdout

**Left unbuilt**
- group unknown-subcommand `Args` + RunE, group Long pin, 27/27b, `claude` Short rewrite, README (18) — SCENARIO-17

**Traps**
- `Remaining` must be nil, not empty, when no copy exists: existing `assert.Equal(…UninstallResult{…})` rows break otherwise
- `homepath.Abbreviate("", "/srv/x")` returns `~/srv/x`; only `claudePath` guards it
- `%q` wraps the abbreviated text; abbreviating after quoting yields `"~..."` wrong for `"` paths
- No uninstall fixture today carries a non-user entry; do not add one to the `reports_each_outcome` table (its stderr is asserted empty)
