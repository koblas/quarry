---
id: SCENARIO-02
status: done
---

# SCENARIO-02: Renumbered file verifies

Cadence: code-first — no write-safety guard, atomic adapter or bug-fix-with-Failure touched
Acceptance test: `internal/snapshot/renumbering_test.go` `Test_sync_and_import_verifies_a_file_whose_entities_were_renumbered`
Acceptance test (SCENARIO-01, folded): `cmd/quarry/run_renumbering_test.go` `Test_run_verifies_a_file_with_the_original_entity_numbering`
Acceptance test (SCENARIO-04, folded): `cmd/quarry/run_renumbering_test.go` `Test_run_names_a_missing_column_of_a_renumbered_file_under_its_own_numbering`
Acceptance test (SCENARIO-05, folded): `cmd/quarry/run_renumbering_test.go` `Test_run_refuses_a_join_table_whose_number_belongs_to_another_entity`
Acceptance test (SCENARIO-06, folded): `cmd/quarry/run_renumbering_test.go` `Test_run_names_items_of_an_entity_absent_from_z_primarykey_as_the_reference_writes_them`
Narrow loop: `go test ./internal/quicken/v9/... ./internal/snapshot/ ./cmd/quarry/ -run 'resolve|entit|renumber|inserted|primary|schema|mismatch'`
Mutation checks: duplicate-`Z_NAME` arm in `v9.ResolveName` → `Test_resolve_name_keeps_the_reference_name_when_z_name_is_duplicated`; `snapshot.WithEntityResolver` line in `cmd/quarry/run.go` → `Test_run_refuses_a_join_table_whose_number_belongs_to_another_entity`; mixed-item display arm in snapshot → `Test_run_names_items_of_an_entity_absent_from_z_primarykey_as_the_reference_writes_them`
Runs: A (1-3) | B1 (4-5) | B2 (6) | V (7-8)
Size: OWNS A RUN — 5 batches (v9 resolver, fixture, snapshot read, snapshot compare, wiring+folds); 1 feature package (`snapshot`) plus the `v9` reference library and its `v9fixture`

## Implementation Plan

### Acceptance (red)
Steps 1-2 are real code, not stubs: the fixture builds renumbered names through the v9 resolver (BR-R1), and a stubbed fixture emits the old DDL, so the acceptance test would pass on arrival.
- [x] Step 1: new `internal/quicken/v9/entities.go` `referenceEntities` + `ResolveName` + `entities_test.go` — pinned map (BR-R5 values); `ResolveName(name string, entities map[string][]int64) (string, bool)`, where `ok` = the name carries no entity number or its number resolved. Unit rows: each BR-R2 form (`Z_15USERTAGS`, `Z_76USERTAGS1` disambiguator kept, `Z78_TRANSACTION`), non-numbered literal (`Z_PK`, `Z_ENT`, `Z_OPT`, `Z_PRIMARYKEY`, `Z_METADATA`, `Z_MODELCACHE`, `ZACCOUNT`), entity absent → literal/false, duplicate `Z_NAME` → literal/false, number not in map → literal/false, nil map. `Test_reference_entity_map_keys_equal_the_numbers_in_reference_names` parses table and column names from `v9.Reference` (not `reference.sql` text: index names carry stray numbers) and asserts non-empty
- [x] Step 2: `v9fixture/builder.go:14-24` consts, `:245-283` `NewBuilder`/`WithEntity` doc, `:491-625` `Seed` entity list, `:680-696` `WriteBundle`; new `(*Builder).WithInsertedEntity(name, ent)` + `builder_test.go` — default seed carries the full map (add Account 1, Budget 9, CashFlowTransactionEntry 15, DownloadSession 30, QuickFillRuleSplitEntry 55, Transaction 78). `WithInsertedEntity` adds the row and shifts **every** builder entity `>= ent` by one (CFT 79→80 collides with SmartCFT 80 otherwise), and records the shifted numbers as the DDL numbering; `WithEntity`/`WithoutEntity` after it change `Z_PRIMARYKEY` only. `WriteBundle` renames tables/columns on the template copy with `ALTER TABLE … RENAME [COLUMN]` via `v9.ResolveName`; never rewrite DDL text. Tests: renumbered bundle has `Z_16USERTAGS`, `ZATTACHMENT.Z79_TRANSACTION`, `Z_1BUDGETS.Z_10BUDGETS`, and the `Z_PRIMARYKEY` rows; `WithoutEntity` after insert leaves the DDL renumbered
- [x] Step 3: `internal/snapshot/renumbering_test.go` acceptance test + `snapshot.go:28-64` `EntityResolver` type + `WithEntityResolver` option, no-op stub — `WithInsertedEntity("AccountGroup", 2)` bundle through `newImportServer` (`helpers_test.go:410`) with `builtStore()` and `WithEntityResolver(v9.ResolveName)`. Precondition: bundle has `Z_16USERTAGS`. Asserts: nil error, `Verified`, no missing tables/columns, no warnings, importer called once, `Fingerprint != ReferenceFingerprint`, `ReferenceFingerprint` equals the old-numbering sync's. Red: missing `Z_15USERTAGS`

