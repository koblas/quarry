# Retro proposals: phase3b-analysis-tools

pipeline-reviewer retro, 2026-10-03. Proposals only; `.claude/` changes go in their own change.
Evidence: METRICS.md, REVIEW-01/02, STATE.md, phase3a RETRO.md, `.claude/briefs/build.md` (*Build cadence*, *Planning*), `proof.md`, developer Agent.md:70-77.

## Where the tokens went
Subagent total 13.1M IE (3a: 15.8M). Orchestrator upper bound 6.3M (3a: 4.7M).
- Three costliest units: SCENARIO-01 1,819k (14%), SCENARIO-10 1,701k (13%), SCENARIO-09 1,658k (13%). The "-" unit (scoping + gate fix) is 3,182k (24%).
- Fix passes: gate-fix 886k (7%) + checkpoint-fix 397k (3%) = 1,283k, about 10% (3a: 8%). Gate-fix is down from 1,011k; checkpoint-fix is up (2 passes, 3a 348k).
- Build 49%, developer 59% (28 runs, median 227k). Cost is context re-read per run, as in 3a.

## 3a lessons applied ad hoc: effect
The 3a retro proposals are NOT in `.claude/` (grep for load-generator, outcome table, cross-rule, `--baseline-timeout`, comment budget finds nothing). Applying them in prompts gave:
- Rule-4 / data-flow: 0 findings at gate R1 (3a: 1 MAJOR, which cost the 1,011k gate-fix and a second round). Correctness ran an 80-call real-store probe, clean. Supports 3a change 2 (outcome table + cross-rule check at scoping).
- Load incidents: none; mutation sample ran alone (R1 943s for 20 of 55; R2 266s). 3a's R1 sample was 901s after two aborted attempts, so no retry cost here. Supports 3a changes 3 and 4.
- Doc-comment MINORs still recurred: 9 comment/unreachable-reason MINORs at R1, 4 NITs at R2, and checkpoint MINORs at S01-S04. 3a change 1 is still unapplied and still the largest source of cheap-fold churn.
- Gate R1 was still BLOCKED by 1 MAJOR, a test that cannot fail (below). Fix pass was 886k.

## Late findings that belong to an earlier stage
- **Gate R1 MAJOR: `windowRefusal` passthrough unpinned** (window.go:16, mutant `return err`→`return nil` survived; shipped by S03). Same class as the two checkpoint BLOCKs/MAJORs: S09 (silent parse fallback let a deleted schema `Default` pass; warnings non-empty pin; cap order) and S10 (cap-line-last ordering unpinned). Three instances in one feature of "test cannot go red" for an arm the plan did not list: a fallback/default arm, a passthrough arm, a position/ordering claim. *Planning* has per-arm pins for sort tiers and SQL clauses but no line for these three shapes. S03's own checkpoint did not catch it; the mutation sample did, at the final gate.
- **Pre-existing duckdb `QueryRows`/`QueryTable` lost-cancel flake (37/50)**, found only during a V run and fixed standalone (2482e84). A shared adapter's flake surfaced because a new scenario's cancel test reached it. STATE.md notes `platform/sqlite` `QueryRows` has the same unchecked loop, unowned. No earlier stage could cheaply have found it; the gap is that a flaky test is not treated as a lead.
- **Red test that panicked aborted the cmd/quarry test binary (S09).** developer Agent.md:70-72 forbids `panic` in stubs, but a nil/zero document dereferenced by the acceptance test itself panics, which kills every test in the package and hides the red.
- **Renaming a ticked test needs a spec repoint** (S13, and the S04 name kept through the hoist at gate fix 1). Caught by `spec-check.py`, so cheap. It is a trap that constrains refactors, but is already enforced mechanically.
- **Left unowned**: `platform/sqlite` loop, phase3a debts, "David's" NIT, and the PRD currency decision. STATE.md `## Open debts` says "unowned — dies unless re-opened" on most entries. 3a change 6 (assign owners) is still unapplied.

## Ranked rule changes
1. **`.claude/briefs/build.md` → *Planning*: add one line, "arms a test can silently miss".** Every default-by-omission or fallback, every error-passthrough arm of a wrapper/mapper (`windowRefusal`, `accountRefusal`), and every "X is last/first" ordering claim gets a pin that is red when the arm is removed, and the architect names it in `Mutation checks:` (`proof.md` cites). For a default: the parser reports a miss, the caller decides. For ordering: the test puts a competing line after X.
   - Evidence: S09 checkpoint BLOCK, S10 checkpoint BLOCK, gate R1 MAJOR; roughly 397k checkpoint-fix + 886k gate-fix = 1,283k, all in this class. Cite from `test-reviewer` → *Checkpoint mode* rather than restating.
2. **Apply 3a change 1 (comment-budget gate in developer Verify; ideally a script mode).** Cite `go-code.md`; do not copy numbers. Still the most frequent finding in both features.
3. **Apply 3a change 2 (outcome table + cross-rule check at scoping, `intent-and-goal.md` and product-vision Phase 1).** 3b evidence: 0 Rule-4 findings with it, 1 MAJOR and a 1,011k fix without it.
4. **Apply 3a changes 3 and 4** (no-load-generator line in `.claude/briefs/review.md`; `run-reviewers.md` runs the mutation sample alone, `--baseline-timeout` flag). 3b had no incident when done by hand, so this just makes it default.
5. **`.claude/agents/developer/Agent.md` → Acceptance (red): "a red that panics is not red."** If the acceptance test can dereference a zero value from a stub, the stub or the test guards it (assert on the error/`nil` first with `require`), so the binary is not aborted. Narrow: one sentence next to the existing stub rule.
6. **`.claude/briefs/proof.md` or `agent-briefs.md` → Reporting: a flaky test is a finding.** A test seen failing at any rate on re-run (the 37/50 case) is reported with its rate, not re-run until green; the fix is a bug-fix pass (test-first mandatory). Put it in `proof.md` (developer and test reviewers read it), not the always-loaded file.
7. **Low priority**: STATE.md open-debt ownership (3a change 6). Two features in a row ended with long "unowned" lists.

## Notes
- No contradiction or dangling-pointer defects looked for in `.claude/` this run (no `.claude/**` change in the feature); I did not re-verify 3a's Notes.
- Not done: I did not read SCENARIO-*.md plans or the checkpoint outputs, so the claim that the three misses were "not in the plan" rests on REVIEW/METRICS/STATE text only.
