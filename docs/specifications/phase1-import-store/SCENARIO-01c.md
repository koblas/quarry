---
id: SCENARIO-01c
status: done
---

# SCENARIO-01c: sync pairs transfers between the user's accounts

Cadence: code-first
Acceptance test: `cmd/quarry/run_transfers_test.go` `Test_run_pairs_transfers_between_the_users_accounts`
Acceptance test (SCENARIO-07, folded): `internal/importer/transfers_test.go` `Test_import_pairs_cross_currency_and_brokerage_transfers`
Acceptance test (SCENARIO-12, folded): `cmd/quarry/run_transfers_test.go` `Test_run_reports_no_transfers_for_a_file_with_no_transactions`
Acceptance test (SCENARIO-21, folded): `cmd/quarry/run_transfers_test.go` `Test_run_keeps_investment_transactions_out_of_the_cash_transactions_table`
Narrow loop: `go test ./internal/store/... ./internal/importer/... ./internal/cli/... ./cmd/quarry/...`
Mutation checks: transfers kept out of `store.Validation.Failed()` → `Test_import_builds_the_store_with_a_one_sided_transfer`; numeric (not `split-N` string) ordering of the pair's legs → `Test_import_stores_each_split_in_at_most_one_transfer`

Size verdict: **OWNS A RUN.** Spans store, duckstore, importer, cli and cmd tests. Folded 07, 12 and 21 add only tests plus the not_imported count.

**PENDING RULING (orchestrator, before dispatching developer), Interim W2.** Until SCENARIO-08, one-sided legs are stored and counted on the ruled Transfers form (`N paired, M one-sided`). There are no `?` rows, no stderr warning and no `warnings[]` entry. No push may ship in this state. The copy already exists, so only the omission needs ruling. It matters because the real file's 29 name-form legs hit this path on the first sync.
**PENDING RULING (small):** the new stable-ID prefix `xfer-` (P1-7 lists every other prefix).

