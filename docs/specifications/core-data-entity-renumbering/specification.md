# Specification: Survive Core Data entity renumbering

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: `quarry sync` keeps importing a Quicken Classic for Mac file after Quicken adds a Core Data entity. Adding an entity renumbers every later entity, and with it every table and column name Core Data derives from an entity number (`Z_15USERTAGS` → `Z_16USERTAGS`, `Z78_TRANSACTION` → `Z79_TRANSACTION`). quarry resolves those names through the file's own `Z_PRIMARYKEY` instead of matching the numbers literally, so files from before and after the renumbering both verify and import.

**Out of Scope**: adding Quicken's new tables or columns (`ZACCOUNTGROUP`, `ZACCOUNT.ZACCOUNTGROUP`, `ZINVOICE.ZAUTO*`, …) to the reference; any new command, flag, warning, `--json` field or exit code; rewording the existing refusal; normalising the fingerprint; schema references for other Quicken versions.

**Business Rules**: amends `docs/specifications/phase0-snapshot/specification.md` BR-5, BR-6, BR-7 (see *Changes to existing surfaces*). Supersedes the fixed-name assumption in `docs/specifications/phase1-import-store/PREP-c.md:72`.

## Business Rules & Invariants
- **BR-R1 One resolver.** The schema check and every importer query that names an entity-numbered table or column resolve the name through the same function in `internal/quicken/v9`, keyed by entity name. The `v9fixture` builder builds those names the same way, so a fixture can emit either numbering.
- **BR-R2 What counts as entity-numbered.** Any name Core Data derives from an entity number: tables `Z_<n><NAME>`; columns `Z_<n><NAME>`, `Z_<n><NAME><k>`, `Z<n>_<NAME>`. A trailing `<k>` is Core Data's disambiguator and stays literal (`Z_1ACCOUNTS1`, `Z_76USERTAGS1`). `Z_PK`, `Z_ENT`, `Z_OPT`, `Z_PRIMARYKEY`, `Z_METADATA`, `Z_MODELCACHE` carry no entity number and compare literally.
- **BR-R3 Unresolvable → literal.** When the reference entity behind a number is absent from the file's `Z_PRIMARYKEY`, or its `Z_NAME` appears there more than once, the reference name is compared as written. A file with no readable `Z_PRIMARYKEY` table resolves nothing (all literal); the table's absence is itself reported as a missing table.
- **BR-R4 Diff-row naming.** A missing item whose entities all resolve is named under the **file's** numbering; an unresolvable one as the reference writes it; unexpected items always as the file has them. Applies to text rows and to the `--json`/manifest `missing_*`/`unexpected_*` fields alike. Scope (BR-6), row ordering and counts unchanged.
- **BR-R5 Reference entity map.** The reference carries a pinned map from each entity number its numbered names use to the entity name: 1 Account, 9 Budget, 15 CashFlowTransactionEntry, 30 DownloadSession, 55 QuickFillRuleSplitEntry, 76 UserTag, 78 Transaction, 79 CashFlowTransaction. Source: the user's pre-update snapshot `20261025T135127Z` `Z_PRIMARYKEY` (read 2026-11-21), agreeing with `docs/prior-art/hardkoded/test/fixture-data.sql:3-9` where that file has rows. The map's key set equals the set of numbers parsed from `reference.sql`'s numbered names; a test holds that equality and fails on an empty set.
- **BR-R6 Silent.** Renumbering produces no warning, Schema-line suffix or new field. The fingerprint stays literal (BR-7 algorithm unchanged), so a renumbered file records a different `fingerprint` in the manifest and `import_runs.schema_fingerprint`.
- **BR-R7 Importer.** `CashFlowTransactionEntry` is resolved optionally in the importer (no new refusal); the split-tags join table and both its columns are resolved by entity name (CashFlowTransactionEntry, UserTag) with BR-R3 fallback.

---

## Triage Brief

Reproduced: `quarry sync` on the user's live file refuses — missing tables `Z_15USERTAGS`, `Z_55USERTAGS`; missing columns `ZATTACHMENT.Z78_TRANSACTION`, `ZCASHFLOWTRANSACTIONENTRY.Z78_PARENT`, `ZCHECKPAYCHECK.Z79_TRANSACTION`, `ZCONNECTIVITYEVENTRESULT.Z78_TRANSACTION`, `ZIFSBILLPAYPAYMENT.Z79_TRANSACTION`, `ZOFXEMAIL.Z78_TRANSACTION`, `ZQERROR.Z78_TRANSACTION`, `ZQUICKPAYPAYMENT.Z79_TRANSACTION`, `ZRECONCILEDTRANSACTION.Z78_ORIGINALTRANSACTION`, `Z_1BUDGETS.Z_9BUDGETS`, `Z_1DOWNLOADSESSIONS.Z_30DOWNLOADSESSIONS`. Snapshot `20261121T111911Z`: `Z_PRIMARYKEY` 2=AccountGroup (new), 10=Budget, 16=CashFlowTransactionEntry, 31=DownloadSession, 56=QuickFillRuleSplitEntry, 77=UserTag, 79=Transaction, 80=CashFlowTransaction. After normalising numbers: zero missing, 33 extras (all genuinely new) → existing extras path, exit 0.

