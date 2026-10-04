---
id: SCENARIO-09
status: open
---

# SCENARIO-09: Net-worth view covers reported accounts (SCENARIO-18 folded)

Cadence: code-first (no mandatory test-first item: no write-safety guard, no atomic adapter, no bug fix)
Acceptance test: `cmd/quarry/run_net_worth_view_test.go` `Test_run_sql_sums_net_worth_by_type_and_currency_over_the_accounts_quickens_reports_count`
Acceptance test (SCENARIO-18, folded): `cmd/quarry/run_skill_schema_reference_test.go` `Test_skill_schema_reference_carries_each_view_comment` (Len 4 to 5; the last view pin `does_not_name_a_view_the_store_lacks` is deleted, this one then pins `v_net_worth`)
Narrow loop: `go test ./internal/store/duckstore/ -run 'net_worth|Test_query' && go test ./cmd/quarry/ -run 'net_worth|schema_reference|references_name|drift|documents_byte_for_byte' && go test ./internal/report/ ./internal/cli/ -run 'sql_conventions|sql_help'` (check patterns with `-list`)
Mutation checks: reported-account predicate in `netWorthViewDDL` (`reportedAccount`, dropping `a.in_reports` or `NOT a.linked_tracking`) → `Test_net_worth_leaves_out_accounts_not_in_reports_and_linked_tracking`
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches (3 behaviour + 1 timing), 1 feature package (`internal/store/duckstore`; `internal/report` one copy sentence + pins, no logic; 18 adds only test deletions and re-pins)

Spec: N-6, edge rows closed account / not-in-reports / linked-tracking / rate gap / other currency. No `FormatVersion` bump (views rebuilt on each Replace; S06 precedent). Copy lines delivered: N-6 COMMENT verbatim; conventions sentence (ruling below). Everything else in S18's copy list is owned by 01a/01b/03/06/07 (done) or 10/17 (not here).

**Survey (what exists, no re-plan).** `reportedAccount` (`schema.go:209-211`) = `a.in_reports AND NOT a.linked_tracking`, aliased `a`. `v_balances_daily` columns/widths (STATE): `balance`, `balance_cad`, `balance_usd` DECIMAL(38,2); `sum()` of DECIMAL(38,2) should stay (38,2): the columns test pins it. Fixtures to reuse: `balanceRows`/`cashRows` (`balances_daily_view_test.go:22-40`), `newStoreWithRates` (`views_fx_test.go:42`), `ratesOn` (`rates_test.go:37`), `queryTexts` (`views_test.go:94`), `Account{NotInReports, LinkedTracking, Closed}` (`balances_daily_view_test.go:108-122`). Acceptance template: `run_balances_daily_view_test.go:14-65` (`replaceStoreWithRates`, `usdRate`, `spendRows`).

**Rulings recorded (same as S06).** Conventions sentence = the N-6 COMMENT with a "v_net_worth has " prefix, appended **inside the existing last (balances) paragraph** of `SQLConventions`, not a new paragraph: `v_net_worth has net worth by day, account type and currency over the accounts Quicken's reports count, as quarry networth does; sum balance_cad or balance_usd over one date for the total; a NULL there means no exchange rate for that day.` Wrapped at the existing width.

