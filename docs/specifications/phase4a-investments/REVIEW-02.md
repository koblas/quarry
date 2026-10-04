# Review Report — phase4a-investments, round 2 (re-gate)

### Target
Fix pass `6d349ba..c318043` (findings from `REVIEW-01.md`).

### Triggered reviewers
- test-reviewer: blocked in round 1; the fix added tests.

### Skipped reviewers
- arch-reviewer, correctness-reviewer, refactor-advisor: PASS WITH FOLLOW-UPS in round 1; the fix's production edits are constant renames/splits and a shared `store.Action*` vocabulary with unchanged values (behaviour-neutral). arch's "no compile-time guard" finding was already satisfied at `cmd/quarry/run.go:29`.

### Gate inputs
- uncovered-diff vs 6d349ba: 0 added lines
- test-stats (base 6d349ba): cmd/quarry 679 (+1), importer 243 (+2), duckstore 561 (+1)
- `mutation-sample: 4 sampled of 4 candidates since 6d349ba — 4 killed, 0 survived, 0 non-viable, 0 timed out; 0 uncovered lines skipped; 274s`

### Round-1 findings
- MAJOR 1 walk date arm — closed (`Test_check_shares_counts_rows_of_every_date_including_future_and_before_2001`)
- MAJOR 2 security currency other than CAD/USD — closed (`Test_import_keeps_a_security_currency_other_than_CAD_or_USD`)
- MAJOR 3 whitespace-only security name — closed (`Test_import_keeps_a_whitespace_only_security_name_as_recorded`)
- MINOR S.6 lots without transactions end-to-end — closed (`Test_run_sync_fails_a_holding_with_a_lot_and_no_transactions_against_zero_shares`)
- MINOR stale spec lines — fixed by orchestrator (6d349ba)
- Remaining MINOR/NIT — deferred in STATE.md (gate round 1 deferred list)

### New findings
none

### Verdict: PASS
