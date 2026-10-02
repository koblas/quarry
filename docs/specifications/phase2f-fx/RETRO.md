# Retro: phase2f-fx (pipeline-reviewer, 2026-10-02)

Proposed `.claude/` rule changes, ranked by how much late-caught work each would have prevented (METRICS.md → Caught late). They are not applied yet.

1. **`build.md` → Planning: state the invariant behind any stored "already done" record.** When code records that a range is covered (rate floor, fetched span, watermark, cache) and later skips work because of it, the plan states the invariant that makes a wrong record impossible. It also requires an exhaustive small-grid test over Need × Have, with each side as its own row, plus a two-operation check on the real store. This would have caught the R1 BLOCKER and R2 MAJOR on `fx/plan.go`.
2. **`build.md` → Planning: inject adapters through one seam and pin the wiring.** Adapters are injected through an option or a run/env parameter, never a package var. The plan names the injection point, the test fake, and a test that fails if the shipped wiring line is deleted. This would have caught the R1 and R2 MAJORs in `cmd/quarry/run.go`.
3. **`build.md` → Light lane: a plan written in the light lane pins every ruled copy line it delivers.** That includes the help Long text asserted at wrap width. A scenario with more than 3 lines of ruled copy is not LIGHT. This would have caught the status Long final-pass MAJOR.
4. **`build.md` → Planning: one fault test per classified reason, not just per call.** Any range derived from data gets an inverted row and an empty row. This would have caught the Valet decode reason and the inverted Need.
5. **CLAUDE.md → Rule copy: reconcile earlier scenarios.** A copy ruling that changes a shared format lists the earlier pins it affects, and those are re-asserted via Open debts. This would have caught the final-pass copy MAJORs.
6. **test-reviewer → Checkpoint mode: add two questions.** Is a stored-coverage invariant stated and grid-pinned? Would deleting each wiring line fail a test?

Not recommended: skipping checkpoints (the saving comes from more complete plans), or changing the fix-pass cap.
