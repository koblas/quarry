# Retro: phase3d-skill

pipeline-reviewer, 2026-10-03. Read-only.

## Where the tokens went
- Subagents: 9.36M IE (3c: 13.83M). Orchestrator upper bound: 2.92M.
- Developer: 55% (21 runs).
- Fix passes: 1,529k, about 16% (3c: about 5.4%).
  - 3 gate-fix passes, which hit the cap.
  - 3 checkpoint-fix passes.
- Gate rounds:
  - R1: BLOCKED, 3 MAJORs.
  - R2: PASS WITH FOLLOW-UPS.
  - Final product-vision pass: SHIP WITH CHANGES, 3 MAJORs.
  - R3: BLOCKED, 2 MAJORs.
  - R4: PASS WITH FOLLOW-UPS.
- STATE.md open debts are now about 25 lines, nearly all unowned.

## Findings caught late, by class
- **A. Ruled copy asserting behaviour that was never checked against the code.**
  - The README privacy line, caught twice.
  - The SKILL.md null renders.
  - The recipe currency not tied to the reporting currency.
  - Byte pins confirmed these strings were present but not that they were true.
  - The R1 NIT "needs a copy ruling" was deferred, and came back as a MAJOR in the final pass.
- **B. Input-domain arm, or sibling parity, missing from the plan.**
  - The recipe `ELSE` arm. This is 3b #1, still unapplied.
  - The partial-period caveat that `cash-flow.md` already had. This is 3c #2, extended to prose.
- **C. Hand-kept registry not tied to disk.** The recipe maps.

## Ranked rule changes (each ships as its own `.claude/` change)
1. **product-vision Phase 1 and build.md → *Planning*.** A ruled sentence that asserts behaviour ("only", "never", "nothing", a null render) cites the `file:line` that makes it true. The architect lists a verification step for each.
2. **review.md severity, with the CLAUDE.md "Rule copy" rule.** A finding that "needs a copy ruling" is at least MAJOR and is ruled in that round.
3. **build.md → *Planning*.** Carry 3b #1 over to SQL: an out-of-domain row for every CASE or enumerated param. Every mirrored set names its disk-derivation test.
4. **test-reviewer Checkpoint mode, for static content.** Check that sibling files agree on the same rule, and that each pinned claim is true.
5. **Carry-overs.** 3a #1 (comment budget), 3a #6 (an owner or expiry for each open debt), 3c #2 and #3.
