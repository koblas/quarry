---
id: SCENARIO-01
status: open
---

# SCENARIO-01: quarry search lists matching transactions newest first, flagged, with their splits

Cadence: code-first. Nothing is on the mandatory set: the `v_cash_flow` edit only extracts a read-only view's fragments.
Acceptance test: `cmd/quarry/run_search_json_test.go` `Test_run_search_json_lists_every_transaction_newest_first_flagged_with_its_splits`
Acceptance test (SCENARIO-05, folded): `cmd/quarry/run_search_window_test.go` `Test_run_search_without_since_or_until_searches_every_date`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/report/... ./internal/cli/ ./cmd/quarry/ -run 'search|Search|cash_flow|spending_|reads|window_flags|help_prints|no_store|interrupt|home_is_unset|usage_hint'`
Mutation checks: transfer leg (drop `OR x.to_split_id = <alias>.id`) → `Test_cash_flow_leaves_out_what_quicken_reports_leave_out` + `Test_search_flags_transfers_and_excluded_from_the_cash_flow_fragments`; `reportedAccount` (drop `AND NOT a.linked_tracking`) → same pair; excluded_from_reports (drop it from the shared transaction fragment) → same pair; search order tie-break `source_id DESC`→`ASC` → `Test_search_lists_newest_first_breaking_a_date_tie_by_source_id`; `date DESC`→`ASC` → same test; `count(*) OVER ()` replaced by a count after the cut → `Test_search_counts_every_match_when_the_limit_cuts`; open-bound arm (`$n IS NULL OR`) dropped → `Test_search_bounds_dates_only_where_given` (neither-bound row) + the SCENARIO-05 acceptance test
Runs: A (1-2) | B1 (3-5) | B2 (6) | V (7-8)
Size: OWNS A RUN: 4 batches, 1 feature package (`report` + `report/document`; `duckstore` is its Store adapter)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_search_helpers_test.go` (new) `searchRows` fixture + `decodeSearchJSON` (DisallowUnknownFields); `run_search_json_test.go` (new) acceptance with a transfer pair, a report-excluded transaction, one in a not-in-reports and one in a linked-tracking account, a 2-split transaction with split memos, a same-date pair inserted out of source order, and the whole §2.4 document expected; `run_search_window_test.go` (new) folded S05 with rows dated 2025-12-31 and 2027-01-15 under `spendEnv` (now 2026-09-29), both listed, `since`/`until` null. Use `require` on the exit code before decoding, so the test cannot panic
- [x] Step 2: signature-only stubs: `internal/store/store.go` (after `:744`) `SearchWindow{Since, Until *time.Time}`, `SearchParams{Window, AccountIDs, Limit}`, `SearchRow`, `SearchSplit`, `Search{Rows, Matched, Transactions}`; `internal/report/store.go:11-28` `Store.Search`; `internal/store/duckstore/search.go` (new) `(*Store).Search`; `internal/report/search.go` (new) `SearchRequest`, `Search`, `(*Server).Search`; `internal/report/window.go` `ParseSearchWindow`; `internal/report/document/search.go` (new) `Search`, `NewSearch`; `internal/report/fakes_test.go:11-90` `fakeStore.Search`, plus any other implementer the build lists. Expected red for both tests: the `require` on the exit code fails with stderr `unknown command "search"`, because the command is registered only in step 5

### Build
- [x] Step 3: `internal/store/duckstore/schema.go:163-195`: extract a transfer-leg fragment parameterised by split alias and a reported-transaction fragment (`reportedAccount` + `NOT t.excluded_from_reports`). `cashFlowViewDDL` then uses both, with the same semantics, and `views_test.go`/`views_fx_test.go` stay green and unedited. `duckstore/search.go`: rows statement (transaction-grain CTE: flags from the shared fragments, `count(*) OVER ()`, one order const, `LIMIT $n` bound nil when Limit is 0; then splits joined in split `source_id` order) and a span statement (`filter.go` `marks`, no `reportedAccount`). `search_test.go` (new):
  - `Test_search_flags_transfers_and_excluded_from_the_cash_flow_fragments`, one row each: from-leg only, to-leg only, orphan leg, excluded_from_reports, not-in-reports, linked tracking, both flags, system category (no flag), zero uncategorized split (no flag)
  - `Test_search_lists_newest_first_breaking_a_date_tie_by_source_id`: limit 0, plus limit 1 on the same-date pair (the higher source id survives)
  - `Test_search_counts_every_match_when_the_limit_cuts`: matched uncut; control limit == matched > 0 (nothing cut); a 2-split transaction counted once; limit 1 on a 2-split newest row keeps both splits; limit 0 lists every one
  - `Test_search_lists_each_transactions_splits_in_source_order`: no splits → empty; NULL and `""` memo; uncategorized; transfer-leg category nil
  - `Test_search_bounds_dates_only_where_given`: neither, since only, until only, both; each bound inclusive, with one day outside
  - `Test_search_keeps_only_the_named_accounts`: none, one, two, a not-in-reports account
  - `Test_search_spans_the_named_accounts_transactions`: all; a named not-in-reports account gives a non-zero span; an empty store gives the zero range
  - `read_faults_test.go:21-40`: `Search` readOp in `rowReads`, plus a span-statement fault (`passQueries: 1`, query and scan) after the `charges_test.go:298` precedent
