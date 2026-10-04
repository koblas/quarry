# phase4c-networth — current state

Scenarios complete: SCENARIO-01a (with SCENARIO-02 folded). Last updated by SCENARIO-01a.

## Binding decisions
- Investment cash row: id `txn-<Z_PK>`, `investment_transaction_id = itxn-<Z_PK>` (last column of `transactions`, NULL on register rows; `transactionRows` is positional against the DDL), splits `split-<entry Z_PK>` through the same `mapSplits` path as register entries — Z_PK is unique across ZTRANSACTION entities (SCENARIO-01a, N-1). 01b's pairing and splits-sum coverage depend on entries flowing through `mapSplits`.
- Row iff investment amount != 0, keyed on amount not action; reinvest_dividend (amount 0) and share-only actions get no row. SCENARIO-02's tick rides on this (N-1/N-3 (b)).
- Cash row date = `investment_transactions.date` (posted else entered); `PostedDate` set when a posted day exists; payee nil.
- Unmapped `ZRECONCILESTATUS` refuses a sync only for investment rows with amount != 0, via the shared status helper and `reasonTransactionStatus` (no new copy). Probe: 0 new refusals on the real file.
- `store_info.format_version` is 8 (`FormatVersion`).

## Left unbuilt
- No-entry investment transaction's NULL-category split — SCENARIO-01b. Interim deviation: a non-zero entry-less investment transaction gets its row and no split, so the splits-sum check fails the sync (loud). Probe found 0 on the real file.
- Transfer-target investment entries reach `pairTransfers` unchanged via `mapSplits` but are unpinned — SCENARIO-01b.
- `duplicateQuery`/`unlinkedTransferQuery` `investment_transaction_id IS NULL` predicates and findings Long — SCENARIO-01b / 05 (N-4).
- `v_balances_daily`, the conventions sentences for it, `v_net_worth`, `v_account_balances`; cashflow/spend/findings/holdings Long; SKILL.md; PRD L169 — SCENARIO-06/09/07/18.

## Traps
- `mapSplits` must run after `mapInvestmentTransactions` (importer.go); otherwise investment entries silently take the skip path and every cash row fails the splits check.
- Investment cash rows are appended to `transactions` before `pairTransfers` (it builds `accountOf` from it) — moving them later breaks pairing.
- cmd fixtures add investment entries with no `CategoryTag`, so `uncategorized` findings appear in cmd goldens; 01b re-pins findings goldens again.
- `offenders.firstError` sorts by `lessOffender`; tied offenders matter in refusal tests.

## Open debts
- Conventions text (`internal/report/sql_conventions.go`, mirrored in `internal/cli/sql_test.go`, `cmd/quarry/run_shared_documents_test.go`, `schema.md`): ruled sentence ends "...security and shares:" and the old "Their amount is..." follows with a capital T after the colon — for the final product-vision pass (SCENARIO-01a).
- `Test_run_sync_imports_investment_transactions_with_named_actions` dropped its v_spending/v_cash_flow assertions; SCENARIO-03 must re-cover cash flow of investment rows, categorised and uncategorised (SCENARIO-01a).
- 01b inherits: entry-less non-zero investment fails splits-sum (interim deviation); transfer-target entries unpinned.
- Checkpoint 01a MINOR: buy row's absence from `v_cash_flow` unpinned (`cmd/quarry/run_investment_cash_test.go`, main test) — SCENARIO-03 pins it.
- Checkpoint 01a MINOR: cash-row `Currency` unpinned for a USD brokerage (`internal/importer/investment_cash_test.go`, fields test) — add a USD case on next importer touch (01b).
- Checkpoint 01a MINOR: comment budgets — `mapInvestmentTransactions` doc (`internal/importer/investments.go`) 3 lines, `mapSplits` doc (`internal/importer/splits.go:19-26`) ~10 lines with bad reflow; trim to 1-2 (01b touches both).
- Checkpoint 01a NIT: two entries on one investment transaction not pinned (01b).
