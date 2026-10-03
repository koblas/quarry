---
id: SCENARIO-03
status: open
---

# SCENARIO-03: --min and --max compare the amount without its sign

Cadence: code-first (no mandatory test-first item: no write guard, no atomic adapter, no bug fix)
Acceptance test: `cmd/quarry/run_search_amount_test.go` `Test_run_search_min_and_max_compare_the_amount_without_its_sign`
Narrow loop: `go test ./internal/report/ ./internal/store/duckstore/ ./internal/cli/ ./cmd/quarry/ -run '(?i)amount|search|min|max'`
Mutation checks: `abs` dropped from the predicate -> `Test_search_amount_bounds_compare_the_absolute_amount` (and the acceptance `--min 100` row); `>=`/`<=` made strict -> the at-value rows of the same test; min/max arm swapped -> the one-cent-outside rows; 16-digit cap moved to 15 or 17 -> `Test_ParseAmount_*` digit-bound rows; `min > max` check moved after the window parse -> `Test_run_search_refuses_min_above_max_before_a_bad_since`
Runs: A (1) | B1 (2-3) | B2 (4) | V (5-6)
Size: OWNS A RUN — 3 batches, 1 feature package (`internal/report`; `store`, `duckstore`, `document`, `cli`, `cmd/quarry` are layers of it)

## What exists (all anchored; nothing to re-derive)
- `store.SearchParams` `internal/store/store.go:755-760`: no Min/Max. `duckstore` binds `$1..$4` fixed, accounts from `searchFirstAccount = 5` (`search.go:17-23`); `searchArgs` `:123-137`; WHERE `:44-47`; span query numbers its own `$1` and is untouched.
- `report.SearchRequest`/`Search`/`Server.Search` `internal/report/search.go:17-70`; sibling `WindowError` (`window.go:20-60`) and `ParseSearchWindow` are the shape to mirror. No amount parser exists anywhere (`platform/money` is currency/FX only), so the home is `internal/report/amount.go`.
- `document.Money(cents)` `document/common.go:63` is the normalizer (`1250` -> `"12.50"`); `document.Search` already has `Min`/`Max *string` (never set) at `document/search.go:12-24`, `NewSearch` `:44-58`.
- CLI: `internal/cli/search.go:84-123` RunE (window parsed first), `:124-127` flag binding; caption `render_search.go:47-53`; `formatMoney` `render.go:283`. Only caller of `SearchRequest` outside tests is `cli/search.go:106`.
- Surveyed callers of the changed shapes: `SearchRequest` (cli/search.go:106, report/search_test.go) and `store.SearchParams{}` (report/search_test.go:24,157,164) — added fields are nil-able, existing literals compile and keep passing.

