# Review Report — phase3a-mcp-core, gate round 1

### Target
ff46a3cf760dd07c3b6033173c863e5bc6b60cb2..HEAD (changed files; pipeline mode)

### Triggered reviewers
- arch-reviewer, correctness-reviewer, refactor-advisor: cmd/**, internal/** Go files
- test-reviewer: **/*_test.go

### Skipped reviewers
- api-reviewer: no HTTP files
- pipeline-reviewer: no .claude/** changes

Coverage gate: 0 uncovered added lines; 2 declared unreachable (internal/mcp/query_refusal.go:38, internal/mcp/result.go:43 — both judged sound by correctness-reviewer).
Mutation sample: 20 sampled of 76 — 20 killed, 0 survived; 901s. (Two earlier attempts aborted at baseline: internal/report timeout; cmd/quarry baseline fail under load — cause not attributed.)

### BLOCKER
none

### MAJOR
- correctness-reviewer: internal/mcp/result.go:58 (`errorLog`) — stderr log line copies the isError text, so DuckDB reasons put row values and SQL identifiers on stderr (reproduced: `query failed: Conversion Error: Could not convert string 'Chequing' to INT32 ...`). Breaks Rule 4. Fix per spec §2.1a (gate R1 copy ruling, commit 677fd6a): errorLog stops reading result.Content; handler records the classifier's log line per call (ctx slot, not `_meta`); delete errorText; SDK argument refusals (no recorded line) log `refused the call's arguments; details went to the client only`; update S05 pins (run_mcp_query_test.go:155-156,170) and add the CAST-of-stored-value row asserting no value/SQL on stderr; data_quality config refusal log line; fallthrough line.

### MINOR
- correctness-reviewer: internal/mcp/server.go:89 — stdin EOF straight after requests drops pending responses (SDK v1.8.0, no drain). No contract break → STATE.md open debt.
- test-reviewer: cmd/quarry/run_mcp_timeout_test.go:35-43 — 1 s margin; callCtx expires at same 2 s so elapsed assert is dead; derive callCtx from 30 s ctx, keep generous bound (timeout+10s).
- test-reviewer: cmd/quarry/run_mcp_cancel_test.go:21,46 — cancelBound 3 s absolute; raise to ≥10 s.
- test-reviewer: internal/mcp/log_internal_test.go:86 — mutex pinned only under -race; use overlap-detecting fake writer.
- test-reviewer: logic in test bodies — cmd/quarry/run_mcp_query_test.go:117, internal/mcp/data_quality_test.go:357 — move into case struct.
- test-reviewer: internal/mcp/timeout_test.go:84-133 — real timers (synctest optional).
- refactor-advisor: internal/cli/status.go:58-66 — StatusIgnore called three times; restore single-path shape.
- refactor-advisor: internal/mcp/server.go:35-60 — missing WithReport/WithConfig fails at first call; validate in NewServer/Serve.
- refactor-advisor: internal/store/duckstore/schema_read.go Schema — compose method (four near-identical phases).
- refactor-advisor: same deadline/cancel fact stated on four symbols (refusal.go:28-29, store/query.go InterruptedBy, duckstore/query.go ~37, mcp/query_refusal.go) — keep on InterruptedBy.
- refactor-advisor: store.Relation.Kind bare string — typed RelationKind + tier method.
- refactor-advisor: doc budgets — report/describe_schema.go DescribeSchema, duckstore/schema_read.go Schema, mcp/server.go Serve, mcp/data_quality.go dataQuality/capFindings/capItems, cli/run.go MCPServeFunc.

### NIT
- correctness-reviewer: internal/mcp/data_quality.go:30 — no clamp on in.Limit (schema only guard).
- correctness-reviewer: internal/mcp/server.go:66 WithTimeout sub-second renders "0 seconds".
- test-reviewer: internal/mcp/data_quality_test.go 526 lines — optional split.
- refactor-advisor: tools.go stoppedLine param shadows `tool`; status enum hardcoded vs finding constants; cli/mcp.go Long duplicates maxRows/tool names and RunE nil serve; "mcp" literal thrice; result.go unreachable comment names unexported document.sqlCell; document/sql.go sqlFloat double-value arg; data_quality capFindings/capItems rebuild FindingsListing.

### Strengths
- Real-store acceptance tests with CLI `--json` oracles and raw-frame key-order checks.
- Deadline vs cancel told apart against the real driver's chain shape.
- mcp/cli peer structure and document placement clean (arch PASS).

### Verdict: BLOCKED
1 MAJOR (correctness: stderr data leak).
