---
id: SCENARIO-04
status: open
---

# SCENARIO-04: --category matches the category and any category under it (folds SCENARIO-08, the CLI refusal matrix)

Cadence: code-first (no bug fix, write guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_search_category_test.go` `Test_run_search_category_lists_the_category_and_everything_under_it`
Acceptance test (SCENARIO-08, folded): `cmd/quarry/run_search_refusals_test.go` `Test_run_search_refuses_bad_input_with_the_ruled_line_and_exit_code`
Narrow loop: `go test ./internal/report/... ./internal/store/duckstore/ ./internal/cli/ ./internal/mcp/ ./cmd/quarry/ -run '(?i)search|category'`
Mutation checks (none is a mandatory test-first item; these are the load-bearing guards):
- `starts_with(.., arg || ':')` arm deleted -> child/grandchild rows; the `':'` dropped -> `Foo` vs `Food` row; either `lower()` dropped -> other-case rows
- `EXISTS` swapped for a split JOIN -> `Test_search_category_counts_a_transaction_once_when_several_splits_match`
- unknown flag forced false -> unknown-refused rows, and the known-empty control (exit 0) must stay green; `Changed("category")` swapped for `!= ""` -> `--category ""` row
- `UnknownCategory` -> refusal mapping in `Server.Search` removed -> `Test_search_refuses_an_unknown_category_...`; category tested before account -> the order row (bad account + bad category)
Runs: A (1-2) | B1 (3-4) | B2 (5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (`internal/report`; `store`, `duckstore`, `document`, `cli`, `mcp` one-liner, `cmd/quarry` are layers); S08 FOLDED

## What exists (anchored; do not re-derive)
- Probe (run by the architect, scratch test deleted): text `"\xff"` and `"caf\xc3"` exit 1 `quarry: cannot read the store at ~/…/quarry.duckdb: unknown error; run quarry sync to rebuild it` (unruled, wrong advice); text with NUL exits 0, no-match warning. `--account` resolves in Go (no bind), unaffected. Ruled since: Step 5.
- `internal/store/store.go:752-760` SearchParams (no Category); `:787-796` `store.Search` (Rows, Matched, Transactions). `duckstore/search.go:89-97` consts (`searchFirstAccount = 7`), `:110-142` rows query, `:146-151` span query (numbers its own `$1`; accounts from `$1`), `:155-181` `Search` (two `QueryRows`), `:184-198` `searchArgs`.
- `report/search.go:21-27` SearchRequest, `:30-40` Search, `:56-73` `Server.Search` (CheckSearchText -> `namedAccounts` -> store -> `readRefusal`). `report/refusal.go:19-23` kinds, `:85-91` `unknownAccountRefusal` (the pattern).
- `cli/search.go:253-258` help, `:314-392` command (`searchAmounts` Changed pattern `:265-277`), `render_search.go:47-57` `searchCaption`; `document/search.go:11-24` already has `Category` field (unset), `:50-66` `NewSearch`.
- `mcp/result.go:72-83` `refusalLine` switch is `exhaustive`-linted (`mcp/window.go:28` is the WindowError kind, unaffected).
- Existing tests cover most single refusal rows in text mode only (`run_search_limit_test.go:148`, `run_search_amount_test.go:116-165`, `run_search_json_test.go:226-245`, `run_usage_test.go:249` `--bogus`): leave them; the matrix adds the `--json` cross, the exit-1 rows and the unknown flags.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: new `cmd/quarry/run_search_category_test.go` `Test_run_search_category_lists_the_category_and_everything_under_it` — `--json` ids per arm over a fixture (Food, Food:Groceries, Food:Groceries:Organic, Foo, one hidden category, one known category with no transactions): `Food`, `food:groceries`, `Foo` (not Food), hidden, known-empty (exit 0 + no-match line). Today fails at exit 2 `unknown flag: --category`, an assertion, not a panic; `require` the exit code before decoding. No stubs needed (command-slice test).
- [x] Step 2: new `cmd/quarry/run_search_refusals_test.go` `Test_run_search_refuses_bad_input_with_the_ruled_line_and_exit_code` — one table over a built store, each row run as text and with `--json`: asserts exit, exact stderr, empty stdout. Rows: two texts, `--limit -1`, `""` and `"  "`, `--min "-12"`, `--max abc`, min>max, `--since 2024-13`, since>until (all 2); `--account Nope`, ambiguous `--account Visa` (two accounts named Visa), `--account ""` (`no account named ""; run quarry accounts --all to list them`), `--category Fod`, `--category ""` (all 1); `--currency CAD`, `--csv` (cobra `unknown flag: --…; Run 'quarry search --help' for usage.`, 2). Generalise `assertSearchRefused` (`run_search_amount_test.go:168`, hard-codes exit 2 and an empty HOME) into a helper taking rows/exit code; do not copy it. Red on the category rows only.

### Build
- [x] Step 3 (batch 1): category resolution and predicate, no third statement.
  - `store.go:752-760` `SearchParams.Category *string` (nil = no filter; `""` is a category that names nothing, so never `Text`-style `""`=none); `:787-796` `Search.UnknownCategory bool` (a bool, not a sentinel: `duckstore.Search` wraps every error in `openFault`, which would print the "unknown error; rebuild" line).
  - `duckstore/search.go`: `searchCategory = "$7"`, `searchFirstAccount = 8`; add `searchCategoryMatch` to the rows WHERE (`EXISTS` over `splits s2 JOIN categories`: `lower(full_path) = lower($7)` OR `starts_with(lower(full_path), lower($7) || ':')`, nil via `CAST($7 AS VARCHAR) IS NULL`; never `LIKE`, never a JOIN to the outer splits); span query takes the category as `$1`, accounts from `$2`, and a third column `NOT (CAST($1 AS VARCHAR) IS NULL OR EXISTS(SELECT 1 FROM categories WHERE lower(full_path) = lower($1)))`; `Search` scans it into `found.UnknownCategory`; `searchArgs` and the span args add the category. Rows statement is Q1, span+category Q2: the existing fault rows (`Test_row_reads_*` Q1; `Test_search_returns_the_span_{query,scan}_fault_as_another_fault`, `passQueries: 1`, Q2) stay unchanged and green.
  - `report/refusal.go:19-23` `RefusalUnknownCategory`, helper `unknownCategoryRefusal(arg)` (message `no category named %q; list them with quarry sql "SELECT full_path FROM categories ORDER BY full_path"`, `Kind`, `Arg`); `report/search.go` `SearchRequest.Category`/`Search.Category *string`, passed through, refusal built after the read when `UnknownCategory` (account resolution already precedes it).
  - `mcp/result.go:72-83` add `case report.RefusalUnknownCategory:` returning a new const `unknownCategoryLog` = `refused the call's category: it names no category; details went to the client only` (spec §4.2 row 13: carries no `Arg`); row in `mcp/log_classes_internal_test.go:66`. S10 owns the wrapper and wiring.
  - Tests: new `duckstore/search_category_test.go`: `Test_search_category_matches_the_category_and_everything_under_it` (exact, child, grandchild, other case both directions, `Foo`≠`Food`, `Food:Groc` prefix-only, hidden); `…_counts_a_transaction_once_when_several_splits_match` (Food + Food:Groceries splits, `Matched` 1); `…_keeps_both_filters_with_named_accounts_and_text` (`$8` numbering, span `$2`); `…_flags_unknown_only_when_no_path_equals_it` (Fod, `""`, `Food:`, `%`, nil → false; known-empty → false with the span set; known on a store with no transactions); `…_runs_two_statements` (`spyReadDB{passQueries: 2, queryFault}` with a category succeeds). `search_amount_test.go` add the ±999999999999999999-cent top row (Min equal found, Min+1 none; S03 checkpoint). `report/search_test.go`: passes/echoes category, nil passes nil, unknown -> `RefusalError{Kind, Arg}` exact message, account refusal beats category (needs a `searchErr` on `fakeStore`, `fakes_test.go:86`), store fault with a category stays the store refusal.
- [x] Step 4 (batch 2): `--category` flag, caption, echo. `cli/search.go` `categoryArg`, `Flags().StringVar("category", …)` with the ruled help, pointer from `Changed("category")` into `req.Category`; `render_search.go:47-57` caption arm `, category %q` after dates, before amount; `document/search.go:50-66` `Category: s.Category`. Tests: `Test_search_help_lists_the_category_flag` (`cli/report_help_test.go:293` pattern, verbatim help); caption rows in `render_search_internal_test.go` (category alone, before the amount, with text+accounts+dates, `%q` quoting); `document` echo as given / null (key order is the existing byte-literal test); cmd: `Test_run_search_json_echoes_the_category_as_given_and_null_when_absent`, `Test_run_search_text_names_the_category_in_the_caption`, `Test_run_search_category_narrows_with_account_text_and_amount`. Step 1 goes green.
- [x] Step 5 (batch 3): refusal matrix, UTF-8 refusal and order pins; Step 2 goes green. Ruled copy (spec "Mid-feature copy ruling (SCENARIO-04)"): text or `--category` not valid UTF-8 is a usage refusal, exit 2, before the store opens, value quoted `%q`: `quarry: search text "\xff" is not valid UTF-8; set your terminal or script to UTF-8` / `quarry: --category "\xff" is not valid UTF-8; set your terminal or script to UTF-8`.
  - Code (`report`): a parts-carrying error beside `CheckSearchText` (`InvalidUTF8Error{Field, Value}`, `Field` text or category); no new `RefusalKind`, nothing in `mcp.refusalLine`. `cli.searchArgs` runs it after blank text (text, then `--category`) and words the ruled `UsageError`. `Server.Search` need not repeat it (MCP always sees valid UTF-8).
  - Tests: `Test_run_search_refuses_bad_input_with_the_ruled_line_and_exit_code` gains the rows `\xff` text, `caf\xc3` text, `--category "\xff"` (exit 2, text and `--json`); `report` unit test for the check (valid, invalid, nil, multibyte valid control); `Test_run_search_refuses_in_the_ruled_order`: invalid text + bad `--min` gives the UTF-8 line, invalid text + invalid category gives the text line, blank text + invalid category gives the blank line.
  - Order pins in `run_search_refusals_test.go` (`Test_run_search_refuses_in_the_ruled_order`): bad min < bad since < unknown account < unknown category (pairs: min+since, since+account, account+category -> account line exit 1, category+missing store -> store line, a valid category with a missing store); count/limit/blank already pinned (`run_search_limit_test.go:148`). `Test_run_search_text_with_a_nul_byte_matches_nothing_and_exits_0` (probe result). `--account` with invalid UTF-8 (green on arrival, say so).

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc budgets (S03 checkpoint): `report/search.go:17-21` (5 lines; move "does not compare the bounds" to `ParseSearchAmounts`), `store.go:752-756` (5 lines), `duckstore/search.go:32-33`-era `searchAmountRange` doc (2 lines -> 1), doc on `RefusalUnknownCategory`/`Category`; NIT `useDigits` used on one row only. `amount.go:49` Error() fallthrough: leave (WindowError precedent).

### Verify
- [ ] Step 7: full verification (`agent-briefs.md` → *Verification*) + `spec-check.py phase3c-search`; tick SCENARIO-04 and SCENARIO-08 (SCENARIO-08 line names "delivered by SCENARIO-04" then its test last); STATE.md rewrite; `status: done`.

## Handoff

**Binding decisions:**
- `store.Search.UnknownCategory bool` is set by the span statement; `Server.Search` turns it into `RefusalError{Kind: RefusalUnknownCategory, Arg}` after the read — a sentinel from inside `duckstore.Search` would be wrapped into the "unknown error; rebuild" store line.
- `Category` is `*string` end to end (`SearchParams`, `SearchRequest`, `Search`); `--category ""` is given-and-unknown, never "no filter" (S09's handler must pass a pointer for a given `""`).
- Binds: rows `$7` category, accounts from `$8` (`searchFirstAccount`); span `$1` category, accounts from `$2`. Still two statements; resolution rides the span statement.
- `mcp.refusalLine` has a row-13 class line for the category kind already (S10 owns the wrapper `"no category named …; call describe_schema…"`, wiring and acceptance rows).

**Left unbuilt:**
- `search_transactions`, MCP wrappers for blank text/amount/category (S09/S10).

**Traps:**
- `UnknownCategory` is read after rows: a rows-statement fault on a store whose category is unknown prints the store line, not the category line (Rule S9 puts category before read); only fault injection reaches it, accepted deviation.
- `searchSpanQuery` numbers binds from `$1`; the category takes `$1` there and `$7` in the rows statement — mixing them breaks only when accounts are named (hence the combined-filter pin).
- Never `LIKE`, never a JOIN to the outer splits (double-counts `matched`).

## Phase report

Run B2 (step 5) done; next V (steps 6-7). Narrow loop green, `golangci-lint run ./...` 0 issues. Step 2 matrix green (text and `--json`), step 1 green.
- `internal/report/search.go`: `SearchInput` (`SearchInputText`, `SearchInputCategory`), `InvalidUTF8Error{Field, Value}` (`Error()` is the ruled CLI line), `CheckUTF8(field, *string)`. No `RefusalKind`, nothing in `mcp.refusalLine`, `Server.Search` does not call it (MCP decoder yields valid UTF-8, so no `// unreachable:` line exists to mark; S10 owes the stdio pin in STATE).
- `internal/cli/search.go`: `searchArgs(limit, category *string)` runs text UTF-8 after blank text, then category UTF-8, as `UsageError` (Args, so before min/since and the store).
- `cmd/quarry/run_search_refusals_test.go`: matrix gained `\xff` text, `caf\xc3` text, `--category "\xff"`; new `Test_run_search_refuses_in_the_ruled_order` (since<account, account<category, UTF-8 text<min, text<category, blank<category-UTF-8, limit<category-UTF-8, category-UTF-8<min; min<since already pinned in `run_search_amount_test.go`), `..._reads_the_store_before_it_looks_up_the_category`, `..._refuses_text_that_is_not_valid_UTF_8_before_the_store_opens`, `..._text_with_a_nul_byte_matches_nothing_and_exits_0`, `..._an_account_that_is_not_valid_UTF_8_is_an_unknown_account` (green on arrival: account resolves in Go). Helper `builtStore`. `report/search_test.go`: two `CheckUTF8` tests.
- Mutations (all red): text UTF-8 call deleted, category call deleted, category check before blank text, `!utf8.ValidString` un-negated, label branch flipped.
- Not touched, V owns: step 6 doc budgets (`report/search.go` SearchRequest doc, `store.go:752-756`, `duckstore/search.go` `searchAmountRange` doc, `RefusalUnknownCategory`/`Category` docs, `useDigits` NIT), step 7 verify/tick/STATE.
