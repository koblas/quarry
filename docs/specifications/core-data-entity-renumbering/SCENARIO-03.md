---
id: SCENARIO-03
status: open
---

# SCENARIO-03: Split tags are imported from the renumbered join table (folds SCENARIO-07)

Cadence: code-first (no mandatory test-first item: no write-safety guard, no atomic adapter, no bug-fix finding)
Acceptance test: `cmd/quarry/run_renumbering_test.go` `Test_run_imports_the_split_tags_of_a_file_whose_entities_were_renumbered`
Acceptance test (SCENARIO-07, folded): `cmd/quarry/run_renumbering_test.go` `Test_run_sync_from_imports_a_snapshot_whose_manifest_recorded_a_refusal_under_the_old_label`
Narrow loop: `go test ./internal/importer/ ./internal/quicken/v9/... -run 'SplitTag|Entit|Builder|Renumber|Join'` then `go test ./cmd/quarry/ -run 'Renumber|Refusal_under'`
Mutation checks: delete `importer.WithEntityResolver(v9.ResolveName)` in `cmd/quarry/run.go` → both acceptance tests; join names left literal in the importer query → `Test_import_reads_split_tags_from_the_renumbered_join_table`; CashFlowTransactionEntry moved to `requiredEntities` → `Test_import_reads_split_tags_literally_when_cashflowtransactionentry_is_absent`; `ImportFrom` trusting `recorded.Schema` → the SCENARIO-07 test
Runs: A (1-2) | B1 (3-5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (`importer`; `v9fixture` is test support, `cmd/quarry` wiring does not count); FOLD of SCENARIO-07 (`--from` re-resolves by construction, `internal/snapshot/from.go:62`; no production code of its own)

Existence (read, not re-planned): `v9.ResolveName` (`internal/quicken/v9/entities.go:31`), `snapshot.WithEntityResolver` wiring (`cmd/quarry/run.go:75`), `writeManifestForTest` (`cmd/quarry/run_from_test.go:518`), `readSyncArtifacts`/`referenceMatch` (`run_sync_result_test.go:40-53`), `renumberSchema` (`v9fixture/renumber.go:21-24` builds the `ddl`-as-`map[string][]int64`).
Surface surveyed: the importer's only entity-numbered text is `splits.go:113` (table `Z_15USERTAGS`, columns `Z_15CASHFLOWTRANSACTIONENTRIES`, `Z_76USERTAGS`); `mapSplitTags` has one caller (`importer.go:131`), `resolveEntities` one (`importer.go:59`). Both map to the resolver; nothing else numbered stays literal.
Success line: the fixture adds no table or column outside the reference (AccountGroup exists only as a Z_PRIMARYKEY row), so the Schema line is `referenceMatch` with no `, plus … not in it`, no warning, empty stderr.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `v9fixture/builder.go:441-446` `LinkUserTag` doc, `:658-662` `Seed` insert, `renumber.go:21-24` — join table and both columns named through `v9.ResolveName` over `b.ddl` (extract the ddl→`map[string][]int64` helper both use), never `b.entities`; `builder_test.go` row: `WithInsertedEntity("AccountGroup", 2)` + `LinkUserTag` reads back from `Z_16USERTAGS(Z_16CASHFLOWTRANSACTIONENTRIES, Z_77USERTAGS)`; existing `:21`/`:58` row (`WithEntity("UserTag", 9001)`, literal `Z_15USERTAGS`) stays green unchanged and pins that `WithEntity` does not move the join names
- [x] Step 2: `cmd/quarry/run_renumbering_test.go` `Test_run_imports_the_split_tags_of_a_file_whose_entities_were_renumbered` — `WithInsertedEntity("AccountGroup", 2)` builder with account, category, tag, split linked by `LinkUserTag`; `sync --quicken`; asserts exit 0, empty stderr, Schema line `referenceMatch`, `split_tags` rows (duckdb `SELECT split_id, tag_id FROM split_tags`). Red at the exit-code assertion (importer queries `Z_15USERTAGS`: "read split tags"), not at `Seed`

### Build
- [ ] Step 3: `internal/importer/entities.go:11-61` `resolveEntities`, `importer.go:14-37,59,131`, `splits.go:113-139` — batch 1: `type EntityResolver` + `WithEntityResolver` option (unset = identity, so the 20+ importer tests and the UserTag-override tests `entities_test.go:16-18` keep literal names); `CashFlowTransactionEntry` added to `optionalEntities` (no refusal); `resolveEntities` also returns every Z_ENT per Z_NAME it read (duplicate stays visible; same query, so no new fault row); `splitTagsQuery` becomes a builder of table + two columns resolved from that map; doc `splits.go:115` per spec *Changes to existing surfaces*. Tests in `entities_test.go`/`splits_test.go` through `Server.Import` with `importer.WithEntityResolver(v9.ResolveName)` against `WithInsertedEntity` builders: `Test_import_reads_split_tags_from_the_renumbered_join_table`; control, same file with no resolver option → refusal "read split tags" (literal arm); `Test_import_reads_split_tags_literally_when_cashflowtransactionentry_is_absent` (`WithoutEntity`, old numbering, links still imported, no new refusal); duplicate `Z_NAME` for CashFlowTransactionEntry (extra `Z_PRIMARYKEY` row via `execOn`, `splits_test.go:~197` pattern) → literal join names; UserTag absent keeps the existing refusal (`entities_test.go:30-48`). Faults: `Z_PRIMARYKEY` and split-tags rows of `everySourceQuery` (`import_test.go:481-499`) stay and still pass with the resolver set; one row run with the resolver set (renumbered file, query fault surfaces "read split tags")
- [ ] Step 4: `cmd/quarry/run.go:77` — batch 2: `importer.NewServer(importer.WithStore(st), importer.WithEntityResolver(v9.ResolveName))`; Step 2's test goes green; this is the wiring pin (mutation: delete the option)
- [ ] Step 5: `cmd/quarry/run_renumbering_test.go` `Test_run_sync_from_imports_a_snapshot_whose_manifest_recorded_a_refusal_under_the_old_label` — batch 3: sync the Step 2 file, rewrite the manifest's `schema` as a `+quarry.1` refusal (decode `snapshot.Manifest`, set Schema, `Encode`, write; sha untouched), delete the store, `sync --from <id>`; asserts exit 0, empty stderr, Schema line `referenceMatch`, `split_tags` rebuilt in the new store. Green on arrival (re-comparison already works); the `recorded.Schema` mutation proves it can fail

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; `go doc ./internal/importer` reads right for `WithEntityResolver`/`EntityResolver`

### Verify
- [ ] Step 7: `.claude/scripts/verify.sh <start> ./internal/importer/... ./internal/quicken/... ./cmd/quarry/...` + `.claude/scripts/spec-check.py core-data-entity-renumbering`; tick SCENARIO-03 and SCENARIO-07 (`delivered by SCENARIO-03`, test last on the line); rewrite STATE.md; `status: done`

## Handoff

**Binding decisions**
- The importer declares its own `EntityResolver func(name string, entities map[string][]int64) (string, bool)` and `WithEntityResolver`; `cmd/quarry/run.go` injects `v9.ResolveName` — importer and snapshot may not import `v9` (feature packages do not import each other's data package; same shape as `snapshot.WithEntityResolver`). Unset = identity.
- Builder join-table names follow `b.ddl` (`WithInsertedEntity`), never `b.entities` (`WithEntity`) — a Z_PRIMARYKEY-only override must not move DDL names, as in SCENARIO-02.
- `CashFlowTransactionEntry` is optional in `resolveEntities`; its absence means literal names, not a refusal (BR-R7).

**Left unbuilt**
- Cmd-level row "table unresolved, column resolved" (STATE.md Open debts) — unowned.
- Resolving names other than the split-tags join in the importer: none exist (`splits.go:113` is the only numbered text).

**Traps**
- A test that wires the resolver and also does `WithEntity("UserTag", n)` queries `Z_<n>USERTAGS` (file whose Z_PRIMARYKEY and DDL disagree). `entities_test.go:16-18` and `builder_test.go:21` are safe only because neither wires it; do not add the resolver to the shared `importInto` helper.
- `Seed` on a renumbered builder outside `WriteBundle` is not renamed (`builder.go:743`); only `WriteBundle` calls `renumberSchema`.
- `--from` must keep re-comparing: SCENARIO-07's rewritten manifest is only a decoy; its green depends on `ImportFrom` ignoring `recorded.Schema`.

## Phase report

Run A (steps 1-2) done; acceptance test red, Build not started.

- `internal/quicken/v9/v9fixture/renumber.go`: new `ddlEntities()` (b.ddl as `map[string][]int64`, used by `renumberSchema`) and `userTagJoin()` (table + both columns resolved through `v9.ResolveName` over b.ddl).
- `internal/quicken/v9/v9fixture/builder.go`: `Seed` inserts into `userTagJoin()` names (`fmt.Sprintf("%q")`), `LinkUserTag` doc updated; `fmt` import added.
- `internal/quicken/v9/v9fixture/builder_test.go` (end of file): `Test_builder_links_a_user_tag_through_the_renumbered_join_table` (reads back `Z_16USERTAGS(Z_16CASHFLOWTRANSACTIONENTRIES, Z_77USERTAGS)`); green. Existing `WithEntity("UserTag", 9001)` row untouched and green.
- `cmd/quarry/run_renumbering_test.go` (end of file): `Test_run_imports_the_split_tags_of_a_file_whose_entities_were_renumbered`.
  Red at the exit-code assertion (`run_renumbering_test.go:179`): expected 0, actual 1, message `quarry: cannot build the store in ~/Library/Application Support/quarry: no such table: Z_15USERTAGS; run quarry sync --from <id>` (the importer's "read split tags" wrap is not in the message the user sees).
- Next run (B1): Steps 3-5 in `internal/importer` and `cmd/quarry/run.go:77`. Do not touch the fixture again. Narrow `-run` patterns are lowercase (e.g. `renumbered`).
