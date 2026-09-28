---
id: SCENARIO-10
status: open
---

# SCENARIO-10: Missing tables or columns fail the schema check (folds SCENARIO-09)

Size verdict: OWNS A RUN, absorbing SCENARIO-09 (FOLD, decided at scoping) — extras-only is a
thin variant of the same outcome-policy branch and shares every renderer this scenario adds.

Cadence: code-first (no bug fix, no write-safety guard, no new atomic adapter — BR-9 is a
pure decision on data `Sync` already computes).
Acceptance test: `cmd/quarry/run_test.go` `Test_run_reports_a_schema_mismatch`
Acceptance test (SCENARIO-09, folded): `cmd/quarry/run_test.go` `Test_run_reports_extra_schema_only_as_a_warning`
Narrow loop: `go test ./internal/platform/sqlschema/... ./internal/snapshot/... ./internal/cli/... ./cmd/quarry/... -run 'CountPhrase|Mismatch|Extras|Warning|SchemaLine|schema_mismatch|extra_schema|verified_false|missing_column|extra_tables_only'`
Mutation checks:
- `!manifest.Schema.Verified` branch gating `mismatchError` in `Sync` → `Test_run_reports_a_schema_mismatch` (exit code) must redden if removed or inverted.
- "no warning when missing is also present" guard → `Test_sync_does_not_populate_warnings_when_extras_are_accompanied_by_missing_entries` must redden on its own if removed (verify individually, per proof.md).

