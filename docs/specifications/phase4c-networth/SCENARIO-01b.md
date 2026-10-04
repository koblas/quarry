---
id: SCENARIO-01b
status: open
---

# SCENARIO-01b: Investment cash rows pair transfers and keep entry-less transactions (absorbs SCENARIO-05)

Cadence: code-first (no bug fix, write-safety guard or atomic adapter; N-4 predicates are read-side SQL)
Acceptance test: `cmd/quarry/run_investment_cash_test.go` `Test_run_sync_pairs_an_investment_transfer_entry_and_gives_an_entry_less_investment_one_uncategorized_split`
Acceptance test (SCENARIO-05, folded): `cmd/quarry/run_findings_investment_cash_test.go` `Test_run_findings_leaves_investment_cash_rows_out_of_duplicate_and_unlinked_transfer`
Narrow loop: `go test ./internal/importer/ -run 'Investment|Entry|Transfer'`; `go test ./internal/store/duckstore/ -run 'duplicate|unlinked'`; `go test ./internal/cli/ -run 'Findings'`; after batch 3 also `go test ./cmd/quarry/ -run 'Findings|Investment|Status|Sync|Share|Commission|Plugin|Import'`
Mutation checks: synthetic split id and source id derivation (use `split-<Z_PK>`/`+Z_PK`) → `Test_import_gives_an_entry_less_investment_transaction_a_split_that_collides_with_no_entry_split`; entry-less split limited to investment cash rows (apply to every transaction) → `Test_import_still_fails_validation_for_a_register_transaction_with_no_entry`; `b.investment_transaction_id IS NULL` in `duplicateQuery` → its register-then-investment arm; `a.investment_transaction_id IS NULL` in `unlinkedTransferQuery` → its investment-then-register arm
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches, 2 packages (importer; store/duckstore) plus a `internal/cli` Long line; sizing pass accepted this. Absorbs 05.