### Build
- [x] Step 4: `snapshot.go:290-327` `inspectContent`, `:263-288` `buildManifest`, `from.go:44-62` — read `Z_ENT, Z_NAME` rows via `QueryRows` after a `sqlite_master` existence check; return `map[string][]int64` beside the schema; both callers pass it to `compareSchema`. Rows (in `renumbering_test.go`): table absent → only `- table Z_PRIMARYKEY`, numbered names literal; NULL `Z_NAME` row skipped; read fault (a `Z_PRIMARYKEY` without `Z_NAME`) through Sync → `contentRefusal` copy, snapshot discarded; same fault through `ImportFrom` → `fromContentRefusal` (follow the `from_test.go:518-583` prepared-snapshot pattern). Not `// unreachable`
- [x] Step 5: `snapshot.go:346-350` `compareSchema` + new `internal/snapshot/resolve.go` `resolveReference` — option unset = literal compare (no change at the 25 `WithReference` sites); resolve each reference table and column name, union columns if two reference tables resolve onto one name; `Compare`, then show a missing column as the reference writes it (table and column) unless both parts are `ok`, and re-sort to `diff.go:55-71` order; unexpected untouched. `schemaInfoFromDiff` (`:361-380`) keeps counts and `ReferenceFingerprint` over the unresolved reference. Server tests: duplicate-name union row; mixed item reference-named and sorted; all-resolved item under file numbering

- [x] Step 6: `reference.go:17-18` label `+quarry.2`, `reference.sql:5-6` `+quarry.2` correction line naming the map, `cmd/quarry/run.go:63-77` add `snapshot.WithEntityResolver(v9.ResolveName)`; new `cmd/quarry/run_renumbering_test.go` with the four folded tests through `runCapture` (`run_sync_result_test.go:127-153` shape), each asserting full stdout and the `--json` `schema` fields; bump `+quarry.1` at `run_sync_result_test.go:41,432`, `run_transfers_test.go:41`, `run_import_test.go:96` (not `render_internal_test.go`: false grep hit).
  01: `NewBuilder().WriteBundle` (full map seeded, not `OpenBundle`, whose `fixture.go:75-90` seeds three) → exact Schema line, empty stderr, exit 0.
  04: inserted entity + drop `ZATTACHMENT.Z79_TRANSACTION` (drop its index first if SQLite refuses) → `- column  ZATTACHMENT.Z79_TRANSACTION`, refusal copy, snapshot kept, exit 1.
  05: `WithEntity("CashFlowTransactionEntry", 16)` → `- table   Z_16USERTAGS`, `+ table   Z_15USERTAGS`, exit 1.
  06: inserted entity + `WithoutEntity("UserTag")` → `- column  Z_15USERTAGS.Z_76USERTAGS`, `- column  Z_55USERTAGS.Z_76USERTAGS1`, `+ column  Z_16USERTAGS.Z_77USERTAGS`, `+ column  Z_56USERTAGS.Z_77USERTAGS1`, exit 1

### Sweep
- [x] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `ResolveName`, `WithInsertedEntity`, `WithEntityResolver`, `inspectContent`, `compareSchema`; `docs/specifications/phase0-snapshot/specification.md:26-28` BR-5/6/7 edits verbatim from *Changes to existing surfaces*

