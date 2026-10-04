---
id: SCENARIO-02
status: open
---

# SCENARIO-02: Sync imports investment transactions with named actions

Cadence: code-first (no write-safety guard, atomic adapter or bug fix touched)
Acceptance test: `cmd/quarry/run_investments_test.go` `Test_run_sync_imports_investment_transactions_with_named_actions`
Narrow loop: `go test ./internal/importer/ ./internal/store/duckstore/ ./internal/quicken/v9/v9fixture/ && go test ./cmd/quarry/ -run 'Investment|Spend|Cashflow|ImportRuns|Transfers|UnusedCategory'`
Mutation checks: `Z_ENT = ?` in the new investment-transactions query → `Test_import_leaves_out_an_investment_row_of_another_entity` · `Z_ENT = ?` in the new positions query → `Test_import_leaves_out_a_position_row_of_another_entity` · `securities.go:19` `Z_ENT = ?` → existing `securities_test.go:92` `Test_import_leaves_out_a_security_row_of_another_entity` (STATE debt) · `transactions.go:47` cash `Z_ENT = ?` → `Test_run_spend_and_cashflow_are_unchanged_by_investment_transactions` · share snap-tolerance comparison → `Test_parse_shares_snaps_residue_within_tolerance_and_refuses_beyond`
Runs: A (1-2) | B1 (3) | B2 (4) | B3 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches, 1 feature package (importer) plus its duckstore adapter and v9fixture, as S01. Multi-package plan; developer `sonnet` is fine.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `internal/quicken/v9/v9fixture/builder.go:15-23,61-74,189-230,239-246,261-268,449-455,530-578` + `builder_test.go:124-151` — `EntPosition` (49), `PositionRow{Entity, Account, Security, Deleted}` + `Builder.Position`; `TransactionRow` gains `Type *int64`, `Units`, `Position`, `Numerator`, `Denominator`, `Commission` (NULL when zero/""); `Builder.InvestmentTransaction` defaulting `Entity` to InvestmentTransaction; Seed inserts ZPOSITION rows and the new ZTRANSACTION columns; `"Position"` in the Z_PRIMARYKEY loop, `maxPKForEntity` and `WithEntity` doc; builder test reads each back (typeof real/integer as `:146-147`)
- [x] Step 2: `cmd/quarry/run_investments_test.go` (after `:88`) `Test_run_sync_imports_investment_transactions_with_named_actions` — SCENARIO-02 Given verbatim: one row per I4-2 code (13) in a brokerage account with positions, cash-only dividend (no position), zero-unit `add_shares`/`remove_shares`, 1:12 split; asserts action, shares, amount, commission, security_id (NULL on cash-only), split columns only on split; `transactions`, `v_spending`, `v_cash_flow` hold only the cash rows. Stub: `investment_transactions` DDL in `internal/store/duckstore/schema.go:74-87` so red is the row assertion, not a missing table; `query_test.go:30-34` relation list and the `schema.md` golden go red with it until Steps 3 and 7 — expected

