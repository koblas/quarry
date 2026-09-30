---
id: SCENARIO-08
status: done
---

# SCENARIO-08: findings history that cannot be carried forward restarts with a warning

Cadence: code-first (no mandatory test-first item: no write-safety guard, atomic adapter or bug fix)
Acceptance test: `cmd/quarry/run_findings_carry_test.go` `Test_run_sync_from_warns_and_restarts_findings_history_when_the_previous_findings_table_repeats_an_id`
Narrow loop: `go test ./internal/store/... ./internal/importer/ ./internal/snapshot/ ./cmd/quarry/ -run 'findings|history|carry|warn|restart|unreadable|sync'` (lowercase-first patterns; `cmd/quarry` pins `run_config_test.go:316,332` are named `..._quotes_an_unknown_config_key...`, caught by `sync`)
Mutation checks: `seen` id check in `readFindings` → `Test_replace_names_a_repeated_findings_id_as_the_findings_fault` (without it the PK fails the build); store-level marker in `readHistory` → `Test_replace_restarts_history_when_the_previous_store_cannot_be_read` (extended with `StoreUnreadable`); findings-only branch in `importVerified` → `Test_sync_and_import_warns_of_a_findings_fault_alone_without_the_import_history_line`
Runs: A (1-2) | B1 (3) | B2 (4-5) | V (6-7)
Size: OWNS A RUN — 3 batches, `duckstore`+`importer` / `snapshot` / `cmd` pins (batch 3 is mostly pins: dropping `(M new)` and the fixed clause falls out of `FindingsCarried == false`; no neighbour to FOLD into); not LIGHT (>3 steps)

Inventory (step 5 of the brief): no new port. `store.Replaced`/`store.Result` gain two fields; `importer.go:136` copies them; `snapshot/import.go:115-118` is the only reader. Implementers of `Store.Replace`: `duckstore.Store` and the importer test fake `store_fake_test.go:31` (grep: no other).

## Implementation Plan