**Deviation from STATE trap (S06 "count(col) = count(*)").** Grain is day x type x currency and rate presence depends only on (currency, day), so every account in a row has a rate or none does: NULL-when-any = NULL-when-all = plain `sum()` over NULLs. Use plain `sum(balance_cad)`; a `count` guard is an equivalent mutant (no test can redden it), so no mutation check is claimed for it. Still one pass over `v_balances_daily` joined to `accounts`, no subqueries.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_net_worth_view_test.go` `Test_run_sql_sums_net_worth_by_type_and_currency_over_the_accounts_quickens_reports_count` — `run sql --csv` over `replaceStoreWithRates`, one date: counted CAD chequing x2 (one closed), USD chequing, a not-in-reports and a linked-tracking account; assert every column; keep to the Gherkin. Also `run_skill_schema_reference_test.go:173` Len 4 to 5 (SCENARIO-18's red, at the `require.Len`).
- [ ] Step 2: `schema.go` after `:354` `netWorthViewDDL` + `netWorthViewComment` stub (columns only); append `netWorthViewDDL()` at the end of the `duckstore.go:454` Exec (after `accountBalancesViewDDL()`, ordering of the two untouched); `duckstore/query_test.go:31-35` `storeRelations` gains `v_net_worth` (its `NotEmpty` loop needs a row). Red at the CSV assertion. Expected red outside the loop until Step 6: `Test_skill_schema_reference_matches_the_committed_file`, `…_does_not_name_a_view_the_store_lacks`.

### Build
- [ ] Step 3: `schema.go` `netWorthViewDDL` — grain, filter, counts, `balance`; new `internal/store/duckstore/net_worth_view_test.go`. Rows: one row per date x type x currency; `accounts` counts members; two accounts of one type and currency sum into one row; same type other currency and same currency other type are separate rows; closed account counted (and stays counted on later days); `Test_net_worth_leaves_out_accounts_not_in_reports_and_linked_tracking` (each its own row, control = counted account differing in that one flag; a date whose only accounts are left out has no row); an account is absent before its first transaction (bound: day before / day of); negative balance (liability) keeps its sign.
- [ ] Step 4: `schema.go` `netWorthViewDDL` — `balance_cad`, `balance_usd` as plain `sum`; `COMMENT ON VIEW` from `netWorthViewComment` (N-6 verbatim, `strings.ReplaceAll` quote escape like `schema.go:354`). Rows: converted = sum of per-account rounded values (two accounts whose rounded sum differs from the rounded total); CAD row before the first rate keeps `balance_cad`, `balance_usd` NULL, USD mirror; EUR row both NULL (out-of-domain); rate gap takes prior rate; one date's converted columns summed across rows = the total (the doc'd use). `Test_net_worth_view_lists_its_columns_in_order` (types incl. `accounts` BIGINT, widths 38) and `…_carries_its_note`, mirroring `balances_daily_view_test.go:419-460`.
- [ ] Step 5: timing re-confirmation on the built view — throwaway test (delete before commit; worktree must end clean), S06's synthetic stores through `duckstore.Replace` at 1x (14,000 txns, 30 accounts / 9 investment, 21 securities, 104k prices, 3.6k rates) and 3x (42,000 txns, 90 accounts). Time warm `SELECT … FROM v_net_worth WHERE date IN (<200 month ends>)`, plus one date and whole view. **Pass = under 1 s** (S06 draft: 80 ms at 1x, 0.49 s at 3x). Over 1 s: stop, return `PARTIAL: N-6 timing fails` with the numbers; fallback needs a scoped product re-ruling. Record numbers in `## Phase report`.
- [ ] Step 6: copy + pins + folded SCENARIO-18 deletions.
  - Copy: `internal/report/sql_conventions.go:44-48` append the ruled sentence to the last paragraph; `sql_conventions_test.go:19-23` add `v_net_worth`, `:44-47` also assert the paragraph ends with the sentence; re-pin hand copies `internal/cli/sql_test.go:249-253`, `cmd/quarry/run_shared_documents_test.go:390-394`; `duckstore/doc.go:9,13` name the view; regenerate `plugin/skills/quarry/references/schema.md` with `go test ./cmd/quarry -run Test_skill_schema_reference_matches_the_committed_file -update`.
  - SCENARIO-18: delete `phase4ViewPattern` (`run_skill_references_test.go:19-22`, var block shrinks to `findingTypeLine`); `:55-58` `phase4OrQuickenNames` becomes `quickenTableNames` (Z-table matches only); `:98-109` test renamed `…_flags_crafted_quicken_text`, delete its `a Phase 4 view` case (`:100`); `:154-161` renamed `Test_references_name_no_quicken_table`; delete `Test_skill_schema_reference_does_not_name_a_view_the_store_lacks` (`run_skill_schema_reference_test.go:179-183`); `run_skill_drift_names_test.go:221-225` `unknown_view` example re-pointed to a name the store never has (`v_forecast`) with the same expected message. Note: this example does not go red on its own (`relations` is the local list at `:191`), it only stops being true to life; re-point anyway.

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `netWorthViewDDL`/`netWorthViewComment` within budget.

### Verify
- [ ] Step 8: full verification + `.claude/scripts/spec-check.py phase4c-networth` → tick SCENARIO-09 and SCENARIO-18 (18's line: "delivered by SCENARIO-09", test last on the line); STATE.md rewrite (drop the S09 trap/left-unbuilt entries; add the timing numbers); `status: done`.

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `v_net_worth` columns `date, type, currency, accounts, balance, balance_cad, balance_usd`; sums are DECIMAL(38,2) — S10-S17's networth reader and MCP `net_worth` read it by these names; the total for a date is the sum of one converted column over its rows, NULL meaning a missing rate.
- Converted sums are plain `sum` over `v_balances_daily` rounded per-account values — rate presence is uniform within (currency, day), so no `count` guard is needed.
- Conventions: the `v_net_worth` sentence is the last sentence of the balances paragraph (not a new paragraph); S10 owns SKILL/PRD copy.
- `quickenTableNames` replaces `phase4OrQuickenNames` — the Phase-4 view pin is gone; nothing guards references against naming a view the store lacks except the drift tests' `relations` list.

**Left unbuilt:**
- `quarry networth`, MCP `net_worth`, the `Store`/report reader of `v_net_worth` — S10 onwards.
- SKILL description/§4/:73, PRD L169 — S10; SKILL §9 and `--help` Tools line — S17.

**Traps:**
- A "closed" account still counts on days before it closed; `v_balances_daily` carries it through today, so a closed account's final balance keeps appearing in `v_net_worth` until a transaction zeroes it.
- `Test_query_prints_every_column_of_each_table_and_view` needs `newBuiltStore` to yield a `v_net_worth` row; a reported account with a transaction is enough.
- The `unknown_view` drift example is built on a local `relations` slice, not the store; it never goes red when a view is added.
