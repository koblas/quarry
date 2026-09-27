---
name: tdd
description: The red-green-refactor cycle. Use for a scenario's acceptance test, and for every unit test on the mandatory test-first set named in .claude/briefs/build.md → Build cadence.
argument-hint: <what-to-implement>
allowed-tools: Read, Write, Edit, Glob, Grep, Bash
---

Implement using strict TDD: **$ARGUMENTS**

## Where this applies

`.claude/briefs/build.md` → *Build cadence* decides. Short form:

- **Always** — scenario's one acceptance test, at its boundary, before any production code.
- **Always** — inner-loop unit tests on the mandatory test-first set that section names.
- **Otherwise** — code-first small batches: code one behaviour, test it in same batch, refactor
  while green. *Iron Law* below not apply to those unit tests; refactor step and every
  test-quality rule still do.

## Iron Law (on the test-first set)

**No production code without failing test first.** No watch test fail = no know if test right thing. Code before test must die + reimplement from test — no exceptions.

## Picking the next test (ZOMBIES + TPP)

Cycle below tell *how* do red-green-refactor. **ZOMBIES** + **Transformation Priority Premise** tell *which* test write next.

Walk **ZOMBIES** in order — earlier categories force simplest production transformations, strongest discriminators for catching inversions + off-by-ones. Resist jump to "many" or "mixed" — One tests carry more info than look.

- **Z**ero — empty / null input → force default-return implementation
- **O**ne — exactly one item → force real filtering / branching logic
- **M**any — N>1 items → force aggregation / iteration
- **B**oundary — edges, off-by-ones, window limits
- **I**nterface — contract shape, optional vs required
- **E**xceptions — error paths
- **S**imple — keep each transformation small

Each new test force **next-simplest transformation** in production code (Uncle Bob TPP). Rough priority, simplest first:

1. nothing → constant
2. constant → variable
3. statement → conditional (`if`)
4. scalar → list / aggregation
5. unconditional → loop

Implementation emerge = whatever satisfy contract — SQL, in-memory, HTTP, whatever. **Downstream of test list, not source.** If design tests against existing implementation (e.g. already-written SQL string), work backward: write tests against *port contract*, let implementation be accidental.

Pair every test with **mutation question** (see `go-testing` skill): no mutation of production code make test fail = test vacuous.

References: James Grenning, ["TDD Guided by Zombies"](https://blog.wingman-sw.com/tdd-guided-by-zombies); Robert C. Martin, ["The Transformation Priority Premise"](https://blog.cleancoder.com/uncle-bob/2013/05/27/TheTransformationPriorityPremise.html).

## The cycle (one behavior at a time)

Each behavior = one tracer bullet: RED → GREEN → REFACTOR. Then next behavior. No batch tests across behaviors.

### 1. RED — Write one failing test

- Write smallest test describing next behavior.
- Run suite. New test must fail.
- **Hard gate: paste failing output before proceeding.** No output, no GREEN.
- Verify failure is expected reason (missing feature — not typo, not missing import).
- Test pass without new code = behavior already exist → pick different test.

### 2. GREEN — Make it pass

- Write minimum production code to pass failing test.
- Run suite. All tests must pass.
- **Hard gate: paste passing output before proceeding.** No output, no REFACTOR.
- No add behavior beyond current test requires.
- No refactor yet.

### 3. REFACTOR — Improve design

- Remove duplication, improve naming, separate concerns.
- Keep tests green throughout. Break = change too aggressive — undo, try smaller.
- Keep domain rules in domain/application; keep domain framework-free.

Return to RED for next behavior.

## Anti-pattern: writing tests in bulk

**No write all of layer tests first, then all production code.** Bulk tests validate imagined behavior, not real — commit to contract guess no running code confirmed. Each cycle learn from previous.

Architect plan tell *which* tests + *what order*. No tell setup, assertions, fake API — those design decisions made during cycle, one test at a time.

## Rationalization prevention (test-first set)

LLMs generate plausible excuses for skip/defer TDD. Common ones + why fail:

| Excuse | Reality |
|---|---|
| "I'll add tests after the implementation" | You won't, or write tests pass by definition — validate what you wrote, not what should work. |
| "This is too simple to test" | Simple code break too. Test take 30 seconds. |
| "I need to see the implementation shape first" | That's spike. Spike, throw away, then TDD. |
| "I'm just refactoring, not adding behavior" | Existing tests must pass throughout. No tests = write characterization tests first. |
| "Writing the test first would be slower" | TDD faster than debugging. Catch errors at cheapest moment. |
| "The test is hard to write — I'll come back to it" | Hard-to-test code = hard-to-use code. Test = design feedback. Listen. |

Catch self composing excuse not on list = still excuse.

## Red flags — stop and restart from RED (test-first set)

- Writing implementation before test.
- Test pass immediately without new code.
- Cannot explain why test failed.
- Reasoning begin with "just this once."
- Manual testing claims replace automated verification.

**Response**: delete code written without test. Restart from RED.

## Project conventions

- Test structure, naming, fakes, assertions, minimality, API ordering, other test-quality rules → follow `go-testing` skill.
- Each file read once, written once.