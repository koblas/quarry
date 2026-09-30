---
id: SCENARIO-06
status: done
---

# SCENARIO-06: a finding no longer found is marked fixed on the next sync (folds SCENARIO-07, reopen)

Cadence: code-first (read-only carry, merge and copy; no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_sync_findings_test.go` `Test_run_sync_marks_a_finding_no_longer_found_fixed_at_the_build_time`
Acceptance test (SCENARIO-07, folded): `cmd/quarry/run_sync_findings_test.go` `Test_run_sync_reopens_a_fixed_finding_with_its_first_found_at_and_not_new`
Narrow loop: `go test ./internal/store/duckstore/ -run 'Replace|History|Finding' && go test ./internal/importer/ ./internal/cli/ -run 'Finding|Store|JSON' && go test ./cmd/quarry/ -run 'Findings|Import|From|Json'`
Mutation checks: reopen keeps `first_found_at` in `mergeFindings` → `Test_replace_reopens_a_fixed_finding_keeping_its_first_found_at`; `(M new)` gated on the carried flag in `findingsPhrase` → `Test_findingsPhrase` case "new but not carried"
Runs: A (1) | B1 (2-3) | B2 (4) | V (5-6)
Size: OWNS A RUN — 3 batches, 1 feature package (`duckstore`) + `internal/store` types, a one-line `internal/importer` passthrough and `internal/cli`; `snapshot` untouched (departure from sizing); folds SCENARIO-07

Contract: second `quarry sync` after the payee's splits are categorized prints last line `Findings  none open, 1 fixed since the last sync`, exit 0; the store keeps `uncategorized:payee-N` with `fixed_at` = `store_info.built_at` and no `finding_items` rows. `sync --json` `store.findings` = `{open:0, ignored:0, fixed:1, new:0, newly_fixed:1}`. Reopen: `Findings  1 open; run quarry findings to list them` (no `(1 new)`), `first_found_at` = the first sync's value, `fixed_at` NULL.
Ruled strings pinned here (Surface & Copy → Sync): `12 open (3 new), 2 fixed since the last sync; run quarry findings to list them`; `none open, 2 fixed since the last sync`; `none open`; `12 open; run quarry findings to list them` (New=12, not carried). `J ignored` and the `4 ignored` example are SCENARIO-16's — not pinned. M and K grouped with `humanize.Thousands` like N (precedent, not ruled copy).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_sync_findings_test.go` (after :101) both acceptance tests — two/three syncs from `DocumentsA`/`DocumentsB`/`DocumentsC` bundles in one home (precedent `run_import_runs_test.go:74-95`), builders mint payees in identical call order so `payee-N` matches; assert through `stringMap` SQL against `store_info.built_at`. No stubs needed; red at the Findings-line / `fixed_at` assertion

### Build
- [x] Step 2: `internal/store/duckstore/history.go:20-23` `history` + `:55-77` `readHistory` + new `readFindings` beside `readRuns` (`:96-144`) — carried findings keyed by id plus a carried flag surfaced as `store.Replaced.FindingsCarried` (`internal/store/store.go:218-226`, returned at `duckstore.go:329`), read on the SAME read connection before close, AFTER `readRuns` and even when `readRuns` failed; presence via a `duckdb_columns()` query on `findings` mirroring `runColumnsQuery` (`:38`): zero columns → not carried, silent; a findings read fault → not carried, silent (08 adds its warning). Tests in `history_test.go`: v4 store carries findings (flag true, incl. an EMPTY findings table); format-3 store (`newStoreFile` DDL with `import_runs` only) → `HistoryFault` nil, not carried; broken `import_runs` + intact `findings` (DDL fixture) → `HistoryFault` set AND `FindingsCarried` true; findings query fault and scan fault via `spyReadDB` `passQueries` past the runs reads → import_runs rows carried and `HistoryFault` nil (positive control — confirm the query index, do not assume 2); `Test_replace_reads_the_previous_history_before_it_creates_the_build_file` still `opened, closed, created`; `history_faults_test.go:211-237` unchanged in meaning
- [x] Step 3: `internal/store/duckstore/findings.go:37-53` `loadFindings`, `:112-125` `mergeFindings(detected, carried, builtAt)`, `duckstore.go:394-409` `build` (:401 call); `internal/store/store.go:203-216` `Result` gains `FindingsCarried`; `internal/importer/importer.go:135-136` copies it; `internal/importer/store_fake_test.go:13-30` fake returns it. Merge by branch: detected+carried → carried `first_found_at`, `fixed_at` NULL, not new; detected only → `builtAt`, new; carried open not detected → `fixed_at` = `builtAt`, NewlyFixed; carried fixed not detected → row unchanged (Fixed, not NewlyFixed); items only for detected. Tests in `findings_test.go` (two/three `Replace` calls on one dir, `withFindingCandidates` then `minimalRows`): `Test_replace_marks_a_carried_finding_not_detected_fixed_at_the_build_time` (items gone); `Test_replace_keeps_a_fixed_finding_fixed_at_its_first_fix_time` (3rd build: Fixed 1, NewlyFixed 0); `Test_replace_reopens_a_fixed_finding_keeping_its_first_found_at`; `Test_replace_counts_new_and_newly_fixed_as_the_findings_stamped_with_the_build_time` (Counts equal SQL counts of `first_found_at = (SELECT built_at FROM store_info) AND fixed_at IS NULL` and `fixed_at = (SELECT built_at FROM store_info)`); `import_runs_test.go:79-90` sibling for `FindingsCarried`
- [x] Step 4: `internal/cli/render.go:97` + `:101-108` `findingsPhrase(counts, carried)` — clauses per Contract; `render_internal_test.go:279-295` `Test_findingsPhrase` gains every ruled string above plus "new but not carried" and "carried, none new"; `Test_run_sync_json_counts_a_finding_fixed_since_the_last_sync` in `run_sync_findings_test.go` (the JSON object in Contract); `Test_run_sync_from_an_older_snapshot_reopens_and_fixes_findings` — sync A (payee-X uncategorized), sync B (X categorized, payee-Y uncategorized), `sync --from <A id>`: X reopened keeping first_found_at, Y fixed, line `Findings  1 open, 1 fixed since the last sync; run quarry findings to list them`

### Sweep
- [x] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `history`, `readHistory`, `readFindings`, `mergeFindings`, `Replace` (`duckstore.go:276-280`), `Result`/`Replaced` (`store.go:203-222`) naming the carried flag; fold STATE open debts `findings.go:12-13`, `:84-85`, `duckstore.go:391-393`, `render.go:86-89` (same functions touched)

### Verify
- [x] Step 6: full verification + `spec-check.py phase2d-findings` → tick SCENARIO-06 with its acceptance test and SCENARIO-07 "delivered by SCENARIO-06" with its folded test; rewrite STATE.md

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `first_found_at`/`fixed_at` compare by exact equality with `store_info.built_at` in SQL; sync counts come from the merge branch, and `Test_replace_counts_new_and_newly_fixed_as_the_findings_stamped_with_the_build_time` pins that both agree — 19's `status` counts reuse that SQL definition
- Carried timestamps are written back as scanned; only `builtAt` (the `store_info.built_at` value) is ever stamped — never SQL `now()`
- `FindingsCarried` (on `store.Replaced` and `store.Result`) is true iff the previous store's `findings` table was present and read — empty table included; `(M new)` renders only when it is true — 08's fault path sets it false and the clause drop falls out
- Findings read runs even after an `import_runs` fault, on the same connection, after `readRuns` — spec row "only import_runs faulty → existing CF2 line" leaves findings carried; 08 must keep this
- Presence of `findings` is a column-list query; zero columns → silent. 08's "required column missing → incomplete" reads the same column set, adding no query

**Left unbuilt** — named so nobody assumes it exists:
- Findings carry fault reasons (`its findings table repeats an id`, `its findings table is incomplete`), `Replaced.FindingsFault`, the findings-only and store-level warnings — 08; until then a findings read fault silently starts findings history
- `J ignored` clause, `Counts.Ignored` — 16

**Traps** — things that look right and are not:
- Go `builtAt` carries nanoseconds, DuckDB TIMESTAMP stores microseconds: never `Equal()` a read-back time against `builtAt` in Go
- `findings.id` is PRIMARY KEY: a repeated carried id reaching `appendTable` fails the whole build (exit 1) — 08 adds a `seen` check at read time that turns it into a warning
- `spyReadDB` fails EVERY query after `passQueries`, so a runs-fault spy test also fails the findings read — it cannot show findings survive a runs fault; the DDL fixture test does
- `newBuiltStore`'s previous store holds `one-sided-transfer:xfer-3` (from `minimalRows`), so every `Replace` over it now carries findings — count tests on fresh dirs stay `New == Open`; pins checked: every `Findings` line pin in `cmd/quarry` and `run_json_test.go:99` is a first sync, none shift

## Phase report

Run V done. Sweep: `golangci-lint run ./...` 0 issues; doc comments on `history`, `readHistory`, `readFindings`, `mergeFindings`, `Replace`, `Result`/`Replaced` already name the carried flag (Replace doc re-wrapped). Folded the six checkpoint comment MINORs (`findings.go` oneSidedTransferQuery and detectUncategorized docs, `doc.go` re-wrap plus the findings carry, `duckstore.go` `build` doc, `render.go` `renderStore` doc, `json.go` `newStoreDocument` doc), comments only.
Verify: go build ok; full suite `go test -count=1 -coverpkg=./... ./...` rc=0; `uncovered-diff.py` 0 uncovered since 5d9fd1b; `go test -race` on duckstore, importer, cli, cmd/quarry rc=0. test-stats: cmd/quarry 287 (+4), internal/cli 162 (+0, table cases added), internal/importer 143 (+1), internal/store/duckstore 224 (+9), TOTAL 816 (+14).
Ticked SCENARIO-06 and SCENARIO-07 (delivered by SCENARIO-06) in `specification.md`; STATE.md rewritten.
