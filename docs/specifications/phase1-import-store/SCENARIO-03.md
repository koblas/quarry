---
id: SCENARIO-03
status: open
---

# SCENARIO-03: rebuild from an earlier snapshot without Quicken (absorbs 16)

Size verdict: OWNS A RUN — one new `snapshot.Server` method plus `sync` flag wiring; no `cmd/quarry` production change (`newServerFactory` already sets snapshot dir, home, store path).
Cadence: code-first — no write-safety guard, no new write path or atomic adapter (the store is written only through the existing `Replace`).
Acceptance test: `cmd/quarry/run_from_test.go` `Test_run_rebuilds_the_store_from_an_earlier_snapshot_without_quicken`
Acceptance test (SCENARIO-16, folded): `cmd/quarry/run_usage_test.go` `Test_run_refuses_from_with_quicken_or_without_a_value`
Narrow loop: `go test ./internal/snapshot/ ./internal/cli/ ./cmd/quarry/ -run 'ImportFrom|import_from|resolveFrom|From|from|SyncAndImport|sync_help|usage'`
Mutation checks:
- F5 hash guard in `(*Server).ImportFrom` → `Test_import_from_never_imports_a_snapshot_whose_hash_changed`
- current-reference re-check skips the import → `Test_import_from_skips_the_import_when_the_current_reference_no_longer_matches`

