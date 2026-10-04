# Review 02 — phase3d-skill (re-gate after fix pass 1)

## Target
Range `922b864..ae2cc95`. Mutation sample: skipped, because no production Go changed.

## Triggered reviewers
- correctness-reviewer: it raised MAJORs 1 and 2.
- test-reviewer: it raised MAJOR 3 and owns the test refactor.

## Skipped reviewers
- arch-reviewer: no change to imports or placement.
- refactor-advisor: its findings are deferred in STATE.md.

## BLOCKER
None.

## MAJOR
None. All three round-1 MAJORs are closed:
- The no-ELSE currency CASE: the correctness reviewer confirmed it with a probe, and CAD and USD behaviour is unchanged.
- The partial-period bullet, which is pinned.
- The `recipes []recipeSpec` registry plus a disk-listing test with a non-empty guard.

## MINOR
- test-reviewer — `cmd/quarry/run_skill_recipes_test.go:381-397`: the loops and `if` in the body of `Test_recipe_registry_lists_every_sql_file_on_disk` should move into helpers.

## NIT
- test-reviewer — `run_skill_recipes_test.go:382`: the registry check reads the top level of `references/sql/` only.
- test-reviewer — `run_skill_references_test.go:67`: the partial-period pin covers only the second half of the bullet.

## Verdict: PASS WITH FOLLOW-UPS
