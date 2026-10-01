---
id: SCENARIO-01
status: done
---

# SCENARIO-01: sync records findings and prints a Findings line

Cadence: test-first — a new fallible step (detection) inside duckstore `Replace`'s build → CheckpointClose → rename window, the temp-then-rename atomicity adapter
Acceptance test: `cmd/quarry/run_sync_findings_test.go` `Test_run_sync_records_findings_and_prints_the_findings_line`
Acceptance test (SCENARIO-02, folded): `cmd/quarry/run_sync_findings_test.go` `Test_run_sync_records_a_one_sided_transfer_with_its_from_split_as_the_item`
Acceptance test (SCENARIO-03, folded): `cmd/quarry/run_sync_findings_test.go` `Test_run_sync_uncategorized_findings_hold_what_cashflow_counts`
Narrow loop: `go test ./internal/finding/ ./internal/store/... ./internal/importer/ ./internal/cli/ && go test ./cmd/quarry/ -run 'Findings|Sync|Transfers|Success|Schema|Import|Validation|StoreFaults|NeverReplaces|Help'`
Mutation checks: detection error dropped in `build` (`return nil` for the detection call) → `Test_replace_keeps_the_previous_store_when_detection_fails` and cmd row `a detection fault` of `Test_run_never_replaces_the_store_when_the_build_fails` | uncategorized detector reading raw `splits WHERE category_id IS NULL` instead of `v_cash_flow` → `Test_run_sync_uncategorized_findings_hold_what_cashflow_counts`
Runs: A (1) | B1 (2-3) | B2 (4-5) | V (6-7)
Size: OWNS A RUN — 4 batches, 1 feature package (`duckstore`) + new stdlib-only leaf `internal/finding`; `store`/`importer` pass-through and `internal/cli` rendering

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_sync_findings_test.go` (new) — three `run([]string{"sync","--quicken",…})` tests over `v9fixture` bundles; no stubs needed (they read the store with `duckdb.OpenReadOnly` + SQL). Assert stdout FIRST so red is at an assertion:
  - 01: two uncategorized splits of one payee, one no-payee uncategorized split, one one-sided transfer (reuse `run_transfers_test.go:197-204` shape). stdout ends `Findings  3 open; run quarry findings to list them\n` (HasSuffix — do not pin the `?` rows, SCENARIO-04 removes them); `findings` ids = {`uncategorized:payee-<pk>`, `uncategorized:no-payee`, `one-sided-transfer:xfer-<pk>`}, `type` column per id; every `first_found_at` = `store_info.built_at` (SQL equality); exit 0
  - 02: `finding_items` for `one-sided-transfer:xfer-N` is exactly one row (`transaction_id` of the from-split's transaction, `split_id` = from-split, payee/category NULL)
  - 03 (departure, see Handoff): `quarry cashflow` has no uncategorized count, so the comparison goes through its totals. Fixture: the only flow in reported accounts is uncategorized splits of distinct magnitudes (income and spending), one in a closed account, one future-dated, one in a not-reported account (`UsedInReports: 0`), plus a categorized split only in the not-reported account. Assert uncategorized items' split set = the reported uncategorized splits, and `quarry cashflow --json --since … --until …` (window covering the future date) Income and Spent totals = sums over `finding_items ⋈ splits` by sign

### Build
- [x] Step 2: `internal/finding/` (new: `doc.go`, `finding.go`, `finding_test.go`) — `Type` + eight consts and `Types()` in ruled group order; `ID(type, entity)`, `PairID(type, a, b)` (numeric source-id order: `txn-9` before `txn-10`), `NoPayee` entity; `Status` (`open|ignored|fixed`) + `StatusOf(fixed, ignored bool)` per P2d-3 (fixed-and-ignored → fixed); fix table all 8 rows (`Fix` JSON sentence, text heading, text group-fix clause) pinned verbatim from spec `## Surface & Copy` fix table; `Counts{Open, Ignored, Fixed, New, NewlyFixed}`. Stdlib only
- [x] Step 3: `internal/store/duckstore/schema.go:78-109` add `findings` + `finding_items` DDL exactly per P2d-1; `duckstore.go:23-24` `FormatVersion` 3→4; `duckstore.go:52-58` `DB` gains `QueryRows` (port survey below); fallout `query_test.go:28-31` `storeRelations` (+`findings`, `finding_items`); tests: `Test_replace_creates_the_findings_tables` (columns/nullability per P2d-1). `open_test.go:180-186`, `status_test.go:35`, `duckstore_test.go:179`, `run_store_info_test.go:73`, `run_status_json_test.go:100` follow the constant — expect green; `run_read_refusals_test.go:97,114` are v2/no-row stores, unaffected
- [x] Step 4 (test-first): `internal/store/duckstore/findings.go` (new) — `detectFindings(ctx, db)` running two detectors on the build connection: one-sided = `transfers WHERE to_split_id IS NULL` joined to the from-split; uncategorized = `v_cash_flow WHERE category_id IS NULL`, grouped by payee (NULL → `NoPayee`), one item per split. `mergeFindings(detected, builtAt)` → findings rows (`first_found_at` = `builtAt`, `fixed_at` NULL), item rows, `finding.Counts` (Open = New = detected count). Hook in `duckstore.go:387-430` `build` after `transfers`/`import_runs` load, `store_info` stays last; `build` returns counts; `Replace` (`:278-326`) puts them in `store.Replaced.Findings`. Export the two detector queries via `export_test.go`; `faultDB` (`duckstore_test.go:477-494`) gains a query-fault hook keyed by query (inject `ioFault`). Tests red first: `Test_replace_keeps_the_previous_store_when_detection_fails` table — one-sided query, uncategorized query, append `findings`, append `finding_items` (4 rows, previous bytes unchanged, error names the step); `Test_replace_reports_the_findings_counts`; contract of ids/items against a `minimalRows` variant incl. no-payee
- [x] Step 5: `internal/store/store.go:200-220` `Result.Findings`, `Replaced.Findings` (`finding.Counts`); `internal/importer/importer.go:128-138` copy through; `store_fake_test.go:22-29` returns configured counts + `Test_import_returns_the_stores_findings_counts`. `internal/cli/render.go:86-105` `renderStore` appends `writeFindings` after `writeTransfers` (NOT in `renderStoreFailure`); `findingsPhrase`: `none open` | `N open; run quarry findings to list them` (`humanize.Thousands`), no `(M new)`/fixed/ignored clauses; table test pins `none open`, `1 open; …`, `1,204 open; …`. `internal/cli/sync.go:46-50` insert Changes item 1 paragraph verbatim; pin it in `run_usage_test.go:50-54`. Pin fallout (add the `Findings` line): `render_internal_test.go:250,272`, `run_validation_test.go:94`, `run_success_test.go:64`, `run_schema_test.go:114`, `run_import_test.go:102`, `run_transfers_test.go:85,134,173,220` — count = distinct payees with uncategorized splits (all no-payee → `1 open`) + one-sided transfers. `run_store_faults_test.go:130-133` fixture gains one uncategorized transaction + new row `a detection fault` (`duplicateTable: "findings"`, existing `cannot build the store in …` line naming the duplicated finding id)

