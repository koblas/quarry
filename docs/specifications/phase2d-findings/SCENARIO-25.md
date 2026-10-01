---
id: SCENARIO-25
status: done
---

# SCENARIO-25: a payee whose category goes back and forth is in mixed categories

Cadence: code-first (read-only detection and rendering; no write guard, atomic adapter or bug fix)
Acceptance test: `cmd/quarry/run_sync_mixed_test.go` `Test_run_sync_records_costco_as_mixed_categories_and_not_shell`
Narrow loop: `go test ./internal/store/duckstore/ -run 'mixed|findings|Findings|replace'` then `go test ./internal/report/ ./internal/cli/` (whole packages) and `go test ./cmd/quarry/ -run 'mixed|findings'`
Mutation checks: revisit guard `changes > distinct-1` in the walk → `Test_replace_flags_a_payee_only_when_a_category_is_revisited`; walk order (date, then source id) → `Test_replace_walks_a_payees_transactions_by_date_then_source_id`
Runs: A (1-2) | B1 (3) | B2 (4-5) | V (6-7)
Size: OWNS A RUN — 3 Build batches (change-count walk; payee/category rows + sort; JSON/CSV cells), `report` + duckstore adapter + cli

Survey (step 5): no new port; `store.Store.Findings` keeps its signature. `store.FindingItem` (`internal/store/store.go:368-382`) gains `Transactions int`. LSP `findReferences` on `FindingItem`: duckstore reader, `report` sorts, `cli` renderers/JSON; fakes use keyed literals. `duckstore.findingItem` (`findings.go:48-51`) gains `payeeID, categoryID`; `mergeFindings` (`findings.go:184-207`) writes `nil, nil` for them today. `finding.MatchDays` (`finding.go:36-38`) is the precedent for the new constant.

