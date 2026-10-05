---
id: SCENARIO-01a
status: done
---

# SCENARIO-01a: Investment cash joins transactions

Cadence: code-first — nothing on the mandatory set (no write-safety guard, no atomic adapter)
Acceptance test: `cmd/quarry/run_investment_cash_test.go` `Test_run_sync_gives_each_investment_transaction_that_moves_cash_a_row_in_transactions`
Acceptance test (SCENARIO-02, folded): `cmd/quarry/run_investment_cash_test.go` `Test_run_sync_gives_a_reinvested_dividend_no_row_and_no_income`
Narrow loop: `go test ./internal/importer/ ./internal/store/duckstore/` then `go test ./cmd/quarry/ -run 'InvestmentCash|Investment|Transfers|Status|Share|Commission|Unused|ImportRuns|SharedDocuments|Skill|StoreInfo|Pre4a'`
Mutation checks: amount ≠ 0 guard in the investment cash-row builder (`investments.go`) → `Test_import_gives_no_cash_row_to_an_investment_transaction_with_amount_zero` (B1)
Runs: A (1-2) | B1 (3-5) | B2 (6) | V (7-8)
Size: OWNS A RUN — 4 batches, 1 feature package (importer) + store adapter column

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_investment_cash_test.go` (new) — sync a v9fixture brokerage with: buy and dividend (one entry each; dividend's entry in an income category, buy's in a system category), add_shares, remove_shares, split, reinvest_dividend (amount 0, each with a 0.00 entry as the real file has, so a deleted guard fails at the no-row assertion, not the splits check), plus one register row in the same account. Main test asserts via SQL: one `transactions` row per non-zero investment (`txn-<pk>`, `investment_transaction_id = itxn-<pk>`, NULL on the register row), one split per entry with entry's category and amount, account cash = sum(transactions.amount), no row for the share-only actions, `store_info.format_version` 8. Folded test: reinvest has no row and `v_cash_flow` income is the dividend only
- [x] Step 2: `internal/store/store.go:87-105` `Transaction.InvestmentTransactionID *string`; `internal/store/duckstore/schema.go:44-57` column `investment_transaction_id VARCHAR` (last column); `duckstore.go:597-610` `transactionRows` writes it — stub so the test fails at its row assertion, not at a SQL error

### Build
- [x] Step 3: fixture entries first (harmless today: entries under investments take the skip path). Helper in `internal/importer/helpers_test.go` adding the one entry; give it to the success-path (`importInvestments`) entry-less fixtures in `investments_test.go` (18 callers), `lots_test.go:269-333`, `import_runs_test.go:26`. Refusal-path tests (`importInvestmentsRefused`) unchanged. Package stays green
- [x] Step 4: importer read + build. `investments.go:46-55` `investmentsQuery` adds `ZRECONCILESTATUS` and `COALESCE(ZEXCLUDEFROMREPORTS,0)<>0`; `:94-103` `investmentRow`, `:115-121` scan; extract the status switch at `transactions.go:130-141` into one helper both builders call (same `reasonTransactionStatus` refusal, no new copy), applied only to rows with amount ≠ 0; `:151-203` `buildInvestmentTransaction` (or a sibling) also yields, when `Amount != 0`, a `store.Transaction` (id `txn-<pk>`, date = the investment's date, `PostedDate` = posted day when present as register rows do, status, excluded, memo, payee nil, `InvestmentTransactionID`) and a `txnRef`; `:107-135` `mapInvestmentTransactions` returns them. Tests in `investments_test.go`: every row field; status 0/1/2/NULL and an unmapped status refused on a non-zero row, not on a zero-amount one; excluded on/off/NULL; memo empty vs set; posted-else-entered date; `Test_import_gives_no_cash_row_to_an_investment_transaction_with_amount_zero` — one row per action (add, remove, split, reinvest, and a 0.00 buy) against a non-zero control
- [x] Step 5: `importer.go:85-102,125-151` — move `mapSplits`/`readCategoryRefs`/`mapSplitTags` after `mapInvestmentTransactions`, pass investment refs into `mapSplits` alongside `txnRefs`, append cash rows to `transactions` **before** `pairTransfers` (`:135` builds `accountOf` from it; `validate`'s index too), counts follow; `splits.go:19-25,52` docs (the skip now covers Smart and zero-amount investments only). Tests: entry → split `split-<entryPK>` with category/amount/memo; entry under a zero-amount investment stays in `ReferencedCategoryIDs`; entry-less non-zero investment fails the splits check (`store.ErrValidationFailed`, `Splits.Mismatched` names it). Premise flips: `category_refs_test.go:33-37` case re-pointed at a zero-amount investment; `transfers_test.go:177-205` drop the `investmentLinkLeg` arm (the entry is now a split it can pair with); `import_runs_test.go:40` counts
- [x] Step 6: store + cmd + copy — narrow loop here is `go test ./cmd/quarry/` unfiltered (Findings lines shift in sync blocks a `-run` filter misses). `duckstore.go:24-25` `FormatVersion = 8`; `duckstore_test.go:166-189` sibling `Test_replace_stores_the_investment_transaction_id` (set vs NULL). cmd re-pins: `run_transfers_test.go:146-180` delete (coverage moved to Step 1); `run_investments_test.go:177-234` re-point `syncThenReport` at share-only and zero-amount actions (premise "investments leave spend unchanged" now false for Groceries-categorized cash), `:356`; `run_status_shares_test.go:26`; `run_shared_documents_test.go:132` literal 7→8; whatever else the package run reports. Copy (*Changes to existing surfaces*, verbatim): `internal/report/sql_conventions.go:19-21` investment sentence; hand copies `internal/cli/sql_test.go:213`, `cmd/quarry/run_shared_documents_test.go:354`; regenerate `plugin/skills/quarry/references/schema.md` with `go test ./cmd/quarry -run Test_skill_schema_reference_matches_the_committed_file -update`; `docs/initial-prd.md:125` drop "(from Phase 4c)"

### Sweep
- [x] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `InvestmentTransactionID`, the status helper, the changed `mapInvestmentTransactions`/`mapSplits` contracts

### Verify
- [x] Step 8: full verification + `spec-check.py phase4c-networth` → tick SCENARIO-01a and SCENARIO-02 ("delivered by SCENARIO-01a") with their acceptance tests; write `STATE.md` (first one for this feature)

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Investment cash row id `txn-<Z_PK>`, `investment_transaction_id = itxn-<Z_PK>`, splits `split-<entry Z_PK>` through the same `mapSplits` path as register entries — N-1 (Z_PK unique across ZTRANSACTION entities); 01b's pairing and splits-sum coverage depend on entries flowing through `mapSplits`.
- Row iff investment amount ≠ 0, keyed on amount, not action — N-1/N-3 (b); SCENARIO-02's tick rides on it.
- Cash row date = `investment_transactions.date` (posted else entered), not the register's entered-first rule.
- `investment_transaction_id` is the last column of `transactions`; `transactionRows` is positional against the DDL.

**Left unbuilt** — named so nobody assumes it exists:
- No-entry investment transaction's NULL-category split — 01b. **Deviation:** until then a non-zero entry-less investment transaction gets its row and no split, so the existing splits-sum check fails the sync (loud, never silent); the probe found 0 on the real file.
- Transfer-target investment entries: reach `pairTransfers` unchanged via `mapSplits` (N-1's ruled path) but are unpinned — 01b. **Deviation**, named for the orchestrator.
- Unmapped `ZRECONCILESTATUS` refusal applies to investment rows with amount ≠ 0 only (new way a file can stop syncing; the probe saw 0 reconciled, other codes unreported). **Deviation** — no new copy, reuses `reasonTransactionStatus`.
- **Orchestrator ruling 2026-10-04:** all three deviations accepted. Probe: investment rows with amount ≠ 0 carry `ZRECONCILESTATUS` NULL 149 / 0 283 / 1 888, all codes register rows already use (NULL 152, 0, 1, 2), so deviation 3 adds no refusal on the real file.
- `duplicateQuery`/`unlinkedTransferQuery` `investment_transaction_id IS NULL` predicates and findings Long — 01b (N-4).
- `v_balances_daily`, the conventions sentences for it/`v_net_worth`/`v_account_balances` — 06/09/07.

**Traps** — things that look right and are not:
- `mapSplits` ran before investments were mapped (`importer.go:92` vs `:125`); without the move, investment entries silently take the skip path and every row fails the splits check.
- cmd fixtures add investment entries with no `CategoryTag`, so `uncategorized` (and, before N-4, possibly duplicate/unlinked-transfer) findings appear in cmd goldens now; 01b re-pins findings goldens again.
- `offenders.firstError` sorts by `lessOffender`; moving `mapSplits` changes insertion order only, but check refusal tests with tied offenders.

## Phase report

**Run V (steps 7-8) — verified, committed.** `go build ./...` ok; `golangci-lint run ./...` 0 issues; covered full suite `go test -count=1 -coverpkg=./... ./...` rc=0; `uncovered-diff.py --profile ... da6065f`: 0 uncovered added lines; `go test -race` on importer, store, duckstore, report, cli, cmd/quarry green; `spec-check.py phase4c-networth` OK.
- test-stats (--base da6065f --changed): cmd/quarry 724 (+1), internal/cli 479 (+0), internal/importer 254 (+10), internal/report 386 (+0), internal/store/duckstore 616 (+1); TOTAL 2459 (+12), tempdir 863 (+3), disk 777 (+3).
- specification.md: SCENARIO-01a and SCENARIO-02 ticked (02 delivered by 01a). STATE.md written. Nothing left for this scenario.
