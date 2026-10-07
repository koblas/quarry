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

**Leaf-name diff is the real "no case dropped" gate.** `test-stats --run`'s pass column counts table parents too, so it can rise while a case vanishes; its tempdir/disk columns drop falsely when `t.TempDir()` moves into a helper (say so in the commit, don't chase it). Record leaf names before editing and diff after:

```bash
go test -count=1 -json <pkg> > $TMPDIR/<pkgname>-<phase>.json
python3 -c '
import json,sys; n=set()
for l in open(sys.argv[1]):
    e=json.loads(l)
    if e.get("Action")=="pass" and e.get("Test"): n.add(e["Test"])
print("\n".join(sorted(x for x in n if not any(m.startswith(x+"/") for m in n))))' $TMPDIR/<pkgname>-<phase>.json > $TMPDIR/<pkgname>-<phase>-leaves.txt
```

Every leaf missing after must map to a named new case (list the mapping for collapsed groups in your report), or be a named duplicate. Also diff string literals and `assert.`/`require.` lines old→new for each collapsed group.

## Style lessons from the pilot (internal/importer, commits caa71c2b..95a64bec — read its `helpers_test.go` and `statements_test.go` as the model)

- Work in three commits: (1) pure file moves/merges, identical leaf set; (2) hoist helpers, no rename/removal; (3) table collapses + duplicate removal.
- Helper params are `tb testing.TB` (the `thelper` linter requires the name).
- A helper may own `require.NoError` / error-class checks only if its name says so (`mustImport`, `importRefused`); never an assertion on result data. Every test keeps at least one visible assertion in its Then — no test whose only Then is a helper call.
- Tables vary **data** (inputs + exact expected values). Don't build tables of setup closures; keep separate tests when only the setup differs.
- Don't rename existing subtests just to normalise style — that changes leaf identity.
- Fixture builders that seeded rows implicitly behind a test's back: make the seeding explicit in Given.
- Prose file references in specs (not Progress lines) may go stale; leave them.

## Commit

One commit (or a few, each green) per package/group: `test(<pkg>): consolidate test files and share helpers`. Body: files before→after, test lines before→after, top-level tests before→after, leaf PASS before→after, coverage % before→after, any removed duplicate named. End with the attribution trailer you were given.

## Report back

The same numbers, list of new helpers, any cited-spec paths updated, anything left alone and why.
