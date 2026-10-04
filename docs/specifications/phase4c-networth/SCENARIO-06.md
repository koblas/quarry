---
id: SCENARIO-06
status: open
---

# SCENARIO-06: Daily balances combine cash and holdings

Cadence: code-first
Acceptance test: `cmd/quarry/run_balances_daily_view_test.go` `Test_run_sql_combines_cash_and_valued_holdings_from_v_balances_daily`
Narrow loop: A/B1 `go test ./internal/store/duckstore/ -run 'balances_daily|Test_query' && go test ./cmd/quarry/ -run 'v_balances_daily'`; B2 adds `go test ./cmd/quarry/ -run 'schema_reference|references_name|documents_byte_for_byte' && go test ./internal/report/ ./internal/cli/ -run 'sql_conventions|sql_help'` (patterns checked with `-list`)
Mutation checks: `holdings_unvalued` count predicate in `balancesDailyViewDDL` → `Test_balances_daily_counts_each_holding_it_cannot_value`
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches, 1 feature package (`internal/store/duckstore`; `internal/report` touched for one copy const + its pin, no logic)

Spec: `specification.md` SCENARIO-06, N-5 (COMMENT verbatim), edge rows "Excluded-from-reports transaction", "Future-dated transaction", "USD security in a CAD account", "Rate gap". No `FormatVersion` bump: format 8 is unshipped on this branch and `build` (`duckstore.go:454`) recreates every view on each Replace (4b SCENARIO-02 precedent).

**N-6 precondition — PASSED (no fallback, no product re-ruling).** Throwaway test (deleted; worktree clean) built a synthetic store through `duckstore.Replace` and created a draft `v_balances_daily` + draft `v_net_worth` on the real `v_holdings`. Warm timings, Apple M-series, real store never touched:
- 1× (14,000 txns, 30 accounts / 9 investment, 21 securities, 840 spans, 104,151 prices, 3,588 rates, 2013-02-07→today): one date 33 ms; today 31 ms; `IN` 200 month ends 80 ms; whole view (149,513 rows) 0.6 s; draft `v_net_worth` one date 32 ms, 200 month ends 80 ms.
- 3× (42,000 txns, 90 accounts, 2,520 spans, 312,453 prices): one date 0.17 s; 200 month ends 0.50 s (whole view 0.48 s); `v_net_worth` 200 month ends 0.49 s.
- Draft computed `balance_usd` by plain division, not `convertedToWide`'s integer form — negligible at ~6k filtered rows, but these are timings of a draft. S09 re-confirms on the real `v_net_worth`.

