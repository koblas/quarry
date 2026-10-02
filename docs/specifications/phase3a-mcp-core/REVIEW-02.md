# Review Report — phase3a-mcp-core, gate round 2 (re-gate after fix pass 1, 47c8db3)

### Target
de08c8e19653dc10a724db5f00d7031bc7e7add7..HEAD

### Triggered reviewers
- correctness-reviewer (prior MAJOR + production logic), test-reviewer (tests changed), refactor-advisor (prior findings claimed closed)

### Skipped reviewers
- arch-reviewer: PASS in round 1; fix touched no imports, placement or wiring

Coverage gate: 0 uncovered added lines; 1 declared unreachable (internal/mcp/result.go:91).
Mutation sample: 4 sampled of 4 — 4 killed, 0 survived; 214s.

### BLOCKER
none

### MAJOR
none (round-1 MAJOR stderr leak confirmed closed by real-store probes: 23 query casts, SDK refusals, bad configs, bad stores)

### MINOR
- correctness-reviewer: internal/mcp/result.go:55 — every report.RefusalError logged verbatim, but unknownAccountRefusal/ambiguousAccountRefusal (internal/report/refusal.go:63,68) embed caller text; unreachable from the four current tools — must be classified when spending/cash_flow/recurring/anomalies tools land (phase 3b).
- correctness-reviewer: internal/store/duckstore/schema_read.go:57,69,81,89 (status.go:71,92, findings_read.go:88) — statement-time read faults become OpenFaultOther and log DuckDB reason verbatim; §2.1a premise ("before any statement") does not hold; no failure constructible with a quarry-built store — copy-ruling question.
- correctness-reviewer: internal/mcp/server.go:84 — errIncomplete returned after ready() (TTY hint) already called; test-only reachable.
- refactor-advisor: internal/mcp/result.go:54-65,100-116 — logSlot `set` flag → default line.
- refactor-advisor: internal/mcp/query.go:13,19, query_refusal.go:14-15,38-42 — verbatim() at call sites → on sentinel declarations.
- refactor-advisor: internal/mcp/query_refusal.go:25-26, result.go:~113 — 3-line docs on unexported funcs.
- test-reviewer: internal/mcp/query_refusal_test.go:20 — local failedLog duplicates failedLogLine.
- test-reviewer: internal/mcp/result.go:91 — unreachable reason garbled; must name document.sqlFloat NaN/Inf mapping (proof.md).

### NIT
- refactor-advisor: configRefusalLog placement; statusIgnore comment on warning-vs-refusal.
- test-reviewer: log_internal_test.go:60-64 name overclaims; :24 helper branch; overlap test doesn't pin one Write per line / per-call slot; data_quality enum refusal stderr unpinned; query_refusal_test.go:13 "and" name.

### Verdict: PASS WITH FOLLOW-UPS
