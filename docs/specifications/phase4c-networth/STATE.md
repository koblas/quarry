# phase4c-networth — current state

Scenarios complete: SCENARIO-01a (with 02 folded), SCENARIO-01b (with 05 folded), SCENARIO-03 (with 04 folded), SCENARIO-06, SCENARIO-07 (with 08 folded). Last updated by SCENARIO-07.

## Binding decisions
- Investment cash row: id `txn-<Z_PK>`, `investment_transaction_id = itxn-<Z_PK>` (last column of `transactions`, NULL on register rows; `transactionRows` is positional against the DDL), splits `split-<entry Z_PK>` through the same `mapSplits` path as register entries — Z_PK is unique across ZTRANSACTION entities (SCENARIO-01a, N-1). Entries flowing through `mapSplits` carry transfer links into `pairTransfers` unchanged; pinned for numeric, investment-to-investment, cross-currency, name-form and amount-mismatch links (SCENARIO-01b).
- Row iff investment amount != 0, keyed on amount not action; reinvest_dividend (amount 0) and share-only actions get no row (SCENARIO-01a/02).
- Cash row date = `investment_transactions.date` (posted else entered); `PostedDate` set when a posted day exists; payee nil.
- Unmapped `ZRECONCILESTATUS` refuses a sync only for investment rows with amount != 0, via the shared status helper and `reasonTransactionStatus` (SCENARIO-01a).
- Entry-less non-zero investment transaction gets one synthetic split: id `split-itxn-<txn Z_PK>`, SourceID `-<txn Z_PK>`, NULL category, no memo; derived only from the source Z_PK so a re-sync reproduces it. Investment cash rows only: a register transaction with no entry still fails the splits-sum check (SCENARIO-01b).
- N-4: `duplicateQuery` / `unlinkedTransferQuery` (`duckstore/findings.go`) require `investment_transaction_id IS NULL` on both aliases, in WHERE; findings Long says so (SCENARIO-01b/05).
- `store_info.format_version` is 8 (`FormatVersion`).
- `v_balances_daily` (`duckstore/schema.go` `balancesDailyViewDDL`, built after `holdingsViewDDL()` in the `duckstore.go:454` Exec; no FormatVersion bump, views are recreated every Replace): columns in N-5 order; `cash` DECIMAL(18,2); `holdings_value`, `balance`, `balance_cad`, `balance_usd` DECIMAL(38,2) via `convertedToWide(..., 38)` — S09's sums inherit 38 (SCENARIO-06).
- `holdings_value` / `holdings_unvalued` are NULL outside brokerage/retirement; inside, `holdings_value` is 0.00 when nothing is valued and `holdings_unvalued` counts left-out holdings (S14a warnings read it). A holding is valued iff `v_holdings` gives a value in the account's currency (own currency -> `value`, else `value_cad`/`value_usd`); anything NULL is unvalued (SCENARIO-06).
- `v_account_balances` = `accounts` LEFT JOIN `v_balances_daily` at `current_date`, built after it; no-row account is cash 0.00 / balance 0.00; columns `…, closed, active, cash, holdings_value, balance, balance_cad, balance_usd`, balances `DECIMAL(38,2)`. `store.AccountBalance.Balance` is `int64`, never nil (`Cash int64`, `HoldingsValue *int64` nil outside brokerage/retirement); `NeedsRate` no longer tests it, so a USD brokerage with no rate shows `no rate` and raises the rate warning. `IsInvestmentAccount` now means only "sync never checks its cash" (SCENARIO-07).
- `quarry accounts --json` key order: `…, balance, cash, holdings_value, converted_balance, …`; `balance` and `cash` strings never null, `holdings_value` null for a non-investment account. Position mirrors N-5, not ruled in Surface & Copy (SCENARIO-07).
- Balances line phrase `balancesExtrasPhrase` (`render.go`) is the one site for sync, DIFFER and status: `N investment account's|accounts' cash not checked`. Investment cash flow needs no view change: rows flow through `v_cash_flow` by category kind, pinned in `run_investment_cashflow_test.go` (SCENARIO-03/07).
- Conventions' last paragraph is the balances paragraph (`internal/report/sql_conventions.go`; mirrored in `internal/cli/sql_test.go`, `cmd/quarry/run_shared_documents_test.go`; `schema.md` is generated, `go test ./cmd/quarry/ -run Test_skill_schema_reference -update`): S09 appends its `v_net_worth` sentence there (SCENARIO-06/07).

