---
id: SCENARIO-01
status: open
---

# SCENARIO-01: A split with no parent transaction is skipped (folds 02-05)

Cadence: code-first (no bug-fix, write-safety or atomicity item: a spec amendment collapsing refusals into the existing silent-skip path)
Acceptance test: `internal/importer/transactions_test.go` `Test_import_skips_a_split_with_no_parent_transaction`
Acceptance test (SCENARIO-02, folded): `internal/importer/dangling_references_test.go` `Test_import_skips_a_split_whose_parent_transaction_does_not_exist`
Acceptance test (SCENARIO-03, folded): `internal/importer/transactions_test.go` `Test_import_skips_a_transaction_with_no_account_and_its_split`
Acceptance test (SCENARIO-04, folded): `internal/importer/transfers_test.go` `Test_import_keeps_a_link_to_a_skipped_split_as_one_sided`
Acceptance test (SCENARIO-05, folded): `internal/importer/validation_test.go` `Test_import_fails_validation_when_a_transaction_lost_its_only_split`
Narrow loop: `go test ./internal/importer/ -run 'Import|Offender|FirstError'`
Mutation checks (`proof.md` -> *Mutation verification*; B run that builds the guard):
- skip also drops the transaction (prune any transaction left with no stored split) -> S5 test (NoError instead of `ErrValidationFailed`)
- NULL-only skip in `mapSplits` (`!parent.Valid` skips, `txns` miss refuses) -> S2 test, plus the Smart-parent guard `dangling_references_test.go:81-113`
- NULL-only skip in `mapTransactions` (`!account.Valid` skips, `accounts` miss refuses) -> nonexistent-account test
- refuse when amount != 0 on a parentless entry (zero-only special case) -> nonzero-amount test
Runs: A (1-2) | B1 (3-4) | B2 (5) | V (6-8)
Size: OWNS A RUN — 2 Build batches, 1 feature package (`internal/importer`); absorbs SCENARIO-02..05 (S4, S5 need no production code)

Surface: none new (spec `## Surface & Copy`). Importer tests assert through `Server.Import` + `fakeStore`; "nothing printed / exit 0" is guarded by the unchanged `cmd/quarry` and `internal/snapshot` regression tests (spec triage list), run in V's full suite.
Surveyed (item 5): callers of `existingAccounts` (`importer.go:64,90` -> `transactions.go:91,113`) and `existingTransactions` (`importer.go:85,94` -> `splits.go:27,50`) are the only readers; `mapAccounts` (`accounts.go:54`) and `surveyTransactions` (`transactions.go:54`) have no test callers (grep, no LSP needed for unexported file-local symbols).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `transactions_test.go:326-334` flip `Test_import_refuses_a_split_with_no_transaction` -> S1 (NULL parent, amount 0: `NoError`, no split stored); `transactions_test.go:196-206` flip `Test_import_refuses_a_transaction_with_no_account` -> S3 (NULL account + one split under it: `NoError`, no transaction, no split); new S4 in `transfers_test.go` beside `:259` (imported leg whose numeric link = `QuickenID` of a parentless entry: `result.Validation.Transfers.OneSided` holds the leg, model on `:189-205`); new S5 in `validation_test.go` (transaction whose only entry has `Parent` = nonexistent id: `ErrValidationFailed`, V1 wording `1 transaction does not equal the sum of its splits` asserted as neighbouring tests do)
- [x] Step 2: `dangling_references_test.go:65-78` flip -> S2 (parent 999, amount 12.34: `NoError`, no split). No new production symbols, so no stubs; run Narrow loop and confirm S1-S5 fail at their assertions (refusal instead of success), not at compile

### Build
- [x] Step 3 (B1, split side): `splits.go:46-57` collapse the `!parent.Valid` and `!existingTransactions` refusals into one silent skip (`!parent.Valid` -> return nil; `txns` miss already skips); drop `existingTransactions` param (`:27`), reword doc `:19-25`; `transactions.go:50-79` `surveyTransactions` drop `existing` map (`:57,66-68,78`) and return `(int, error)`, drop `Z_PK` from `transactionSurveyQuery` (`:50`) and the `pk` scan; `importer.go:85,94` follow; delete `reasonSplitNoTransaction` (`reasons.go:82-84`)
- [x] Step 4 (B1, tests): branch rows and outcomes — (a) NULL parent, amount 0 -> skipped (S1); (b) NULL parent, amount 12.34 -> skipped, `Rows` excludes it (R3; new `Test_import_skips_a_split_with_no_parent_whatever_its_amount`); (c) parent id 999 -> skipped (S2); (d) parent deleted/Smart/excluded -> still silent (existing guards `dangling_references_test.go:81-113`, unchanged); (e) skipped entry with a `split_tags` link -> `NoError`, no split_tags stored (flip `coverage_test.go:97-108`); (f) S4 and S5 green with no further production code; (g) `offenders_internal_test.go:39-49` re-point the literal to a live reason-10 form (e.g. `reasonTransactionNoDate`), same `(and 1,000 more)` assertion
- [ ] Step 5 (B2, transactions side): `transactions.go:109-120` collapse the two `off.add` refusals plus `accounts` miss into one silent skip (`!account.Valid` or `accounts` miss -> return nil); drop `existingAccounts` param (`:91`), reword doc `:81-88`; `accounts.go:48-67,108-110` drop `existing` map and third return, fix doc `:48-53`; `importer.go:64,90` follow; delete `reasonTransactionNoAccount` (`reasons.go:66-68`). Tests: (a) NULL account + entry -> both skipped (S3); (b) account 999 -> skipped (flip `dangling_references_test.go:32-45` to `Test_import_skips_a_transaction_whose_account_does_not_exist`); (c) deleted/excluded account -> still silent (existing, unchanged); (d) valid account -> imported (control, existing)

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; confirm `mapSplits`/`mapTransactions`/`mapAccounts`/`surveyTransactions` docs state the contract only (no history, no spec ids); grep-with-positive-control for `existingAccounts|existingTransactions|reasonSplitNoTransaction|reasonTransactionNoAccount` -> 0 in `internal/`; miss = docs prose (multiline-aware)
- [ ] Step 7: `docs/specifications/phase1-import-store/specification.md` — apply verbatim from this spec's `## Product Verdict`: `:31` P1-5d whole-bullet replacement; `:16` sentence; `:303` remove the two reason-10 forms; paragraph after `:303` (the `Required columns` line, `:309`) drop `imported ZTRANSACTION.ZACCOUNT (account)` and `parent link (transaction)` and swap the "required reference ... treated as NULL" sentence per verdict; add the seven edge-case rows to the table at `:319-333` (Output/Input class/Ruling, `Rows` and `Transfers` grouping as neighbours); phase1 `STATE.md` grep found no restatement of the rule (lines checked: none) — leave alone