## Decisions this plan makes
- Parser home `report`: `ParseSearchAmounts(min, max *string) (SearchAmounts, error)` (nil = flag not given, `""` = given and refused) does min -> max -> min>max (Rule S9) and returns `SearchAmounts{Min, Max *int64}` in cents. CLI RunE calls it BEFORE `searchWindow`; S09's handler calls the same func. Min>max lives here, not in `Server.Search`: the refusal needs the raw strings as given and must beat a bad since, which `Server.Search` (after the window) cannot do. `SearchRequest.Amounts` is trusted input; `Server.Search` does not re-check (a Min>Max request just matches nothing).
- `AmountError{Kind, Bound, Value, Other}`, kinds `AmountNotAnAmount`, `AmountMinAboveMax`; `Error()` is the CLI line verbatim (`--min "-12" is not an amount; use digits with up to 2 decimals and no sign, such as 25 or 19.99`; `--min 50 is more than --max 20`, raw values). CLI wraps it as `UsageError` (exit 2), as windows are.
- Store bind: `store.SearchParams.Min, Max *int64` (cents). New binds are `$5`/`$6` (`searchMin`, `searchMax`), `searchFirstAccount` becomes 7; every placeholder stays a named const. Predicate: `(CAST($5 AS BIGINT) IS NULL OR CAST(abs(t.amount) * 100 AS BIGINT) >= $5)` and the `<= $6` twin (nil needs the cast, as `$4` does). `searchArgs` preallocation `4+len` becomes `6+len`. S04's category takes `$7`, accounts from `$8`.
- `report.Search` carries `Amounts`; `document.NewSearch` sets `Min`/`Max` to `Money(cents)`.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_search_amount_test.go` (new) `Test_run_search_min_and_max_compare_the_amount_without_its_sign` — one `runWith` table over the four spec rows (`--min 100`, `--max 20`, `--min 20 --max 50`, `--min 42.17 --max 42.17`) on `searchRows` with a -150.00 charge, 120.00 deposit, -99.99, -20.00, -20.01, 20.00, 50.00, 50.01, 42.17 and -42.17; assert listed ids per row. Fails at the id assertion (flag is unknown today: exit 2 vs wanted 0), no panic; no stubs needed (cmd slice)

### Build
- [x] Step 2 (batch 1, `internal/report/amount.go` new + `amount_test.go`): `ParseSearchAmounts`, `SearchAmounts`, `AmountError`; table `Test_ParseSearchAmounts_*`: accepted (`12`,`12.5`->1250,`12.50`,`0`,`0.05`,`9999999999999999.99`), one row per refused class (`1,234.56`, `-12`, `+12`, `$12`, `12.`, `.5`, `12.345`, `1e2`, leading space, trailing space, `""`), 16 digits accepted vs 17 refused (control differs by one digit), error parts per class, `Error()` lines pinned verbatim for min and max, order rows (bad min beats bad max; bad max beats min>max; min==max accepted; min>max by one cent refused; `50` vs `20` raw text kept in `Other`/`Value`), nil/nil and nil/one-sided
- [x] Step 3 (batch 2, `store.go:755-760`, `duckstore/search.go:17-47,123-137`, `report/search.go:17-70`, `document/search.go:44-58`): `SearchParams.Min/Max`; `$5/$6` binds, predicates, accounts shift; `SearchRequest.Amounts` passed through, `Search.Amounts` echoed; `document` Min/Max via `Money`. Tests: `duckstore` `Test_search_amount_bounds_compare_the_absolute_amount` (negative found by Min only, deposit found, each bound at value and one cent outside, Min==Max finds exactly the value, Max-only/Min-only, bound 0, zero-amount row, matched counts transactions not splits for a two-split txn) + `Test_search_amount_combines_with_text_window_account_and_limit` (accounts still match after shift: two named accounts + Min) + S02-debt row `Test_search_text_with_limit_counts_every_text_match` in `search_text_test.go` (3 text + 2 non-matching, Limit 2 -> Matched 3, 2 rows newest first); `report` `Test_search_passes_the_amounts_to_the_store` (exact `SearchParams`) and echo; `document` min/max `"12.50"` vs null

### Build (cadence continues)
- [x] Step 4 (batch 3, `cli/search.go:84-127`, `render_search.go:47-53`): `--min`/`--max` string flags with the §2.2 help verbatim; RunE calls `ParseSearchAmounts` (pointer set only when `Changed`) before `searchWindow`, error -> `UsageError`; caption appends `, amount <range>` after dates via `formatMoney` (`20.00 to 50.00` | `at least` | `at most` | `exactly` when equal). Tests: `Test_searchCaption_names_the_amount_range` (all four arms, grouped `1,234.00`, with text and accounts, each arm differs in one variable; min==max is `exactly`, not `20.00 to 20.00`); help pin for both flag lines (`-h` output at wrap width); cmd `Test_run_search_json_echoes_min_and_max_normalized` (`12.5` -> `"12.50"`, null when absent) and text caption; cmd refusal rows `Test_run_search_refuses_a_bad_amount_with_the_ruled_line` (`--min "-12"` exact line, `--max "1,234.56"`, `--min ""`, 17 digits; exit 2; stdout empty; no store opened), `Test_run_search_refuses_min_above_max_before_a_bad_since`, `Test_run_search_refuses_a_bad_min_before_a_bad_since_and_before_a_bad_max`, `Test_run_search_refuses_blank_text_before_a_bad_min`; S02-debt cmd row `search gym --limit 1` (matched = text matches, cut line "of N")

### Sweep
- [ ] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `ParseSearchAmounts`, `AmountError`, `SearchAmounts`; `golangci-lint --fix` may rewrite the order-sensitive `document` byte-literal test to `JSONEq` (undo, see STATE traps)

### Verify
- [ ] Step 6: full verification block (`agent-briefs.md`) + `.claude/scripts/spec-check.py phase3c-search`; tick SCENARIO-03 with its acceptance test; rewrite `STATE.md` (remove S03 from Left unbuilt, close the S02 text×limit debt)

## Handoff

**Binding decisions:**
- `report.ParseSearchAmounts(min, max *string) (SearchAmounts, error)` is the only amount parser and the only min -> max -> min>max check; CLI calls it before `searchWindow`, S09's handler must call it before `ParseSearchWindow` — Rule S9 puts amounts before the window and the raw values must survive into the error.
- `AmountError{Kind, Bound, Value, Other}`: S10 words the MCP lines from these parts (`min "-12" is not an amount; … such as "25" or "19.99"`; `min 50 is more than max 20`); `Error()` is the CLI line only.
- Amounts are `*int64` cents everywhere past the parser; `SearchParams.Min/Max` are nil = no bound; the echo is `document.Money(cents)`.
- Store binds: `$1..$4` as before, `$5` min, `$6` max, accounts from `$7`. S04 adds category as `$7` and moves accounts to `$8` through the `searchFirstAccount` const — still no third statement.
- Caption order is dates, then `, category` (S04), then `, amount` (S03); S04 inserts its part before the amount arm.

**Left unbuilt:** `SearchParams.Category`, `RefusalUnknownCategory`, caption category part, `--category` (S04); S08 refusal matrix incl. `--json` + refusal and unknown flags (S04/S08); `search_transactions` handler, its amount wording and min-as-JSON-number refusal (S09/S10).

**Traps:**
- `CAST($5 AS BIGINT) IS NULL`: a bare `$5 IS NULL` fails to infer the bind type, as `$4` does.
- `abs` must wrap `t.amount` (the transaction), never split amounts (Rule S4: splits are not compared); a two-split transaction must count once in `matched`.
- Pointer from `Changed("min")`, not from `!= ""`: `--min ""` must be refused, not ignored.
- A Min>Max `SearchRequest` handed straight to `Server.Search` is not refused (by design); do not add a second check there without moving the raw-value order tests.

## Phase report

Runs A (1), B1 (2-3) and B2 (4) done. Run V (5-6) remains.

- Step 2 (B1): `internal/report/amount.go`: `ParseSearchAmounts(least, most *string)`, `SearchAmounts{Min, Max *int64}` (cents), `AmountError{Kind, Bound, Value, Other}` returned by value; `Error()` is the CLI line verbatim. Tests `amount_test.go`.
- Step 3 (B1): `SearchParams.Min/Max`; duckstore `$5`/`$6` (`searchFirstAccount` = 7), `searchAmountRange`; `SearchRequest.Amounts` / `Search.Amounts`; `document.NewSearch` Min/Max via `Money`. S02 text x limit store row added.
- Step 4 (B2): `internal/cli/search.go`: `searchAmounts(cmd, &minArg, &maxArg)` (pointer only when `Changed`; any parser error -> `UsageError{msg: err.Error()}`, like `searchWindow`) runs in RunE before `searchWindow`; `--min`/`--max` string flags with the ruled help (placeholder `amount`). `render_search.go`: caption appends `, amount <range>` via `searchAmountRange` (`exactly` when equal, `A to B`, `at least`, `at most`, `formatMoney`).
- Tests added: `internal/cli/render_search_internal_test.go` (2 caption tests), `internal/cli/report_help_test.go` (`Test_search_help_lists_the_min_and_max_flags`), `cmd/quarry/run_search_amount_test.go` (JSON echo, text caption, refusal rows, three order pins, blank-text-before-min; helper `assertSearchRefused`), `cmd/quarry/run_search_limit_test.go` (`Test_run_search_text_with_limit_counts_every_text_match_and_cuts_the_oldest`, closes the S02 cmd row).
- Order mutation (searchWindow before searchAmounts) reddened `refuses_min_above_max_before_a_bad_since` and the `a_bad_min_beats_a_bad_since` / `a_bad_max_beats_a_bad_since` rows. `Changed("min")` -> `*least == ""` reddened `an_empty_min_is_refused,_not_ignored`.
- Green: narrow loop (report, duckstore, cli, cmd/quarry); acceptance `Test_run_search_min_and_max_compare_the_amount_without_its_sign` green; `golangci-lint run ./...` 0 issues. Not yet run: full covered suite, `uncovered-diff.py`, `spec-check.py`, spec tick, STATE rewrite (run V).