## Left unbuilt
- Warnings 3-5 on `quarry accounts` (unpriced holding, no currency, other currency) and `HoldingsUnvalued` on `AccountBalance` — SCENARIO-14a. Until then an account with `holdings_unvalued > 0` shows its balance with those holdings left out, silently (SCENARIO-07).
- `v_net_worth`, `phase4ViewPattern` deletion (`run_skill_references_test.go:21` now `\bv_net_worth\b`), `:182` `v_net_worth` arm of `run_skill_schema_reference_test.go`, drift `unknown_view` example — SCENARIO-09. Remaining `Changes to existing surfaces` rows — SCENARIO-10/18.
- SKILL.md description edit (drop "dividend totals") and SKILL.md:73 ("Net worth: quarry does not compute net worth yet", still true) — SCENARIO-10 (SCENARIO-03).

## Traps
- `mapSplits` must run after `mapInvestmentTransactions` (importer.go); otherwise investment entries silently take the skip path and every cash row fails the splits check.
- Investment cash rows are appended to `transactions` before `pairTransfers` (it builds `accountOf` from it) — moving them later breaks pairing.
- `addEntrylessSplits` runs after `mapSplits` and before `off.firstError()`: an entry refused into `off` also leaves its transaction split-less, harmless only because the refusal follows. Do not move the refusal (SCENARIO-01b).
- `links[i]` belongs to `splits[i]`: append to both together or `pairTransfers` pairs the wrong legs (SCENARIO-01b).
- The `uncategorized` finding reads `v_cash_flow`, which drops transfer legs; an uncategorised transfer entry never raises it (SCENARIO-01b).
- cmd fixtures add investment entries with no `CategoryTag`, so `uncategorized` findings appear in cmd goldens (SCENARIO-01a). `cmd` store helpers bypass the importer: an investment cash row in a cmd fixture is built by hand with `InvestmentTransactionID` (SCENARIO-06/07).
- `offenders.firstError` sorts by `lessOffender`; tied offenders matter in refusal tests.
- `accountBalancesViewDDL()` must stay after `balancesDailyViewDDL()` in the `duckstore.go:454` Exec; reordering fails at `CREATE VIEW` with a catalog error from `Replace`, not at an assertion. `v_account_balances` feeds `resolveAccounts`, so every `--account` command pays its cost (S06 timing: whole view 0.6 s at 1x) (SCENARIO-07).
- S09: at 3x scale (42,000 txns) a 200-date `IN` cost as much as the whole view (~0.5 s); `v_net_worth` must be one pass over `v_balances_daily` (aggregate with `count(col) = count(*)` for NULL-when-any-rate-missing), no per-row subqueries; re-confirm timing on the real view (SCENARIO-06).
- The `v_balances_daily` COMMENT names `quarry networth` before that command exists; the skill drift test scans only code spans, so schema.md prose does not trip it (SCENARIO-06).
- Fixture RRSP (retirement, no holdings) reads 1,000.00 (its cash) in cmd goldens, not "not valued" (SCENARIO-07).

- `v_account_balances` timing (SCENARIO-07 B1 throwaway, 1× synthetic: 14k txns, 30 accounts, 21 securities, 104k prices, 3.6k rates): `SELECT * FROM v_account_balances` 70-78 ms warm, `Store.Accounts` 68-70 ms; every `--account` resolve (`report/accounts.go:71`) pays this. Re-time on the real store in SCENARIO-19.

## Open debts
- Checkpoint 07 MINOR: setup-narrating comments — `cmd/quarry/run_status_test.go:63`, `cmd/quarry/run_accounts_investment_balance_test.go:16`; delete on next touch.
- Checkpoint 07 MINOR: closed investment account with cash/holdings not pinned in `accounts --all --json` (`cmd/quarry/run_accounts_json_test.go`); no constructible failure today.
- Conventions text (`internal/report/sql_conventions.go`, mirrors above): ruled sentence ends "...security and shares:" and the old "Their amount is..." follows with a capital T after the colon — final product-vision pass (SCENARIO-01a).
- `plugin/skills/quarry/references/findings.md` says nothing about investment rows (duplicate / unlinked-transfer ignore them) — not in the spec's copy table; final product-vision pass (SCENARIO-01b).
- Checkpoint 01b MINOR: const doc budgets (1 line) exceeded — `entrylessSplitIDFormat` (`internal/importer/splits.go:~91`), `duplicateQuery`/`unlinkedTransferQuery` (`internal/store/duckstore/findings.go:13-15,22-23`). Trim on next touch.
- Checkpoint 01b MINOR: no pin that an unreadable amount on an investment transaction's entry still refuses the sync (the synthetic split would otherwise mask it; guarded today by shared `off.firstError`). Add a case to `Test_import_refuses_an_investment_value_quarry_cannot_read` (`internal/importer/investments_test.go:~407`) on next importer touch.