### Sweep
- [x] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments (`finding` package doc, `duckstore` package doc mentions findings, `DB.QueryRows`, `Replaced`/`Result.Findings`); PRD `docs/initial-prd.md` per Changes item 8 at current lines :126, :136 (spec says :135), :172, :228, :227 (spec says :229), :232, Milestones Phase 2 row (2d = every finding type) — NOT the unused-category line (SCENARIO-28)

### Verify
- [x] Step 7: full verification + `spec-check.py phase2d-findings` → tick SCENARIO-01, and SCENARIO-02 / 03 as "delivered by SCENARIO-01" with their tests; write `STATE.md` (first for 2d)

Port survey (`DB.QueryRows`, grep — LSP answers from a sibling worktree per 2c trap): implementers `internal/platform/duckdb` `*DB` (already has `QueryRows`), `duckstore_test.go:477` `faultDB`, `cmd/quarry/run_store_faults_test.go:24` `faultDB` — both embed `duckstore.DB`, no stub needed. `build` calls on `DB`: `Exec`, `AppendRows` (+ new `QueryRows`); `CheckpointClose`/`Close` stay in `Replace`.

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `internal/finding` is the one owner of type names/order, id grammar, `StatusOf`, fix table, `Counts`; stdlib only — `store`, `duckstore`, `snapshot`, `report`, `findings`(11) import it; `internal/store` importing it is intended
- Detection runs in Go over `DB.QueryRows` on the build connection, inside `build` before `store_info`; any detection/append error is a build error — P2d-5 (previous store untouched, exit 1)
- `findings.first_found_at` = the `builtAt` `build` writes to `store_info.built_at` (never a fresh `time.Now()`) — P2d-4 `new`/`newly_fixed` (06, 16, 19) compare by equality
- `mergeFindings(detected, builtAt)` is the merge seam: 06 adds the carried-findings parameter (read in `history.go` beside `readRuns`) and sets fixed_at/reopen; 01 has no carried findings
- Detectors select only from `transfers` and `v_cash_flow` (P2d-7); uncategorized entity = `payees.id` or `finding.NoPayee`
- Findings line lives in `renderStore` only (after Transfers, before `Pruned`), never in `renderStoreFailure`

**Left unbuilt** — named so nobody assumes it exists:
- `(M new)`, `K fixed since the last sync`, `J ignored` clauses and the "history was carried" flag — 06 (new/fixed), 16 (ignored). `Counts.New == Counts.Open` in 01 by definition, so rendering `New` before 06 adds the flag would print `(N new)` on every first sync
- `Counts.Ignored` stays 0 from `duckstore` — 16 sets it via a `snapshot` option
- Carry of `findings` from the previous store — 06; carry faults — 08
- `sync --json` `store.findings`, one-sided warning / `?` rows removal — 04
- No exported finding row type for reads — 11 designs the read port

**Traps** — things that look right and are not:
- A duplicated EMPTY append does not fail: `duplicateTable: "findings"` needs a fixture with ≥ 1 finding
- Uncategorized is per payee: a fixture whose splits all lack a payee is `1 open`, not the split count
- A one-sided transfer's from-split is a transfer leg, so `v_cash_flow` never lists it as uncategorized
- SCENARIO-03 through cashflow totals is blind to zero-amount splits (`v_cash_flow` drops uncategorized zero amounts anyway); `--since/--until` must cover the future-dated split

## Phase report

Run V done: Sweep (PRD edits per Changes item 8 at :126, :136, :172, :227, :228, :232, Milestones Phase 2 row; `duckstore/doc.go` mentions findings), full verify green (go test rc=0, uncovered-diff 0, lint 0 issues, `-race` ok), SCENARIO-01/02/03 ticked, `spec-check.py` OK, `STATE.md` written, `status: done`.

Added in V: `faultDB.scanFault` (`duckstore_test.go`) and two scan-fault rows in `Test_replace_keeps_the_previous_store_when_detection_fails` covering the `return err` in each detector's scan callback; both mutated (`return nil`) and went red.
