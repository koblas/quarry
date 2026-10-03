# phase3c-search — current state

Scenarios complete: SCENARIO-01 (folds 05), SCENARIO-02 (folds 06, 07, 12a). Last updated by SCENARIO-02. Phase 3a/3b decisions (`docs/specifications/phase3a-mcp-core/STATE.md`, `docs/specifications/phase3b-analysis-tools/STATE.md`: MCP transport, `handler`, `WithClock`, `capList`, `windowRefusal`, `accountRefusal`, the equality harness) still bind S09/S11.

## Binding decisions
- `report.Store.Search` runs exactly two statements: rows, then span. S02-S04 add only WHERE predicates plus args through the one builder (`searchRowsQuery`/`searchArgs`), never a statement. S04's category check folds into the span statement, or the read-fault rows re-point (SCENARIO-01)
- The cut and `matched` happen at transaction grain inside the CTE (`count(*) OVER ()` before `LIMIT`), before the split join; `searchOrder` (date, source id, then `txn_id`, all DESC) serves the CTE and the outer SELECT. Counting after the cut, or a second copy of the order, double-counts splits or masks a mutation; dropping the `txn_id` tiebreak interleaves two transactions sharing date and source id (SCENARIO-01)
- `store.SearchWindow` open bounds are nil pointers, never a zero `time.Time`: `--since 0001` parses to exactly the zero time. Both bounds are civil days, inclusive (SCENARIO-01)
- Limit 0 binds nil to `LIMIT $3` (DuckDB 1.5, go-duckdb v2.10505.0: `LIMIT NULL` is no limit). `lower('CAFÉ') = lower('café')` holds, which retires S02's case-folding risk (SCENARIO-01)
- `transfer`/`excluded` flags come only from the fragments in `duckstore/schema.go` (`transferLeg(alias)`, `reportedTransaction`, `reportedAccount`), shared with `cashFlowViewDDL`; re-deriving either is a defect (SCENARIO-01)
- `SearchSplit.Transfer` means the split is a leg in `transfers`; a split with `transfer_account_id` but no transfers row is false (pinned in `duckstore/search_test.go`) (SCENARIO-01)
- `SearchParams` has no Min/Max/Category field until S03/S04 add each with its predicate, so a test cannot set an unused field and see no filter; `Text` ("" = none) is in (SCENARIO-01, SCENARIO-02)
- `newSearchCommand(newReport, jsonOut)` takes no clock and no config loader (Rule S8, enforced by the signature); `--limit` flag default is 0 (so help prints no default) and `searchLimit` applies `defaultSearchLimit` = 500 unless `Changed`; negative is a `UsageError` (SCENARIO-01, SCENARIO-02)
- Help lists flags alphabetically like every sibling; no help pin asserts flag order (SCENARIO-01)
- Caption is `Transactions[ matching %q] in <accountsCaption>, <dates>` (`searchCaption`/`searchDates`). The `, category` and `, amount` parts are owed by S04 and S03: each lands with `report.Search` carrying the field plus its predicate. The footer counts `Matched`, not rows (SCENARIO-01, SCENARIO-02)
- Blank text is one rule: `report.CheckSearchText(*string)` / `ErrBlankSearchText` (TrimSpace==""), run in cobra `Args` (before RunE: before min/max, window, store open) and first in `Server.Search`. S09's handler must call it before `ParseSearchWindow`; S10 words it for MCP (SCENARIO-02)
- `searchArgs(&limit)` refusal order is count (>1 text), negative `--limit`, blank text; S03 must parse `--min`/`--max` in RunE before `searchWindow` and keep that `Args` order (SCENARIO-02)
- Text predicate is `contains(lower(..), lower($4))` over payee, `t.memo`, and an EXISTS over split memos (a JOIN would double-count `matched`); the bind is nil for "", so `$4` is cast in `CAST($4 AS VARCHAR) IS NULL`; `$1..$4` fixed, accounts from `$5` (SCENARIO-02)
- `document.SearchWarnings` is the only no-match composer (one line, four variants: store/named x span/none; `[]string{}` never nil); S09 reuses it. The cut line `searchCutNote` is appended per surface after it, so CLI `--json` `warnings` equals the stderr lines; cut and no-match never coexist (SCENARIO-02)
- Memo ""→null is pinned at store level (`duckstore` `Test_search_gives_no_memo_for_a_null_or_empty_memo`), not in the document; `renderSearch` lists a transfer before a category and relies on the store contract that a transfer leg has no category (SCENARIO-01)
- Cell escaping (`escapeCell`: `\n`, `\t`, `\r`) applies to account, payee, category and memo (SCENARIO-01)

