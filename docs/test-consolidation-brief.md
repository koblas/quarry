# Test consolidation brief

Refactor of `_test.go` files only, package by package. No production code changes, no behaviour change.

## Goal

1. **Fewer, larger files.** Merge small test files that cover one production file / one feature into one file named after it (e.g. `recurring_new_test.go`, `recurring_state_test.go`, `recurring_order_test.go` → `recurring_test.go`). Target: roughly one test file per production file or per command, plus one `helpers_test.go` (black-box) and at most one `helpers_internal_test.go` (white-box).
2. **Shared code.** Hoist duplicated setup (fixture builders, store seeding, run-and-capture wrappers, assertion helpers) into the package's helpers file. Helpers take `t testing.TB` and call `t.Helper()`.
3. **Table-driven where tests differ only in data.** Collapse near-identical top-level tests into one table test with `t.Run` subtests.
4. **Read-only fixtures built once** (`sync.Once` or a package-level lazy builder) when several tests build the same expensive immutable fixture. Never share a mutable fixture across tests.

## Hard rules

- **Spec-cited tests are frozen.** `docs/specifications/*/specification.md` Progress lines cite `` `path/file_test.go` `Test_name` ``. Any top-level test named there keeps its exact name and stays a top-level func (never folded into a table). It may move files — if it does, update the cited path in every `specification.md` that names it, in the same commit. Find them with:
  `grep -rhoE '`[^`]+_test\.go` `[^`]+`' docs/specifications/*/specification.md | grep '<pkgdir>/'`
  Do not touch `SCENARIO-*.md`, `STATE.md`, `METRICS.md`, `REVIEW-*.md` (audit trail).
- **No assertion weakens.** Every expected value stays exact. Never replace equality with `Contains`, never drop an assertion, never drop a case. When merging two tests that assert different things, the result asserts the union.
- **Package split stays.** `package x` (`*_internal_test.go`) and `package x_test` files cannot merge with each other.
- `t.Chdir` / `t.Setenv` tests cannot be `t.Parallel()`. Keep existing `t.Parallel()` calls; don't add one to a test that touches process state.
- Subtest names: lower_snake like the existing `Test_` names.
- Coverage of production code must not drop. Leaf test count (subtests included) must not drop, except an exact duplicate you remove and name in the commit message.
- Lint: `golangci-lint fmt ./...` then `golangci-lint run ./...` must exit 0 with `0 issues`. Never edit `.golangci.yaml`; `//nolint` only per `.claude/rules/agent-briefs.md`.
- Only touch `_test.go` files in your assigned package (and spec citation paths). A helper genuinely needed by several packages: report it, do not create a shared package.

## Measure (before you edit, and after)

From repo root, with `<pkg>` like `./internal/importer/`:

```bash
go test -count=1 -coverprofile=$TMPDIR/<pkgname>-before.out <pkg> && go tool cover -func=$TMPDIR/<pkgname>-before.out | tail -1
.claude/scripts/test-stats.py --run <pkg>
ls <pkgdir>/*_test.go | wc -l ; cat <pkgdir>/*_test.go | wc -l
```

Repeat with `-after` at the end. Also run `.claude/scripts/test-stats.py --base <start> <pkg>` and `.claude/scripts/spec-check.py --run <every slug whose spec cites this package>`; spec-check problems must not exceed the baseline in `docs/test-consolidation-spec-baseline.log` for that package.

## Commit

One commit (or a few, each green) per package/group: `test(<pkg>): consolidate test files and share helpers`. Body: files before→after, test lines before→after, top-level tests before→after, leaf PASS before→after, coverage % before→after, any removed duplicate named. End with the attribution trailer you were given.

## Report back

The same numbers, list of new helpers, any cited-spec paths updated, anything left alone and why.
