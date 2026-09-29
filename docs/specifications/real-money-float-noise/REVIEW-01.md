# Review Report — round 01

### Target
changed files, range 25c8c4646909..HEAD (money.go, offenders.go, splits.go, transactions.go, money_internal_test.go, statements_test.go, transactions_test.go)

### Triggered reviewers
- arch-reviewer, correctness-reviewer, refactor-advisor: internal/importer/*.go
- test-reviewer: internal/importer/*_test.go

### Skipped reviewers
- api-reviewer: no HTTP files
- pipeline-reviewer: no .claude/** files

Coverage gate: 0 uncovered added lines. Mutation sample: 9 sampled — 8 killed, 1 survived (equivalent: money.go:73 fast-path bypass), 110s.

### BLOCKER
none

### MAJOR
none

### MINOR
- correctness-reviewer: internal/importer/money.go:105-110 — `big.Rat.SetString` failure returns `0, moneyOK`; comment claims only huge negative exponent can fail, but SetString also fails on >1e6 fraction digits (`"1."+1_000_001 zeros` → 0 cents). Unreachable from SQLite REAL rendering (≤17 sig digits). Fix: decide below-tolerance from digits (exponent ≥ len(intPart)+6 → 0) before big.Rat; make `!ok` return moneyNotANumber with `// unreachable:`.
- test-reviewer: money_internal_test.go — nothing pins the ≤2-decimal fast path stays allocation-free (mutation survivor money.go:73). Fix: `testing.AllocsPerRun` == 0 for `parseMoney("real","12.34")`.
- refactor-advisor: money.go:~72 `realCents` does grammar + compute, >30 lines — Compose method (`splitRealText`).
- refactor-advisor: money.go:~110 REAL bound stated three ways (intVal, dollars Rat, cents) — one helper.
- refactor-advisor: money.go:~127 `big.NewRat(100, …)` magic 100 / per-call allocs — package-level rats.
- refactor-advisor: money.go:~46 `parseMoney` doc lost which fault is returned — restore one sentence.
- refactor-advisor: money.go:~99 `!ok` → zero pattern (same as correctness MINOR).

### NIT
- refactor-advisor: offenders.go:20-22, splits.go:21-25, transactions.go:85-87 — "beyond the snap tolerance" repeated; define once.

### Strengths
- Exact `big.Rat` comparison with boundary rows on both sides of the tolerance for both signs; acceptance tests assert through `Server.Import` with SQLite-rendered text.

### Verdict: PASS WITH FOLLOW-UPS
