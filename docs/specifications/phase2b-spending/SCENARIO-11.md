---
id: SCENARIO-11
status: open
---

# SCENARIO-11: spend groups by tag and warns about multi-tagged splits

Cadence: code-first (no bug fix, write-safety or atomicity item touched)
Acceptance test: `cmd/quarry/run_spend_by_test.go` `Test_run_spend_by_tag_counts_a_two_tag_split_under_both_tags_once_in_the_total_and_warns`
Narrow loop: `go test ./internal/store/duckstore/ -run 'pending' && go test ./internal/cli/ -run 'pend' && go test ./cmd/quarry/ -run 'Test_run_spend'`
Mutation checks: Total built from the tag-joined rows (double-counts a two-tag split) -> `Test_spending_by_tag_counts_a_two_tag_split_under_both_tags_and_once_in_the_total`; INNER JOIN on split_tags (untagged bucket dropped) -> `Test_spending_by_tag_groups_untagged_splits_under_a_nil_key_first`; W1 counts split_tags rows not splits (3-tag split counts 3) -> `Test_spending_by_tag_counts_splits_with_several_tags_not_their_tags`; SQL `> 1` -> `>= 1` (a one-tag split counted) -> same test's one-tag control; count ignoring window / income splits -> same test's outside-window and income multi-tag splits; cli `> 0` -> `>= 0` (prints "0 splits carry") -> `Test_spend_by_tag_prints_no_warning_when_no_split_has_two_tags`; singular at 1 -> `Test_spend_by_tag_warns_with_the_singular_phrase_for_one_split`; warning before stdout -> `Test_spend_by_tag_writes_no_warning_when_stdout_fails`; count computed for other groupings -> `Test_spending_by_category_reports_no_multi_tag_splits`
Runs: A (1-2) | B1 (3-4) | B2 (5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (report untouched; store types + duckstore adapter + cli)

## Decisions this plan makes
- **Multi-tag count = a second statement on the same `openRead` handle**, run only when `params.By == SpendByTag`, returned as `store.Spending.MultiTagSplits int`. One statement is not clean: the tag rows are join-multiplied while Total and the count are per split, so a marker row (`grouping = 2`, fake currency) would fork the shared scan. `report.Spending` embeds `store.Spending`, so `report.Server.Spend` needs no change.
- **Tag query is its own SQL, same four result columns (`key, currency, cents, grouping`) as `spendingQuery`**, so the scan loop is shared: tag rows = `v_spending` LEFT JOIN `split_tags` LEFT JOIN `tags` grouped by tag name + currency (HAVING sum <> 0; order `tag IS NOT NULL, lower(tag), tag, currency`), UNION ALL totals = `v_spending` grouped by currency alone (grouping 1, zero-net kept). Rows and Totals stay one statement. `tags.name` has no UNIQUE: de-duplicate `(split_id, tag name)` before summing so two same-named tags on one split count once.
- W1 copy is composed in `internal/cli`: `humanize.Count(n, "split carries", "splits carry") + " more than one tag, so the rows add up to more than the total"`, emitted through `emit(..., "quarry: warning: ", warnings)` and passed unprefixed into `renderSpendingJSON` `warnings[]`.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_spend_by_test.go` `Test_run_spend_by_tag_counts_a_two_tag_split_under_both_tags_once_in_the_total_and_warns` — `spend --by tag` over a two-tag split, a one-tag split and an untagged split; asserts stdout table (`Tag` header, `(no tag)` first, tags by `lower(name)`, one Total per currency counting each split once) and stderr exactly the W1 line. `cmd/quarry/run_spend_test.go:20-58` `spendSplit` gains `tags []string`; `spendRows` gains a `Tags` table and `SplitTags` links.
- [x] Step 2: `internal/store/store.go:321-359` `SpendByTag` (after `SpendByPayee`), `Spending.MultiTagSplits` (doc: splits in the window carrying more than one tag, set only for `SpendByTag`); `internal/cli/spend_grouping.go:155-158` table entry `{name: "tag", header: "Tag", missing: "(no tag)"}`. Red: totals/rows wrong until the store reads tags (`ErrUnsupportedGrouping` from duckstore, exit 1).

### Build
- [x] Step 3: `internal/store/duckstore/spending.go:25-31,39-70` `spendingQueries[SpendByTag]` (tag SQL above) — Tests in `spending_test.go` (`addSplit` in `views_test.go:44-55` needs a tags field or a `SplitTags` append): `Test_spending_by_tag_counts_a_two_tag_split_under_both_tags_and_once_in_the_total`, `Test_spending_by_tag_groups_untagged_splits_under_a_nil_key_first`, sort (`Test_spending_by_tag_sorts_ignoring_case_then_byte_order_then_currency`), zero-net tag omitted but kept in Total, two same-named tags on one split count once, CAD-before-USD Totals.
- [x] Step 4: `spending.go:39-70` `Spending` — after the main read, when `params.By == store.SpendByTag`, a `multiTagQuery` const (count of `v_spending` splits in window whose `split_tags` count > 1, same window args) sets `MultiTagSplits`. Tests: `Test_spending_by_tag_counts_splits_with_several_tags_not_their_tags` (3-tag and 2-tag splits -> 2; one-tag control excluded; multi-tag split outside the window and a multi-tag income split excluded; exactly-1 and 0 cases), `Test_spending_by_category_reports_no_multi_tag_splits`. Fault tests via `spyReadDB` (`spending_test.go:165-185` pattern) for the second statement: query fault and scan fault on it (spy must fail the second call, not the first), and connection still closed once.
- [ ] Step 5: `internal/cli/spend.go:116-139`, `internal/cli/json_spend.go:219-248` — `spendWarnings(spending)` returns `[]string{}` or the one W1 line (`> 0` only), passed to both `renderSpendingJSON(spending, warnings)` (replaces the literal `[]string{}` at :237) and `emit`; `spendTagRowDocument{Tag *string "tag"; Currency; Spent}` chosen in `spendRowDocumentFor` (:243, becomes a switch). Update `spend_test.go:68` refusal list: drop `"tag"` (`month` stays). Tests (fake store `MultiTagSplits`, `internal/cli/fakes_test.go` needs no change): text `Tag` header and `(no tag)` label; JSON `tag` key null + `warnings[]` unprefixed + `by":"tag"`; `Test_spend_by_tag_warns_with_the_singular_phrase_for_one_split` (1), plural with thousands (`1,234 splits carry`), `Test_spend_by_tag_prints_no_warning_when_no_split_has_two_tags` (0: stderr empty, `warnings: []`), `Test_spend_by_tag_writes_no_warning_when_stdout_fails` (failing writer + count > 0: stderr empty, error is the stdout-write refusal), `--by category` with a count set prints no warning.

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `SpendByTag`, `MultiTagSplits`, `spendWarnings`; the `Spend` Long text (`spend.go:109-110`) already states the multi-tag rule and stays verbatim.

### Verify
- [ ] Step 7: full verification block + `spec-check.py phase2b-spending`; tick SCENARIO-11 with its acceptance test; rewrite STATE.md (SCENARIO-12 adds `SpendByMonth` the same way; `Left unbuilt` drops `SpendByTag`).

## Handoff

**Binding decisions:**
- `store.Spending.MultiTagSplits` is the W1 input, set by duckstore only for `SpendByTag`, counted over `v_spending` splits in the window (not income, not outside the window) — S12 (month) leaves it 0; S14's `--account` filter must apply to this count too, as it does to rows and Totals.
- Tag Totals are per split (rows sum >= Total for tags); category/payee Totals keep coming from `GROUPING SETS`. The tag query is the first that cannot: SCENARIO-12 (month) may reuse `spendingQuery`.
- W1 is built in `internal/cli` (`spendWarnings`) and reaches `warnings[]` unprefixed; E1/E2/W2 (S17, S14) append to the same slice, so keep it a `[]string` assembled before `renderSpendingJSON`, stdout written before any of them (`emit`).

**Left unbuilt:** `SpendByMonth` + `partial` row field + Status column (S12); `--since/--until` (S13); `--account` and `AccountIDs` in duckstore (S14); E1/E2 (S17).

**Traps:**
- `Test_spend_refuses_a_by_that_names_no_grouping_before_reading_the_store` lists `"tag"` as refused: it goes red the moment the table entry lands; remove it, do not weaken S4.
- `tags.name` is not unique and `split_tags` has no FK: LEFT JOIN `tags` after `split_tags`, or an untagged split and a dangling link look the same.
- `GROUPING SETS ((tag, currency), (currency))` over the tag-joined source double-counts Total for a multi-tag split: the exact bug the first mutation check pins.

## Phase report

Run B1 (steps 3-4) done; acceptance test now differs only at its stderr assertion (W1 warning missing: `expected "quarry: warning: 1 split carries ..."`, `actual ""`); stdout table already matches.

Files:
- `internal/store/duckstore/spending.go:13-75` `splitTagNames` CTE, `spendingByTagQuery` (tag rows + per-currency totals over splits, `$1`/`$2` reused so args stay two), `multiTagSplitsQuery`, `spendingQueries[SpendByTag]`; `Spending` runs the count on the same handle after the main read when `By == SpendByTag`.
- `internal/store/duckstore/spending_tag_test.go` (new, 14 tests: rows/total/nil-first/sort/CAD-USD/zero-net/same-name/dangling link, count 2/1/0, category 0, query fault, scan fault, close once).
- `internal/store/duckstore/views_test.go` `splitSpec.tags` (tag ids) -> `SplitTags`; `fakes_test.go` `spyReadDB.passQueries` (first N non-check queries run for real).
- Mutations run, each reddened: total from tag-joined rows, INNER JOIN, count split_tags rows (5 not 2), `> 1`->`>= 1`, ignore window, include income, count for every grouping, drop the second query. Lint `0 issues` on `./internal/store/...`.

Green: `go test ./internal/store/...`. Not done: step 5 (cli `spendWarnings`, `spendTagRowDocument`, cli tests), Sweep, Verify; mutations for cli `> 0`, singular, warning-before-stdout belong to B2.
