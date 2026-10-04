# phase4c-networth — current state

Scenarios complete: SCENARIO-01a (with 02 folded), SCENARIO-01b (with 05 folded), SCENARIO-03 (with 04 folded). Last updated by SCENARIO-03.

## Binding decisions
- Investment cash row: id `txn-<Z_PK>`, `investment_transaction_id = itxn-<Z_PK>` (last column of `transactions`, NULL on register rows; `transactionRows` is positional against the DDL), splits `split-<entry Z_PK>` through the same `mapSplits` path as register entries — Z_PK is unique across ZTRANSACTION entities (SCENARIO-01a, N-1). Entries flowing through `mapSplits` carry transfer links into `pairTransfers` unchanged; pinned for numeric, investment-to-investment, cross-currency, name-form and amount-mismatch links (SCENARIO-01b).
- Row iff investment amount != 0, keyed on amount not action; reinvest_dividend (amount 0) and share-only actions get no row (SCENARIO-01a/02).
- Cash row date = `investment_transactions.date` (posted else entered); `PostedDate` set when a posted day exists; payee nil.
- Unmapped `ZRECONCILESTATUS` refuses a sync only for investment rows with amount != 0, via the shared status helper and `reasonTransactionStatus` (SCENARIO-01a).
- Entry-less non-zero investment transaction gets one synthetic split: id `split-itxn-<txn Z_PK>`, SourceID `-<txn Z_PK>`, NULL category, no memo; derived only from the source Z_PK so a re-sync reproduces it. Investment cash rows only: a register transaction with no entry still fails the splits-sum check (SCENARIO-01b).
- N-4: `duplicateQuery` / `unlinkedTransferQuery` (`duckstore/findings.go`) require `investment_transaction_id IS NULL` on both aliases, in WHERE; findings Long says so (SCENARIO-01b/05).
- `store_info.format_version` is 8 (`FormatVersion`).
- Investment cash flow needs no view change: rows flow through `v_cash_flow` by category kind (income kinds count, margin interest is spending, `system`-kind buys and sells count as neither, uncategorized by sign), pinned in `cmd/quarry/run_investment_cashflow_test.go`; cashflow Long and spend Long (own paragraph) carry the ruled sentence (SCENARIO-03).

## Left unbuilt
- `v_balances_daily`, the conventions sentences for it, `v_net_worth`, `v_account_balances`; holdings Long; remaining `Changes to existing surfaces` rows — SCENARIO-06/07/09/10/18.
- SKILL.md description edit (drop "dividend totals") and SKILL.md:73 ("Net worth: quarry does not compute net worth yet", still true) — SCENARIO-10 (SCENARIO-03).

## Traps
- `mapSplits` must run after `mapInvestmentTransactions` (importer.go); otherwise investment entries silently take the skip path and every cash row fails the splits check.
- Investment cash rows are appended to `transactions` before `pairTransfers` (it builds `accountOf` from it) — moving them later breaks pairing.
- `addEntrylessSplits` runs after `mapSplits` and before `off.firstError()`: an entry refused into `off` also leaves its transaction split-less, harmless only because the refusal follows. Do not move the refusal (SCENARIO-01b).
- `links[i]` belongs to `splits[i]`: append to both together or `pairTransfers` pairs the wrong legs (SCENARIO-01b).
- The `uncategorized` finding reads `v_cash_flow`, which drops transfer legs; an uncategorised transfer entry never raises it (SCENARIO-01b).
- cmd fixtures add investment entries with no `CategoryTag`, so `uncategorized` findings appear in cmd goldens (SCENARIO-01a).
- `offenders.firstError` sorts by `lessOffender`; tied offenders matter in refusal tests.

## Open debts
- Conventions text (`internal/report/sql_conventions.go`, mirrored in `internal/cli/sql_test.go`, `cmd/quarry/run_shared_documents_test.go`, `schema.md`): ruled sentence ends "...security and shares:" and the old "Their amount is..." follows with a capital T after the colon — for the final product-vision pass (SCENARIO-01a).
- `plugin/skills/quarry/references/findings.md` says nothing about investment rows (duplicate / unlinked-transfer ignore them) — not in the spec's copy table; final product-vision pass (SCENARIO-01b).
- Checkpoint 01b MINOR: const doc budgets (1 line) exceeded — `entrylessSplitIDFormat` (`internal/importer/splits.go:~91`), `duplicateQuery`/`unlinkedTransferQuery` (`internal/store/duckstore/findings.go:13-15,22-23`). Trim on next touch.
- Checkpoint 01b MINOR: no pin that an unreadable amount on an investment transaction's entry still refuses the sync (the synthetic split would otherwise mask it; guarded today by shared `off.firstError`). Add a case to `Test_import_refuses_an_investment_value_quarry_cannot_read` (`internal/importer/investments_test.go:~407`) on next importer touch.