### Build
- [x] Step 3: `internal/store/store.go:138-180,192-201,234-245`, `duckstore/schema.go:92-124`, `duckstore/duckstore.go:51-55,467-477,593-631`, `duckstore/history.go:33-44,308-345`, `duckstore/status.go:16-31,65-70` — `store.InvestmentTransaction` (shares/split sides int64 millionths, amount/commission cents, nullable as I4-1), `Rows.InvestmentTransactions`, `Counts.InvestmentTransactions`; converter with one nullable-decimal helper reusing `priceWidth/priceScale` and `moneyWidth/moneyScale`; append before `import_runs`; `investment_transactions_rows` nullable BIGINT after `prices_rows` via `importRunRows`, `optionalRunColumns`, `carriedRun`, COALESCE in `statusQuery`. Tests: `duckstore_test.go:52-67,112-116` round trip (NULL and non-NULL of every nullable column), `:352-363` fault-table row `investment_transactions`, converter fault test (shares outside DECIMAL(18,6)), `history_test.go:100-111` NULL carry, `status_test.go:73-84`, `query_test.go:30-34` relation list
- [ ] Step 4: `internal/importer/entities.go:15-24`, new `internal/importer/investments.go`, `importer.go:107-145`, `import_runs_test.go:26,51`, `import_faults_test.go:69-97,160-188` — `positionEntity` optional; `mapPositions` (non-deleted, own `Z_ENT`, account imported → security PK); `mapInvestmentTransactions` (own `Z_ENT`, non-deleted, dates `CAST … AS REAL`, account skip first, date posted-else-entered, I4-2 action map with NULL-code and unmapped-code refusals in S.5 copy plus their offender class, account currency, memo as `transactions.memo`, commission NULL/0/snaps-to-0.00 → NULL, security via position → imported security else NULL, split columns only when action is `split`); wire Rows/Counts/import run. New `investments_test.go`: 13-code table, unmapped 5/14/21, NULL code, NULL/unmapped code in deleted or excluded account imports nothing and refuses nothing (control: live account refuses), posted vs entered arms, deleted txn, another-entity txn and position (each with positive control row), position deleted / in deleted account / security deleted → NULL security on zero shares (control: live → `sec-N`), no Position entity, no InvestmentTransaction entity, commission NULL/0/residue/1.50, split columns NULL on a non-split row carrying numerator; fault rows `read investment transactions` (match `ZUNITS`) and `read positions` (match `FROM ZPOSITION`), each fixture gaining one investment row and one position. Convert every existing investment fixture site to `b.InvestmentTransaction` with a mapped `Type`: `cmd/quarry/run_sync_unused_category_test.go:27`, `run_import_runs_test.go:36`, `run_transfers_test.go:154,158`, `internal/importer/category_refs_test.go:35,102`, `transfers_test.go:190`, `import_runs_test.go:26`, `not_imported_test.go:40,59,97`
- [ ] Step 5: `internal/importer/price.go:8-42` (twin), `money.go:13-28`, `reasons.go:118-124`, `offenders.go:16-28`, `investments.go` — `parseShares` (exact on decimal text, snap tolerance 1e-9 named constant, bound 10^12 shares, sign kept); refusals in S.5 copy: undated (checked before every other refusal), shares beyond 6 / not a number / too large, amount ×3, commission ×3, NULL amount (copy pending — see Handoff); placeholders (plain error naming source id, see Handoff) for non-zero shares with no resolvable security and for a split side NULL, zero or unreadable. Tests: `price_internal_test.go`-style `Test_parse_shares_snaps_residue_within_tolerance_and_refuses_beyond` (1e-9 in, just above out, 6 vs 7 dp, 10^12−0.000001 vs 10^12, negative, integer, exponent, text/blob); Import-level one row per S.5 reason verbatim; `Test_import_fails_on_shares_without_a_security` with zero/NULL-shares control importing NULL security; `Test_import_fails_on_a_split_with_an_unreadable_ratio` (NULL, 0)
- [ ] Step 6: `cmd/quarry/run_investments_test.go` `Test_run_spend_and_cashflow_are_unchanged_by_investment_transactions` — two fixtures differing only in investment rows (brokerage account in both); the investment row's entry uses a category the cash rows use, dated in the same window; `spend` and `cashflow` stdout (text and `--json`) byte-equal across arms, compared raw (a per-build field that differs → stop and report, do not strip); the "with" arm asserts `investment_transactions` non-empty. `v_cash_flow` (`schema.go:194-217`) filters only `in_reports`/`linked_tracking`, so a leaked brokerage entry does reach the output

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `Import` (`importer.go:42-48`), `Rows`, `Counts`, new symbols; trim `securitiesQuery`/`quotesQuery`/`isDecimalText` docs if touched (STATE debt); exact-count bumps where a fixture's investment row now counts; regenerate `schema.md` (`go test ./cmd/quarry/ -run Test_skill_schema_reference_matches_the_committed_file -update`)

### Verify
- [ ] Step 8: full verification + `spec-check.py phase4a-investments` → tick SCENARIO-02 with its acceptance test; STATE.md drops the securities `Z_ENT` debt once its mutation reddened the existing test

## Handoff

**Before dispatching B3** — copy ruling needed (CLAUDE.md *Rule copy*): NULL `ZAMOUNT` on an investment row has no S.5 line. Proposed: `an investment transaction on <date> in "<account>" has no amount` (cash `reasonTransactionNoAmount` with "investment " added). Not ruled; developer must not invent it.