**Affected surface**
- Compare: `internal/platform/sqlschema/diff.go:19` `Compare` (names only). `internal/snapshot/snapshot.go:346-380`: Verified = no missing tables && no missing columns (`:370`); extras → `schemaWarnings`, exit 0 (BR-9). Refusal copy `internal/snapshot/outcome.go:23-40`, render `internal/cli/render.go:49-61`. `compareSchema` callers `snapshot.go:283`, `internal/snapshot/from.go:62` — `--from` re-compares, does not trust the manifest.
- Scope `internal/snapshot/scope.go:11-31`: all `Z*` tables except Z_METADATA, Z_MODELCACHE. `Z_PRIMARYKEY` is in scope and in `reference.sql:91`.
- Reference `internal/quicken/v9/reference.go:12-31` (embedded `reference.sql`, `ReferenceLabel`); wiring `cmd/quarry/run.go:63,75` (`snapshot.WithReference`). `internal/quicken/v9` imports only `platform/*`; `snapshot` and `importer` do not import it today — keep injecting.
- Fingerprint `sqlschema.Fingerprint` only at `snapshot.go:371-372`; stored at `internal/snapshot/import.go:182`, `manifest.go:35`; never compared.
- Importer `internal/importer/entities.go:11-61` `resolveEntities` (required CashFlowTransaction, CategoryTag, UserTag; optional investment entities), called `importer.go:59`. Only numbered import query: `internal/importer/splits.go:113` `splitTagsQuery`, `mapSplitTags` called `importer.go:131`.
- Fixture `internal/quicken/v9/v9fixture/builder.go:14-24` entity consts, `:269-276` `WithEntity`/`WithoutEntity`, `:389-396,608` `LinkUserTag`, `:492,616-627` Z_PRIMARYKEY seed (9 entities; no Account, CashFlowTransactionEntry, Transaction); `fixture.go:75-90`; template from `ReferenceDDL` `template.go:55`.

**Callers**
| Symbol / string | Caller | Via |
| --- | --- | --- |
| `sqlschema.Compare` | `internal/snapshot/snapshot.go:347`; test `internal/platform/sqlschema/diff_test.go:104` | LSP |
| `compareSchema` | `snapshot.go:283`, `from.go:62` | grep |
| `v9.Reference` | `cmd/quarry/run.go:63`; tests `internal/snapshot/scope_internal_test.go:37`, `helpers_test.go:603` | grep |
| `v9.ReferenceDDL` | `reference.go:24`; `v9fixture/template.go:55` | grep |
| `v9.ReferenceLabel` | `cmd/quarry/run.go:75`; ~25 test sites in `internal/snapshot/*_test.go`; literals in `cmd/quarry/run_{transfers,sync_result,import}_test.go` | grep |
| `mapSplitTags` | `internal/importer/importer.go:131` | LSP |
| `resolveEntities` | `internal/importer/importer.go:59` | grep |
| `Builder.LinkUserTag` | `cmd/quarry/run_import_test.go:62,70`; `internal/importer/splits_test.go:165-234`; `import_test.go:47,469`; `v9fixture/builder_test.go:39` | grep |
| numbered literals (prod) | `internal/importer/splits.go:113,115`; `v9fixture/builder.go:389,391,608` | grep |
| numbered literals (tests) | `internal/importer/import_test.go:498`; `internal/snapshot/scope_internal_test.go:19,30`; `v9fixture/builder_test.go:17,58` | grep |

**Already exists — do not re-plan:** `resolveEntities` by-name resolution; Z_PRIMARYKEY in reference and scope; fixture `WithEntity`/`WithoutEntity`; extras warning path (`outcome.go:45-61`, `schemaWarnings`).

**Trap (SCENARIO-03):** `internal/importer/entities_test.go:16-18` (UserTag 9003) and `v9fixture/builder_test.go:21` (UserTag 9001) change entity numbers but keep literal DDL names; once the join table resolves, any such test reaching `mapSplitTags` queries `Z_9003USERTAGS`. `securities_test.go:110` safe (Security not in the map).

## Product Verdict

**SHIP WITH CHANGES** (product-vision, 2026-11-21). Correctness repair, no new surface. Rulings: accept both numberings; do not add new Quicken tables/columns to the reference (would refuse pre-update files); label → `+quarry.2` with a pinned entity map beside `reference.sql`; renumbering silent; fingerprint literal; failure modes surface through the existing refusal with no new copy.