### Verify
- [ ] Step 8: full verification per `.claude/rules/agent-briefs.md` (single covered `go test`, `uncovered-diff.py`, `-race` on `./internal/importer/...`, lint, `test-stats.py`); create `docs/specifications/parentless-split/STATE.md`; `spec-check.py parentless-split`; tick SCENARIO-01 with its acceptance test and SCENARIO-02..05 each with "delivered by SCENARIO-01" plus its own acceptance test (test reference last on the line); set `status: done`

## Handoff

**Binding decisions:**
- One collapse point per ownership edge: `mapSplits` skips on NULL parent or `txns` miss; `mapTransactions` on NULL account or `accounts` miss — the `txns`/`accounts` maps hold only imported rows, so deleted, dangling, Smart/Investment and offender-excluded owners are indistinguishable by design (R2). Do not reintroduce an "exists" survey to tell them apart.
- `Rows`/counts stay stored-rows only; no skip counter (spec Out of Scope, R7).
- Transaction offenders stay in `off`; the skip return for an entry under an offending transaction is unchanged.

**Left unbuilt:** any output about skipped rows, `--json` field, Phase 2 cleanup finding (spec Out of Scope); Smart/Investment parent handling unchanged (`not_imported` count stays in `surveyTransactions`).

**Traps:**
- `surveyTransactions` still needs `ent`, `account`, `deletionCount` for the investment count; only `Z_PK`/`existing` go — dropping the `hasInvestment`/`accountImported` guard breaks `not_imported_test.go`.
- `internal/snapshot` and `cmd/quarry/run_bundle_refusals_test.go:151` `has no accounts` is the phase0 ZACCOUNT-empty path, unrelated; leave it.
- Fixture: `EntryRow.Parent: 0` / `TransactionRow.Account: 0` write NULL (`v9fixture/builder.go:65-72`); nonexistent refs need an explicit id such as 999.
- Skipping must not touch the parent transaction: S5 relies on V1 (`checkSplits`, `validate.go:164-166`) still seeing a split-less transaction.

## Phase report

Run B1 (steps 3-4) done, committed. Whole `internal/importer` package green except the one B2 test (below). Narrow loop note: `-run 'Import|Offender|FirstError'` is case-sensitive and misses the lowercase `Test_import_*` names; run `go test ./internal/importer/` (whole package, ~2s) as the narrow loop.

Production: `splits.go` `mapSplits` (silent skip on NULL parent / `txns` miss; `existingTransactions` param + doc reworded); `transactions.go` `surveyTransactions` now `(int, error)`, `Z_PK`/`existing` gone; `importer.go` follows; `reasons.go` `reasonSplitNoTransaction` deleted.

Tests: `coverage_test.go` split_tag link flipped -> `Test_import_skips_a_split_tag_link_whose_split_has_no_parent_transaction` (NoError, no SplitTags); `transactions_test.go` new `Test_import_skips_a_split_with_no_parent_whatever_its_amount` (valid txn+child kept, parentless 12.34 skipped); `offenders_internal_test.go` re-pointed to `reasonTransactionNoDate`. S1, S2, S4, S5 green.

Still red (B2 owns): `Test_import_skips_a_transaction_with_no_account_and_its_split` (S3, refusal from `mapTransactions`). `dangling_references_test.go:32-45` `Test_import_refuses_a_transaction_whose_account_does_not_exist` not yet flipped (currently green, still asserts refusal; goes red only if B2 changes `mapTransactions` -- flip it there).

Mutations (all restored, diff byte-identical): (1) prune transactions with no stored split in importer.go -> `Test_import_fails_validation_when_a_transaction_lost_its_only_split` red ("Expected error with "validation failed" in chain but got nil"), also `_reports_a_transaction_with_no_splits`; (2) txns miss refuses in `mapSplits` -> `Test_import_skips_a_split_whose_parent_transaction_does_not_exist` and `Test_import_skips_an_entry_whose_parent_is_a_smart_transaction` red; (4) refuse parentless entry when amount not 0/0.00 -> `Test_import_skips_a_split_with_no_parent_whatever_its_amount` red (also S4 and split_tag test). Mutation 3 (NULL-only in `mapTransactions`) is B2's.

Not done, do not redo above: steps 5-8; grep sweep for `existingAccounts|reasonTransactionNoAccount` waits for B2.
