# Review Report — gate round 2 (re-gate after fix pass 1)

### Target
`3c6b459..d381494` — fix pass 1 (F1 60b0081, feb4005; F2 d381494), 34 files.

### Triggered reviewers
- correctness-reviewer (blocked in round 1; production Go changed).
- test-reviewer (blocked in round 1; tests changed).

### Skipped reviewers
- arch-reviewer: passed in round 1; the fix changed no imports, packages or wiring.
- refactor-advisor: its round-1 MINOR items are deferred, not claimed closed.

### Gate data
- `go test` rc=0; `uncovered-diff.py` 0 since 3c6b459; lint 0; spec-check OK.
- test-stats TOTAL 2774 (+34).
- mutation-sample: 7 of 7 sampled — 7 killed, 0 survived, 661 s.

### BLOCKER
None.

### MAJOR
None after the orchestrator ruling below.

### MINOR
- **test** `internal/report/document/holdings_left_out_test.go:143-146` ("a priced CAD holding in a EUR account is not a missing rate") pins silence for a CAD or USD holding in an account whose currency is neither CAD nor USD. The holding is left out of the balance and no warning is printed. **Orchestrator ruling:** the importer refuses accounts outside CAD and USD (`internal/importer/accounts.go:27`), so no synced store can contain one, and the reviewer's own rule downgrades the finding to MINOR. Spec note added: accounts in other currencies are out of scope. Follow-up: rename or re-comment the row so it does not read as intended behaviour (STATE Open debts).

### NIT
- **test** `holdings_left_out_test.go:243-349`: helpers sit below the tests that use them. Move them up beside the other helpers.

### Prior findings
- **correctness**: the BLOCKER (kind-6 warning), the MAJOR (`Store.Accounts` HUGEINT), the history caption guard and the single as-of are all closed.
- **test**:
  - closed: negative investment cash; sibling readers (anomalies, recurring, `spend --by payee`); CLI↔MCP parity; sync twice; kind-6 pins; past-64-bit Accounts arm.
  - recorded, needing no test: the reinvest wording (spec only).

### Verdict: PASS WITH FOLLOW-UPS
