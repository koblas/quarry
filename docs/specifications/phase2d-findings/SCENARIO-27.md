---
id: SCENARIO-27
status: open
---

# SCENARIO-27: categories that differ only in case, punctuation or a plural are similar

Cadence: code-first (read-only detection and rendering; no write guard, atomic adapter or bug fix)
Acceptance test: `cmd/quarry/run_sync_similar_categories_test.go` `Test_run_sync_records_similar_expense_categories_and_not_the_income_one`
Narrow loop: `go test ./internal/finding/` | `go test ./internal/store/duckstore/ -run 'replace|findings|Findings'` | `go test ./internal/report/ ./internal/cli/` (whole packages) | `go test ./cmd/quarry/ -run 'similar|findings|sync'`
Mutation checks: kind in the key (income never joins expense) → `Test_CategoryKey` income/expense rows + `Test_replace_never_groups_an_income_and_an_expense_category`; ids of an income and an expense group with one key differ → same test (build would fail on PK otherwise); `>= 4` runes not bytes → `Test_CategoryKey` row `Ées`; `ies`→`y` and `ss` kept → rows `Utilities`, `Business`; hidden included → `Test_replace_flags_hidden_similar_categories`; `>= 2` per key → `Test_replace_flags_similar_categories_only_with_two_sharing_a_key`; read gate `f.type = 'similar-categories'` → `Test_Findings_gives_only_a_similar_categories_item_the_category_total` (hand-built row: a mixed item also carries `category_id`)
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches (key; detector; read + sort + text; JSON/CSV), `finding` leaf + duckstore adapter + `report` + `cli`

Survey (step 5): no new port; `store.Store.Findings` unchanged. `store.FindingItem.Category`, `Splits` exist (`store.go:379-380`); `findingItem.categoryID` and `mergeFindings` already write `category_id`. Callers of changed symbols: `findingOrder` (one, `report/findings.go`), `findingsGroupCount` (one), `newFindingEntryDocument` (`json_findings.go:67`, csv shares it). Grep (quoted glob) confirmed no code parses a finding id back into type/entity (`idSeparator` only builds it in `finding.ID`; `LastIndex` is `numericSuffix` for pair ids), so a `:` inside an entity is safe; `newFindingItemDocument` has one caller (`json_findings.go:70`). LSP not needed (no signature changes).