**Binding decisions** — a later scenario must not contradict these without saying so:
- `mapPositions` returns position Z_PK → (account, security) for non-deleted positions of the Position entity in imported accounts — S04 sums lots over this same map, so a position in a skipped account never forms a holding
- `split_new_shares`/`split_old_shares` are DECIMAL(18,6) through `parseShares`, set only on `action = 'split'` — S04's split multiply reads them
- Shares are int64 millionths with the price width/scale; snap tolerance 1e-9 on decimal text — S04's gate tolerance 0.000001 assumes exact stored shares
- Date is posted-else-entered (I4-1), the reverse of cash transactions — S04 orders a holding's walk by it
- Account skip precedes every refusal; undated precedes every other refusal, so no "no date" variants of the share/amount copy exist
- `investment_transactions_rows` is a nullable BIGINT after `prices_rows`; `FormatVersion` stays 6
- `Builder.InvestmentTransaction` is the only way tests add investment rows

**Left unbuilt** — named so nobody assumes it exists:
- Ruled copy for split ratio and shares-without-security — SCENARIO-03 turns the placeholders into offenders and repoints `Test_import_fails_on_a_split_with_an_unreadable_ratio`, `Test_import_fails_on_shares_without_a_security` (do not add twins); it also rules whether a non-number/beyond-scale split side uses the ratio copy
- Security with no name still fails via `rows.Scan` (S01 trap) — SCENARIO-03
- `ZLOT`, Rows clause, `--json` keys, Shares line — SCENARIO-04; `surveyTransactions`/`store.NotImported` — SCENARIO-09

**Traps** — things that look right and are not:
- `not_imported` still counts investment rows that now also import — expected until S09; do not "fix"
- `TransactionRow.Entity` defaults to CashFlowTransaction: an investment row added with `b.Transaction` silently becomes cash
- Fault table row `ZTRANSACTION` matches `ZPOSTEDDATE`, which the investment query also contains — correct only while the cash query runs first; new rows match `ZUNITS` / `FROM ZPOSITION`
- Existing investment fixture rows have NULL `ZTYPE` and now refuse; `transactions_test.go:252` stays green only because its row has no account

## Phase report

Run B1 (step 3) done, code-first. Store side green on `go test ./internal/store/...` and `golangci-lint run ./internal/store/...` (0 issues); `importer`/`quicken` unaffected.

Changed:
- `internal/store/store.go`: `InvestmentTransaction` (Shares/Commission/SplitNewShares/SplitOldShares `*int64`, Amount `int64`, SecurityID/Memo `*string`), `Rows.InvestmentTransactions`, `Counts.InvestmentTransactions`.
- `internal/store/duckstore/schema.go`: `investment_transactions_rows BIGINT` after `prices_rows` (the `investment_transactions` DDL stub is unchanged and matches the converter).
- `internal/store/duckstore/duckstore.go`: `investmentTransactionRows` + `decimalCell` (NULL for nil, wraps `<column>: ...`; errors joined under `investment transaction <id>: `), appended before `import_runs`; `importRunRows` appends the count.
- `history.go` (`optionalRunColumns`, `carriedRun` targets/values), `status.go` (COALESCE + Counts scan).
- Tests: `duckstore_test.go` (`minimalRows` gains `inv-1` all-set and `inv-2` all-NULL rows, count 20; round-trip x2, largest value, five out-of-range rows, `investment_transactions` in the append-fault table), `history_test.go` NULL carry, `status_test.go` NULL read, `query_test.go` relation list.

Still red by design: acceptance test (importer does not emit rows yet — B2) and `cmd/quarry` `Test_skill_schema_reference_matches_the_committed_file` (Step 7 regenerates `schema.md`).

Next run must not: re-add the DDL stub, change `FormatVersion`, or treat the schema.md golden as a regression. B2 sets `Counts.InvestmentTransactions` from `len(Rows.InvestmentTransactions)` where the importer builds Counts (`importer.go:107-145`). No mutation checks belong to B1 (the plan's mutation line names only importer queries, `transactions.go`, share snapping).
