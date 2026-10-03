# Review Report — phase3b-analysis-tools, gate round 2 (re-gate after fix pass 1, d3a7d2e)

### Target
a11fb7f9783b935ccc4906b6e142c085559d642a..HEAD

### Triggered reviewers
test-reviewer (blocked R1), correctness-reviewer (production parse logic moved), refactor-advisor (findings claimed closed). Skipped: arch-reviewer (its only MINOR, store.Parse* placement, judged by refactor-advisor as correct; no import or wiring change).

Mutation sample (alone): 6 of 6 killed; 266s. Coverage: 0 uncovered; 2 declared unreachable (internal/mcp/cash_flow.go:28, spending.go:31). Tests (--base a11fb7f): 698 (+6).

### BLOCKER / MAJOR
none — R1 MAJOR (windowRefusal passthrough) closed and mutation-proven.

### MINOR
none

### NIT
- refactor: internal/report/document/warnings.go:27,41,51,61 function docs no longer restate `word`/never-nil (now in doc.go only); internal/report/document/doc.go:5-11 "never nil" stated twice.
- test: cmd/quarry/run_mcp_anomalies_test.go has no exact left-out warnings test (covered by CLI-vs-MCP equality).
- correctness: internal/mcp/window.go:40 unreachable reason says the exhaustive linter "fails the build" — it fails the lint gate.

### Verdict: PASS WITH FOLLOW-UPS
