---
id: SCENARIO-28
status: open
---

# SCENARIO-28: an unused category is reported only when nothing imported or counted uses it

Cadence: code-first (read-only detection and rendering; no write guard, atomic adapter or bug fix)
Acceptance test: `cmd/quarry/run_sync_unused_category_test.go` `Test_run_sync_records_unused_categories_but_not_one_an_investment_or_budget_uses`
Narrow loop: `go test ./internal/quicken/v9/v9fixture/ ./internal/importer/` | `go test ./internal/store/duckstore/ -run 'replace|findings|Findings'` | `go test ./internal/report/ ./internal/cli/` (whole packages) | `go test ./cmd/quarry/ -run 'unused|findings|sync|status'`
Mutation checks: entry under a non-imported parent recorded (drop the append on the `splits.go:49-52` skip) → acceptance test (investment-only category); each new source read dropped → its row in `Test_import_records_the_categories_every_non_imported_reference_uses` (budget also → acceptance test); deleted source rows skipped → same test's deleted rows; a descendant's use marks the ancestor used → `Test_replace_counts_a_parent_used_when_a_subcategory_is`; hidden subtree blocks → `Test_replace_never_reports_a_hidden_category_or_one_under_it`; top node only → `Test_replace_reports_an_unused_parent_once_with_its_subcategories_as_items`; kind gate → `Test_replace_never_reports_a_system_category`
Runs: A (1-2) | B1 (3) | B2 (4-5) | B3 (6-7) | V (8-9)
Size: OWNS A RUN — 5 batches (importer reference reads; detector; fixture fallout; sort + text; JSON/CSV pins), `importer` + v9fixture + duckstore adapter + `report` + `cli`

Gate (P2d-7): the importer CAN see the reference. `entriesQuery` (`importer/splits.go:11-17`) already reads `ZCATEGORYTAG` of every non-deleted entry; an entry under a non-imported parent is dropped only at the skip `splits.go:49-52`; `importer/not_imported_test.go:39-42` already builds an entry under an `EntInvestmentTransaction` parent. Real-file fidelity is P2d-14's reference check.

Ruling (P2d-7, commit 055b8c6): every category reference the v9 reference maps counts as use, of any kind; only income/expense are reported. Sites: (a) `ZCASHFLOWTRANSACTIONENTRY` under ANY non-imported parent (investment, Smart, NULL, dangling, deleted, excluded account) — the existing `mapSplits` skip, no new read, `surveyTransactions` untouched; (b) `ZBUDGETLINEITEM.ZCATEGORYTAG`, (c) `ZLOANSPLITENTRY.ZCATEGORY`, (d) `ZACCOUNT.ZLOANINTERESTCATEGORY`, (e) `ZQUICKFILLRULESPLITENTRY.ZCATEGORYTAG` — one new query each, `COALESCE(ZDELETIONCOUNT, 0) = 0` and category NOT NULL (a deleted row is gone in Quicken, like a deleted entry). A reference is recorded iff its category exists (`existingCategories`); the field holds `cat-N` ids deduplicated and sorted, nil when none.

Survey (step 5): no new port; `Source.QueryRows` is the only call the new reads make. Signature changes: `mapSplits` (one caller, `importer.go:94`), `loadFindings`/`detectFindings` (one caller each, `duckstore.go:402`, `findings.go:63`) — grep, gopls not needed. The snapshot schema gate already covers every Z table, so the new reads add no scope. Persistence: none — the refs' only consumer is `build()` (`duckstore.go:402`); carried findings match by id; `--from` re-imports (`snapshot/import.go:136`). Read needs no change: the `c` join (`findings_read.go:30`) is ungated, `cs` gated, so items get `Category` = path, `Splits`/`Transactions` 0.

