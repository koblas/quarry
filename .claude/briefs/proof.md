# Brief: proving a claim

For `developer` and test / correctness reviewers — what counts as evidence. `architect` reads it only when plan names mutation check. Read with `.claude/rules/agent-briefs.md` (core).

## Mutation verification

Guard, test, or "absence" claim proven by breaking thing and seeing specific test go red — not by suite being green.

**Who mutates what — one rule, stated only here.** Developer runs a mutation for exactly the entries on plan's `Mutation checks:` line. Architect puts one entry there for **every mandatory test-first item** the scenario touches (`build.md` → *Build cadence*: bug fix, write-safety guard, atomicity / exclusive-create adapter), plus any other guard it judges load-bearing; `none` valid only under `Cadence: code-first`. Every other code-first test is proven falsifiable **on paper**: author can name the mutation it rules out (`go-testing` → *mutation question*); reviewer who doubts it mutates a `git archive` export (rule below), never the worktree.

**Copy the file aside so a crash cannot leave the mutation behind:**

```bash
B="$TMPDIR/mutation-$(basename <file>).$$"   # unique per run; take FRESH copy before each mutation
cp <file> "$B" && test -f "$B" || exit 1     # a shared name may be a DIRECTORY
# apply the mutation, run the targeted test, observe RED
cp "$B" <file>                               # restore
diff "$B" <file>                             # prove byte-identical; leave "$B" in place — no rm, no request to delete it
```

Interrupted run can die holding gutted guard, and tree then looks merely "failing" not "deliberately broken". Copy make that recoverable.

**Unique backup name, checked to be file.** Fixed path like `$TMPDIR/mutation-backup` shared by every agent in session: if earlier one left *directory* there, `cp <file> "$TMPDIR/mutation-backup"` silently copies INTO it, restore then fails with mutation still live. Only mandated `diff` reveals it.

**Never use `git stash` for this.** Pipeline work runs in git worktrees, and every worktree shares one stash stack with main checkout and any other session: bare `git stash pop` can apply someone else entry. Never reuse old `$TMPDIR` copy either — stale copy silently reverts file to older contents.

Rules:

- **No mutation runs beyond the `Mutation checks:` line** (rule above). Mutation per step = how scenario double its tool calls without proving anything named ones do not. Fix pass has no plan: mutations its brief names (`build.md` → *Fix passes*) plus each guard the pass adds are its line.
- **Verify guards INDIVIDUALLY.** Two guards that only go red when BOTH disabled mean either can be deleted silently. Disable one at a time.
- **Mutation must be directional and compile.** Invert or remove exactly behaviour the test pins (flip comparison, drop guard's `return`) — not break file. Compile failure, or every test failing, proves file parses, nothing more.
- **Quote the red.** Report mutation (file:line, before → after) and paste reddened test's failing assertion output. "Mutation-verified", or test name alone, not claim anyone can check — checkpoint sends it back as fix pass.
- Mutation results go in the report and STATE.md, never in a test comment (`go-testing` → *Test comments*).
- **Run affected package with `-run`, not whole suite.** Mutation targets one file; full-suite run per check = most repeated waste in long scenario.
- **Two reddened tests not two behaviours.** Pair sharing Given, When and Then is one case named twice; mutation report counting both overstates coverage. Check each cited test discriminates something others do not.
- **Reviewers never mutate worktree.** Reviewers run parallel; mutation in shared tree poisons every concurrent run. Mutate `git archive <sha>` export under `$TMPDIR`. Only developer (runs alone) mutates in place.

## Unreachable claims

`// unreachable: <reason>` (`agent-briefs.md` → *Coverage gate*) is claim like any other. Reason states **how** unreachability was established — caller that validates first (`file:line`), type that cannot hold value, `LSP` `findReferences` showing no other entry — not just why branch exists. Reviewer who constructs input reaching it raises MAJOR; "defensive" alone never passes.

## Assertions that prove nothing

Assertions that look like proof and are not recur in few shapes:

- Asserting against constant fixture set, or value copied out of production code being tested. Pin derived by reading code pins nothing.
- Negative assertions satisfied by nothing happening at all — dominant shape. Absence claim needs **control arm** showing thing DOES happen when guard removed, and control must differ from claim in exactly one variable.
- Observables that cannot fire on path under test.
- Asserting store empty without first proving it non-empty and same probe would have seen it.
- Comments overclaiming what test below them covers.
- On test-first set (`build.md` → *Build cadence*): test never seen red before its code. Off that set, code-first test is fine — but one no mutation can redden proves nothing.

- **Fault injected in a shape production never produces.** Fault test must inject the error chain the real adapter returns — driver's own error type included, wrapped as the adapter wraps it — not a convenient stdlib stand-in. One row per shape adapter can surface. Example: disk-full test injecting `*fs.PathError{Err: ENOSPC}` passed while real SQLite backup returned `sqlite3.Error{Code: SQLITE_FULL}` with no errno, so classifier sent real disk-full to wrong refusal. Read adapter's wrap sites before writing fake.

When refactor removes call site, **every existing "was never called" assertion on that fake become unfalsifiable.** Repoint them at new reachable observable, or they pass with guard deleted.
