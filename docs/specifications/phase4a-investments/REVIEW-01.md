# Review Report — phase4a-investments, round 1

### Target
Changed files `58520fc..97ef3f3` (feature branch worktree-phase4a-investments).

### Triggered reviewers
- arch-reviewer, correctness-reviewer, refactor-advisor: `cmd/**/*.go`, `internal/**/*.go`
- test-reviewer: `**/*_test.go`

### Skipped reviewers
- api-reviewer: no HTTP files
- pipeline-reviewer: no `.claude/**` changes

### Gate inputs
- `uncovered-diff: 0 uncovered added line(s) in 0 run(s) since 58520fc`
- test-stats TOTAL 2700 (+153)
- `mutation-sample: 20 sampled of 172 candidates since 58520fc — 20 killed, 0 survived, 0 non-viable, 0 timed out; 0 uncovered lines skipped; 1431s`
- `spec-check.py --run phase4a-investments`: OK

### BLOCKER
none

### MAJOR
1. test-reviewer — `internal/store/duckstore/shares.go:30-36` (`holdingWalkQuery`): I4-5 "All dates count" unpinned; mutant `AND date <= current_date` survives (every fixture dated 2026-03). Fix: `CheckShares` case with a buy dated 2099-01-01 and a pre-2001 row in the same holding, Quicken count including both → Checked 1, 0 mismatches.
2. test-reviewer — `internal/importer/securities.go:57-59`: I4-1 "no refusal for other values" of `ZCURRENCY` unpinned; a CAD/USD whitelist mutant survives. Fix: importer test with a `EUR` security → imports, `Currency == "EUR"`.
3. test-reviewer — `internal/importer/securities.go:50`: whitespace-only `ZNAME` unpinned (S.5 "whitespace follows the account rule"); `TrimSpace` mutant survives. Fix: case with `ZNAME = '  '` → imports with name kept (accounts today accept whitespace names).

### MINOR
- test-reviewer — spec `specification.md` SCENARIO-02 Then ("amount and commission as DECIMAL(18,2)") and S.6 commission row ("snapping to 0.00") stale after SCENARIO-13. (Orchestrator fixes.)
- test-reviewer — S.6 "Holding with lots, no transactions" has no end-to-end `run sync` pin (unit-level pinned). Fix: one cmd case, lot of 3, no transactions → `!` row quarry 0, Quicken 3, difference -3.
- test-reviewer — `cmd/quarry/run_investments_test.go` (480 lines), `internal/importer/investments_test.go` (671 lines) mix behaviours; split by behaviour.
- test-reviewer — acceptance tests `run_investments_test.go:345-374`, `run_share_gate_test.go:78-137`, `run_status_shares_test.go:15-40` hold a text and a `--json` When each; split JSON halves.
- arch-reviewer — `internal/store/duckstore/shares.go:56-130`, `internal/importer/ports.go:26`: share-count rule lives in the adapter; record as intentional (4b reuses via store) or move to a pure walk.
- arch-reviewer — no compile-time `var _ importer.Store = (*duckstore.Store)(nil)` next to `cmd/quarry` wiring.
- correctness-reviewer — `internal/importer/validate.go` `saturatingSub` / `shares.go` `millionthsOf`: a clamped count shows a clamped difference with no marker (gate outcome still correct).
- refactor-advisor — fault→refusal switch written three times (`investments.go` shares/money, `lots.go` `lotUnits`).
- refactor-advisor — `buildInvestmentTransaction` (~55 lines) and `Import` (~100 lines) break Compose method; failure paths return half-built `txn`.
- refactor-advisor — `"split"` action declared as a bare constant in both `internal/importer/investments.go:19` and `internal/store/duckstore/shares.go:19`; drift silently disables split scaling. Define the action vocabulary once in `internal/store`.
- refactor-advisor — `describeShareMismatches` rebuilds account/security maps instead of using `rowIndex`.
- refactor-advisor — round-half-even QuoRem logic twice (`price.go` `roundHalfEven`, `shares.go` `millionthsOf`); scale 1_000_000 under several names.
- refactor-advisor — shares/split columns reuse `priceWidth, priceScale` (`duckstore.go:54-58`); importer `priceScale` used by `sharesFixed`.

### NIT
- refactor-advisor — `err1..err5` + `errors.Join` in `investmentTransactionRows`; bare typeof literals (`"null"`, `"blob"`…); missing docs on `investmentSubject.refuse/day`, row converters; `mapPositions` comment says nil but returns empty map; `sort.Slice` vs `slices.SortFunc`; hard-coded "6 decimal places"; floating block comment in `reasons.go:~123`.
- test-reviewer — `syncThenReport(t, withInvestments bool)` flag argument; byte-identical-store test overlaps the SCENARIO-06 acceptance sentinel.

### Strengths
- Mutation-resistant rounding grids (ties to even, bound edges, signed MinInt64).
- Write-safety guard pinned with a one-variable control arm.
- Sort tie-break tests make each key decisive.
- Dependency rule, thin main, ports placement all clean (arch).

### Verdict: BLOCKED
3 MAJOR (test-reviewer).