- [x] Step 4: `internal/report/window.go:86-145` `ParseSearchWindow`: no clock, reuses `parseDateBound`, returns the existing `WindowError` (`WindowNotADate`, `WindowSinceAfterUntil` only). `internal/report/search.go` `(*Server).Search`: `namedAccounts` → `store.Search` → `readRefusal(ctx, searchCommand, err)` (const after `anomalies.go:28`), plus `Search.Truncated()` (matched > rows). `document/search.go` `NewSearch`: §2.4 keys, text/category/min/max null for now. Tests:
  - `window_test.go`: open/open; since-only first day and until-only last day for each date form; not-a-date per bound; since > until; same day; `0001` stays a non-nil bound
  - `internal/report/search_test.go` (new): the recording fake sees window, account ids and limit; accounts echoed; unknown account refused; store fault → `RefusalStore`; cancelled ctx → `search interrupted`
  - `document/search_test.go` (new): byte-literal key order; null payee, memo (NULL and `""`), split category and open since/until; `[]` for account_filter, transactions, splits and warnings; truncated true, and false at matched == len(rows) > 0
- [x] Step 5: `internal/cli/search.go` (new) `newSearchCommand(newReport, jsonOut)` (no clock, no config loader). It sets §2.2 `Use`/`Short`/`Long`/`Example` verbatim and `Args: cobra.NoArgs`; registers `--since`, `--until` and `--account` in §2.2 order with §2.2 help; refuses a window as `UsageError`; calls `openReport` → `Server.Search` with `defaultSearchLimit` (500) → `emitReport`, JSON via `marshalDocument(document.NewSearch(...))` and text via a signature-only `renderSearch` that step 6 fills. `root.go:7-38`: `AddCommand` + `newRootCommand` doc comment. Pins:
  - `run_status_test.go:122-135` root help row
  - `internal/cli/report_help_test.go:202-235` window-flags row + `Test_search_help_shows_its_long_text_and_examples`
  - `run_read_refusals_test.go:45-80` rows `search` and `search --account`; `:206-238` `quarry: search interrupted`
  - `run_spend_refusals_test.go:67-95` HOME-unset row; `run_usage_test.go:225-270` usage-hint row
  - `run_search_json_test.go`: given `--since 2026-01 --until 2026-03` echo first and last day; `--account` (account_filter, rows limited, a named linked account's rows flagged excluded); `--since 2024-13` → existing not-a-date line, exit 2; closed, left-out and USD rows in `--json`
  - n/a: `report_clock_test.go` (no clock); `run_read_refusals_test.go:82-120` and `run_config_test.go:218-226` (no config, Rule S8); `run_read_usage_test.go:47,79`, `run_usage_test.go:211` and `currency_test.go` (args and `--currency` belong to S02/S04); `run_analysis_documents_test.go` goldens (3b documents)
- [x] Step 6: `internal/cli/render_search.go` `renderSearch` per §2.5, `render_search_internal_test.go` (new):
  - caption: base, `accountsCaption`, all four date arms
  - columns: Category (distinct in split order, `(transfer)`, `(uncategorized)`, no splits); Memo (transaction memo then distinct differing split memos, ` / `); Amount signed `formatMoney`; Flags (all 4 arms); `accountLabel` closed and USD
  - footer counts matched, not rows; `escapeCell` (`\n`, `\t`, `\r`) on account, payee, category and memo
  - plus one cmd-level text run in `run_search_json_test.go` `Test_run_search_prints_the_transactions_table`

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on the new symbols; `internal/report/doc.go` names search among the commands

### Verify
- [ ] Step 8: full verification + `spec-check.py phase3c-search` → tick SCENARIO-01 and SCENARIO-05 (folded: "delivered by SCENARIO-01", then its test last on the line)

## Handoff

**Binding decisions:**
- `report.Store.Search` runs exactly two statements: rows, then span. S02-S04 add only WHERE predicates plus args through one builder, never a statement. S04's category check must fold into the span statement (Seam 1), or the read-fault rows re-point.
- The cut and `matched` happen at transaction grain inside the CTE, before the split join; one order const serves the CTE and the outer SELECT. Otherwise splits are double-counted, or a mutated copy of the order is masked.
- Open bounds are `store.SearchWindow` nil pointers, never a zero `time.Time`, because `--since 0001` parses to exactly the zero time.
- Limit 0 binds nil to `LIMIT $n`. This is verified in DuckDB 1.5 (go-duckdb v2.10505.0): `LIMIT NULL` means no limit; `count(*) OVER ()` is evaluated before `LIMIT`; `lower('CAFÉ') = lower('café')` is true (this retires S02's product-vision risk).
- `transfer`/`excluded` come only from the extracted fragments in `schema.go`; re-deriving either one is a defect.
- `SearchParams` has no Text, Min, Max or Category field until S02, S03 and S04 add each with its predicate. An unused field would let a test set it and see no filter.
- `newSearchCommand` takes no clock and no config loader (Rule S8, enforced by the signature). CLI passes `defaultSearchLimit` = 500 now, so S02 does not re-point `limit` in the acceptance JSON.
- Help lists flags alphabetically, as every sibling command does (no `SortFlags = false` in `internal/cli`). §2.2's order is only the spec's listing order, so no help pin asserts flag order.

**Left unbuilt:**
- positional `[text]`, two-texts and blank-text refusals, `--limit`, `document.SearchWarnings`, the CLI cut line (S02)
- `--min`/`--max` and the amount parser (S03); `--category`, `RefusalUnknownCategory`, the S08 matrix (S04)
- `search_transactions` (S09/S10)

**Traps:**
- `transactionRange` (`filter.go:47-65`) adds `reportedAccount` when accounts are named. Search's span must not reuse it, or a named left-out account says "they have no transactions".
- `Args: cobra.NoArgs` gives cobra's unruled line for `search costco` until S02 replaces it. No test may pin that line.
- `internal/report/fakes_test.go` implements `report.Store` explicitly, unlike the cli/mcp fakes that embed it, so it needs `Search` to compile.
- Key order is seen only by the `document` byte-literal test; the cmd decode is blind to it.

## Phase report

Runs B1 (steps 3-5) and B2 (step 6) done. Narrow loop green (duckstore, report, document, cli, cmd/quarry); both acceptance tests green; views/views_fx tests unedited. `golangci-lint run ./internal/cli/... ./cmd/...` is 0 issues; full-repo lint and the full suite NOT run (run V: steps 7-8).

B1 files:
- `internal/store/duckstore/schema.go`: `reportedTransaction` const and `transferLeg(alias)` func; `cashFlowViewDDL` uses both. `search.go`: `searchRowsQuery` (CTE `m` at transaction grain: `count(*) OVER ()`, `searchOrder`, `LIMIT $3` bound nil for 0; then LEFT JOIN splits ordered by split source_id, id), `searchSpanQuery` (no `reportedAccount`), `Search`, `scanSearchRow`. `SearchSplit.Transfer` = split is a leg in `transfers` (orchestrator ruling); a split with transfer_account_id but no transfers row is false (pinned in `search_test.go`).
- tests: `duckstore/search_test.go` (new, 15 tests + 2 span-fault tests), `read_faults_test.go` (`Search` in `rowReads`), `report/window_test.go` (7 tests), `report/search_test.go` (new), `report/document/search_test.go` (new, byte-literal), `report/fakes_test.go` (`search`, `gotSearch`).
- production: `report/window.go` `ParseSearchWindow`, `report/search.go`, `report/document/search.go` `NewSearch` (text/category/min/max stay nil), `cli/search.go` (flags via `reportFlags.bind`, `reportFlags.searchWindow`, `defaultSearchLimit`), `cli/root.go`.
- pins: root help row, window-flags row, `Test_search_help_shows_its_long_text_and_examples`, no-store rows, interrupt row, HOME-unset row, usage-hint row, 6 cmd tests in `run_search_json_test.go`.
- B1 acceptance fix: expected `Matched: 9` corrected to 8 (fixture has 8 transactions).

B2 (step 6):
- `internal/cli/render_search.go`: `renderSearch` plus `searchCaption` (`Transactions in <accountsCaption>, <dates>`; four date arms in `searchDates`: all dates / from / through / range), `searchCategoryCell`, `searchMemoCell`, `searchFlagsCell`. Caption has no ` matching %q`, `, category` or `, amount` parts yet: those need `report.Search` to carry text/category/min/max, which S02-S04 add together with the predicate.
- `internal/cli/render_search_internal_test.go` (new, white-box, 12 top-level tests, table-driven per rule) and `Test_run_search_prints_the_transactions_table` in `cmd/quarry/run_search_json_test.go` (full 8-row table over `searchStore()`).
- Removed the unused `searchCommand` const from `internal/cli/search.go` (lint `unused`).
- Mutations run (each red as named): each of the four date-arm words and the `Transactions in ` prefix -> `Test_searchCaption_names_the_dates_the_window_bounds` subtest of that arm; Amount alignment right->left -> pad test + cmd table test; category dedup removed -> `a_repeated_label_shows_once`; `sp.Transfer` arm dropped -> `a_transfer_leg_is_a_transfer`; no-splits label -> `no_splits_is_uncategorized`; memo dedup / non-empty guard / escape / ` / ` separator -> matching `searchMemoCell` subtests; Transfer and Excluded flag arms -> `searchFlagsCell` subtests; footer `Matched`->`len(Rows)` -> `footer_counts_every_match_not_the_rows_listed`; category escape and `, ` separator -> their subtests.

Left for run V: Sweep (step 7: full lint, doc comments, `internal/report/doc.go` names search) and Verify (step 8: covered full suite, `uncovered-diff.py`, `test-stats.py`, `spec-check.py`, tick SCENARIO-01 and folded SCENARIO-05, STATE.md).