Decided taxonomy: `store.Replaced` and `store.Result` gain `FindingsFault *OpenError` (post-open findings read fault only: `Fault` `OpenFaultOther`, `Reason` a fixed phrase, `Err` the cause, mirroring `historyFault`) and `StoreUnreadable bool` (true iff the previous store could not be opened; `HistoryFault` then holds why, `FindingsFault` is nil). Store-level can be `OpenFaultOther` with driver text, so it cannot be inferred from `Fault`.

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_findings_carry_test.go` (new) `Test_run_sync_from_warns_and_restarts_findings_history_when_the_previous_findings_table_repeats_an_id` — `syncBundle`, then `editStore` (`run_read_refusals_test.go:190`) runs `DROP TABLE findings; CREATE TABLE findings (id VARCHAR, type VARCHAR NOT NULL, first_found_at TIMESTAMP NOT NULL, fixed_at TIMESTAMP); INSERT` of one id twice (`schema.go:105-117`: no FK from `finding_items`, so the drop is allowed), `runSyncFrom` (`run_sync_prune_test.go:131`); stderr exactly `quarry: warning: cannot carry findings forward from the previous store (its findings table repeats an id); findings history starts again with this sync\n`, exit 0. Red at that assertion: today the carry read succeeds and the PK fails the build, exit 1.
- [x] Step 2: `internal/store/store.go:205-231` `Result`, `Replaced` — add `FindingsFault`, `StoreUnreadable` (doc each in one line); `internal/importer/importer.go:136` copies both. Data only, no behaviour.

### Build
- [x] Step 3: `internal/store/duckstore/history.go:20-25,47-59,61-102,163-191` + `duckstore.go:286,328` — batch 1, findings fault taxonomy. `history` gains `findingsFault`, `unreadable`; consts `reasonFindingsRepeatID` = `its findings table repeats an id`, `reasonFindingsIncomplete` = `its findings table is incomplete`; `errFindingsRepeatID`; `findingsFault(path, err)` beside `historyFault`; `seen` map in `readFindings`' row callback; `readHistory` returns `history{unreadable: true}` only on the non-`OpenFaultMissing` open failure (`:70-74`; the Missing return at `:71-73` and the stat-absent return stay silent, `unreadable` false); the findings read on any error sets `findingsFault` (any non-repeat error is incomplete, incl. column-query and scan faults and a missing `fixed_at`); `Replace` copies both onto `Replaced`. Tests (`history_faults_test.go`, new `findingsTableDDL`-style helper with omit/NULL cell and no PK, mirrors `importRunsTable`):
  - `Test_replace_names_a_repeated_findings_id_as_the_findings_fault` (control: distinct ids carry, no fault); reason exact, `FindingsCarried` false, `HistoryFault` nil, build succeeds;
  - incomplete matrix, one case each: omitted `id`, `type`, `first_found_at`; NULL `id`, `type`, `first_found_at`; reason `its findings table is incomplete`;
  - rewrite `history_test.go:272-298` (`..._starts_findings_silently_when_the_findings_read_fails`) into the four spy faults (columns query, column scan, rows query, row scan) now asserting the incomplete `FindingsFault`, `1` close, history still `[1 2]`; the absent-table case (`:229`) asserts `FindingsFault` nil;
  - store-level: extend the three cases of `Test_replace_restarts_history_when_the_previous_store_cannot_be_read` (`history_faults_test.go:74-133`) with `StoreUnreadable` true and `FindingsFault` nil; `Test_replace_starts_history_silently_when_the_store_vanishes_before_it_is_opened` (`history_test.go:182`) asserts `StoreUnreadable` false; import_runs-only fault (`history_test.go:258`) asserts `StoreUnreadable` false, `FindingsFault` nil, findings carried; both tables faulty post-open asserts both reasons, `StoreUnreadable` false;
  - after a findings fault nothing is marked fixed: previous store with an open finding the build does not detect, findings unreadable → `Findings.NewlyFixed == 0`, `New == Open`, finding absent from the new store (green on arrival, pin).
  - `internal/importer/import_runs_test.go:63` area + `store_fake_test.go:31` fake fields: `Test_import_passes_the_carry_faults_through_to_the_result` (both fields).
- [x] Step 4: `internal/snapshot/import.go:20-55,56-60,115-118` — batch 2, rendering. `Outcome.findingsWarning` beside `historyWarning`; `warnings()` order: manifest, history, findings, auto-prune; `combinedCarryWarning(reason)` = `cannot carry import history and findings forward from the previous store (<reason>); both start again with this sync`; `findingsRestartWarning(reason)` = `cannot carry findings forward from the previous store (<reason>); findings history starts again with this sync`; `importVerified` picks: `StoreUnreadable` → combined from `HistoryFault` only; else `HistoryFault` → unchanged CF2, `FindingsFault` → findings line (both may print, CF2 first). Reasons via `UnreadableReason(homepath.Abbreviate(...))`. Tests in `import_history_test.go` (split the table at `:32-60`: the three store-level rows set `StoreUnreadable` and want the combined line; import_runs rows keep CF2): pin both findings reasons, combined line per store-level fault, `Test_sync_and_import_warns_of_a_findings_fault_alone_without_the_import_history_line`, both faults = two lines ordered, findings line after the manifest warning and before prune warnings (repoint `:97-105` and `auto_prune_faults_test.go:74-85` to set `StoreUnreadable` and want the combined line); a fault on a not-built store prints nothing (existing `Test_outcome_adds_no_prune_warning...` shape).
- [x] Step 5: `cmd/quarry/run_config_test.go:40` `historyRestartWarning` — batch 3, pins. Rename to `combinedCarryWarning`, value = the combined line for `the file is not a DuckDB database` (six uses `:254-349` and `run_sync_prune_json_test.go:177` follow; `warnings[]` carries it without prefix); `run_import_runs_test.go:110` → combined; `:125` (`id too large`, import_runs only) stays CF2, unchanged. New in `run_findings_carry_test.go`: `Test_run_sync_from_after_a_findings_fault_prints_the_findings_line_without_new_or_fixed_clauses` (previous store holds an open finding the build no longer detects; stdout line is exactly `Findings  N open; run quarry findings to list them`, no `(M new)`, no `fixed since the last sync`) and `--json` variant: `warnings[]` holds the findings line unprefixed, `store.findings.new` == `open`, `newly_fixed` 0. Control: a previous store with intact findings still prints `(M new)`/fixed clause (SCENARIO-06's pin, cite it, don't duplicate).

### Sweep
- [x] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on the two new fields, `findingsFault`, both warning funcs (no history, no ids).

### Verify
- [x] Step 7: full verification + `.claude/scripts/spec-check.py phase2d-findings` → tick SCENARIO-08 with its acceptance test; rewrite STATE.md (drop the "Left unbuilt" 08 line and the `run_config_test` trap; reword the `findings.id` PRIMARY KEY trap to point at the `seen` check in `readFindings`; add the taxonomy below).

## Handoff

**Binding decisions:**
- `Replaced`/`Result` carry `FindingsFault *OpenError` (post-open findings fault, fixed phrase in `Reason`) and `StoreUnreadable bool` (previous store unopenable; `HistoryFault` holds why) — store-level can be `OpenFaultOther`, so the flag is the only reliable discriminator; 16/19 must not re-infer it from `Fault`.
- Any findings read failure other than a repeated id is `its findings table is incomplete`, incl. a missing `fixed_at` (not treated as optional like the import_runs optional columns) — spec rules only `id`/`type`/`first_found_at`; ruling on `fixed_at` is this plan's.
- Both tables faulty post-open prints two lines (CF2 then findings), never the combined line — combined means the store itself was unreadable. NOT in the spec's carry table: needs a scoped product-vision ruling before run A.
- A findings fault leaves `carried.findings` nil and `FindingsCarried` false, so `mergeFindings` marks nothing fixed and `findingsPhrase` drops `(M new)`/fixed clause with no extra code.

**Left unbuilt:** `Counts.Ignored` and `J ignored` clause (16); exported finding row type / read port (11).

**Traps:**
- `editStore` on a format-4 store cannot insert a duplicate into `findings` (PK): `DROP TABLE findings` then recreate without the PK (no FK from `finding_items`, verified in `schema.go`).
- `spyReadDB.passQueries`: 2 fails the findings columns query, 3 the rows query (`history_test.go:272`); the old test asserted silence and must flip, not be added beside.
- The six CF2 pins all use NotDuckDB, a store-level fault, so they all change with the const; `run_import_runs_test.go:125` is import_runs-only and must NOT change.

## Phase report

Run V (steps 6-7) done; all steps ticked, `status: done`.

Sweep: `go build ./...` rc=0; `golangci-lint run ./...` 0 issues; doc comments present on `FindingsFault`/`StoreUnreadable` (both structs), `history.findingsFault`/`unreadable`, `findingsFault`, `findingsRestartWarning`, `combinedCarryWarning`, `carryWarnings`.
Verify: covered full suite `go test rc=0`; `uncovered-diff.py` 0 uncovered added lines; `go test -race` on store/importer/snapshot/cmd/quarry rc=0; `test-stats.py --base 6118e7f --changed` TOTAL 927 (+18): cmd/quarry 293 (+5), importer 144 (+1), snapshot 260 (+7), duckstore 230 (+5).
Spec: SCENARIO-08 ticked with its acceptance test; `spec-check.py phase2d-findings` OK. STATE.md rewritten.