Pinned decisions (unruled in spec, decided here):
- Key owner: `finding.CategoryKey(kind, fullPath string) string` in `internal/finding/finding.go` beside `PayeeKey`; returns the id entity, `""` = skipped. Same literal reading as `PayeeKey`: `strings.ToLower`, non `unicode.IsLetter`/`IsDigit` rune = space, no diacritic folding or NFC. Levels = `full_path` split on `:` (the importer's joiner, `importer/categories.go:102`). Per level: tokens; digits are KEPT (unlike payees: `Auto 2024` -> `auto-2024`); a token of >= 4 RUNES (`utf8.RuneCountInString`, not bytes) is singularised: ends `ies` -> `y`, else ends `s` and not `ss` -> drop the `s`; tokens joined `-`, levels joined `/`. Literal, accepted: `Taxes` -> `taxe` (!= `Tax`), `Pies` -> `py`. A level with no token stays empty (`Auto:&:Fuel` -> `auto//fuel`); no token in any level, or a kind other than income/expense, -> `""`.
- Kind separation and id collision: grouping is by the entity, and `CategoryKey` prefixes `income:` for kind income (expense keeps the bare key, so the spec example id `similar-categories:grocery` holds). Keys never hold `:`, so the prefix cannot collide with an expense key. Without it an income and an expense group with one key (`Gift`/`Gifts` in both) are two findings with one id: `findings.id` is a PRIMARY KEY, so the whole sync would fail. **Id-grammar departure: income group ids read `similar-categories:income:<key>`; ruled by product-vision (spec P2d-2/P2d-7).** The prefix is constant, never conditional on a collision (ids are in users' config.toml).
- Detector: one query `categories c LEFT JOIN (categorySplits) cs ON cs.category_id = c.id WHERE c.kind IN ('income','expense') ORDER BY c.id` (hidden included, used or not); `categorySplits` is a SHARED const (`SELECT category_id, count(*) AS n FROM splits WHERE category_id IS NOT NULL GROUP BY category_id`: every account, closed and excluded included, not `v_cash_flow`) that the read also uses (count not stored, P2d-1). Group in Go by `CategoryKey`; >= 2 categories per key; two categories with an identical `full_path` group. Findings in key order; items in splits desc, `full_path` case-insensitive, category id; item = `category_id` only.
- Read: `cs` join `cs.category_id = fi.category_id AND f.type = 'similar-categories'`; splits column `COALESCE(ts.n, cs.n, 0)`. The `c` join already resolves `fi.category_id` to the path.
- Sort (`findingOrder`): sum of item `Splits` desc, then id. Items never re-sorted by reader/`report`.
- Text: header `Similar categories (N groups)` / `(1 group)` (`findingsGroupCount`, beside PayeeVariants), clause only when an open finding is listed; id line `  <id>  N categories` (id padded to widest across the group, `humanize.Count`, ignored marker at line end, NO splits total); rows `    <full_path>  <N splits>`: paths padded to the widest in the finding, counts right-aligned, `humanize.Count(n, "split", "splits")`, runes. Row label is `*item.Category`, NOT `categoryCell` (its `Splits > 1` arm would print `(split)`).
- JSON/CSV: `newFindingEntryDocument` sets `Splits` (pointer, `0` stays `0`) for `f.Type == SimilarCategories` items only; unlinked items also carry `FindingItem.Splits` and their `splits` stays null. `category` = path, `category_id` set, `transactions` null (item `Transactions` 0), date/account/currency/amount/payee null (no transaction or split).

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_sync_similar_categories_test.go` `Test_run_sync_records_similar_expense_categories_and_not_the_income_one` — v9fixture as `run_sync_payee_variants_test.go:17-45`: expense `Groceries` + `Grocery` (`Type: new(int64(1))`), income `Grocery` (`Type: new(int64(2))`), control expense `Auto`; `syncFindingsBundle`, `stringMap`: exactly `similar-categories:grocery` in `findings`, items = the two expense category ids (`category_id` set, `transaction_id`/`payee_id` NULL); no `income:` finding
- [ ] Step 2: no stub needed; run red at its assertion, plus `go test ./cmd/quarry/ ./internal/... ` once to list fixtures the detector will now hit (categories equal under the key: give look-alikes distinct names, never weaken the rule)

### Build
- [ ] Step 3 (B1, key): `internal/finding/finding.go:41` (beside `PayeeKey`) `CategoryKey` + doc (id contract); `finding_test.go:52` `Test_CategoryKey` table: `Groceries`/`Grocery`/`GROCERY` -> `grocery`; income `Grocery` -> `income:grocery`; `Auto:Fuel` -> `auto/fuel`; `Auto & Fuel` -> `auto-fuel`; `Utilities` -> `utility`; `Business` -> `business`; `Gas`, `Gass` boundary (3 runes untouched, `Cats` -> `cat`); `Ées` -> `ées` (3 runes, 5 bytes), `Cafés` -> `café`; `Auto 2024` -> `auto-2024`; `Taxes` -> `taxe`; `Café` != `Cafe`; `Auto:&:Fuel` -> `auto//fuel`; `""`, `&&`, `::` -> ``; kind `system` -> ``
- [ ] Step 4 (B1, detector): new `internal/store/duckstore/findings_similar_categories.go` (`categorySplits` const, `similarCategoriesQuery`, `detectSimilarCategories`, grouping func as `findings_payee_variants.go:50-75`); `findings.go:78-103` `detectFindings` between variants and `slices.Concat` (order = `finding.Types`); `export_test.go:13-18` `SimilarCategoriesQuery`; fault rows beside `findings_test.go:114-115` (query and scan, `detect similar-categories findings`); new `findings_similar_categories_test.go`: pair flagged; single no (`Test_replace_flags_similar_categories_only_with_two_sharing_a_key`); three in one finding; two keys two findings; hidden one counts (`Test_replace_flags_hidden_similar_categories`); `system` kind never; unused categories (0 splits) count; `Test_replace_never_groups_an_income_and_an_expense_category` (control: income `Grocery` + `Groceries` is its own `income:` finding, ids distinct, build succeeds when both kinds hold the same key); identical `full_path` flagged; item order splits desc, path ci, id (tie rows); splits counted across accounts incl. closed

### Build (B2)
- [ ] Step 5 (B2, read + sort + text): `findings_read.go:19,33` type-gated `cs` join, `COALESCE(ts.n, cs.n, 0)`; `findings_read_test.go` similar items carry path, `Splits`, nil `Transactions`, zero date/account; mixed and unlinked items unchanged; `Test_Findings_gives_only_a_similar_categories_item_the_category_total` (hand-built row, gate). `report/findings.go:147-175` `findingOrder` case `SimilarCategories` + `splitsOf` + doc line; test beside `findings_test.go:50` (`variants` helper pattern: sum desc, id tie). `cli/render_findings.go:74` `findingsGroupCount` group form for `SimilarCategories`, `:120-135` `liveFindingLines` case + new `similarCategoryRows` (reuse `transactionRows`' shape with a splits label; pad ids across findings); new `render_findings_similar_internal_test.go` (model `render_findings_variants_internal_test.go:33-97`): the spec example verbatim, ruled header `Similar categories (1 group): merge each group into one category in Quicken`, `(2 groups)`, ignored-only group has no clause, ids padded across findings, `1 split`, `0 splits`, `1,200 splits`, nested path `Auto:Fuel`, an item with `Splits` 2 prints its count not `(split)`, ignored marker on the id line under `--status all`, non-ASCII pad by runes; end-to-end `cmd/quarry/run_findings_similar_categories_test.go` `Test_run_findings_lists_similar_categories_with_a_row_per_category` (stdout block incl. footer)
- [ ] Step 6 (B2, JSON/CSV): `json_findings.go:67-76` `newFindingEntryDocument` sets `Splits` for `SimilarCategories`; `json_findings_internal_test.go` (item: `category`, `category_id`, `splits` incl. a `0`; `transactions`, `payee`, `payee_id`, date, account, currency, amount null; an unlinked item's `splits` still null); `findings_csv_test.go` (row: category, splits, category_id filled; rest NULL unquoted; `0` splits is `0` not NULL); one `--json` slice in `run_findings_similar_categories_test.go`

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on new symbols (`store.FindingItem.Category`/`Splits` comments name similar-categories; `findingOrder` doc; `findingsGroupCount` doc); `liveFindingLines` `//nolint:exhaustive` and `findingLines`' `// unreachable` return stay (28 remains)

### Verify
- [ ] Step 8: full verification + `spec-check.py phase2d-findings` -> tick SCENARIO-27 with its acceptance test

## Handoff

**Binding decisions**
- `finding.CategoryKey(kind, fullPath)` is the only owner of the similar-categories key and id entity (ids are in users' config.toml; a change needs a ruling). Digits kept, >= 4 RUNES, `ies`->`y`, trailing `s` (not `ss`) dropped, levels split on `:` joined `/`; income entities carry the constant `income:` prefix so an income and an expense group can never share an id (PK would fail the sync).
- Category split count derived on read from the SAME `categorySplits` const the detector selects from, via a `cs` join gated on `f.type = 'similar-categories'`; 28's unused-category items also carry only `category_id`, so 28 adds its own gate and never reuses `cs`.
- `--json`/`--csv` `splits` is filled by type in `newFindingEntryDocument`, never from `FindingItem.Splits > 0` (unlinked items carry it too).

**Left unbuilt**
- Detector, rows and sort for `unused-category`; `findingLines`' `// unreachable` return and `liveFindingLines`' `//nolint:exhaustive` go with it (28).

**Traps**
- `categoryCell` prints `(split)` when `Splits > 1`: similar rows use `*item.Category`.
- Mixed items also carry `category_id`: an ungated `cs` join gives them a splits count; invisible with detector-fed fixtures, hence the hand-built row.
- Existing fixtures with look-alike categories (`Groceries`/`Grocery`, same kind) now raise a finding: rename them, never weaken the rule.
- `-run 'mixed'`-style patterns miss duckstore `Test_replace_*` tests: use `-run 'replace|findings|Findings'`.
