---
id: SCENARIO-07
status: done
---

# SCENARIO-07: Accounts shows investment balances (SCENARIO-08 folded)

Cadence: code-first (no mandatory test-first item: no bug fix, write-safety guard or atomic adapter)
Acceptance test: `cmd/quarry/run_accounts_investment_balance_test.go` `Test_run_accounts_shows_an_investment_balance_as_cash_plus_holdings_value`
Acceptance test (SCENARIO-08, folded): `cmd/quarry/run_status_test.go` `Test_run_status_says_investment_accounts_cash_is_not_checked`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/report/ ./internal/cli/ -run 'Account|Balances|Conventions'` and `go test ./cmd/quarry/ -run 'accounts|status|validation|sql|SchemaReference|shared_documents'`
Mutation checks: LEFT JOIN in `accountBalancesViewDDL` (inner join drops a future-dated-only account) → `Test_account_balances_lists_an_account_with_no_balance_row_as_zero`; `cash + holdings` in the Balance (cash only) → `Test_accounts_reads_an_investment_accounts_cash_and_valued_holdings`
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches; feature `report` with its `store` port and `duckstore` adapter, plus `cli` (sizing pass: OWNS A RUN)

Survey (no new port; `store.Store.Accounts` keeps its signature, `AccountBalance` grows). Production readers of `AccountBalance.Balance`: `duckstore/accounts.go:46-50` scan, `report/accounts.go:36` `NeedsRate`, `cli/render_accounts.go:33,79-85`, `cli/json_accounts.go:42`; nothing else (grep `\.Balance\b`, `AccountBalance{`; LSP `findReferences` on the field for the rest). No memory adapter of `Store.Accounts` exists; tests use `duckstore` and hand fakes (`report/fakes_test.go`, `cli/fakes_test.go`).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_accounts_investment_balance_test.go` (replaces `run_accounts_not_valued_test.go`, delete it) `Test_run_accounts_shows_an_investment_balance_as_cash_plus_holdings_value` — hand-built rows as `run_balances_daily_view_test.go:14-60` (brokerage CAD: deposit + priced holding; chequing); `accounts --currency native` Balance cell = cash + holdings, `--json` `cash` non-null on every row, `holdings_value` set on brokerage and null on chequing. Also add the SCENARIO-08 test: `run_status_test.go` `Test_run_status_says_investment_accounts_cash_is_not_checked` — two investment accounts built with `v9fixture` (as the deleted file's brokerage + IRA), Balances line reads `2 investment accounts' cash not checked`
- [x] Step 2: `store.go:607-618` `AccountBalance` — signature stubs only: add `Cash int64`, `HoldingsValue *int64`; `Balance` becomes `int64`; stub every site `go vet ./...` lists so both acceptance tests compile and fail at their assertions (text still `not valued`; Balances line still old copy)

### Build
- [x] Step 3 (batch 1, view + adapter): `schema.go:174-208` `accountBalancesViewDDL` — rewrite as `v_account_balances` = `accounts` LEFT JOIN `v_balances_daily` at `current_date` (no-row account: cash 0.00, balance 0.00, NULL converted unchanged from today's rule, `holdings_value` 0.00 for brokerage/retirement and NULL otherwise); delete the investment CASE and the `quoted` loop; columns `…, closed, active, cash, holdings_value, balance, balance_cad, balance_usd` (mirrors N-5 order). `duckstore.go:454` — move `accountBalancesViewDDL()` after `balancesDailyViewDDL()` (it now reads that view). `duckstore/accounts.go:11-19,33-52` — query and scan `cash`, `holdings_value` ×100 into `Cash`/`HoldingsValue`. **Timing precondition**: `v_account_balances` is read by every `--account` resolve (`report/accounts.go:71`), and the view's running window cannot take a pushed-down date filter; throwaway test as SCENARIO-06 N-6 (1x store: 14,000 txns, 30 accounts) must read `SELECT * FROM v_account_balances` in well under 1 s (S06: whole `v_balances_daily` 0.6 s, one date 31 ms). Slower, or no pushdown seen → return `PARTIAL: needs ruling` with the numbers; do not substitute a second balance computation. Tests: `accounts_test.go:46-100` re-pin (investment `rrsp` now `Balance` = its cash, `Cash`, `HoldingsValue` 0.00); new `Test_accounts_reads_an_investment_accounts_cash_and_valued_holdings` (priced holding; unpriced holding left out silently, no warning until SCENARIO-14a; USD holding in CAD account converts), `Test_account_balances_lists_an_account_with_no_balance_row_as_zero` (future-dated-only and no-transaction account), non-investment `HoldingsValue` nil; fault: `Accounts` scan/open errors keep `accounts_test.go` fault rows. Re-pin `query_test.go:24` and `views_fx_test.go:325-340` (`balance`, `balance_cad`, `balance_usd` now `DECIMAL(38,2)`; `cash` `DECIMAL(18,2)`, `holdings_value` `DECIMAL(38,2)`), `views_fx_test.go:242,296` if the width shows, `run_sql_fx_test.go:56-75` typeof of `balance_cad`/`balance_usd`
- [x] Step 4 (batch 2, port): `store.go:38,50-52` docs (drop "cannot compute" / "not valued"; `IsInvestmentAccount` still means sync never checks its balance), `store.go:607-618` `AccountBalance` doc (Balance never nil; `BalanceCAD`/`BalanceUSD` nil only when no rate), `report/accounts.go:22-37` `ConvertedBalance` doc + `NeedsRate` drop `a.Balance != nil`. Tests: `report/accounts_test.go:155-166` ("not valued" row becomes a needs-rate row for a USD brokerage), `cli/fx_warning_internal_test.go:59-68`, `report/fakes_test.go` and `cli/fakes_test.go` literals (`Balance` pointer → value; `go vet ./...` lists them). **Behaviour change to pin**: a USD brokerage with no rate now shows `no rate` and raises the existing rate warning; `cmd/quarry/run_accounts_fx_edges_test.go:90-139` rows `:100,:104,:109` ("a not valued … account") become warning rows, re-pinned here with the asserted lines read from the real output once, then named by what they pin
- [x] Step 5 (batch 3, accounts surface): `render_accounts.go:12-13,33,79-85` — Balance cell via `formatMoney(a.Balance)`; delete `notValuedBalance`, `accountBalance` and its nil arm. `json_accounts.go:15-31,42` — `Cash string`, `HoldingsValue *string` (null for non-investment) placed after `balance`, before `converted_balance`; `Balance` string never null; fix the `:17` doc ("Institution and ConvertedBalance are null when absent or unconverted; HoldingsValue null for a non-investment account"). `accounts.go:14-25` — Long: replace the paragraph at `:17-19` and the sentence at `:24-25` with the two ruled texts (Surface & Copy), re-wrap the rest unchanged. Tests, each its own case: `render_accounts_internal_test.go` (`:38-48,83-84,120,160-200` re-pin "not valued" cells; widening row now from long balances), `json_accounts_internal_test.go:46-85` (null balance row gone; `cash` and `holdings_value` null/non-null rows), `cmd/quarry/run_accounts_json_test.go:32-110`, `run_accounts_test.go:32,51,96-104` (Long at wrap width, text + `--json`), `run_accounts_fx_test.go:57,62`, `run_currency_native_test.go:91`, `run_accounts_fx_edges_test.go:185` (closed brokerage with `--all`), `run_holdings_surfaces_test.go:23` (accounts Long pin). Crosses: investment account × `--all`, × `--currency CAD|USD|native`, × `--json` each named by the existing case it re-pins
- [x] Step 6 (batch 4, folded 08 + copy): `render.go:223` `balancesExtrasPhrase` — `humanize.Count(n, "investment account's cash not checked", "investment accounts' cash not checked")`; one site feeds sync (`:96,518`), DIFFER (`:244,512`) and status (`render_status.go:28`). Pins: `render_internal_test.go:407-414` (3 rows), `render_status_internal_test.go:101`, `run_status_test.go:53`, `run_validation_test.go:92`; new row for `balancesDifferPhrase` with `InvestmentAccounts` (no test exists), plural and singular. `holdings.go:36-37` Long last paragraph → ruled sentence; pin in `run_holdings_surfaces_test.go`. Conventions: `sql_conventions.go:10` → `v_account_balances (v_balances_daily for today) has balance_cad and` re-wrapped; hand copies `cli/sql_test.go:213`, `run_shared_documents_test.go:354`; regenerate `plugin/skills/quarry/references/schema.md` (conventions at `:14`, `v_account_balances` table `:275-288`) with `go test ./cmd/quarry/ -run SchemaReference -update`, then diff

### Sweep
- [x] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; `go doc ./internal/store AccountBalance` and `./internal/report` read true (no "not valued", no "cannot compute")

### Verify
- [x] Step 8: full verification block (`.claude/rules/agent-briefs.md`) + `spec-check.py phase4c-networth`; tick SCENARIO-07 with its acceptance test and SCENARIO-08 as "delivered by SCENARIO-07" with its test; rewrite STATE.md

## Handoff

**Binding decisions:**
- `v_account_balances` reads `v_balances_daily` at `current_date`, so it is built after it (`duckstore.go:454`); `balance`/`balance_cad`/`balance_usd` are `DECIMAL(38,2)` — S06's wide conversion; S09's sums inherit it.
- `store.AccountBalance.Balance` is `int64`, never nil, and `NeedsRate` no longer tests it; `Cash int64`, `HoldingsValue *int64` (nil outside brokerage/retirement) — S09/S10 and S14a read these; S14a adds `HoldingsUnvalued`.
- Columns added to `v_account_balances` and `--json` keys (`cash`, `holdings_value` after `balance`) are not ruled in Surface & Copy; position chosen to mirror N-5 — orchestrator may veto before dispatch.

**Left unbuilt:**
- Warnings 3-5 on `quarry accounts` (unpriced holding, no currency, other currency) and `HoldingsUnvalued` on `AccountBalance` — SCENARIO-14a. Until then an account with `holdings_unvalued > 0` shows its balance with those holdings left out, silently.
- `v_net_worth`, `phase4ViewPattern` deletion, SKILL edits, remaining Changes rows (cashflow/spend/findings Long done in 01b/03) — S09/S10/S18.

**Traps:**
- `NeedsRate` change turns "a not valued … account" rows in `run_accounts_fx_edges_test.go` into warning rows: a USD brokerage with no rate now shows `no rate` and the rate warning; this is correct, not a regression.
- `v_account_balances` feeds `resolveAccounts` (`report/accounts.go:71`), so every `--account` command pays its cost; the Step 3 timing check guards that.
- Mutating `accountBalancesViewDDL` order in `duckstore.go:454` fails at `CREATE VIEW` with a catalog error from `Replace`, not at a test assertion.
- `schema.md` is generated; hand-editing it fails the schema reference test.

## Phase report

Run V (steps 7-8) done; scenario complete. Both acceptance tests green; full covered suite rc=0; `uncovered-diff.py` vs 1d5c656: 0 uncovered added lines; `go test -race` on store, report, cli, cmd/quarry green; lint `0 issues`; spec-check OK.

Added in V: `cmd/quarry/run_accounts_investment_balance_test.go` `Test_run_accounts_converts_an_investment_balance_as_cash_plus_holdings_value` (USD brokerage, cash 900.01 + valued holding 20.00 = 920.01; `--currency CAD` text and `--json` converted 1,150.01 at 1.25; `--currency USD` 920.01). Green on arrival: the behaviour was built in B1/B2; B1's cash-only-Balance mutation reddens it as well.

Nothing left for later runs. Remaining work belongs to other scenarios (see STATE.md).