**Blocked on a copy ruling before dispatch** (orchestrator → scoped `product-vision`): F1–F4 conditions, and `errNoAccountsTable`/`errNoAccounts` (no F-row), have no ruled text until SCENARIO-15. Proposed interim, S4-in-S3 precedent: F3 frame with the cause as `<reason>` — `quarry: <~snapshot path> is not a quarry snapshot (<cause>); pass a snapshot taken by quarry sync with --from <snapshot>`, exit 1 — plus "no push or PR may ship an F1–F4 case in the interim frame (final-gate BLOCKER)". Same ruling: F3-vs-F5 precedence for a changed file that no longer opens as SQLite (integrity runs before the hash). F5 and M1b use their ruled copy verbatim here.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_from_test.go` `Test_run_rebuilds_the_store_from_an_earlier_snapshot_without_quicken` — pattern `run_success_test.go:32-68`. Plain `sync --quicken <bundle>` first, keep its stdout and the manifest bytes; `os.Rename` the bundle out of `Documents` (Quicken absent; `OpenBundle`'s cleanup only closes a handle), delete `quarry.duckdb`; then `sync --from <id>`. Assert exit 0, empty stderr, stdout byte-equal to the plain sync's, manifest bytes unchanged, `quarry.duckdb` present, still exactly one `.sqlite` + one `.json` in `snapshots/`.
- [ ] Step 2: `internal/cli/sync.go:116-117` register `--from` (help string verbatim, `specification.md` *Command and flags*); `:60-77` RunE branches on `Changed("from")` to `srv.ImportFrom`; new `internal/snapshot/from.go` `(*Server).ImportFrom(ctx, from string) (Outcome, error)` signature-only stub. Test must fail at its exit-code assertion, not at "unknown flag".

### Build
- [ ] Step 3: `internal/snapshot/snapshot.go:215-272` `buildManifest` — extract the integrity / account-count / hash / schema reads into one unexported inspector returning everything but `Source`/`TakenAt`/paths; `buildManifest` becomes a thin wrapper. `:162-167` — extract the W1 decision into a helper (`[]string{}` when no extras-only) used by `Sync` and `ImportFrom`. Behaviour-neutral: existing `Sync` tests stay green unchanged and cover both extracted bodies.
- [ ] Step 4: `internal/snapshot/import.go:73-104` — extract `:86-103` (importer nil check, `Import`, V1 / `importFailureRefusal` branches) into an unexported method taking a verified `Manifest`; `SyncAndImport` calls it. Existing `sync_and_import_test.go` stays green unchanged.
- [ ] Step 5: `internal/snapshot/from.go` unexported `resolveFrom(home, snapshotDir, value)` → (snapshot path, manifest path): `/` or `.sqlite` suffix = path (`homepath.Expand` + `filepath.Abs`, precedent `bundle.go:29-35`), else `snapshotDir/<value>.sqlite`; manifest = `.sqlite` trimmed + `.json`. No `EvalSymlinks` — the ID form must join the same unresolved `snapshotDir` `FinalPaths` used, or Step 1's stdout equality breaks under macOS `/var`→`/private/var`. White-box `from_internal_test.go` `Test_resolveFrom_*` table: ID, `~/x.sqlite`, `a/b` (no suffix), relative `x.sqlite`, absolute path.
- [ ] Step 6: `internal/snapshot/from.go` `ImportFrom` happy path. Order: resolve → read manifest file → JSON-decode `Manifest` → inspect (Step 3) → build the returned Manifest (decoded `Source`/`TakenAt`; resolved `Path`/`Manifest`; recomputed `Bytes`/`SHA256`/`Accounts`/`Schema`; Warnings via Step 3's helper with `Source` as bundle path) → Step 4's import tail with `SnapshotRef{Path, SHA256: recomputed, SchemaFingerprint: recomputed Schema.Fingerprint}`. Never constructs `Destination` or `Source`. New `internal/snapshot/import_from_test.go` (reuse `fakeImporter`/`newImportServer`, `sync_and_import_test.go:22-37,60-71`; take the snapshot with `Sync`): `Test_import_from_passes_the_same_snapshot_ref_as_a_plain_sync` (ref equals what `SyncAndImport` passed for that file AND `SHA256` equals `sha256.Sum256` of the file bytes computed in the test; manifest bytes unchanged; Outcome paths/Source), `Test_import_from_imports_a_snapshot_whose_manifest_said_unverified_once_the_current_reference_matches` (edit manifest `verified:false`), `Test_import_from_reports_a_v1_failure_like_a_plain_sync` (fake returns `ErrValidationFailed`; Store set, `Built` false).
- [ ] Step 7: `ImportFrom` guards, after inspect and before the import tail: F5 (recomputed SHA-256 ≠ manifest's) verbatim `specification.md:269`; then `!Schema.Verified` = M1b verbatim `:271` via a sibling of `outcome.go:21-31` `mismatchError`, returned as `MismatchError` with `Store` nil and the recomputed Manifest (RunE renders DIFFERS). Tests: `Test_import_from_never_imports_a_snapshot_whose_hash_changed` (edit manifest `sha256`; exact F5; importer 0 calls; `Store` nil), `Test_import_from_skips_the_import_when_the_current_reference_no_longer_matches` (server reference with an extra table; exact M1b; `errors.As` `MismatchError`; 0 calls; `Outcome.Manifest.Schema` is the recomputed diff, not the recorded one).
- [ ] Step 8: read/decode/inspect failures go through ONE unexported classifier `fromRefusal(home, snapshotPath, err)` (interim frame above, wrapping so `errors.Is` reaches the cause), then `FailureOutcome(ctx, …)` as `Sync` does. `Test_import_from_refuses_without_importing_when_the_snapshot_cannot_be_read` (rows: manifest missing, manifest not JSON, snapshot not SQLite; assert only error non-nil, 0 importer calls, `errors.Is` on the cause — no copy, so 15 rewrites none of it). No ctx-cancelled row: `FailureOutcome` replaces the cause with `InterruptedRefusal()`, and that branch is existing, covered code.
- [ ] Step 9: `internal/cli/sync.go:22-23,51-59` Args — after the positional check: U3 (`from` and `quicken` both `Changed`, custom text, not `MarkFlagsMutuallyExclusive`), then U4 (`from` Changed and `TrimSpace` empty) as a `UsageError`, exact `specification.md:278-279`. `:33-50` Long gains the `--from` paragraph, Example its second line (`:141-177`). Tests: `cmd/quarry/run_usage_test.go` `Test_run_refuses_from_with_quicken_or_without_a_value` — rows `--from X --quicken Y`, `--from=`, `--from=   `, bare `--from` (cobra control, same U4 text); bundle in Documents as `:53-78`; exit 2, stdout empty, no `snapshots/`; extend `Test_run_prints_the_sync_help` (`:33-51`) with the paragraph, Example line and flag help.

### Sweep
- [ ] Step 10: fix what `go build ./... && golangci-lint run ./...` reports; doc comments on `ImportFrom` (≤4 lines, names F5/M1b outcomes and "never writes the snapshots dir"), `resolveFrom`, `fromRefusal`, the inspector and the two extracted helpers (1–2 lines each); `snapshot` `doc.go` names `ImportFrom`.

### Verify
- [ ] Step 11: full verification + `.claude/scripts/spec-check.py phase1-import-store` → tick SCENARIO-03 with its acceptance test, and SCENARIO-16 "— FOLD → 03, delivered by SCENARIO-03 —" with its folded test last on the line.

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `ImportFrom` never constructs or calls `Destination` (no `Prepare`: no snapshots-dir mkdir, no snapshot-leftover sweep) nor `Source` — this is what makes "never rewrites the manifest" and "never touches Quicken" structural. The store leftover sweep still runs, unchanged, inside `Replace`.
- `--from` precedence: resolve → manifest read/decode → inspect → F5 → M1b → import (V1, I2, S4, S1–S3 as today). SCENARIO-15 slots F1–F4 into `fromRefusal` and must keep this order.
- Returned Manifest: resolved current paths, recorded `Source`/`TakenAt`, everything else recomputed; W1 recomputed against the current reference through the helper `Sync` shares — so `--json` `schema` reflects the re-check, not the file on disk.
- U3 is checked before U4; both before any server is built (no home needed).

**Left unbuilt** — named so nobody assumes it exists:
- F1, F1b, F2, F2b, F3 reasons, F4 copy — SCENARIO-15, inside `fromRefusal` only. `errNoAccountsTable`/`errNoAccounts` under `--from` have no F-row — 15 needs a ruling.
- SCENARIO-17's command-level test (M1b stderr + DIFFERS stdout, exit 1) — 15; will be green on arrival, since 03 returns M1b as a `MismatchError` and RunE already renders it.

**Traps** — things that look right and are not:
- `SnapshotRef.SchemaFingerprint` = recomputed `Schema.Fingerprint` (`Fingerprint(scopeSchema(actual))`, independent of the reference) — never `ReferenceFingerprint`, never the manifest's recorded value.
- The decoded manifest has zero `ReferenceTables`/`ReferenceColumns` (`json:"-"`): return or render the decoded `Schema` anywhere and the Schema line reads `(0 tables, 0 columns)` — always the recomputed one.
- The inspector checks integrity before hashing, so a changed snapshot that no longer opens as SQLite refuses through `fromRefusal`, not F5 — safe either way; F3-vs-F5 order is in the pending copy ruling.
- `snapshotID` in S/I2/O1b copy is the file basename, so a path-form `--from ~/x.sqlite` names `x`.
