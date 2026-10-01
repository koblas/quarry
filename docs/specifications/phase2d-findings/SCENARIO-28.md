---
id: SCENARIO-28
status: open
---

# SCENARIO-28: an unused category is reported only when nothing imported or counted uses it

Cadence: code-first (read-only detection and rendering; no write guard, atomic adapter or bug fix)
Acceptance test: `cmd/quarry/run_sync_unused_category_test.go` `Test_run_sync_records_unused_categories_but_not_one_an_investment_uses`
Narrow loop: `go test ./internal/importer/` | `go test ./internal/store/duckstore/ -run 'replace|findings|Findings'` | `go test ./internal/report/ ./internal/cli/` (whole packages) | `go test ./cmd/quarry/ -run 'unused|findings|sync|status'`
Mutation checks: investment reference recorded (drop the append on the `splits.go:49-52` skip) → acceptance test; survey set and count share one predicate → `Test_import_records_the_categories_of_counted_investment_entries_only`; a descendant's use marks the ancestor used → `Test_replace_counts_a_parent_used_when_a_subcategory_is`; hidden subtree excluded → `Test_replace_never_reports_a_hidden_category_or_one_under_it`; top node only (parent used or absent) → `Test_replace_reports_an_unused_parent_once_with_its_subcategories_as_items`; kind gate → `Test_replace_never_reports_a_system_category`
Runs: A (1-2) | B1 (3-5) | B2 (6-7) | V (8-9)
Size: OWNS A RUN — 5 batches (importer refs; detector; fixture fallout; sort + text; JSON/CSV pins), `importer` + duckstore adapter + `report` + `cli`

Gate (P2d-7): the importer CAN see the reference. `entriesQuery` (`importer/splits.go:11-17`) already reads `ZCATEGORYTAG` of every non-deleted entry; an investment entry is dropped only at the skip `splits.go:49-52`; `importer/not_imported_test.go:39-42` already builds an entry under an `EntInvestmentTransaction` parent. Real-file fidelity (does Quicken store an investment category on the entry?) is P2d-14's reference check, not provable here.

Survey (step 5): no new port, no new source query. Signature changes: `surveyTransactions` (one caller, `importer.go:85`), `mapSplits` (one caller, `importer.go:94`), `loadFindings`/`detectFindings` (one caller each, `duckstore.go:402`, `findings.go:63`) — grep, gopls not needed. Existing fault rows `ZTRANSACTION ids` / `ZCASHFLOWTRANSACTIONENTRY` (`import_faults_test.go:77,79,151,153`) already cover both reads. Persistence: none — the refs' only consumer is `build()` (`duckstore.go:402`); carried findings match by id; `--from` re-imports (`snapshot/import.go:136`). Read needs no change: the `c` join (`findings_read.go:30`) is ungated, `cs` gated, so items get `Category` = path, `Splits`/`Transactions` 0.

