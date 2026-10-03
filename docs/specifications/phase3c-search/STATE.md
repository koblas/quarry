# phase3c-search — current state

Scenarios complete: SCENARIO-01 (folds 05). Last updated by SCENARIO-01. Phase 3a/3b decisions (`docs/specifications/phase3a-mcp-core/STATE.md`, `docs/specifications/phase3b-analysis-tools/STATE.md`: MCP transport, `handler`, `WithClock`, `capList`, `windowRefusal`, `accountRefusal`, the equality harness) still bind S09/S11.

## Binding decisions
- `report.Store.Search` runs exactly two statements: rows, then span. S02-S04 add only WHERE predicates plus args through the one builder (`searchRowsQuery`/`searchArgs`), never a statement. S04's category check folds into the span statement, or the read-fault rows re-point (SCENARIO-01)
- The cut and `matched` happen at transaction grain inside the CTE (`count(*) OVER ()` before `LIMIT`), before the split join; `searchOrder` serves the CTE and the outer SELECT. Counting after the cut, or a second copy of the order, double-counts splits or masks a mutation (SCENARIO-01)
- `store.SearchWindow` open bounds are nil pointers, never a zero `time.Time`: `--since 0001` parses to exactly the zero time. Both bounds are civil days, inclusive (SCENARIO-01)
- Limit 0 binds nil to `LIMIT $3` (DuckDB 1.5, go-duckdb v2.10505.0: `LIMIT NULL` is no limit). `lower('CAFÉ') = lower('café')` holds, which retires S02's case-folding risk (SCENARIO-01)
- `transfer`/`excluded` flags come only from the fragments in `duckstore/schema.go` (`transferLeg(alias)`, `reportedTransaction`, `reportedAccount`), shared with `cashFlowViewDDL`; re-deriving either is a defect (SCENARIO-01)
- `SearchSplit.Transfer` means the split is a leg in `transfers`; a split with `transfer_account_id` but no transfers row is false (pinned in `duckstore/search_test.go`) (SCENARIO-01)
- `SearchParams` has no Text/Min/Max/Category field until S02/S03/S04 add each with its predicate, so a test cannot set an unused field and see no filter (SCENARIO-01)
- `newSearchCommand(newReport, jsonOut)` takes no clock and no config loader (Rule S8, enforced by the signature); CLI passes `defaultSearchLimit` = 500 so S02 does not re-point `limit` in the acceptance JSON (SCENARIO-01)
- Help lists flags alphabetically like every sibling; no help pin asserts flag order (SCENARIO-01)
- Caption is `Transactions in <accountsCaption>, <dates>` (`searchCaption`/`searchDates`: all dates / from / through / range). The ` matching %q`, `, category` and `, amount` parts are owed by S02 (text), S04 (category), S03 (min/max): each lands with `report.Search` carrying the field plus its predicate. The footer counts `Matched`, not rows (SCENARIO-01)
- Cell escaping (`escapeCell`: `\n`, `\t`, `\r`) applies to account, payee, category and memo (SCENARIO-01)

## Left unbuilt
- positional `[text]`, two-texts and blank-text refusals, `--limit`, `document.SearchWarnings`, the CLI cut line, the caption's ` matching` part, `SearchParams.Text` (SCENARIO-02)
- `--min`/`--max`, the amount parser, the caption's amount part, `SearchParams.Min/Max` (SCENARIO-03)
- `--category`, `RefusalUnknownCategory`, the S08 refusal matrix, the caption's category part, `SearchParams.Category` (SCENARIO-04)
- bad-input refusal matrix incl. the `search costco` line cobra's `NoArgs` prints today (SCENARIO-08)
- `search_transactions` tool, its cut line (SCENARIO-09 / SCENARIO-11)

## Traps
- `transactionRange` (`duckstore/filter.go`) adds `reportedAccount` when accounts are named. Search's span must not reuse it, or a named left-out account says "they have no transactions" (SCENARIO-01)
- `Args: cobra.NoArgs` gives cobra's unruled line for `search costco` until S02 replaces it; no test may pin that line (SCENARIO-01)
- `internal/report/fakes_test.go` implements `report.Store` explicitly (cli/mcp fakes embed it), so a new Store method needs a stub there to compile (SCENARIO-01)
- Key order of the search document is seen only by the `document` byte-literal test (`//nolint:testifylint`: `JSONEq` is blind to order; `golangci-lint run --fix` rewrites it to `JSONEq`, undo that); the cmd decode is blind (SCENARIO-01)
- Row loops over `*sql.Rows` need a per-row `ctx.Err()` check (`rows.Err()` misses a cancel); `platform/sqlite` `QueryRows` still lacks it (unowned) (phase3b)
- `go test -run 'Window'` is case-sensitive; use `(?i)window` (phase3b)

## Open debts
- `platform/sqlite` `QueryRows` has the unchecked per-row `ctx.Err()` loop: unowned — dies unless re-opened (phase3b)
- Phase3b/3a deferred MINOR/NIT lists stay in their STATE files (unowned); this feature does not close them
- `instructions` still names "David's" (spec §3.7 NIT): fix if S09 touches it; else unowned — dies unless re-opened (phase3b)