**Deviations for the orchestrator to rule before run A:**
1. **Conventions copy (needs a scoped copy ruling).** Spec says "append the `v_balances_daily` sentence (N-5 COMMENT)". Planned text, as a new final paragraph of `SQLConventions`: `v_balances_daily has one row per account per day from its first transaction through today; cash is the sum of its transactions to that day, holdings_value its holdings' value in its own currency (NULL outside brokerage and retirement accounts), balance is cash plus holdings_value, as quarry accounts and quarry networth use; filter by date.` A new paragraph (not inside the investment one) because `Test_sql_conventions_list_the_action_values` (`sql_conventions_test.go:37-41`) pins the action list as the text's suffix and `run_holdings_surfaces_test.go:30-38` pins "Neither includes cash … split." as contiguous.
2. **Width.** `holdings_value`, `balance`, `balance_cad`, `balance_usd` are DECIMAL(38,2) via `convertedToWide(…, 38)`, not `convertedTo`: `maxHoldingValue` (`holdings_value_test.go:14`) is 24 digits and overflows DECIMAL(18,2). `cash` stays DECIMAL(18,2).
3. **Pins beyond `:182`.** `phase4ViewPattern` (`run_skill_references_test.go:21`) must narrow to `v_net_worth` and its case at `:101` go, or the regenerated schema.md fails `Test_references_name_no_phase_4_view_or_quicken_table`.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_balances_daily_view_test.go` `Test_run_sql_combines_cash_and_valued_holdings_from_v_balances_daily` — `run sql --csv` over `replaceStoreWithRates` (`run_sql_fx_test.go:32`), template `run_holdings_view_test.go:14-58`. One CAD brokerage: a register txn plus a hand-built investment cash row (`InvestmentTransactionID` set — the store helper bypasses the importer); holdings priced CAD, priced USD (converted), unpriced, EUR. Assert every N-5 column for one date, incl. `holdings_unvalued` = 2. Keep to the Gherkin; edge arms live in duckstore tests.
- [x] Step 2: `internal/store/duckstore/schema.go:290` (after `holdingsViewDDL`) `balancesDailyViewDDL` + `balancesDailyViewComment` stub (columns only, no rows) appended to the Exec at `duckstore.go:454` **after** `holdingsViewDDL()`; `duckstore/query_test.go:31-35` `storeRelations` gains the view — red at the CSV assertion. Expected red outside the A/B1 loop until Step 6: `Test_skill_schema_reference_matches_the_committed_file` (from here), `…_carries_each_view_comment` (from Step 5).

### Build
- [ ] Step 3: `schema.go` `balancesDailyViewDDL` — grain + cash; new `internal/store/duckstore/balances_daily_view_test.go` (fixtures like `holdingRows`, `holdings_view_test.go:22-37`; today via `localToday()`, `accounts_test.go:19`, as `holdings_view_test.go:58-92`). Rows: first day = first txn when it precedes the first holding, and = first holding `from_date` when that precedes; last day today; no rows for an account with neither; no rows when its only txn is future-dated; closed, not-in-reports, linked-tracking each listed. Cash: a txn counts on its date and not the day before (bound); a day without txns carries prior cash; excluded-from-reports included; a future-dated txn absent from `cash` and `balance_cad` before its date. Non-investment account: `holdings_value` and `holdings_unvalued` NULL.
- [ ] Step 4: `schema.go` `balancesDailyViewDDL` — holdings arms off `v_holdings` (`schema.go:265-289`) in the account's currency; `Test_balances_daily_counts_each_holding_it_cannot_value` (rows: no price, NULL currency, EUR, CAD↔USD before the first rate) plus value rows: own currency as is; USD holding in CAD account and CAD holding in USD account converted and rounded at the date's rate; nothing held (day between spans) 0.00/0; all held unvalued → 0.00 with count; retirement valued like brokerage; largest holding (`maxDecimal18x6`) through `v_balances_daily` without overflow. Out-of-domain: EUR account — EUR holding as is, CAD holding unvalued.
- [ ] Step 5: `schema.go` `balancesDailyViewDDL` — `balance`, `balance_cad`, `balance_usd`, `usd_cad` (ASOF `fx_rates`, `convertedToWide(…, 38)`, `convert_sql.go:37-50`) + `COMMENT ON VIEW` from `balancesDailyViewComment` (N-5 verbatim). Rows: before the first rate a CAD account has `balance_cad` = balance and `balance_usd` NULL, a USD account the mirror; rate gap takes the prior rate; EUR account both NULL. `Test_balances_daily_view_lists_its_columns_in_order` and `…_carries_its_note`, mirroring `holdings_view_test.go:158-194`.
- [ ] Step 6: copy + pins — `internal/report/sql_conventions.go:4-44` new final paragraph (Deviation 1 text); `sql_conventions_test.go:19-23` add `v_balances_daily`, `:37-41` reworked so the action list still closes the investment paragraph; re-pin hand copies `internal/cli/sql_test.go:208-246`, `cmd/quarry/run_shared_documents_test.go:349-387` at existing wrap; `run_skill_schema_reference_test.go:173` Len 3→4, `:182` drop `v_balances_daily` (keep `v_net_worth`); `run_skill_references_test.go:21` pattern → `\bv_net_worth\b`, delete case `:101`; `duckstore/doc.go:8-12` name it; regenerate `plugin/skills/quarry/references/schema.md` with `go test ./cmd/quarry -run Test_skill_schema_reference_matches_the_committed_file -update`.

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `balancesDailyViewDDL`/`balancesDailyViewComment` within budget.

### Verify
- [ ] Step 8: full verification + `.claude/scripts/spec-check.py phase4c-networth` → tick SCENARIO-06 with its acceptance test.

## Handoff

**Orchestrator ruling 2026-10-04:** the conventions sentence is ruled — it is the spec's N-5 COMMENT text with the subject "v_balances_daily has" prepended, which is what *Changes to existing surfaces* ("append the v_balances_daily and v_net_worth sentences (N-5, N-6 comments)") asks for; new last paragraph of `SQLConventions` as planned. DECIMAL(38,2) widening, `phase4ViewPattern` narrowing to `v_net_worth`, and no FormatVersion bump are accepted.

**Binding decisions** — a later scenario must not contradict these without saying so:
- `v_balances_daily` columns in N-5 order; `cash` DECIMAL(18,2); `holdings_value`, `balance`, `balance_cad`, `balance_usd` DECIMAL(38,2) — the largest-holding fixture overflows 18 digits. S07's `v_account_balances` and S09's sums inherit 38.
- `holdings_value` and `holdings_unvalued` are both NULL outside brokerage/retirement; inside, `holdings_value` is 0.00 when nothing is valued (nothing held, or all unvalued) and `holdings_unvalued` counts the left-out holdings — S14a's warnings read that count.
- A holding is valued iff `v_holdings` gives a value in the account's currency: own currency → `value`, else `value_cad`/`value_usd` for a CAD/USD account; anything NULL is unvalued.
- The view reads `v_holdings`, so it is built after `holdingsViewDDL()` in `duckstore.go:454`.
- The conventions' last paragraph is the balances paragraph; S09 appends its `v_net_worth` sentence there, S07 adds "(v_balances_daily for today)" at the `v_account_balances` sentence in paragraph 1.

**Left unbuilt** — named so nobody assumes it exists:
- `v_account_balances` rebuilt on this view, `cash`/`holdings_value` on it — S07.
- `v_net_worth`, `phase4ViewPattern` deletion, `:182` `v_net_worth` arm, drift `unknown_view` example — S09.

**Traps** — things that look right and are not:
- S07: `accountBalancesViewDDL()` is first in the `duckstore.go:454` Exec; on top of `v_balances_daily` it must move after it. `query_test.go:24` pins `v_account_balances.balance` as DECIMAL(18,2) — the 38 width breaks it.
- S09: at 3× scale a 200-date `IN` cost as much as the whole view; `v_net_worth` must be one pass over `v_balances_daily` (aggregate with `count(col) = count(*)` for the NULL-when-any-rate-missing rule), no per-row subqueries.
- The COMMENT names `quarry networth` before that command exists; the skill drift test scans only code spans, so prose in schema.md does not trip it.
- `cmd` store helpers bypass the importer: an investment cash row must be built by hand with `InvestmentTransactionID`.

## Phase report

Run A (steps 1-2) done; red.

Files:
- `cmd/quarry/run_balances_daily_view_test.go` — acceptance test. Fixture: one CAD brokerage `acct-cad`; register deposit +1000.00 and investment cash row `txn-inv-cad` -100.00 on 2026-03-02; holdings bought 03-02 (CAD priced 2.0 x 10 = 20.00, USD priced 1.0 x 20 USD = 25.00 CAD at 1.25, unpriced CAD, EUR priced); rates 03-05 1.25 and 03-12 1.30. Queries 2026-03-10: expects `cash 900.00, holdings_value 45.00, holdings_unvalued 2, balance 945.00, balance_cad 945.00, balance_usd 756.00, usd_cad 1.250000`.
- `internal/store/duckstore/schema.go:292-315` — `balancesDailyViewComment` (N-5 text, final) and `balancesDailyViewDDL()` stub: typed NULL columns `WHERE false`, comment already attached. B1 replaces the SELECT body; B2/Step 5 keep the comment const as is.
- `internal/store/duckstore/duckstore.go:454` — `balancesDailyViewDDL()` appended after `holdingsViewDDL()`.
- `internal/store/duckstore/query_test.go:33` — `storeRelations` gains `v_balances_daily`.

Red now (expected):
- `Test_run_sql_combines_cash_and_valued_holdings_from_v_balances_daily` at its assertion: actual is the header only, no data row.
- `Test_query_prints_every_column_of_each_table_and_view/v_balances_daily`: "Should NOT be empty" (view has no rows yet) — goes green with Step 3.
- Outside the narrow loop until Step 6: `Test_skill_schema_reference_matches_the_committed_file` (not run here), later `…_carries_each_view_comment`.

Do not redo: the stub already carries the column list and types (cash DECIMAL(18,2); holdings_value, balance, balance_cad, balance_usd DECIMAL(38,2); holdings_unvalued BIGINT; usd_cad DECIMAL(10,6)).
