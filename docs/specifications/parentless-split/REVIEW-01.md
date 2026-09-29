# Review Report — round 01

### Target
changed files, range 757e287b54a2..HEAD (internal/importer: accounts.go, importer.go, reasons.go, splits.go, transactions.go + 6 test files)

### Triggered reviewers
- arch-reviewer, correctness-reviewer, refactor-advisor: internal/importer/*.go
- test-reviewer: internal/importer/*_test.go

### Skipped reviewers
- api-reviewer: no HTTP files
- pipeline-reviewer: no .claude/** files

Coverage gate: 0 uncovered added lines. Mutation sample: 2 sampled — 2 killed, 31s.

### BLOCKER
none

### MAJOR
none

### MINOR
- correctness-reviewer: internal/importer/transfers.go:65-68 — a transfer link written only on the skipped entry's side vanishes silently (imported leg has NULL ZTRANSFER). Pre-existing for deleted parents; now reachable for NULL/dangling parents and no-account transactions. Orchestrator probe of user snapshot 20260929T154207Z: 1347 linked entries, 0 lacking a back-link — not observed in real data.
- test-reviewer: transactions_test.go:329-340 — `Test_import_skips_a_split_with_no_parent_transaction` duplicates `..._whatever_its_amount`; fold, rename survivor.
- test-reviewer: coverage_test.go:95-107, dangling_references_test.go:71-83, transactions_test.go:196-210 — skip tests assert emptiness only, no kept-row control arm.
- test-reviewer + refactor-advisor + correctness (NIT): transactions.go:70 — stale error text "read transaction ids"; `surveyTransactions` now only counts investments.
- test-reviewer + refactor-advisor (NIT): splits.go:19-24, transactions.go:76-82, accounts.go:48-52 — unexported doc comments over 1-2 line budget.
- refactor-advisor: transactions.go:102 — lookup before `!account.Valid`; split into two steps like splits.go.

### NIT
- refactor-advisor: accounts.go:48-51 — optional trim.

### Strengths
- S5 pins that a skipped split leaves its real parent failing V1; S4 pins the transfer consequence; dangling-account and whatever-amount tests carry kept-row controls. Net code deletion; no dead plumbing left.

### Verdict: PASS WITH FOLLOW-UPS
