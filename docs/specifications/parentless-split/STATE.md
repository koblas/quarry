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
- None.
