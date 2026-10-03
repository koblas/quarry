# phase3c-search — current state

Scenarios complete: SCENARIO-01 (folds 05), SCENARIO-02 (folds 06, 07, 12a), SCENARIO-03. Last updated by SCENARIO-03. Phase 3a/3b decisions (`docs/specifications/phase3a-mcp-core/STATE.md`, `docs/specifications/phase3b-analysis-tools/STATE.md`: MCP transport, `handler`, `WithClock`, `capList`, `windowRefusal`, `accountRefusal`, the equality harness) still bind S09/S11.

## Binding decisions
- `report.Store.Search` runs exactly two statements: rows, then span. S02-S04 add only WHERE predicates plus args through the one builder (`searchRowsQuery`/`searchArgs`), never a statement. S04's category check folds into the span statement, or the read-fault rows re-point (SCENARIO-01)
- The cut and `matched` happen at transaction grain inside the CTE (`count(*) OVER ()` before `LIMIT`), before the split join; `searchOrder` (date, source id, then `txn_id`, all DESC) serves the CTE and the outer SELECT. Counting after the cut, or a second copy of the order, double-counts splits or masks a mutation; dropping the `txn_id` tiebreak interleaves two transactions sharing date and source id (SCENARIO-01)
- `store.SearchWindow` open bounds are nil pointers, never a zero `time.Time`: `--since 0001` parses to exactly the zero time. Both bounds are civil days, inclusive (SCENARIO-01)
- Limit 0 binds nil to `LIMIT $3` (DuckDB 1.5, go-duckdb v2.10505.0: `LIMIT NULL` is no limit). `lower('CAFÉ') = lower('café')` holds, which retires S02's case-folding risk (SCENARIO-01)
- `transfer`/`excluded` flags come only from the fragments in `duckstore/schema.go` (`transferLeg(alias)`, `reportedTransaction`, `reportedAccount`), shared with `cashFlowViewDDL`; re-deriving either is a defect (SCENARIO-01)
- `SearchSplit.Transfer` means the split is a leg in `transfers`; a split with `transfer_account_id` but no transfers row is false (pinned in `duckstore/search_test.go`) (SCENARIO-01)
- `SearchParams` has no Category field until S04 adds it with its predicate, so a test cannot set an unused field and see no filter; `Text` ("" = none) and `Min`/`Max` (`*int64` cents, nil = no bound) are in (SCENARIO-01, SCENARIO-02, SCENARIO-03)
- `newSearchCommand(newReport, jsonOut)` takes no clock and no config loader (Rule S8, enforced by the signature); `--limit` flag default is 0 (so help prints no default) and `searchLimit` applies `defaultSearchLimit` = 500 unless `Changed`; negative is a `UsageError` (SCENARIO-01, SCENARIO-02)
- Help lists flags alphabetically like every sibling; no help pin asserts flag order (SCENARIO-01)
- Caption is `Transactions[ matching %q] in <accountsCaption>, <dates>[, category][, amount <range>]` (`searchCaption`/`searchDates`/`searchAmountRange`: `A to B`, `at least`, `at most`, `exactly` when equal). S04 inserts `, category` before the amount arm and lands it with `report.Search` carrying the field plus its predicate. The footer counts `Matched`, not rows (SCENARIO-01, SCENARIO-02, SCENARIO-03)
- Blank text is one rule: `report.CheckSearchText(*string)` / `ErrBlankSearchText` (TrimSpace==""), run in cobra `Args` (before RunE: before min/max, window, store open) and first in `Server.Search`. S09's handler must call it before `ParseSearchWindow`; S10 words it for MCP (SCENARIO-02)
- Refusal order: `Args` (`searchArgs(&limit)`: count >1 text, negative `--limit`, blank text), then RunE `searchAmounts` (min -> max -> min>max), then `searchWindow`. S04's matrix extends this order, never reorders it (SCENARIO-02, SCENARIO-03)
- `report.ParseSearchAmounts(min, max *string)` is the only amount parser (nil = flag not given, `""` = given and refused; pointer from `Changed`, not `!= ""`). Grammar `^[0-9]{1,16}(\.[0-9]{1,2})?$`, cents `*int64`. Returns `AmountError{Kind, Bound, Value, Other}` by value; `Error()` is the CLI line, S10 words MCP lines from the parts. S09's handler must call it before `ParseSearchWindow`, after `CheckSearchText` (SCENARIO-03)
- `Server.Search` trusts `SearchRequest.Amounts` (a Min>Max request matches nothing, is not refused): the check lives only in the parser because the refusal needs the raw strings and must beat a bad since (SCENARIO-03)
- Store binds: `$1..$4` fixed, `$5` min, `$6` max (`searchMin`/`searchMax`), accounts from `$7` (`searchFirstAccount`). Predicate is `abs(t.amount)` at transaction grain (never split amounts), `>=`/`<=` inclusive, nil needs `CAST($n AS BIGINT) IS NULL`. S04's category takes `$7`, accounts move to `$8` through the const; still no third statement (SCENARIO-03)
- `document.Search.Min`/`Max` echo `document.Money(cents)` (`12.5` -> `"12.50"`), null when the flag is absent (SCENARIO-03)
- Text predicate is `contains(lower(..), lower($4))` over payee, `t.memo`, and an EXISTS over split memos (a JOIN would double-count `matched`); the bind is nil for "", so `$4` is cast in `CAST($4 AS VARCHAR) IS NULL`; `$1..$4` fixed, accounts from `$5` (SCENARIO-02)
- `document.SearchWarnings` is the only no-match composer (one line, four variants: store/named x span/none; `[]string{}` never nil); S09 reuses it. The cut line `searchCutNote` is appended per surface after it, so CLI `--json` `warnings` equals the stderr lines; cut and no-match never coexist (SCENARIO-02)
- Memo ""→null is pinned at store level (`duckstore` `Test_search_gives_no_memo_for_a_null_or_empty_memo`), not in the document; `renderSearch` lists a transfer before a category and relies on the store contract that a transfer leg has no category (SCENARIO-01)
- Cell escaping (`escapeCell`: `\n`, `\t`, `\r`) applies to account, payee, category and memo (SCENARIO-01)

