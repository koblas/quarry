# core-data-entity-renumbering — current state

Scenarios complete: SCENARIO-01, 02, 04, 05, 06 (all delivered by SCENARIO-02). Last updated by SCENARIO-02.

## Binding decisions
- `v9.ResolveName(name, map[string][]int64) (string, bool)` is the one resolver; the slice value keeps a duplicate `Z_NAME` visible so it stays literal. Snapshot, fixture and the importer (SCENARIO-03) use it (SCENARIO-02)
- `snapshot` does not import `v9`: it declares `EntityResolver` and reads raw `Z_PRIMARYKEY` rows in `inspectContent`, so `--from` re-resolves with no new code; `cmd/quarry/run.go` injects `snapshot.WithEntityResolver(v9.ResolveName)` (SCENARIO-02)
- Option unset = literal compare; the 25 `WithReference` test sites and `sqlschema.Schema{}` references are unchanged (SCENARIO-02)
- Display rule: a missing column whose table or column did not resolve is shown wholly as the reference writes it, else under the file's numbering; comparison always uses resolved names (SCENARIO-02)
- Counts (82 tables / 1,838 columns) and `reference_fingerprint` are over the unresolved reference; `fingerprint` stays literal, so a renumbered file records a different one (SCENARIO-02)
- `ReferenceLabel` is `+quarry.2`; `docs/prior-art/README.md:60` keeps `+quarry.1` as history (SCENARIO-02)
- Fixture DDL numbering (`WithInsertedEntity`) and `Z_PRIMARYKEY` rows (`WithEntity`/`WithoutEntity`) are independent; the default seed carries the full reference map (SCENARIO-02)

## Left unbuilt
- Importer split-tags query `importer/splits.go:113` and its doc `:115` — SCENARIO-03
- `Builder.LinkUserTag` and the `Seed` insert at `v9fixture/builder.go:389-396,608`, still `Z_15USERTAGS` — SCENARIO-03
- End-to-end renumbered success through `cmd/quarry` (reaches the real importer) — SCENARIO-03
- `--from` of a snapshot refused earlier — SCENARIO-07 (re-resolution already works by construction; needs its own acceptance test) (SCENARIO-02)

## Traps
- `LinkUserTag` on a `WithInsertedEntity` builder fails in `Seed` ("no such table `Z_15USERTAGS`") until SCENARIO-03 (SCENARIO-02)
- A renumbered success through `cmd/quarry` reaches the real importer, which still queries `Z_15USERTAGS`; 02's acceptance test therefore uses a fake importer (SCENARIO-02)
- `importer/entities_test.go:16-18` and `v9fixture/builder_test.go:21` override UserTag but keep literal DDL names; harmless while they skip snapshot compare, SCENARIO-03 must recheck them (SCENARIO-02)
- Fixture renames go through a placeholder name so one rename never lands on a name still in use (SCENARIO-02)
- Test names are lowercase (`Test_resolve_name_…`); a capitalised `-run` pattern matches nothing and reports `ok [no tests to run]` (SCENARIO-02)

## Open debts
- Optional cmd-level row "table unresolved, column resolved": `(tableOK || true) && columnOK` stays green through `cmd/quarry`; held only by the snapshot test `internal/snapshot/renumbering_test.go` (`…when_its_table_did_not_resolve`) — unowned, dies unless re-opened (SCENARIO-02)