Pinned decisions (accepted by the coordinator, recorded in P2d-7):
- used(c) = c or any descendant referenced (split or `ReferencedCategoryIDs`). blocked(c) = c hidden, an ancestor hidden, or a hidden descendant. reportable(c) = kind income/expense, not used, not blocked. Reported iff reportable and parent absent or USED (literal).
- Items: top node first, then every descendant by `full_path` case-insensitive, then id (reader keeps `fi.rowid`); `category_id` only.
- Sort: `items[0].Category` case-insensitive, then id. Text: `  <id padded to widest in group>  <full_path>[ (and N subcategories)]<ignored marker>`, N = items-1, `humanize.Count(n, "subcategory", "subcategories")`, clause only when N > 0.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_sync_unused_category_test.go` `Test_run_sync_records_unused_categories_but_not_one_an_investment_or_budget_uses` — fixture as `run_sync_similar_categories_test.go:13-33` plus `not_imported_test.go:37-42`: expense `Parking` (no reference), expense `Brokerage Fees` referenced only by an entry under an `EntInvestmentTransaction` in a `BROKERAGENORMAL` account (the gate), expense `Charity` referenced only by a budget line item, hidden expense `Old`, expense parent `Vacation` with child `Hotel`, both unreferenced; names not look-alikes under `CategoryKey`; `syncFindingsBundle` + `stringMap`: unused-category findings exactly `cat-<Parking>`, `cat-<Vacation>`; Vacation's items `cat-<Vacation>`, `cat-<Hotel>` (`transaction_id`/`payee_id` NULL)
- [ ] Step 2: `internal/quicken/v9/v9fixture/builder.go:126-139` + `Seed` (`:298-360`): `BudgetLineItem`, `LoanSplitEntry`, `QuickfillRuleSplitEntry` (each `{Category int64; Deleted bool}`, zero ref writes NULL) and `AccountRow.LoanInterestCategory` (`:38-52`); `builder_test.go:18` seeds and reads back each; run the acceptance test red at its assertion

### Build
- [ ] Step 3 (B1, importer references): `store/store.go:137-149` `Rows.ReferencedCategoryIDs []string` + doc (not a table; nil when none — whole-struct `Rows` asserts); `importer/splits.go:19-91` `mapSplits` records `cat-N` on the `!ok`/no-parent skips when the category exists; new `importer/category_refs.go` (four queries, b-e, one collector) ; `importer.go:84-121` wiring, dedupe + sort into the Rows literal; fault rows in BOTH tables `import_faults_test.go:70-82,144-156` (query and scan) for each new query — seed one row per table there so the scan arm is reached; new `importer/category_refs_test.go` `Test_import_records_the_categories_every_non_imported_reference_uses` (table: investment, Smart, NULL-parent entry, budget, loan split, loan interest, quickfill → recorded; deleted entry/budget/loan split/account/quickfill rule, missing category → not; a split's category not added; duplicates once, sorted) and `Test_import_records_no_category_references_without_any` (nil)

### Build (B2)
- [ ] Step 4 (B2, detector): new `store/duckstore/findings_unused_category.go` (`unusedCategoryQuery`: categories LEFT JOIN `categorySplits` for a used flag; tree walk in Go per pinned rules); `findings.go:60-108` `loadFindings`/`detectFindings` take the referenced ids, detector last; `duckstore.go:402` passes `rows.ReferencedCategoryIDs`; `export_test.go:13-20` `UnusedCategoryQuery`; fault rows beside `findings_test.go:116-117` (query, scan → `detect unused-category findings`); new `findings_unused_category_test.go` (`Test_replace_*`): unused leaf flagged; `Test_replace_reports_an_unused_parent_once_with_its_subcategories_as_items` (3 levels, item order); `Test_replace_reports_an_unused_child_of_a_used_parent`; `Test_replace_counts_a_parent_used_when_a_subcategory_is` (split, and referenced id, on a grandchild); `Test_replace_never_reports_a_hidden_category_or_one_under_it` (hidden leaf; hidden parent with unused child; visible unused parent with hidden child → nothing); `Test_replace_never_reports_a_system_category` (a used `system` child keeps its expense parent used; expense child of an unused `system` parent not reported); income reported; split in a closed and an excluded-from-reports account counts; a referenced id alone keeps a category off
- [ ] Step 5 (B2, fallout): `go test ./cmd/quarry/ ./internal/...`; every fixture category with no reference now raises a finding (likely `run_sync_findings_test.go`, `run_status_findings_test.go`, `run_findings_ignore_test.go`, `run_transfers_test.go`, `run_sync_duplicates_test.go`). Store queries may filter by type; text/count pins give the category a split (distinct amounts — equal ones raise `duplicate`/`unlinked-transfer`) or a budget line, or drop it; never weaken the detector

### Build (B3)
- [ ] Step 6 (B3, sort + text): `report/findings.go:145-180` `findingOrder` case `UnusedCategory` (path ci, id), drop its `//nolint:exhaustive`, post-switch return gets `// unreachable:` (`knownFindings` lists only `finding.Types()`); replace `report/findings_test.go:177-182` with `Test_findings_sorts_unused_categories_by_path_ignoring_case` (`auto:parking` before `Bank`; same path → id). `cli/render_findings.go:119-140` `liveFindingLines` case + new `unusedCategoryRows`; drop `//nolint:exhaustive` and the generic id-line loop (post-switch return `// unreachable:` as above); new `render_findings_unused_internal_test.go`: spec example verbatim (header + `  unused-category:cat-17  Auto:Parking`, `  unused-category:cat-40  Vacation (and 3 subcategories)`), `(and 1 subcategory)`, no clause at 0, ids padded (`cat-9` vs `cat-40`), ignored marker at line end under `--status all`; delete `render_findings_internal_test.go:236-259`, the case at `render_findings_status_internal_test.go:116-122`, move `:133-138` to an `Uncategorized` fixture; end-to-end `cmd/quarry/run_findings_unused_category_test.go` `Test_run_findings_lists_unused_categories_with_their_subcategory_count` (stdout incl. footer)
- [ ] Step 7 (B3, JSON/CSV pins, no production edit expected): `cli/json_findings_internal_test.go` unused item: `category` path, `category_id` set, `splits`/`transactions`/`payee`/`payee_id`/date/account/currency/amount null; `cli/findings_csv_test.go` row: category + category_id filled, rest NULL unquoted; one `--json` slice in `run_findings_unused_category_test.go`

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments (`store.FindingItem.Category` names unused-category; `findingOrder`, `liveFindingLines`, `Rows`, `mapSplits` docs); `docs/initial-prd.md:229` row + bullet after :232: the unused-category safety rule (every mapped category reference counts — splits, investment/scheduled entries, budgets, loans, memorized-payee rules; hidden excluded; check-first fix copy) per Changes item 8

