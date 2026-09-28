---
id: SCENARIO-02
status: open
---

# SCENARIO-02: --json reports the store result alongside the manifest (absorbs 10, 18)

Size verdict: OWNS A RUN. The work is one package (`internal/cli`), with command-slice tests in `cmd/quarry` and a doc sweep in `internal/snapshot`. It is sized for a Sonnet developer.

Cadence: code-first. Nothing here is on the mandatory test-first set, so no write guard, atomic adapter or bug fix is touched.
Acceptance test: `cmd/quarry/run_json_test.go` `Test_run_reports_the_store_result_alongside_the_manifest_as_json`
Acceptance test (SCENARIO-10, folded): `cmd/quarry/run_json_test.go` `Test_run_lists_never_reconciled_accounts_in_json_and_succeeds`
Acceptance test (SCENARIO-18, folded): `cmd/quarry/run_schema_test.go` `Test_run_reports_a_schema_mismatch_as_json`
Narrow loop: `go test ./internal/cli/ ./cmd/quarry/ -run 'JSON|Json|json|Money'`
Mutation checks: none

**Pending ruling. Get it before dispatching the developer.** The spec does not say what `payee` holds in `splits.mismatched[]` and `one_sided[]` when the transaction has no payee. The candidates are `""` and `null`; human output uses `(no payee)`. This needs a scoped `product-vision` copy ruling into specification.md → `--json`. The developer implements whatever value is ruled, and Steps 1, 4 and 6 assert it.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_json_test.go` (new) `Test_run_reports_the_store_result_alongside_the_manifest_as_json` — build the fixture with the v9fixture builder: one reconciled account that matches, one paired transfer, and one-sided legs of all three kinds (numeric link to nothing, a name that matches an account, a name that matches none). Copy the shape from `run_transfers_test.go:185-214`.
  - Run `sync --quicken … --json` and expect exit 0.
  - `snapshot` and `schema` decode equal to the manifest file's.
  - `warnings` equals the manifest's `warnings` followed by the W2 text (P1-11 "+ import warnings").
  - The `store` object matches a JSON literal via `assert.JSONEq`. That one assertion catches: money as a number instead of a 2-decimal string, a missing key instead of `null`, and leaked `source_id`/`closed`/`active` keys.
  - stderr carries the W2 line.
- [ ] Step 2: `internal/cli/json.go` (new) `renderJSON(snapshot.Outcome) ([]byte, error)` — signature-only stub, wired at `internal/cli/sync.go:88-94` in place of `outcome.Manifest.Encode()`. Step 1 must fail on its `store` assertion.

### Build
- [ ] Step 3: `internal/cli/json.go` — document DTOs and the `jsonMoney(cents)` renderer. Four decisions are already fixed (the full list is in Handoff):
  - key order is snapshot, schema, store, warnings;
  - `store` is a pointer with **no `omitempty`**;
  - nil lists are encoded as `[]`;
  - indentation matches `Manifest.Encode`.

  Mark the encode error `// unreachable:` the way `manifest.go:64-73` does.

  Unit tests go in `internal/cli/json_internal_test.go` (new):
  - `Test_jsonMoney` — a table covering 0, -1, -50 (sign with a zero integer part), 120417, and -100000000 (no thousands grouping).
  - `Test_renderJSON_encodes_absent_lists_as_empty_arrays` — nil `NeverReconciled`, `Balances.Mismatched`, `Splits.Mismatched` and `OneSided`, with a populated control arm.
- [ ] Step 4: `internal/cli/sync.go:80-85` — remove `|| *jsonOut` and the stale interim comment at `:80-81`, so a V1 failure writes the JSON with `"built": false` and then returns the V1 error.
  - Tests go in `run_json_test.go`: `Test_run_prints_the_unbuilt_store_as_json_when_validation_fails`. Use one balance mismatch and one split mismatch, plus one one-sided leg.
  - Assert:
    - `built: false`;
    - `path` is the absolute store path;
    - the `balances.mismatched[]` entry has `statement_date` `YYYY-MM-DD` and string `quarry`/`quicken`/`difference`;
    - the `splits.mismatched[]` entry has string `amount`/`splits_total`;
    - `one_sided[]` is present;
    - `warnings` has no W2 entry;
    - the stderr V1 line is exact;
    - exit is 1.
  - Fault test: `Test_run_reports_the_o1b_refusal_when_stdout_fails_during_a_v1_json_write`, using `failingWriter` (`run_success_test.go:28-30`) and asserting the O1b copy.