Orchestrator rulings on sizing-pass outcomes (2026-11-21): map source = pre-update snapshot (BR-R5); `Z_PRIMARYKEY` absent → resolve nothing, absence reported as missing table; a query fault on an existing `Z_PRIMARYKEY` returns through the same error path other `inspectContent` reads use (no new copy); duplicate `Z_NAME` tested as a resolver unit case; `CashFlowTransactionEntry` optional in the importer (BR-R7); 04–06 assert `--json` fields as well as text.

## Surface & Copy

All existing; literal text unchanged except the label.
- Success Schema line: `Schema    matches reference hardkoded/quicken-skills@752107b+quarry.2 (82 tables, 1,838 columns)`; with extras `…, plus <phrase> not in it` (existing phrase). Counts stay 82 / 1,838.
- Extras warning: unchanged (`extrasWarningText`, `outcome.go:46-58`).
- Mismatch refusals (sync and `--from`): unchanged (`outcome.go:23-40`), exit 1, snapshot and manifest kept.
- `--json` / manifest: no field added or removed; `schema.reference` = new label; `fingerprint` literal.
- **Duplicated looked-up entity** (mid-feature copy ruling, product-vision 2026-11-21, from gate R1 correctness BLOCKER). Every entity the importer looks up (required or optional) must resolve to exactly one Z_ENT; otherwise refuse via the existing `*UnmappableError` path (`internal/importer/entities.go`, beside the missing-entity reason). Reason text:
  - one name: `the snapshot lists the CategoryTag entity more than once, so quarry cannot tell which records are its`
  - several (sorted, joined ", " and " and "): `the snapshot lists the CategoryTag and Security entities more than once, so quarry cannot tell which records are theirs`
  - missing and duplicated together: only the existing missing-entity refusal.
  - No count, no "Z_PRIMARYKEY" in the copy. Rendered through `internal/snapshot/import.go:230` unchanged, stderr, exit 1, for `sync` and `sync --from` alike: `cannot import snapshot <id>: the snapshot lists the CategoryTag entity more than once, so quarry cannot tell which records are its; ~/Library/Application Support/quarry/<store> was not changed; run quarry sync --from <id> once quarry supports it`. No MCP counterpart.
  - Which stage refuses (product-vision ruling, gate R2, option A): whichever check meets the duplicate first. On a renumbered file a duplicated entity whose number appears in reference names fails the schema check first — its names stay as written (BR-R3), so the existing mismatch refusal is the ruled outcome (`outcome.go:23-40`, exit 1, snapshot and manifest kept). The duplicate copy above covers every case that reaches the importer (original numbering, and entities whose number appears in no reference name). Two refusals, both exit 1, store unchanged — deliberate, not a defect.

### Changes to existing surfaces
- `internal/quicken/v9/reference.go:18`: `ReferenceLabel = "hardkoded/quicken-skills@752107b+quarry.2"`; tests pinning `+quarry.1` move with it.
- `docs/specifications/phase0-snapshot/specification.md` BR-5: label now `…+quarry.2`.
- Same, BR-6, append: "Entity-numbered names are compared after resolving each number through the file's `Z_PRIMARYKEY` (core-data-entity-renumbering BR-R2)."
- Same, BR-7: "The same algorithm runs over the reference; exact match ⇔ equal fingerprints." → "The same algorithm runs over the reference, over names as written. Equal fingerprints imply a match. A file Quicken renumbered matches with a different fingerprint (names are compared after entity resolution; see core-data-entity-renumbering BR-R1–R4)."
- `internal/importer/splits.go:115` doc: "reads Z_15USERTAGS" → "reads the CashFlowTransactionEntry–UserTag join table (resolved by entity name)".
- `internal/quicken/v9/reference.sql` header: add a `+quarry.2` correction line naming the entity map.
- `internal/importer/entities.go:33-34` doc: drop "(a name listed twice stays visible)"; becomes "…or an *UnmappableError naming every missing required entity, or every looked-up entity listed more than once." `found[name] = ent` (last wins) must not reach a caller for a duplicated name; `numbers` stays.
- The importer now refuses a duplicated looked-up entity (above); the Product Verdict's "no new copy" holds for the schema check only.

**User-verified, outside the pipeline**
1. `quarry sync` on the live file → exit 0, `matches reference hardkoded/quicken-skills@752107b+quarry.2 …, plus … not in it`, extras warning.
2. `quarry sync --from 20261121T111911Z` → exit 0.
(Live file has 0 split-tag links; SCENARIO-03 is proven by test only.)

---

## Scenarios (Gherkin)