### Verify
- [ ] Step 9: full verification + `spec-check.py phase2d-findings` → tick SCENARIO-28 with its acceptance test

## Handoff

**Binding decisions**
- `store.Rows.ReferencedCategoryIDs` (sorted, deduplicated `cat-N`, nil when none) is the only carrier of non-split category references; never persisted — `build()` detection is its only consumer, and every sync incl. `--from` re-imports.
- Recorded sources: entries under any non-imported parent (the `mapSplits` skip), budget line items, loan split entries, account loan interest category, quickfill rule split entries; deleted rows and missing categories skipped. A later reference site joins this one collector, never a second field.
- Used = self or any descendant referenced (any kind); hidden anywhere in the subtree or above blocks; reported iff income/expense, unused, unblocked, parent absent or used. Items top node first, then descendants path ci, id.

**Left unbuilt**
- Nothing of `unused-category`; `ZFITRANSACTION` category columns (downloaded-transaction metadata, not a user reference) are not read — not in the ruling's list.

**Traps**
- Every unreferenced fixture category now raises `unused-category`: filter store queries by type, or reference the category (split with a distinct amount, or a budget line).
- New source queries must not contain an existing fault-table match substring (`ZTYPENAME`, `ZPARENTCATEGORY`, `FROM ZCASHFLOWTRANSACTIONENTRY`, `ZDELETIONCOUNT, 0) FROM ZTRANSACTION`), or a fault row hits the wrong query.
- `report.findingOrder` and `cli.liveFindingLines` lose `//nolint:exhaustive` together, or `nolintlint` flags the survivor.

## Phase report