## Left unbuilt
- `--min`/`--max`, the amount parser, the caption's amount part, `SearchParams.Min/Max` (SCENARIO-03)
- `--category`, `RefusalUnknownCategory`, the S08 refusal matrix, the caption's category part, `SearchParams.Category` (SCENARIO-04)
- bad-input refusal matrix remainder: unknown flags, `--json` + refusal prints nothing, `--account ""`, `--category ""` (SCENARIO-04 / SCENARIO-08)
- `search_transactions` tool, its cut line, blank-text MCP wording (SCENARIO-09 / SCENARIO-10 / SCENARIO-11)

## Traps
- `transactionRange` (`duckstore/filter.go`) adds `reportedAccount` when accounts are named. Search's span must not reuse it, or a named left-out account says "they have no transactions" (SCENARIO-01)
- `readCommandArgs` (`cmd/quarry/run_config_test.go`) must NOT gain search: search never reads config (pinned by `Test_run_search_ignores_a_malformed_config`, whose mutant is the factory loading config) (SCENARIO-02)
- `contains(NULL, x)` is NULL: the OR-chain is safe, never negate it. A nil bind to `$4` needs the VARCHAR cast (SCENARIO-02)
- Named left-out account: `SearchWarnings` says "their transactions run" off the unfiltered span; it must not use `appendEmptyWindowWarning` (skips left-out accounts) (SCENARIO-02)
- `internal/report/fakes_test.go` implements `report.Store` explicitly (cli/mcp fakes embed it), so a new Store method needs a stub there to compile (SCENARIO-01)
- Key order of the search document is seen only by the `document` byte-literal test (`//nolint:testifylint`: `JSONEq` is blind to order; `golangci-lint run --fix` rewrites it to `JSONEq`, undo that); the cmd decode is blind (SCENARIO-01)
- Row loops over `*sql.Rows` need a per-row `ctx.Err()` check (`rows.Err()` misses a cancel); `platform/sqlite` `QueryRows` still lacks it (unowned) (phase3b)
- `go test -run 'Window'` is case-sensitive; use `(?i)window` (phase3b)

## Open debts
- `platform/sqlite` `QueryRows` has the unchecked per-row `ctx.Err()` loop: unowned — dies unless re-opened (phase3b)
- Phase3b/3a deferred MINOR/NIT lists stay in their STATE files (unowned); this feature does not close them
- `instructions` still names "David's" (spec §3.7 NIT): fix if S09 touches it; else unowned — dies unless re-opened (phase3b)
- Checkpoint S02 MINOR (fold into S03's run): text crossed with limit unpinned — add store row (3 text matches + 2 non-matches, Limit 2 → Matched 3, 2 rows newest first) in `internal/store/duckstore/search_text_test.go`, optional cmd row `search gym --limit 1` (matched = text matches, cut line "of N")
- Checkpoint S02 MINOR (owned by S04's S08 matrix): text with invalid UTF-8 or embedded NUL (`quarry search $'\xff'`) unruled — probe and pin current behaviour; if it surfaces as an unruled store refusal, get a copy ruling
- Checkpoint S02 NIT: `cmd/quarry/run_search_limit_test.go` `fiveSearchStore` comment restates its name