Pinned decisions (unruled in spec, decided here — see Handoff):
- Reference = any split row (`splits.category_id`, every account incl. closed/excluded) OR a `store.Rows.InvestmentCategoryIDs` entry. Any kind counts as use (a `system` child makes its parent used); only `income`/`expense` are reported.
- used(c) = c or any descendant referenced. blocked(c) = c hidden, an ancestor hidden, OR a hidden descendant (deleting the parent in Quicken takes the hidden child; fewer reports wins). reportable(c) = kind income/expense, not used, not blocked. Reported iff reportable and parent is absent or USED (literal "parent used or absent": a child of an unused `system` parent, or of an unused parent blocked by a hidden sibling, is not reported).
- Items: top node first, then every descendant in `full_path` case-insensitive, then id order (reader keeps `fi.rowid`); `category_id` only.
- Sort: `items[0].Category` case-insensitive, then id. Text: `  <id padded to widest in group>  <full_path>[ (and N subcategories)]<ignored marker>`, N = items-1, `humanize.Count(n, "subcategory", "subcategories")`, clause only when N > 0.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_sync_unused_category_test.go` `Test_run_sync_records_unused_categories_but_not_one_an_investment_uses` — fixture as `run_sync_similar_categories_test.go:13-33` plus `not_imported_test.go:37-42`: expense `Parking` (no split), expense `Brokerage Fees` referenced only by an entry (`CategoryTag`) under an `EntInvestmentTransaction` in a `BROKERAGENORMAL` account, hidden expense `Old` (no split), expense parent `Vacation` with child `Hotel` (`ParentCategory`), both unused; names not look-alikes under `CategoryKey`; `syncFindingsBundle` + `stringMap`: unused-category findings exactly `unused-category:cat-<Parking>`, `unused-category:cat-<Vacation>`; Vacation's items `cat-<Vacation>`, `cat-<Hotel>` (`transaction_id`/`payee_id` NULL)
- [ ] Step 2: no stub; run red at its assertion (no unused-category row)

### Build
- [ ] Step 3 (B1, importer): `store/store.go:137-149` `Rows.InvestmentCategoryIDs []string` + doc (not a table; nil when none — whole-struct `Rows` asserts); `importer/transactions.go:51-74` survey query gains `Z_PK`, `surveyTransactions` returns the set of counted investment PKs, count = its length (one predicate); `importer/splits.go:19-91` `mapSplits` takes the set and, on the `!ok` skip, records `cat-N` for an entry whose parent is in it, same category predicate as `splits.go:75-78` (exists, not uncategorized); `importer.go:84-121` wiring + Rows literal; tests in `not_imported_test.go`: `Test_import_records_the_categories_of_counted_investment_entries_only` (counted parent recorded; investment txn in a deleted account, deleted entry, `uncategorized`/missing category, Smart-entity parent, entity absent → not recorded; `NotImported` count unchanged); `Test_import_records_no_investment_categories_without_investment_entries` (field nil)
- [ ] Step 4 (B1, detector): new `store/duckstore/findings_unused_category.go` (`unusedCategoryQuery`: categories LEFT JOIN `categorySplits` for a used flag; tree walk in Go per pinned rules); `findings.go:60-108` `loadFindings`/`detectFindings` take the investment ids, detector appended last; `duckstore.go:402` passes `rows.InvestmentCategoryIDs`; `export_test.go:13-20` `UnusedCategoryQuery`; fault rows beside `findings_test.go:116-117` (query, scan → `detect unused-category findings`); new `findings_unused_category_test.go` (`Test_replace_*`): unused leaf flagged; `Test_replace_reports_an_unused_parent_once_with_its_subcategories_as_items` (3 levels, item order); `Test_replace_reports_an_unused_child_of_a_used_parent`; `Test_replace_counts_a_parent_used_when_a_subcategory_is` (split, and investment id, on a grandchild); `Test_replace_never_reports_a_hidden_category_or_one_under_it` (hidden leaf; hidden parent with unused child; visible unused parent with hidden child → nothing); `Test_replace_never_reports_a_system_category` (and a used `system` child keeps its expense parent used; expense child of an unused `system` parent not reported); income reported; split in a closed account and an excluded-from-reports account counts; investment id alone keeps a category off
- [ ] Step 5 (B1, fallout): `go test ./cmd/quarry/ ./internal/...`; every fixture category with no split now raises a finding (likely `run_sync_findings_test.go`, `run_status_findings_test.go`, `run_findings_ignore_test.go`, `run_transfers_test.go`, `run_sync_duplicates_test.go`). Store queries may filter by type; text/count pins get the category a split (distinct amounts — equal ones raise `duplicate`/`unlinked-transfer`) or drop the unused category; never weaken the detector

### Build (B2)
- [ ] Step 6 (B2, sort + text): `report/findings.go:145-180` `findingOrder` case `UnusedCategory` (path ci, id), drop its `//nolint:exhaustive`, post-switch return gets `// unreachable:` (`knownFindings` lists only `finding.Types()`); replace `report/findings_test.go:177-182` with `Test_findings_sorts_unused_categories_by_path_ignoring_case` (`auto:parking` before `Bank`; same path → id). `cli/render_findings.go:119-140` `liveFindingLines` case + new `unusedCategoryRows`; drop `//nolint:exhaustive` and the generic id-line loop (post-switch return `// unreachable:` as above); new `render_findings_unused_internal_test.go`: spec example verbatim (header + `  unused-category:cat-17  Auto:Parking`, `  unused-category:cat-40  Vacation (and 3 subcategories)`), `(and 1 subcategory)`, no clause at 0, ids padded (`cat-9` vs `cat-40`), ignored marker at line end under `--status all`; delete `render_findings_internal_test.go:236-259`, the case at `render_findings_status_internal_test.go:116-122`, move `:133-138` to an `Uncategorized` fixture; end-to-end `cmd/quarry/run_findings_unused_category_test.go` `Test_run_findings_lists_unused_categories_with_their_subcategory_count` (stdout incl. footer)
- [ ] Step 7 (B2, JSON/CSV pins, no production edit expected): `cli/json_findings_internal_test.go` unused item: `category` path, `category_id` set, `splits`/`transactions`/`payee`/`payee_id`/date/account/currency/amount null; `cli/findings_csv_test.go` row: category + category_id filled, rest NULL unquoted; one `--json` slice in `run_findings_unused_category_test.go`

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments (`store.FindingItem.Category` names unused-category; `findingOrder`, `liveFindingLines`, `Rows` docs); `docs/initial-prd.md:229` row + bullet after :232: the unused-category safety rule (importer investment-reference condition, hidden excluded, check-first fix copy) per Changes item 8

