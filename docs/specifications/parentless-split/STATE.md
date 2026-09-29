# parentless-split — current state

Scenarios complete: SCENARIO-01..05 (02-05 folded into 01). Last updated by SCENARIO-01.

## Binding decisions
- One skip point per ownership edge: `mapSplits` skips on NULL parent or `txns` miss; `mapTransactions` on NULL account or `accounts` miss. The `txns`/`accounts` maps hold only imported rows, so deleted, dangling, Smart/Investment and offender-excluded owners are indistinguishable by design — do not reintroduce an "exists" survey to tell them apart (SCENARIO-01, R2).
- `Rows`/counts are stored rows only; no skip counter, no new output, `--json` field or exit code (SCENARIO-01, spec Out of Scope, R7).
- Skipping never touches the parent transaction: V1 (`checkSplits`, `validate.go`) still sees a split-less transaction and fails, so a real transaction that lost its entry is refused (SCENARIO-01, R5).
- Transaction offenders stay in `off`; entries under an offending transaction still skip silently (SCENARIO-01).
- Reason-10 forms `a transaction (source id N) has no account` and `a split (source id N) has no transaction` no longer exist in code or in phase1 spec (SCENARIO-01, R6).

## Left unbuilt
- Any output about skipped rows, `--json` field, Phase 2 cleanup finding — unowned, spec Out of Scope (SCENARIO-01).
- Smart/Investment parent handling unchanged (`not_imported` count stays in `surveyTransactions`) (SCENARIO-01).

## Traps
- `surveyTransactions` still needs `ent`, `account`, `deletionCount` for the investment count; dropping the `hasInvestment`/`accountImported` guard breaks `not_imported_test.go` (SCENARIO-01).
- `internal/snapshot` and `cmd/quarry/run_bundle_refusals_test.go` `has no accounts` is the phase0 empty-ZACCOUNT path, unrelated to this rule (SCENARIO-01).
- v9fixture: `EntryRow.Parent: 0` / `TransactionRow.Account: 0` write NULL; a nonexistent reference needs an explicit id such as 999 (SCENARIO-01).
- Narrow-loop `-run` regex `Import|Offender|FirstError` is case-sensitive and misses `Test_import_*`; run `go test ./internal/importer/` (SCENARIO-01).

## Open debts
- internal/importer/transfers.go:65-68 — one-way transfer link on a skipped entry vanishes silently; not observed in user data (1347 links, all bidirectional) (REVIEW-01 correctness MINOR). Final product-vision: ship as debt, not R4; if ever handled, a Phase 2 "transfer-looking entry with no pair" cleanup finding, not import-time copy.
- internal/importer/transactions_test.go:329-340 — fold duplicate NULL-parent test into `..._whatever_its_amount` (REVIEW-01).
- coverage_test.go:95-107, dangling_references_test.go:71-83, transactions_test.go:196-210 — add kept-row control arms (REVIEW-01).
- internal/importer/transactions.go:70 — stale error text "read transaction ids"; transactions.go:102 split account guard into two steps (REVIEW-01).
- splits.go:19-24, transactions.go:76-82, accounts.go:48-52 — trim unexported doc comments to budget (REVIEW-01).
- internal/importer/transactions_test.go:196-210 (S3), :330-340 (S1), dangling_references_test.go (S2) — acceptance tests discard `result`; assert `result.NotImported` is zero and validation not failed, so "nothing printed" (R7) is pinned (checkpoint MINOR).
- internal/importer/splits.go:45, transactions.go:102 — `!parent.Valid` / `!account.Valid` redundant with map miss (equivalent mutants) (checkpoint NIT).