Pinned decisions (unruled in spec, decided here):
- Count is NOT stored (P2d-1 fixes `finding_items`' columns): the read derives it, type-gated on `f.type = 'mixed-categories'`, from the SAME SQL const the detector selects from (one owner of "transactions with exactly one categorized `v_cash_flow` row, payee not NULL"). `Transactions` 0 = not applicable (a mixed item is always >= 1).
- Detector walk: per payee, rows ordered by `date`, then `transactions.source_id` (BIGINT, numeric); categories compared by `category_id`; flagged iff `n >= finding.MixedMin` and `distinct >= 2` and `changes > distinct-1`. Id `finding.ID(MixedCategories, <payees.id>)` (`payee-N`); no-payee transactions never qualify.
- Items: one per category (`payee_id`, `category_id`; `transaction_id`/`split_id` NULL), written in order transactions desc, category path case-insensitive, category id; the reader keeps insert order (`fi.rowid`).
- Read gives a mixed item the payee name (join on `fi.payee_id`), the category `full_path` (join on `fi.category_id`), `Transactions`; `Date` zero, `Account*` empty, `Amount` 0.
- Text (`liveFindingLines` case `MixedCategories`): id line `  <id>  <payee>  N categories, M transactions` (id and payee padded to widest across findings, then `ignoredMarker`), then four-space rows `<category path>  <N transactions>` (paths padded to widest within the finding, counts right-aligned, two-space gap, `humanize.Count`). Header/fix from `Fix()`; group `(N)` = default branch of `findingsGroupCount`.
- Sort (`findingOrder`): sum of item `Transactions` desc, payee case-insensitive, id (string compare).
- JSON/CSV: `findingItemDocument` `Date`, `AccountID`, `Account`, `Currency`, `Amount` become `*string`, null iff the item has neither `transaction_id` nor `split_id`; mixed item fills `payee` (name), `category` (path), `transactions`, `payee_id`, `category_id`; `splits` null. Existing types' documents stay byte-identical.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_sync_mixed_test.go` `Test_run_sync_records_costco_as_mixed_categories_and_not_shell` — v9fixture (template `run_sync_unlinked_test.go`, `run_sync_duplicates_test.go:17-40`): Costco Groceries, Household, Groceries, Auto:Fuel; Shell Auto for years then Auto:Fuel; assert the store holds `mixed-categories:<Costco payee id>` with three items (payee_id, category_id) and no finding for Shell
- [x] Step 2: no stub needed (no new symbol); run red at its assertion, plus the `cmd/quarry` narrow loop once to list fixtures the detector will now hit

### Build
- [x] Step 3 (B1, detector): `internal/finding/finding.go:36-38` `MixedMin = 3` (next to `MatchDays`, one-line doc); new `internal/store/duckstore/findings_mixed.go` (shared row const, `mixedCategoriesQuery`, `detectMixedCategories`, unexported walk func); `findings.go:48-51,77-92,184-207` `findingItem` fields, `detectFindings` between unlinked and uncategorized (slice order = `finding.Types`), `mergeFindings` writes both ids; `export_test.go:13-17` `MixedCategoriesQuery`; new `findings_mixed_test.go`: Costco/Shell pair; 2 txns (A,B) no; A,A,B and A,B,B (changes = distinct-1) no; A,B,A (3 = `MixedMin`) yes; 3 categories A,A,B,B,C no and A,B,A,C yes (distinct-1+1); one category x5 no; same date ordered by source id (A src1, A src3, B src2 flagged; B src3, A src2 not); date before source id (src1 A 08-01, src2 A 08-03, src3 B 08-02 flagged); multi-split middle transaction excluded (control: single split flags), two categorized splits of one category excluded, categorized+uncategorized sibling still counts; uncategorized middle row excluded; not-in-reports account excluded; no-payee never; two interleaved payees judged separately; item rows and order (count desc, path, id, tie cases); fault rows in `findings_test.go:107-112` for the query and its scan (`detect mixed-categories findings`)
- [x] Step 4 (B2, read + sort + text): `store.go:368-382` `Transactions` + doc; `findings_read.go:13-27,52-53,68-74` payee name/category path for payee-keyed items and the derived count (type-gated, one query), test in `findings_read_test.go` (name, path, count, zero date/account; count skips a multi-split transaction of that payee/category; duplicate/uncategorized item `Transactions` stays 0). `report/findings.go:145-163` `findingOrder` case + doc line, test beside `findings_test.go:102` (sum desc, `zed`/`Bakery` case tie, id). `cli/render_findings.go:115-138` case `MixedCategories` + `mixedRows` helper; exact-output tests in `render_findings_internal_test.go` (spec example pair with the ruled header/fix and hint, `1 transaction` row, ignored marker under `--status all`, two findings padding id and payee, thousands in `1,200 transactions`) and end-to-end `cmd/quarry/run_findings_mixed_test.go` `Test_run_findings_lists_a_mixed_categories_payee_with_its_category_rows`
- [x] Step 5 (B2, JSON/CSV): `cli/json_findings.go:29-44,83-91` pointer fields, null rule for payee/category items, `Transactions` from the item (0 → null); `csv_findings.go:65-76` date/account/currency/amount via `csvOptional`; doc comments updated. Delete `csv_findings_internal_test.go` (a real type now reaches the counts arm). Tests: `json_findings_internal_test.go` (mixed item keys, nulls; a duplicate item's date/account unchanged), `findings_csv_test.go` (mixed row: date, account, currency, amount NULL unquoted; payee, category, transactions, payee_id, category_id filled), one `--json` slice in `run_findings_mixed_test.go`

### Sweep
- [x] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; fixtures where a payee revisits a category over >= 3 transactions will gain a finding: distinct payees or categories in the fixture, never weaken the detector; doc comments on new symbols; update `newFindingItemDocument`/`FindingItem` docs

### Verify
- [x] Step 7: full verification + `spec-check.py phase2d-findings` → tick SCENARIO-25 with its acceptance test

## Handoff

**Binding decisions**
- Payee/category item counts are derived at read time, type-gated, from one SQL const shared with the detector; `finding_items` gets no count column (P2d-1). 26's transactions-per-payee and 27's splits-per-category follow the same route; `store.FindingItem.Transactions` (0 = n/a) is 26's too; 27's `Splits` must extend the unlinked-only gate, not reuse its meaning.
- `findingItemDocument` `date/account_id/account/currency/amount` are `*string`, null iff no `transaction_id` and no `split_id`: 26-28 items reuse it.
- `mergeFindings` writes `payee_id`/`category_id` from `findingItem`; a later detector fills them the same way.
- Mixed item order is detector-owned (count desc, path, id); the reader and `report` never re-sort items.

**Left unbuilt**
- Detectors, rows, sorts for `payee-variants`, `similar-categories`, `unused-category`; `--json` `splits` stays null; `findingLines`' `// unreachable` return and `liveFindingLines`' `//nolint:exhaustive` go after 28 — 26-28.

**Traps**
- `finding.MixedMin` below 3 is an equivalent mutant: `changes > distinct-1` with `distinct >= 2` already needs 3 transactions; only raising it is observable (A,B,A).
- Spec example spacing (`    Groceries           30 transactions`, 11 spaces) contradicts its own rule; the payee-variants and similar-categories examples match width-of-widest + two-space gap + right-aligned count. Build to the rule and pin it; raised to the orchestrator.
- `payeeOf` (first item's payee) stays valid for mixed only because the reader fills the payee name from `fi.payee_id`.
- A multi-split transaction of the payee is excluded from BOTH detection and the read count; the two must not drift.

## Phase report

Run V done. `go build ./...` ok; `golangci-lint run ./...` 0 issues; covered full suite `go test rc=0`; `uncovered-diff.py` 0 uncovered added lines since b9909f3; `go test -race` on store, report, cli, finding, cmd/quarry rc=0; `spec-check.py phase2d-findings` OK. SCENARIO-25 ticked in specification.md; STATE.md rewritten; status: done. No production edits in V.

test-stats (base b9909f3): cmd/quarry 341 (+3), internal/cli 234 (+8), internal/report 109 (+1), internal/store/duckstore 270 (+10), TOTAL 954 (+22) tests, tempdir 429 (+4), disk 401 (+3).
