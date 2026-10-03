# Retro: phase3c-search

pipeline-reviewer, 2026-10-03. Read-only. Inputs: METRICS.md, REVIEW-01.md, REVIEW-02.md, STATE.md, and the phase3b METRICS.md and RETRO.md.

## Where the tokens went
- Subagents: 13.83M IE (3b: 13.1M, 3a: 15.8M). Orchestrator upper bound: 8.47M.
- Costliest units:
  - SCENARIO-01: 2,715k (20%, including one checkpoint fix pass)
  - "-" (scoping and gate fix): 2,654k (19%)
  - SCENARIO-04: 2,268k (16%)
  - SCENARIO-02: 1,860k (13%)
- Fix passes: gate-fix 533k and checkpoint-fix 213k, 746k in all (about 5.4%; 3b was about 10%).
- Developer: 60% of tokens (25 runs, median 257k, p90 602k). Most of the cost is context re-read per run.
- Gate:
  - R1 BLOCKED by 3 MAJORs. Mutation sample: 20 sampled, 20 killed, 825s.
  - R2 PASS WITH FOLLOW-UPS.

## Findings caught late (all test-quality, none in production behaviour)
1. The MCP `text ""`/`since ""`/`until ""` rows were unpinned (SCENARIO-10). Spec §2.7 rules them, and sibling tools pin them. The planning rule exists; what is missing is a step to copy the sibling tables.
2. A vacuous `assert.Zero` on a zero-initialised sentinel (SCENARIO-04). `proof.md` already covers this. Enforcement missed it at checkpoint.
3. `// unreachable:` fallthroughs on an exported, constructible type (SCENARIO-03 and SCENARIO-10). The grep reason was false. The same shape remains at `internal/mcp/window.go:40`, `internal/mcp/query_refusal.go:49`, `internal/report/window.go:54` and `internal/report/findings.go:175`.

The checkpoint caught a real split-interleave defect at SCENARIO-01, fixed with a `txn_id` tiebreak. The architect should have listed that tiebreak, per *Planning* ("final tiebreak included").

## Earlier proposals
- 3b #1 (pins for fallback, passthrough and ordering arms in `build.md` → *Planning*): not applied.
- 3a change 1 (comment budget gate): not applied. Comment MINORs recur.
- 3a change 6 (owners for open debts): not applied. The "unowned" tails recur.

## Ranked rule changes
1. `.claude/briefs/proof.md` → *Unreachable claims*:
   - An exported or constructible type is not a reason; remove the arm, and under an `exhaustive` switch make the last case the unconditional return.
   - The claimed grep must be positive-controlled.
   - Evidence: R1 MAJOR 3, raised by two reviewers, plus four more instances.
2. `.claude/briefs/build.md` → *Planning*: a new tool or command that joins a family lists its siblings' refusal and edge rows to copy, by `file:line`, including the empty-string rows. Evidence: R1 MAJOR 1.
3. test-reviewer *Checkpoint mode* (or `proof.md` → *Assertions that prove nothing*): a sentinel must be seeded non-zero, and each new test is compared with its nearest sibling's shape. Evidence: R1 MAJOR 2.
4. Carry over 3b #1.
5. Carry over 3a change 1 and change 6.

Changes 1-3 together would have avoided the gate-fix pass and round 2 (about 533k plus the round-2 reviewer cost). Retro findings are MINOR by construction. `.claude/` changes ship as their own change.
