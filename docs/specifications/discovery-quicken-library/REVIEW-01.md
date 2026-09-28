# Review Report — round 01

### Target
Range `5ac3450..5dc65ac`. Coverage: 0 uncovered.

### Triggered reviewers
arch, correctness, test, refactor-advisor.

### MAJOR
1. **correctness** `internal/snapshot/discover.go:33` — Quicken location treats only ENOENT as missing; ENOTDIR (folder or ancestor is a regular file) becomes R3b, so a home that synced from `~/Documents` before now refuses with permissions advice. **Ruling (DQ-4 amended):** ENOTDIR on the Quicken location counts as missing (silent); `~/Documents` unchanged. Fix + test (Quicken parent a regular file, `~/Documents` has one → exit 0).

### MINOR
- **correctness** ELOOP on the Quicken folder gets R3b copy — accepted as is.
- **refactor** Dead `home` param on the two location refusal funcs; narrow the field type to `func(cause error) error`.
- **refactor** `scanForBundles` returns two error shapes; return a `RefusalError` for both (or not-exist) so `DiscoverBundle` drops the `errors.As` branch. (Fix-if-cheap; pairs with MAJOR.)
- **refactor** Doc/comment: drop "return here rather than scanning the second"; drop repeated "for a reason other than not existing".
- **test** Collapse `Test_run_discovers_the_bundle_from_documents_without_quicken` subtests into a table.
- **test** `run_test.go:384-390` use `quickenDocumentsDir(home)` / `filepath.Dir(...)` instead of literals.
- **test** Hoist triplicated root-skip in `Test_run_refuses_when_a_discovery_location_is_unreadable`.
- **test** `innermostCause` duplicates production's unwrap loop; assert template parts + raw cause text instead.

### NIT
- trailing-slash `$HOME` abbreviation; non-atomic two-folder listing — accepted, pre-existing/theoretical.

### Verdict: FAIL
