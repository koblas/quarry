# core-data-entity-renumbering — current state

Scenarios complete: SCENARIO-01..07 (01, 04, 05, 06 delivered by 02; 07 delivered by 03). Feature complete; awaiting final gate. Last updated by SCENARIO-03.

## Binding decisions
- `v9.ResolveName(name, map[string][]int64) (string, bool)` is the one resolver; the slice value keeps a duplicate `Z_NAME` visible so it stays literal. Snapshot, fixture and importer all use it (SCENARIO-02, 03)
- Neither `snapshot` nor `importer` imports `v9`: each declares its own `EntityResolver` + `WithEntityResolver` (unset = identity/literal); `cmd/quarry/run.go` injects `v9.ResolveName` into both (SCENARIO-02, 03)
- `snapshot` reads raw `Z_PRIMARYKEY` rows in `inspectContent`, so `--from` re-resolves by construction and ignores the manifest's recorded schema (SCENARIO-02, 03)
- The importer's only entity-numbered text is the split-tags join (`splits.go` `splitTagsQuery`: table `Z_15USERTAGS`, columns `Z_15CASHFLOWTRANSACTIONENTRIES`, `Z_76USERTAGS`), resolved per import; `CashFlowTransactionEntry` is optional in `resolveEntities` — absent means literal names, no refusal (SCENARIO-03)
- Display rule: a missing column whose table or column did not resolve is shown wholly as the reference writes it, else under the file's numbering; comparison always uses resolved names (SCENARIO-02)
- Counts (82 tables / 1,838 columns) and `reference_fingerprint` are over the unresolved reference; `fingerprint` stays literal (SCENARIO-02)
- `ReferenceLabel` is `+quarry.2`; `docs/prior-art/README.md:60` keeps `+quarry.1` as history (SCENARIO-02)
- Fixture DDL numbering (`WithInsertedEntity`) and `Z_PRIMARYKEY` rows (`WithEntity`/`WithoutEntity`) are independent; `LinkUserTag`/`Seed` join names follow the DDL numbering, never `WithEntity` (SCENARIO-02, 03)

## Left unbuilt
- Nothing owned by this feature remains.

## Traps
- A test that wires the resolver and also does `WithEntity("UserTag", n)` queries `Z_<n>USERTAGS`; `importer/entities_test.go:16-18` and `v9fixture/builder_test.go:21` are safe only because neither wires it — do not add the resolver to the shared `importInto` helper (SCENARIO-03)
- `Seed` on a renumbered builder outside `WriteBundle` is not renamed; only `WriteBundle` calls `renumberSchema` (SCENARIO-03)
- `--from` green depends on `ImportFrom` ignoring `recorded.Schema` (`snapshot/from.go:62`); the SCENARIO-07 test's rewritten manifest is only a decoy (SCENARIO-03)
- Fixture renames go through a placeholder name so one rename never lands on a name still in use (SCENARIO-02)
- Test names are lowercase (`Test_resolve_name_…`); a capitalised `-run` pattern matches nothing and reports `ok [no tests to run]` (SCENARIO-02)

## Open debts
- Optional cmd-level row "table unresolved, column resolved": `(tableOK || true) && columnOK` stays green through `cmd/quarry`; held only by the snapshot test `internal/snapshot/renumbering_test.go` (`…when_its_table_did_not_resolve`) — unowned, dies unless re-opened (SCENARIO-02)