No new port/interface/adapter — `Source`/`Destination` are unchanged, so the surface-survey
step does not apply.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `internal/quicken/v9/v9fixture/fixture.go` (new funcs after `EmptyAccountsBundle`, ~line 88) `MissingSchemaBundle`, `ExtraSchemaBundle` — closed, non-WAL, 1 `ZACCOUNT` row, built from `v9.ReferenceDDL` then mutated (drop 1 table + 2 columns + add 1 column for Missing; add 1 table + 2 columns for Extra), never touching `ZACCOUNT`; pick dropped/added columns that no `CREATE INDEX` in `reference.sql` references and that are not a PK/UNIQUE/FK (SQLite refuses `DROP COLUMN` on those) — use the fixture's own constants, not the spec's illustrative example names (`ZMEMO`/`ZCUSIP` do not exist in the reference). Export the touched identifiers as constants. `fixture_test.go`: one test per builder asserting the actual delta via a direct schema query.
- [ ] Step 2: `cmd/quarry/run_test.go` (new) `Test_run_reports_a_schema_mismatch`, `Test_run_reports_extra_schema_only_as_a_warning` — exit code, full stdout block (rows, `-` before `+`, missing table's columns not repeated), exact stderr line built from the fixture's constants, snapshot+manifest files still present, and the manifest's `warnings` field: `[]` for the mismatch case, `[<W1 body, no "quarry: warning: " prefix>]` for the extras-only case. Must fail at these assertions (today: exit 0, no rows, no stderr line), not at compile.

### Build
- [ ] Step 3: `internal/platform/sqlschema/phrase.go` (new) `CountPhrase(tables, columns int) string` — "N table(s) and M column(s)", zero-clause omission, singular/plural. `phrase_test.go`: table test, 0/1/N tables × 0/1/N columns, excluding 0/0 (never called — no diff-clause is rendered when both counts are zero, so no copy is ruled for it; don't invent a string).
- [ ] Step 4: `internal/snapshot/outcome.go` (new) `MismatchError` (value type, `Error()` excludes "quarry: "), `mismatchError(home, bundlePath, snapshotPath string, info SchemaInfo) MismatchError`, `extrasWarningText(bundlePath, manifestPath string, info SchemaInfo) string` — both via `sqlschema.CountPhrase`; `mismatchError` abbreviates `snapshotPath` with `homepath.Abbreviate`, `extrasWarningText` does not (manifest is named by basename). `snapshot.go:133-164` `Sync` — after `manifest.Snapshot.Path/Manifest` set (line 142) and before `Encode` (line 144): set `manifest.Warnings` from `extrasWarningText` only when unexpected counts > 0 and missing counts == 0; after `CommitSnapshot` (line 162), if `!manifest.Schema.Verified`, return `manifest, mismatchError(...)` instead of `manifest, nil`. `Sync`'s doc comment must state it now returns a populated `Manifest` alongside a non-nil error for a schema mismatch.
- [ ] Step 5: `internal/snapshot/sync_test.go` — repoint the two existing tests that assert `require.NoError` on a verified-false `Sync` (`Test_sync_reports_verified_false_when_the_reference_names_a_table_the_bundle_lacks`, lines 57-73; `Test_sync_reports_a_missing_column_when_the_reference_names_one_the_bundle_lacks`, lines 75-90) to `errors.As` into `MismatchError` instead, and add the files-still-on-disk assertion to each. Extend `Test_sync_stays_verified_and_lists_unexpected_tables_when_the_bundle_has_extra_tables_only` (lines 92-107) with the `Warnings` assertion. Add `Test_sync_does_not_populate_warnings_when_extras_are_accompanied_by_missing_entries` (combine both reference mutations). Run `findReferences` on `(*Server).Sync` first to confirm no other test still asserts `NoError` on a verified-false result.
- [ ] Step 6: `internal/cli/render.go:39-57` `schemaLine`/`renderSuccess` — extend `schemaLine` to the missing (with optional ", N not in reference" clause) and extras-only ("plus … not in it") states via `sqlschema.CountPhrase`; add a diff-row writer, label field width 8, `-` rows (tables then columns) before `+` rows (tables then columns). `render_internal_test.go`: table test over Schema-line wording (exact / missing-tables-only / missing-columns-only / missing-both / missing+extras / extras-only) plus a row-ordering test.
- [ ] Step 7: `internal/cli/sync.go:46-74` `RunE` — after `srv.Sync`, classify via `errors.As(err, &mismatch)`; when `err != nil && !mismatch`, unchanged early return; otherwise render stdout (block or `--json`) from the returned manifest regardless of mismatch, then print each `manifest.Warnings` entry as `"quarry: warning: "+entry` to `cmd.ErrOrStderr()`, then (if mismatch) return `&runtimeError{err: err}` for the exit-1 mapping already in `cmd/quarry/run.go`. `cmd/quarry/run_test.go`: add `Test_run_reports_a_schema_mismatch_as_json` asserting stdout is byte-identical to the written manifest file (BR-10) and carries `"verified":false` with non-empty diff arrays, exit 1.

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports; doc comments on `CountPhrase`, `MismatchError`, `Sync`'s new return contract, the two new fixture builders.

### Verify
- [ ] Step 9: full verification + `.claude/scripts/spec-check.py phase0-snapshot` → tick SCENARIO-10 and SCENARIO-09 (folded, note its acceptance test) with their acceptance tests.

## Handoff

**Binding decisions:**
- `Sync` returns `(Manifest, MismatchError)` — not `(Manifest{}, err)` — when the schema check
  fails: both snapshot and manifest are committed to disk first (BR-9 "kept"), and `RunE`
  renders stdout from the returned manifest in every case before checking the error, so the
  existing `runtimeError`/`Execute`/`run.go` exit-1 plumbing needed no changes.
- `Manifest.Warnings` holds W1's body only when unexpected tables/columns exist **and** no
  table/column is missing — a mismatch that also has extras never gets a warning, only M1.
- `sqlschema.CountPhrase(tables, columns int) string` is the one shared count-phrase builder
  for the stdout Schema line's two clauses, M1's body and W1's body; both `internal/snapshot`
  and `internal/cli` call it directly (platform package, no dependency-rule issue).

**Left unbuilt:**
- `v9fixture.MissingSchemaBundle`/`ExtraSchemaBundle` are this scenario's only fixture
  builders that mutate a real on-disk schema — no other scenario needs them.
- SCENARIO-11 (collision suffix), 12 (partial sweep), 13 (R13) remain fully unowned.

**Traps:**
- M1 and W1 name the **bundle** by `filepath.Base` in both, but differ on the other path: M1
  gives the **snapshot** as a `~`-abbreviated path (`homepath.Abbreviate`, like every R-series
  refusal), W1 gives the **manifest** as a bare `filepath.Base`. Don't apply one rule to both.
- Warnings text excludes the *entire* "quarry: warning: " prefix, not just "quarry: " — `RunE`
  must prepend both words itself; `run.go` never adds it (unlike the error path).
- `ALTER TABLE ... DROP COLUMN` needs SQLite ≥3.35 (mattn/go-sqlite3 v1.14.52 satisfies this)
  and refuses a column that is a PK/UNIQUE/FK or referenced by a `CREATE INDEX`, trigger, or
  view — check `reference.sql`'s indexes before picking which column to drop, and assert the
  delta happened rather than trust the statement silently.
- The spec's illustrative names (`ZLOT`, `ZCASHFLOWTRANSACTIONENTRY.ZMEMO`, `ZSECURITY.ZCUSIP`,
  `ZACCOUNT.ZNEWFLAG`, `ZTAG.ZCOLOR`, `ZNEWENTITY`) are copy examples, not real reference
  entries — `ZMEMO`/`ZCUSIP` don't exist in `reference.sql`. Fixtures use their own constants.
- Never drop or alter `ZACCOUNT` in the new fixtures — `buildManifest`'s content checks
  (table exists, ≥1 row) run before the schema diff and must stay satisfied.