User-visible contract: `quarry sync` stdout adds a `Transfers` line after `Splits`, in both the success block and the V1 block: `none` / `N paired` / `N paired, M one-sided`. Rows gains a `; N investment transactions not imported` clause, singular at 1 and omitted at 0. stderr and exit codes are unchanged.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_transfers_test.go` (new).
  - `Test_run_pairs_transfers_between_the_users_accounts`: chequing↔savings and chequing↔credit-card pairs, where each leg's `EntryRow.Transfer` is the counterpart's `QuickenID` as text. Assert the full stdout block (Rows `2 transfers`, `Transfers  2 paired`). Assert both `transfers` rows via `stringMap` (`run_import_test.go:25`), plus each leg's `splits.transfer_account_id`. Exit 0.
  - `Test_run_reports_no_transfers_for_a_file_with_no_transactions`: accounts, 1 payee, 2 categories, 1 tag and 0 transactions. Assert Rows `0 transactions, 0 splits, 0 transfers, 1 payee, 2 categories, 1 tag`, Splits `no transactions to check`, Transfers `none`.
  - `Test_run_keeps_investment_transactions_out_of_the_cash_transactions_table`: a brokerage account with 2 `EntInvestmentTransaction` rows (with entries) and a chequing→brokerage contribution pair. Assert only the contribution's two transactions are in `transactions`, and that Rows ends `; 2 investment transactions not imported`.
  - All three fail at their stdout assertion.
- [x] Step 2: `internal/importer/transfers_test.go` (new) `Test_import_pairs_cross_currency_and_brokerage_transfers`. Through `Server.Import` with `fakeStore`, cover a CAD→USD pair and a chequing→brokerage pair. Assert both are paired, the cross-currency row keeps both native split amounts, `Validation.Transfers.CrossCurrency == 1`, and the brokerage leg's transaction is in the brokerage account.
  - Stubs so it compiles: in `internal/store/store.go:83-126`, add `Transfer{ID, FromSplitID, ToSplitID *string, CrossCurrency}`, `Rows.Transfers`, `TransferCheck{Paired, CrossCurrency int; OneSided []OneSidedTransfer}`, `OneSidedTransfer{ID, SourceID, OtherAccount, OtherAccountID *string}`, `Validation.Transfers` and `Result.NotImported{InvestmentTransactions int}`.

### Build
- [x] Step 3: `internal/store/duckstore/schema.go:6-62` `transfers` DDL (id PK, from_split_id NOT NULL, to_split_id NULL, cross_currency NOT NULL); `duckstore.go:81-116` `build` appends it via a new `transferRows`.
  - `duckstore_test.go:22-45` `minimalRows` gains one paired and one one-sided row (NULL `to_split_id`). Assert both in the round-trip at `:48-70`.
  - Add a `transfers` case to the append-fault table at `:126-150`.
- [x] Step 4: `internal/importer/splits.go:11-16,26-92`: `entriesQuery` also reads `ZTRANSFER` and `ZQUICKENID`, and `mapSplits` returns each imported split's link. New `internal/importer/transfers.go` `pairTransfers`:
  - A numeric link means ParseInt succeeds; it is resolved against imported splits' quicken ids. Anything else is a name-form leg.
  - Each imported split lands in at most one row. Legs are ordered by numeric source id; `from` = the lower leg.
  - `cross_currency` comes from the two accounts' currencies.
  - It sets `Split.TransferAccountID` and builds `TransferCheck`.
  - Wire it in `importer.go:208-240` after `mapSplits`, before `validate`. Fill `Rows.Transfers`, `Counts.Transfers` and `Validation.Transfers` on **both** returns (`:232` V1 and `:240` success). `Validation.Failed()` at `store.go:127-129` stays balances+splits only.
  - Tests in `transfers_test.go`:
    - `Test_import_stores_each_split_in_at_most_one_transfer`: asymmetric A→B/B→C, a self-link, and legs with source ids 9 and 10.
    - `Test_import_keeps_a_name_form_leg_as_a_one_sided_transfer`: name matches → lowest-source-id account; no match → nil; a numeric link to a missing, deleted or investment entry → one-sided.
    - `Test_import_builds_the_store_with_a_one_sided_transfer`: `Built` is true and `Replace` is called.
- [x] Step 5: `internal/importer/entities.go:253-291`: resolve `InvestmentTransaction` as an **optional** entity (absent → count 0; do not add it to `requiredEntities`).
  - Widen `transactions.go:50-68` `existingTransactionPKs` (same scan, no new query) to count investment rows that are non-deleted and whose account was imported. Set `Result.NotImported` on both `Import` returns.
  - New `internal/importer/not_imported_test.go`:
    - `Test_import_counts_investment_transactions_not_imported`: excludes a deleted row and a row in a deleted account; investment entries yield no splits.
    - `Test_import_counts_no_investment_transactions_without_the_entity` (`WithoutEntity`).
  - If the query text changes, update the match strings in both fault tables (`import_faults_test.go:63-72,135-144`).
- [x] Step 6: `internal/cli/render.go`.
  - New `transfersPhrase(store.TransferCheck)`.
  - `renderStore` (`:108-115`) and `renderStoreFailure` (`:296-321`) each add a `Transfers` line; V1 always uses the success phrase.
  - `rowsPhrase` (`:189-199`) takes the not-imported count (update both callers) and appends the clause.
  - In `render_internal_test.go`:
    - New `Test_transfersPhrase` (none / 1 paired / 3,112 paired / 3,112 paired, 3 one-sided / 0 paired, 2 one-sided).
    - `Test_rowsPhrase` (`:194-224`) gets clause cases for 0 omitted, 1 singular and 1,605 grouped.
    - `Test_renderStore…` (`:226`) and `Test_renderStoreFailure` (`:409`) gain the Transfers line.

### Sweep
- [x] Step 7: fix what `go build ./... && golangci-lint run ./...` reports.
  - Append the Transfers line to every exact-stdout assertion: `run_validation_test.go:94,166,282,346`, `run_success_test.go:64`, `run_schema_test.go:114`, `run_import_test.go:100` (and its `:40` comment), `render_internal_test.go:241,427`.
  - Rewrite the stale `store.Counts` doc (`store.go:93-94`).
  - Add doc comments on every new symbol.

### Verify
- [x] Step 8: full verification + `.claude/scripts/spec-check.py phase1-import-store` → tick 01c, 07, 12 and 21, each with its acceptance test.

## Handoff

**Binding decisions:**
- Transfer id = `xfer-<Z_PK of the from leg>`, and `from` = the leg with the lower numeric split source id. It is **not** the direction the money moved: Phase 2 readers must use amount signs. Each imported split appears in at most one `transfers` row, or the Appender's duplicate PK surfaces as S3.
- One-sided rows: `to_split_id` NULL, `cross_currency` false. `splits.transfer_account_id` = the counterpart's account for a pair, the name-matched account (lowest source id among same-named accounts) for a name-form leg, and NULL otherwise.
- Rows `N transfers` = stored rows (paired + one-sided). `TransferCheck.CrossCurrency` counts pairs only.
- Transfers never fail validation: `Validation.Failed()` excludes them. No amount check on pairs, because no V1 copy exists for one; adding it is a new failure mode that needs a copy ruling.
- `InvestmentTransaction` is optional (absent → 0). `not_imported` counts non-deleted investment rows in imported accounts only (P1-5d).
- `store.Result.NotImported` and `Validation.Transfers` are populated on the V1 return as well as on success.

**Left unbuilt:**
- `?` rows, the W2 stderr line and `warnings[]`: SCENARIO-08. `TransferCheck.OneSided` is **unsorted** and carries no date, account, payee or amount until 08 adds those fields and sorts once in the core (ruled keys).
- The `other_account_id` for a numeric link to a non-imported entry (investment or deleted): stays NULL. SCENARIO-08 rules whether that shows as `unknown`.
- `--json` `transfers` / `not_imported` / `rows.transfers`: SCENARIO-02 reads the Result fields above. `import_runs`: SCENARIO-08.

**Traps:**
- `v9fixture` writes `ZQUICKENID` raw: an entry without an explicit `QuickenID` stores 0, not NULL, so many entries share id 0. The quickenID→split map needs a deterministic duplicate rule.
- `split-9` vs `split-10` compared as strings inverts the pair order. Compare the int64 source ids.
- An account literally named with digits reads as a numeric link. This is accepted: no ruled copy distinguishes it, and the real file was not probed for such names.