## Left unbuilt
- `--category`, `RefusalUnknownCategory`, the S08 refusal matrix, the caption's category part, `SearchParams.Category` (SCENARIO-04)
- bad-input refusal matrix remainder (amount, since/until order pins are built): unknown flags, `--json` + refusal prints nothing, `--account ""`, `--category ""` (SCENARIO-04 / SCENARIO-08)
- `search_transactions` tool, its cut line, blank-text and amount MCP wording incl. min-as-JSON-number refusal (SCENARIO-09 / SCENARIO-10 / SCENARIO-11)

## Traps
- `abs` must wrap `t.amount` (the transaction), never split amounts; a two-split transaction counts once in `matched` (SCENARIO-03)
- Two `// unreachable:` claims in `internal/report/amount.go` (`Error()` fallthrough, `parseAmountBound` ParseInt error) rest on the grammar and a grep of `AmountError{` constructors; a new constructor or a wider grammar voids them (SCENARIO-03)
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
- Checkpoint S02 MINOR (owned by S04's S08 matrix): text with invalid UTF-8 or embedded NUL (`quarry search $'\xff'`) unruled — probe and pin current behaviour; if it surfaces as an unruled store refusal, get a copy ruling
- Checkpoint S02 NIT: `cmd/quarry/run_search_limit_test.go` `fiveSearchStore` comment restates its name
- Checkpoint S03 MINORs (fold into S04's run): `internal/store/duckstore/search_amount_test.go` add top-value row (±999999999999999999 cents, Min equal → found, Min+1 → none) to prove `CAST(abs(t.amount)*100 AS BIGINT)` at the DECIMAL(18,2) edge; doc budgets `internal/report/search.go:17-21` SearchRequest (5 lines; move "does not compare the bounds" to ParseSearchAmounts), `internal/store/store.go:752-756` SearchParams (5 lines), `internal/store/duckstore/search.go:32-33` searchAmountRange const (2 lines → 1). Optional: `internal/report/amount.go:49` Error() fallthrough — test generic wording and drop the unreachable mark, or leave per WindowError precedent. NIT: `cmd/quarry/run_search_amount_test.go` useDigits used on one row only
