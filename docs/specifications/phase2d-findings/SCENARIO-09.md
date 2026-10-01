---
id: SCENARIO-09
status: done
---

# SCENARIO-09: two same-amount transactions within 3 days are a possible duplicate (folds SCENARIO-10)

Cadence: code-first (no mandatory test-first item)
Acceptance test: `cmd/quarry/run_sync_duplicates_test.go` `Test_run_sync_records_two_same_amount_transactions_within_three_days_as_a_duplicate`
Acceptance test (SCENARIO-10, folded): `cmd/quarry/run_sync_duplicates_test.go` `Test_run_sync_does_not_flag_two_reconciled_look_alikes_as_a_duplicate`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/finding/ ./cmd/quarry/ -run 'duplicate|findings|replace'`
Mutation checks: `<= MatchDays` bound -> days 3/4 case; both-reconciled `AND` -> `OR` -> one-reconciled-still-flagged case; `a.amount <> 0` -> zero-amount case; `a.account_id = b.account_id` -> other-account case
Runs: L | V
Size: LIGHT — 2 steps, duckstore. `<start>` = 6109164bd3ef65376d89094384621cdf629b9ba9

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_sync_duplicates_test.go` (new) via `syncFindingsBundle`: (09) two -142.17 txns 2026-08-03 / 08-05, one account -> `duplicate:txn-A+txn-B` (lower numeric first), items = both `txn-N` ids; (10, folded) a reconciled/reconciled pair a day apart plus a reconciled/uncleared pair (control) -> only the control pair is a duplicate. Red: no `duplicate` findings exist.

### Build
- [x] Step 2: `internal/finding/finding.go` const `MatchDays = 3`; `duckstore/findings.go` `duplicateQuery` (self-join `transactions`, same account, equal non-zero amount, `abs(date diff) <= ?`, not both `reconciled`, each pair once) + `detectDuplicates` (id `finding.PairID`, 2 items `transaction_id`), first in `detectFindings`. Rule pins in `duckstore/findings_duplicate_test.go`: days 3 in / 4 out, one reconciled flagged / both not, zero amount, other account, other amount, closed + excluded-from-reports + transfer-leg + payee ignored, 3 matches -> 3 pairs, `txn-9+txn-10` numeric order.
- [x] Step 3: `export_test.go` `DuplicateQuery`; add query and scan cases to `Test_replace_keeps_the_previous_store_when_detection_fails` (`detect duplicate findings`).

### Sweep / Verify
- [x] Step 4 (V): lint, full suite, spec tick (09 + folded 10), STATE.md.

## Handoff
`finding.MatchDays` is the shared `findings.match_days` (unlinked-transfer must reuse it). Duplicate items carry `transaction_id` only, lower numeric id first.

## Phase report
Run V done; scenario complete.
- Sweep: `go build ./...` ok, `golangci-lint run ./...` 0 issues (rc 0).
- Verify: full covered suite `go test rc=0`; `uncovered-diff.py` 0 uncovered added lines since 6109164; `go test -race` duckstore + finding ok.
- test-stats vs 6109164: cmd/quarry 295 (+2), internal/store/duckstore 236 (+6), TOTAL 531 (+8).
- No other package's fixture raised a duplicate; the only fixture change is `twoPayeeBundle` (-10.00 -> -11.00), made in run L.
- Spec ticked: 09 and 10 (delivered by SCENARIO-09); `spec-check.py` run; STATE.md rewritten.