- [ ] Step 5: `run_json_test.go` `Test_run_lists_never_reconciled_accounts_in_json_and_succeeds` (fold 10) — one never-reconciled account that is **closed** and one that is **open and inactive**, so that hardcoded booleans cannot pass. `never_reconciled[]` must be `{id,name,currency,closed,active}` in name-then-source-id order. The Balances clause is unchanged, stderr is empty, and exit is 0.
- [ ] Step 6: `cmd/quarry/run_schema_test.go:131-157` `Test_run_reports_a_schema_mismatch_as_json` (fold 18) — rewrite the test.
  - Assert that the `store` key is **present** and its raw value is `null`. `parsed["store"] == nil` would also pass when the key is absent.
  - `snapshot`, `schema` and `warnings` equal the manifest's.
  - The existing schema, stderr and exit-1 assertions stay.
- [ ] Step 7: `cmd/quarry/run_success_test.go:120-139` `Test_run_prints_the_manifest_as_json_with_the_json_flag` — rewrite in place. With `store` deleted, the decoded stdout equals the decoded manifest file, and the key set is `snapshot, schema, store, warnings`. This closes the STATE.md debt. The test count is unchanged.

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports. Then correct the doc comments this change makes false:
  - `internal/snapshot/manifest.go:9-10` (`Manifest` "same bytes … printed to stdout");
  - `internal/snapshot/manifest.go:60-63` (`Encode` "stdout always identical");
  - `internal/cli/doc.go:1-3` ("--json manifest document").

  Also add doc comments on the new `json.go` symbols (unexported, 1–2 lines each).

### Verify
- [ ] Step 9: run full verification per `.claude/rules/agent-briefs.md` → *Verification* (race on `./internal/cli/ ./cmd/quarry/`), then `.claude/scripts/spec-check.py phase1-import-store`. Tick SCENARIO-02, and tick 10 and 18 as "FOLD → 02, delivered by SCENARIO-02", each with its acceptance test. In STATE.md, delete the two debts: **Interim W2** and the `--json` `store` key.

## Handoff

**Binding decisions:**
- The `--json` DTOs live in `internal/cli/json.go`. `internal/store` types never get json tags, so `SourceID`/`Closed`/`Active` cannot leak into `one_sided[]`/`splits.mismatched[]`. The spec's account-label ruling depends on this.
- The document embeds `snapshot.SnapshotInfo` and `SchemaInfo` as-is. It does not embed `Manifest`, whose `Warnings` field would encode before `store`. `warnings` is `Outcome.Warnings()` verbatim: W1-before-W2 and no-W2-on-V1 are already mutation-proven there, so do not reassemble or re-test them.
- JSON money is `jsonMoney`: 2 decimals, a leading `-`, **no grouping**. It is not `formatMoney`. Dates are `2006-01-02` in UTC. `id` is the quarry ID.
- V1 `path` is `Result.Path`, which `SyncAndImport` sets to the store path on V1. This is the default reading, because the human NOT REBUILT line names the same path. If product-vision rules otherwise, only Step 4's assertion changes.
- 03's `--from` reuses `renderJSON` unchanged, because the `--from` and plain-sync output must match.

**Refusal paths under `--json` (no code, no test):**
- S3, S4, S1/S2 and I2 return before writing, so stdout is empty in both modes (refusal table default).
- M1 prints the document with `store: null`.
- O1b is `StdoutWriteRefusal` with `Store != nil`, and the copy is already ruled.

**Left unbuilt:**
- The M1b `--json` path, and `--from` generally — SCENARIO-03.
- The I2/S1/S2 frames — SCENARIO-14.

**Traps:**
- `omitempty` on `store` silently deletes `store: null`.
- `importer/validate.go:122` appends to `NeverReconciled`, so the list is nil when no account qualifies. The same is true of the mismatch lists and `OneSided`.
- "stdout minus store equals manifest" holds only without W2. When there are one-sided legs, `warnings` gains the W2 text by design.
- Default `json.Encoder` HTML-escapes `&`, `<` and `>`, the same as `Manifest.Encode`. Compare decoded JSON, never raw substrings of names.