### Verify
- [ ] Step 9: full verification + `spec-check.py phase2d-findings` → tick SCENARIO-28 with its acceptance test

## Handoff

**Binding decisions**
- `store.Rows.InvestmentCategoryIDs` (nil when none) is the only carrier of investment category refs; never persisted — detection in `build()` is its only consumer, and every sync incl. `--from` re-imports.
- `surveyTransactions`' PK set and the `NotImported` count come from one predicate; refs recorded only for entries under a counted investment parent, filtered like splits (existing, not uncategorized).
- Used = self or any descendant referenced (any kind); hidden anywhere in the subtree or above blocks; reported iff income/expense, unused, unblocked, parent absent or used. Items top node first, then descendants path ci, id. Unruled readings (P2d-14 may re-rule).

**Left unbuilt — pending ruling** (each a category reference the v9 reference schema holds; a category used only there is reported unused):
- Smart-entity entries (`ZCASHFLOWTRANSACTIONENTRY` under `Z_ENT` SmartCashFlowTransaction) — no new read (survey already has `Z_ENT`); not verified that Smart = scheduled.
- `ZBUDGETLINEITEM.ZCATEGORYTAG`, `ZLOANSPLITENTRY.ZCATEGORY`, `ZACCOUNT.ZLOANINTERESTCATEGORY`, `ZQUICKFILLRULESPLITENTRY.ZCATEGORYTAG` — one new source query each plus rows in both fault tables (`import_faults_test.go`); fix copy names scheduled/budget only, not loans or memorized-payee rules.

**Traps**
- Every split-less fixture category now raises `unused-category`: filter store queries by type, or give the category a split with a distinct amount.
- Investment entries need their parent counted: account imported and non-deleted, `InvestmentTransaction` entity present — otherwise no ref and the category reports.
- Fault tables match the survey by `ZDELETIONCOUNT, 0) FROM ZTRANSACTION`: add `Z_PK` without moving that suffix.
- `report.findingOrder` and `cli.liveFindingLines` lose `//nolint:exhaustive` together, or `nolintlint` flags the survivor.

## Phase report