### Verify
- [x] Step 8: full verification + `spec-check.py core-data-entity-renumbering` → tick SCENARIO-02 and, each with a "delivered by SCENARIO-02" note before its test, SCENARIO-01/04/05/06

## Handoff

**Binding decisions:**
- `v9.ResolveName(name, map[string][]int64) (string, bool)` is the one resolver (BR-R1). Snapshot, fixture and SCENARIO-03's importer use it. The map value is a slice so a duplicate `Z_NAME` stays visible to it (BR-R3).
- `snapshot` does not import `v9`. It declares `EntityResolver` and reads raw `Z_PRIMARYKEY` rows inside `inspectContent`, so `--from` (SCENARIO-07) re-resolves with no new snapshot code.
- Option unset = literal compare. Existing tests and `sqlschema.Schema{}` references stay as they are.
- Display rule (BR-R4, "entities all resolve"): a missing column whose table or column is unresolved is shown entirely as the reference writes it; otherwise it is shown under the file's numbering. Comparison always uses resolved names.
- Counts (82 / 1,838) and `reference_fingerprint` are computed over the unresolved reference. `fingerprint` stays literal (BR-R6).
- The fixture's DDL numbering (`WithInsertedEntity`) and its `Z_PRIMARYKEY` rows (`WithEntity`/`WithoutEntity`) are controlled independently. The default seed carries the full BR-R5 map.

**Left unbuilt** (SCENARIO-03):
- The importer's resolved split-tags query (`importer/splits.go:113`) and its doc (`:115`).
- `Builder.LinkUserTag` and the `Seed` insert at `builder.go:389-396,608`, which still write `Z_15USERTAGS`.
- An end-to-end renumbered success path through `cmd/quarry`.

**Traps:**
- Calling `LinkUserTag` on a `WithInsertedEntity` builder fails in `Seed` ("no such table `Z_15USERTAGS`") until SCENARIO-03.
- A renumbered success through `cmd/quarry` reaches the real importer, which still queries `Z_15USERTAGS`. That is why 02's acceptance test uses a fake importer.
- Fixture renames: rename from the highest number down (or through temporary names) so one rename never lands on a name that still exists.
- `importer/entities_test.go:16-18` and `builder_test.go:21` override UserTag but keep the literal DDL names. That is harmless here because neither goes through snapshot compare; SCENARIO-03 must recheck them.

## Phase report

Run V — steps 7-8 done; scenario complete.

Checkpoint pin and folds:
- New `internal/snapshot/renumbering_test.go` `Test_sync_keeps_every_z_ent_listed_for_a_duplicated_z_name` (renumbered bundle plus a second `UserTag` row at Z_ENT 200; exact `MissingColumns` pair, empty `MissingTables`). Mutation `snapshot.go` `readEntities`: `entities[name] = append(entities[name], number)` -> `entities[name] = []int64{number}` reddened it: `Not equal` on `MissingColumns` (got `Z_16USERTAGS.Z_200USERTAGS`, `Z_56USERTAGS.Z_200USERTAGS1`); restored, diff clean.
- Comments shortened: `referenceEntities`, `numberedNames`, `renumberSchema`, `WithInsertedEntity`, `inspectContent` (keeps the `sqlite.IsBusy` refusal).
- Folds: old-numbering baseline sync moved into `oldNumberingReferenceFingerprint`; `fileEntities` is a func; `renumberedLabel` replaced by `referenceLabel`, from which `referenceMatch` is built (`run_sync_result_test.go`); Narrow loop pattern lowercased.
- phase0-snapshot BR-5/6/7 edited per *Changes to existing surfaces*.

Verify (`verify.sh c578cdeb3450 ./internal/snapshot/... ./internal/quicken/... ./cmd/quarry/...`): go build rc=0; go test rc=0; uncovered-diff rc=0 (0 uncovered, 2 declared unreachable, both pre-existing in `inspectContent`); go test -race rc=0; golangci-lint rc=0 (0 issues). test-stats: cmd/quarry 978 (+4); internal/quicken/v9 7 (+5); v9fixture 15 (+5); internal/snapshot 346 (+10); TOTAL 1346 (+24), tempdir 425 (+17), disk 387 (+11). Two post-verify edits (dup test assertion tightened to `assert.Equal`, `inspectContent` doc clause): build, lint 0 issues and snapshot, v9 and v9fixture tests re-run green.
