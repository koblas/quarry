# Brief: proving a claim

For `developer` and test / correctness reviewers — what counts as evidence. `architect` reads it only when plan names mutation check. Read with `.claude/rules/agent-briefs.md` (core).

## Mutation verification

Guard, test, or "absence" claim proven by breaking thing and seeing specific test go red — not by suite being green. For code-first test (`build.md` → *Build cadence*) this is how test shows it can fail at all.

**Copy the file aside so a crash cannot leave the mutation behind:**

```bash
B="$TMPDIR/mutation-$(basename <file>).$$"   # unique per run; take FRESH copy before each mutation
cp <file> "$B" && test -f "$B" || exit 1     # a shared name may be a DIRECTORY
# apply the mutation, run the targeted test, observe RED
cp "$B" <file>                               # restore
diff "$B" <file>                             # prove byte-identical
```

Interrupted run can die holding gutted guard, and tree then looks merely "failing" not "deliberately broken". Copy make that recoverable.

**Unique backup name, checked to be file.** Fixed path like `$TMPDIR/mutation-backup` shared by every agent in session: if earlier one left *directory* there, `cp <file> "$TMPDIR/mutation-backup"` silently copies INTO it, restore then fails with mutation still live. Only mandated `diff` reveals it.

**Never use `git stash` for this.** Pipeline work runs in git worktrees, and every worktree shares one stash stack with main checkout and any other session: bare `git stash pop` can apply someone else entry. Never reuse old `$TMPDIR` copy either — stale copy silently reverts file to older contents.

Rules:

- **Mutate only guards plan's `Mutation checks:` line names.** Architect picks which guards matter; developer add no mutation checks of own. Mutation per step = how scenario double its tool calls without proving anything named ones do not.
- **Verify guards INDIVIDUALLY.** Two guards that only go red when BOTH disabled mean either can be deleted silently. Disable one at a time.
- Mutation that breaks compilation **not** evidence. If every test fails, you proved file parses, nothing more. Make mutation surgical and still-valid.
- Say which mutation you ran and which test it reddened. "Mutation-verified" alone not claim anyone can check.
- Mutation results go in the report and STATE.md, never in a test comment (`go-testing` → *Test comments*).
- **Run affected package with `-run`, not whole suite.** Mutation targets one file; full-suite run per check = most repeated waste in long scenario.
- **Two reddened tests not two behaviours.** Pair sharing Given, When and Then is one case named twice; mutation report counting both overstates coverage. Check each cited test discriminates something others do not.
- **Reviewers never mutate worktree.** Reviewers run parallel; mutation in shared tree poisons every concurrent run. Mutate `git archive <sha>` export under `$TMPDIR`. Only developer (runs alone) mutates in place.

## Assertions that prove nothing

Assertions that look like proof and are not recur in few shapes:

- Asserting against constant fixture set, or value copied out of production code being tested. Pin derived by reading code pins nothing.
- Negative assertions satisfied by nothing happening at all — dominant shape. Absence claim needs **control arm** showing thing DOES happen when guard removed, and control must differ from claim in exactly one variable.
- Observables that cannot fire on path under test.
- Asserting store empty without first proving it non-empty and same probe would have seen it.
- Comments overclaiming what test below them covers.
- On test-first set (`build.md` → *Build cadence*): test never seen red before its code. Off that set, code-first test is fine — but one no mutation can redden proves nothing.

When refactor removes call site, **every existing "was never called" assertion on that fake become unfalsifiable.** Repoint them at new reachable observable, or they pass with guard deleted.