```gherkin
Scenario: SCENARIO-01 — file with the old numbering still verifies
  Given a Quicken file with the old entity numbering (no AccountGroup) and a full Z_PRIMARYKEY
  When the user runs `quarry sync`
  Then the Schema line reads "matches reference hardkoded/quicken-skills@752107b+quarry.2 (82 tables, 1,838 columns)" with no warning and exit is 0

Scenario: SCENARIO-02 — renumbered file verifies
  Given a file where an entity inserted at Z_ENT=2 shifted every later entity number by one (Z_16USERTAGS, Z79_TRANSACTION, Z_10BUDGETS, …)
  When the user runs `quarry sync`
  Then the schema check reports no missing items and exit is 0

Scenario: SCENARIO-03 — split tags are imported from the renumbered join table
  Given a renumbered file with split lines linked to tags in Z_16USERTAGS
  When the user runs `quarry sync`
  Then split_tags holds those links

Scenario: SCENARIO-04 — a genuinely missing column is still refused
  Given a renumbered file that also lacks a column the reference needs
  When the user runs `quarry sync`
  Then the existing refusal is shown, the "-" row names the item under the file's numbering, the snapshot is kept and exit is 1

Scenario: SCENARIO-05 — a number that points at a different entity is refused
  Given a file with table Z_15USERTAGS whose Z_PRIMARYKEY says CashFlowTransactionEntry=16
  When the user runs `quarry sync`
  Then the rows show "- table Z_16USERTAGS" and "+ table Z_15USERTAGS" and exit is 1

Scenario: SCENARIO-06 — an entity absent from Z_PRIMARYKEY falls back to literal names
  Given a file whose Z_PRIMARYKEY has no UserTag row
  When the user runs `quarry sync`
  Then its numbered items are listed as missing under the reference's own names and exit is 1

Scenario: SCENARIO-07 — a snapshot refused earlier syncs with --from
  Given a snapshot of a renumbered file whose manifest recorded a refusal under +quarry.1
  When the user runs `quarry sync --from <id>`
  Then the snapshot is compared again, verifies, imports and exit is 0
```

---

## Sizing
| Scenario | Verdict — numbers |
| --- | --- |
| SCENARIO-02 | OWNS A RUN — 4 batches (v9 map + label; v9 resolver; snapshot reads Z_PRIMARYKEY in `inspectContent` + option + `run.go` wiring; v9fixture emits renumbered layout with DDL numbering and Z_PRIMARYKEY set independently). Opus architect. Fake importer in acceptance test. |
| SCENARIO-01 | FOLD into 02 — label constant + test literals; its test is 02's old-numbering regression. Fixture must seed the full map. |
| SCENARIO-04 | FOLD into 02 — BR-R4 resolved-name arm of 02's resolver. |
| SCENARIO-05 | FOLD into 02 — number-to-wrong-entity arm. |
| SCENARIO-06 | FOLD into 02 — BR-R3 absent-entity arm. |
| SCENARIO-03 | OWNS A RUN — 2–3 batches, `importer` (resolved split-tags query + wiring `run.go:77`; `LinkUserTag` writes resolved table; doc comment). Sonnet architect. Carries end-to-end wiring check. |
| SCENARIO-07 | FOLD into 03 — "imports" needs the real importer reading the renumbered join table. |

## BDD Acceptance Progress
- [x] SCENARIO-02: Renumbered file verifies — `internal/snapshot/renumbering_test.go` `Test_sync_and_import_verifies_a_file_whose_entities_were_renumbered`
- [x] SCENARIO-01: File with the old numbering still verifies — delivered by SCENARIO-02 — `cmd/quarry/run_renumbering_test.go` `Test_run_verifies_a_file_with_the_original_entity_numbering`
- [x] SCENARIO-04: A genuinely missing column is still refused — delivered by SCENARIO-02 — `cmd/quarry/run_renumbering_test.go` `Test_run_names_a_missing_column_of_a_renumbered_file_under_its_own_numbering`
- [x] SCENARIO-05: A number that points at a different entity is refused — delivered by SCENARIO-02 — `cmd/quarry/run_renumbering_test.go` `Test_run_refuses_a_join_table_whose_number_belongs_to_another_entity`
- [x] SCENARIO-06: An entity absent from Z_PRIMARYKEY falls back to literal names — delivered by SCENARIO-02 — `cmd/quarry/run_renumbering_test.go` `Test_run_names_items_of_an_entity_absent_from_z_primarykey_as_the_reference_writes_them`
- [x] SCENARIO-03: Split tags are imported from the renumbered join table — `cmd/quarry/run_renumbering_test.go` `Test_run_imports_the_split_tags_of_a_file_whose_entities_were_renumbered`
- [x] SCENARIO-07: A snapshot refused earlier syncs with --from — delivered by SCENARIO-03 — `cmd/quarry/run_renumbering_test.go` `Test_run_sync_from_imports_a_snapshot_whose_manifest_recorded_a_refusal_under_the_old_label`