Surveyed (existing, not re-planned): `mapSplits` already routes cash-row entries and keeps each entry's `transferLink` (`splits.go:34-90`); `pairTransfers` takes any split (`transfers.go:23`); `checkSplits` (`validate.go:155`) needs `counts > 0` per transaction. cmd fixtures' `invest` closures (`run_investments_test.go:109,334,453`) already carry uncategorised entries since 01a, so the uncategorised premise is spent. The 01b shifts are (a) N-4 dropping `duplicate`/`unlinked-transfer` findings where a cmd fixture has same-amount investment rows within 3 days, (b) the findings help (`internal/cli/findings_test.go:42-85`). The `cmd/quarry` narrow loop after batch 3 detects (a); findings text, `--json` and `--csv` all read the one `findings` table, so format cells are n/a: the predicate is in the detector.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_investment_cash_test.go` (new test after `:~125`) — sync a brokerage dividend whose entry links a chequing register entry (numeric `Transfer` = counterpart `QuickenID`, opposite signs) plus a misc_income (code 12) with no entry; assert: one paired `transfers` row, both splits' `transfer_account_id` set, neither pair split in `v_cash_flow`, the entry-less row's one NULL-category split (id `split-itxn-<pk>`) is in `v_cash_flow` as income, and exactly one `uncategorized` finding (control: the pair is not counted). Fails now at exit code (splits-sum refusal).
- [x] Step 2: `cmd/quarry/run_findings_investment_cash_test.go` (new) — two same-amount dividends in one brokerage, a sell with a chequing register row of the opposite amount within 3 days and no link; control arms: a register-only same-amount pair in chequing and a register-only unlinked pair; every dividend and the sell carry a categorised entry (no `uncategorized`/`unused-category` noise); assert `findings --type duplicate --json` and `--type unlinked-transfer --json` each list exactly the control pair's finding. Fails now at the assertion (investment pairs also listed), not at sync.

### Build
- [x] Step 3: batch 1, entry-less split. `splits.go` (new `addEntrylessSplits` beside `mapSplits`, `:19-95`), `importer.go:122-126` call between `mapSplits` and `readCategoryRefs`, taking the cash rows (not `transactions`) and appending one zero `transferLink` per split so `links[i]` stays aligned. One NULL-category split, amount = the row's amount, no memo, for each investment cash row with no split. Id `split-itxn-<Z_PK>` and SourceID `-<Z_PK>` (see Handoff). Tests (`investment_cash_test.go`): rewrite `:215-231` (interim failure) to assert the split (id, SourceID, nil category, amount, validation passes); `Test_import_gives_an_entry_less_investment_transaction_a_split_that_collides_with_no_entry_split` (investment txn pk N with no entry beside a register entry whose Z_PK is N, `require.Equal` on the two pks as precondition: ids and source ids unique over `fake.Rows.Splits`); `Test_import_still_fails_validation_for_a_register_transaction_with_no_entry` (control, `SplitsTotal` 0); entry-less amount-0 row gets no row and no split; USD brokerage cash row currency `USD` (debt: `:28-43` fields test, new account row, not `newBrokerage`). Doc trims in the same batch: `investments.go:108-110` `mapInvestmentTransactions`, `splits.go:19-26` `mapSplits` each to 1-2 lines, no reflow leftovers; new func doc 1-2 lines.
- [x] Step 4: batch 2, pins on unchanged code (green on arrival; say so, mutation not applicable). `investment_cash_test.go` (reuse `transferLeg` helper shape, `transfers_test.go:15-21`): transfer-target entries as table rows — numeric link to a chequing register entry (paired, `TransferAccountID` both ways, `Validation.Transfers.Paired` 1, no splits-sum failure); investment to investment across two brokerages; CAD brokerage to USD chequing (`CrossCurrency`); name-form link to `"Chequing"` (one-sided, `OtherAccountID` set); transfer entry whose amount differs from the transaction's (`Validation.Splits.Mismatched` 1). Two-entry NIT: one investment transaction with two entries (7.00 + 5.00 on 12.00) gives two splits `split-<e1>`, `split-<e2>`, no synthetic split, sum validates.
- [x] Step 5: batch 3, N-4 predicates. `internal/store/duckstore/findings.go:15-19` `duplicateQuery` and `:22-32` `unlinkedTransferQuery`: `investment_transaction_id IS NULL` on both aliases (WHERE, not JOIN ON, so the `ON` shapes stay). Tests `findings_duplicate_test.go` and `findings_unlinked_test.go`: rows by `InvestmentTransactionID` on (investment, investment), (register lower id, investment higher id), (investment lower id, register higher id), each differing from its register-register control in that one variable; control still flagged. Existing fault tests (`findings_test.go:109-113`) stay green through `export_test.go:27-31`.
- [x] Step 6: batch 4, findings Long. `internal/cli/findings.go:46-48` insert after the unused-category entry and its blank line (`findingTypesInHelp` stops at that blank, so the paragraph is not parsed as a type), `duplicate and unlinked-transfer compare register entries only, not buys, sells, dividends or other investment transactions.` wrapped at the Long's width; pin verbatim in `findings_test.go:42-85` (`Test_findings_help_says_what_findings_lists_and_how_to_ignore_one`, wrapped text).

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; docs on new symbols (`go doc ./internal/importer`); re-pin any `cmd/quarry` golden the synthetic split or N-4 moved, listing each `file:line` in the phase report.

### Verify
- [ ] Step 8: full verification + `spec-check.py phase4c-networth`; tick SCENARIO-01b and SCENARIO-05 (05 line: `delivered by SCENARIO-01b`, its acceptance test last); close 01b-tagged Open debts in STATE.md.

## Handoff

**Binding decisions:**
- Entry-less non-zero investment transaction gets one synthetic split: id `split-itxn-<txn Z_PK>`, SourceID `-<txn Z_PK>`, NULL category, no memo. Entry ids are `split-<digits>` and entry Z_PKs are positive, but ZCASHFLOWTRANSACTIONENTRY and ZTRANSACTION Z_PKs are separate counters (`v9fixture` `nextPKFor`, `builder.go:284`, counts per table; no code parses split ids) so `split-<txn Z_PK>` can equal a real entry's id. Prefix and negative source id cannot; both derive only from the source Z_PK, so a re-sync reproduces them. `splits.source_id` is documented nowhere as a Z_PK (`schema.md`, `store.go:109`), so a negative value breaks no ruled copy. Source ids stay unique, so `describeOneSided`/`pairTransfers` maps keyed by SourceID cannot confuse them (a synthetic split has no link, never one-sided).
- Synthetic splits cover investment cash rows only; a register transaction with no entry still fails the splits-sum check.
- The N-4 predicates sit in the store adapter queries, not the importer; `Rows` counts keep counting table rows.

**Left unbuilt:**
- `plugin/skills/quarry/references/findings.md` mention of investment rows — not in the spec's copy table; product-vision final pass.
- `v_balances_daily`, `v_net_worth`, cashflow/spend/holdings Long, SKILL.md — SCENARIO-03/06/07/09/10/17.
- Buy row absent from `v_cash_flow`, and cash flow of investment rows — SCENARIO-03 (open debts).

**Traps:**
- A synthetic split is added after `mapSplits` but before `off.firstError()` (`importer.go:136`): an entry refused into `off` also leaves its transaction split-less, so the synthetic split is harmless only because the refusal follows. Do not move the refusal.
- `links[i]` belongs to `splits[i]`: append to both together or `pairTransfers` pairs the wrong legs.
- The `uncategorized` finding reads `v_cash_flow`, which drops transfer legs; an uncategorised transfer entry never raises it (the acceptance test's exact count of 1 relies on that).

## Phase report

Run B2 (steps 5-6) done. Green: `go test` on `internal/store/duckstore`, `internal/cli`, `internal/importer` (whole packages), the `cmd/quarry` narrow loop (`Findings|Investment|Status|Sync|Share|Commission|Plugin|Import`), lint `0 issues`. Both acceptance tests now pass (01b's and 05's, `-v` confirmed).

Files:
- `internal/store/duckstore/findings.go:13-21,26-37` — `duplicateQuery` and `unlinkedTransferQuery` gain `AND a.investment_transaction_id IS NULL AND b.investment_transaction_id IS NULL` (WHERE, `ON` shapes untouched); docs say "register transactions (not investment cash rows)".
- `internal/store/duckstore/findings_duplicate_test.go` — helpers `investmentCash`, `identity`; `Test_replace_does_not_flag_a_duplicate_when_either_transaction_is_an_investment_cash_row` (3 arms). The control is the existing same-day register-register row (`Test_replace_flags_two_same_amount_...`, identical amounts/dates).
- `internal/store/duckstore/findings_unlinked_test.go` — `Test_replace_does_not_flag_an_unlinked_transfer_when_either_transaction_is_an_investment_cash_row` (3 arms; control: existing same-day row).
- `internal/cli/findings.go:50-51` and `findings_test.go` (`Test_findings_help_says_what_findings_lists_and_how_to_ignore_one` long const) — spec sentence verbatim, wrapped at 72, between the unused-category entry and "To keep a finding".

Mutations (all restored byte-identical), each reddened exactly its own subtest:
- duplicate drop `b.` predicate: `.../the_lower_id_a_register_row,_the_higher_id_an_investment_cash_row`
- duplicate drop `a.`: `.../the_lower_id_an_investment_cash_row,_the_higher_id_a_register_row`
- unlinked drop `b.`: lower-register/higher-investment subtest; drop `a.`: lower-investment/higher-register subtest.

Re-pins from the synthetic split / N-4 in `cmd/quarry`: none moved (narrow loop green unchanged, no golden edited).

Not run: step 7 sweep beyond `go build` + lint, full suite, `test-stats.py`, spec tick, STATE.md, debt closure (run V). Findings help Long also appears nowhere else pinned (cmd loop green).
