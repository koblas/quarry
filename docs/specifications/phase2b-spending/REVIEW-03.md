# Review Report — REVIEW-03

### Target
Post-Gate addition SCENARIO-27 (28, 29 folded; linked account tracking), range `f66509262aa3..3bb355a`. The rest of the branch passed REVIEW-02.

Coverage gate: `uncovered-diff: 0 uncovered added line(s) in 0 run(s) since f66509262aa3`. test-stats TOTAL 587 (+18). Mutation sample: `4 sampled of 4 candidates — 4 killed, 0 survived; 81s`.

### Triggered reviewers
- arch-reviewer, correctness-reviewer, refactor-advisor: `cmd/**/*.go`, `internal/**/*.go`
- test-reviewer: `**/*_test.go`

### Skipped reviewers
- api-reviewer: no HTTP surface
- pipeline-reviewer: no `.claude/**` change

### BLOCKER
none

### MAJOR
none

### MINOR
- test-reviewer — `cmd/quarry/run_cashflow_linked_test.go:157-160` never asserts empty stderr; edge row "linked account, no --account → no warning" unpinned at cmd boundary. Fix: `assert.Empty(t, stderr.String())`.
- test-reviewer — `internal/cli/spend_account_test.go:89`, `spend_empty_test.go:121-129`: no `--json` `warnings[]` case carrying W3 or a W3/W2 argv-order mix (spend, cashflow).
- test-reviewer — `internal/cli/spend_account_test.go:146`: refusal-with-W2 test has no W3 twin.
- refactor-advisor — `internal/cli/render_accounts.go:59` `accountStatus` takes four adjacent bools; take the account.
- refactor-advisor — "left out" rule in three places (`reportedAccount` SQL, `Account.LeftOutOfReports`, `leftOutWarnings` switch); cross-reference comments or gate the switch on `LeftOutOfReports()`.
- refactor-advisor — `internal/importer/accounts.go:50-52` NULL defaults restated (same site as the checkpoint debt).

### NIT
- correctness-reviewer — `internal/cli/cashflow.go:100` `cashFlowWarnings` doc still says "left out of reports"; now W2 or W3.
- refactor-advisor — `internal/cli/spend.go:83` same stale phrase; `internal/store/store.go:26-29` `LeftOutOfReports` doc 3 → 2 lines; `internal/cli/json_accounts.go:37` initializer on its own line.
- test-reviewer — `internal/importer/accounts_test.go:741` flag independence pinned one way only; `cmd/quarry/run_accounts_json_test.go:76` redundant order `Contains`.

### Could not check (correctness-reviewer)
- Refusal text/exit code for a format-3 store synced before `linked_tracking` (P2b-2a accepts: unshipped format, re-sync).

### Strengths
- One SQL owner (`reportedAccount`) used by the view and the range query; range tests put far-dated left-out splits beside reported ones so a fragment regression moves the asserted range.
- W3-over-W2 and argv-order interleave pinned directly; importer 1/0/NULL test kills polarity and coalesce mutants.

### Verdict: PASS WITH FOLLOW-UPS
0 BLOCKER, 0 MAJOR, 6 MINOR, 5 NIT. arch PASS; correctness, test, refactor PASS WITH FOLLOW-UPS.
