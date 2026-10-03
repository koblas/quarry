# Review 01 — phase3c-search

## Target
Changed files, range `6db50f15b52a..HEAD` (7f932ba).

Pre-gate:
- Full suite `go test rc=0`.
- `uncovered-diff`: 0 uncovered, 3 declared unreachable.
- `test-stats` TOTAL 2117 (+174).
- Mutation sample: `20 sampled of 90 candidates — 20 killed, 0 survived, 0 non-viable, 0 timed out; 825s`.

## Triggered reviewers
- arch-reviewer, correctness-reviewer, refactor-advisor: `internal/**/*.go`
- test-reviewer: `**/*_test.go`

## Skipped reviewers
- api-reviewer: no HTTP files
- pipeline-reviewer: no `.claude/**` files

## BLOCKER
None.

## MAJOR
1. test-reviewer — `internal/mcp/search_test.go:63-95`, `cmd/quarry/run_mcp_search_refusals_test.go:48-98`: the ruled empty-string rows for `search_transactions` are unpinned. Spec §2.7 rules `text ""` as blank text and `since ""`/`until ""` as not-a-date.
   - Failure: a `searchInput` that maps `""` to nil would treat them as absent, and every test still passes. The sibling tools pin these rows (anomalies_test.go:88, recurring_charges_test.go:86, run_mcp_spending_test.go:138).
   - Fix: add `{"text": ""}`, `{"since": ""}` and `{"until": ""}` rows to both tables, with the exact client line and stderr class line.
2. test-reviewer — `internal/report/search_test.go:109-118`: `Test_search_refuses_an_account_it_cannot_pick_without_reading_matches` asserts `assert.Zero(t, got)` on a zero-initialised sentinel. That assertion is vacuous.
   - Fix: seed a non-zero sentinel (`got := store.SearchParams{Limit: 99}`) and assert it is unchanged, as `Test_search_refuses_blank_text_without_reading` does.
3. test-reviewer and correctness-reviewer — `internal/report/amount.go:49-50` and `internal/mcp/search.go:93`: these `// unreachable:` fallthroughs rest on weak reasons.
   - `AmountError` and `Kind` are exported, so `AmountError{Kind: AmountErrorKind(9)}` is constructible.
   - amount.go's grep reason is also false: amount_test.go builds `AmountError` literals.
   - Fix: remove the fallthroughs so no unreachable arm remains. Make the last case the unconditional return, or another shape the exhaustive linter still guards. Alternatively, test an out-of-range Kind row.

## MINOR
- arch / refactor / correctness — `internal/mcp/search.go:33`: `in.Limit` reaches `report.SearchRequest.Limit` without `effectiveLimit` (query.go:37-41). The SDK refuses 0, -1, 501, null, 1.5 and "10" before the handler runs, and the correctness probe confirmed it, so there is no constructible failure. Nothing pins those refusals for `search_transactions`.
- arch — `internal/cli/search.go:51` and `internal/mcp/tools.go:276`: the 500 default is decided in each delivery layer, and `report.Search` has no default.
- refactor — `internal/cli/search.go:267-302`: the `RunE` closure is about 35 lines and mixes phases. Extract `buildSearchRequest` and `searchWarningsFor`.
- refactor — `internal/report/amount.go` and `internal/mcp/search.go:93`: the `// unreachable:` comments are over budget (superseded by MAJOR 3).
- refactor — `internal/cli/search.go:188-189,213`: the `searchArgs` and `searchLimit` docs walk the body instead of stating the contract.
- refactor — `internal/cli/search.go:192-207`: `UsageError{msg: err.Error()}` is repeated about six times. Name it `usage(err)`.
- refactor — `internal/mcp/search.go`: `textRefusedError`, `amountRefusedError` and `namedRefusedError` are three near-identical string error types.
- refactor — `internal/report/search.go`: `SearchInput` and `InvalidUTF8Error` are CLI-only exported names. No change needed now.
- refactor — `internal/report/amount.go`: the `AmountError` and `AmountErrorKind` docs restate per-kind field use, and the struct doc runs 4 lines.
- test — the CLI `--since ""` / `--until ""` refusals are unpinned. `searchWindow` decides on `Changed`.
- test — `internal/report/search_test.go:153-176`: the `refused bool` field drives a branch in the test body. Split it into two tables.
- test — `cmd/quarry/run_mcp_search_refusals_test.go:198`: the stdio invalid-UTF-8 test hand-rolls pipes and uses `select`. Use the existing helpers.
- test — `cmd/quarry/run_mcp_search_refusals_test.go:96` and `internal/mcp/search_test.go:91`: `NotContains` is redundant after the exact `Equal`.
- test — `cmd/quarry/run_mcp_search_refusals_test.go:35-123`: the acceptance test holds two subtests with different shapes. Split it.
- test — `internal/mcp/search_test.go:106`: `Test_search_transactions_passes_its_arguments_to_the_store` checks three facets.
- test — `internal/mcp/log_classes_internal_test.go:135`: the name `Test_logLine_never_carries_the_callers_account_text` is stale; it now covers category and amount too.
- test — `internal/cli/render_search_internal_test.go:2` and `search_internal_test.go:2`: the white-box headers lack the why.
- test — `cmd/quarry/run_search_category_test.go:166` and `run_search_limit_test.go:60`: the `as_text` / `text_with_limit` names are ambiguous, and the tables mix exit codes.
- test — the ruled-order rows are duplicated across `run_search_refusals_test.go:26,70`, `run_search_limit_test.go:148` and `run_search_text_test.go:145`.
- test — `internal/cli/report_help_test.go:293-320`: three help-flag tests repeat their construction.
- test — `--limit abc` / `--limit 1.5` / a missing flag value are unpinned for search.

## NIT
- arch — the cut-line wording is built separately in the CLI and MCP. This is intentional.
- refactor — the `ParseSearch*` docs describe the refusal order.
- refactor — the `duckstore` `searchArgs` doc line is about 190 characters.
- refactor — `render_search.go:51`: the `searchCaption` doc is a format grammar.
- test — `search_amount_test.go:52`: the 11999 row is indistinguishable from the 12000 row.
- test — near-duplicate "combines with" fixtures in `search_amount_test.go:112` and `search_text_test.go`.
- test — `duckstore/search_test.go` is 489 lines. Acceptable.

## Strengths
- `reportedTransaction` and `transferLeg` are shared with `v_cash_flow`, so the SQL is not duplicated.
- The wrapper tests build errors through the real parse paths.
- The `logLine` never-carries-caller-text table was extended.
- Per-fallible-call fault coverage is complete for both statements.
- Every NOT NULL scan is checked against the DDL.

## Verdict: BLOCKED
There are 3 MAJORs, all owned by test-reviewer; correctness-reviewer also raised MAJOR 3.
