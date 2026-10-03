# Review Report — phase3b-analysis-tools, gate round 1

### Target
755313041c4a0622ea6057f92869e11f00b266d2..HEAD (84 non-doc files)

### Triggered reviewers
arch-reviewer, correctness-reviewer, refactor-advisor (cmd/**, internal/** Go), test-reviewer (**/*_test.go). Skipped: api-reviewer (no HTTP), pipeline-reviewer (no .claude/** change).

Coverage: 0 uncovered added lines; 7 declared unreachable (all upheld by correctness-reviewer). Mutation sample (run alone, before reviewers): 20 of 55 — 19 killed, 1 survived: internal/mcp/window.go:16 `return err`→`return nil`. Tests: 1535 (+110). Correctness ran an 80-call Rule-4 probe on a real store (distinctive ZZQX values through every failure class for all four new tools): no caller/config/data text on stderr.

### BLOCKER
none

### MAJOR
- test-reviewer: internal/mcp/window.go:16 / internal/mcp/window_internal_test.go — `windowRefusal` passthrough unpinned; surviving mutant returns nil → handler returns nil document + nil error (silent success) for any future non-WindowError. Fix: row asserting `windowRefusal(errX)` returns errX (as accountRefusal's passthrough table).

### MINOR (folded into fix pass 1)
- arch + refactor: `parseSpendingGroup`/`parseCashFlowPeriod` (internal/mcp/spending.go:50-73, cash_flow.go:46-54) duplicate cli `parseSpendGrouping`/`parseCashFlowPeriod` — add `store.ParseSpendingGroup`/`store.ParseCashFlowPeriod (value, bool)` beside String(); cli/mcp map the bool to their own error; pin "errors on a miss, never falls back".
- refactor: "today is read once" comment ×4 (spending.go:19-24, cash_flow.go:17-19, recurring_charges.go:16-18, anomalies.go:13-15) → once on Server.now.
- refactor: "unprefixed and never nil… word names the command…" ×4 on document *Warnings (warnings.go:25-65) → once (doc.go or leftOutWarnings).
- refactor + test: unreachable reasons — window.go:17 restates report internals ("report constructs only the kinds above"); window fall-throughs name the exhaustive switch / exhaustive linter; spending.go:25-29, cash_flow.go:~24 caller-side "see parseX" → state mechanism on the line.
- test: internal/mcp/cash_flow_test.go:~160 cap test stub Currency is Native — set money.CAD, pin "is listed in USD, not converted to CAD".
- test: cmd/quarry/run_mcp_spending_test.go config test `Contains "currency":"USD"` ambiguous — decode, assert doc.Currency.
- test: `if c.wantWarning != ""` loop branch (run_mcp_spending_test.go:~72, run_mcp_cash_flow_test.go:~51, run_mcp_recurring_charges_test.go:~58) — move to own test with exact warnings list.
- test: account-refusal tests ×4 (spending, cash_flow, recurring_charges, anomalies) — hoist to one table over tool name (STATE trigger met).
- test: internal/mcp/window_internal_test.go:1 header stale ("charge kinds reach no tool yet") — state combinatorial reason only.
- test: anomalies description thresholds hardcoded (cmd/quarry/run_mcp_descriptions_test.go) — drift test formatting thresholds from report.Anomaly* constants and maxRows.

### MINOR (deferred → STATE)
- arch: document owns noRatesWarning copy (note only).
- refactor: RefusalError/WindowError exported fields allow invalid states (no outside constructors today); WindowNotADate is the zero kind.
- test: redundant `absent` NotContains loops after exact stderr Equal (4 files + 3 tests).
- test: package-level var test data in document/warnings_composers_test.go:14-28.
- test: Test_run_mcp_describes_all_eight_tools pins three behaviours in two subtests.

### NIT (deferred)
refactor: cli json_* pass-through wrappers; spendingInput/cashFlowInput + recurringInput/anomaliesInput identical; resolveCurrency/errUnknownBy live in spending.go, twin const inconsistency; duplicated per-row ctx block in duckdb.go/table.go; money.NativeOf as method; *ByDesc naming + inaccurate comment (tools.go:112-135); `*r.Key` invariant uncommented (document/spending.go:~92). test: InDelta(...,0); reads_back tests overlap; inToolWords test placement; logLine store rows duplicate query refusal test.

### Strengths
Byte-for-byte CLI goldens under a fixed clock; runBothSurfaces CLI-vs-MCP equality with mapped warnings; scriptedContext making cancel races deterministic; error-on-miss making the schema default load-bearing; real-store Rule-4 probe clean.

### Verdict: BLOCKED (1 MAJOR)
