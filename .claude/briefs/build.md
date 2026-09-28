# Brief: building a scenario

For `architect`, `developer`, step-5a checkpoint reviewers, and orchestrator writing fix-pass brief. Read with `.claude/rules/agent-briefs.md` (core).

## Build cadence: the double loop

One statement of when test must fail before code exists. `CLAUDE.md`, `tdd` skill, `architect`, `developer` and `test-reviewer` cite it; none restates it.

**Outer loop — test-first, always.** Each scenario has exactly one **acceptance test** at its observable boundary: command slice through `cli.Run`, or feature-package `Server` method. Written before any production code, must fail **at its assertion, for expected reason**. Failing output goes in developer's report. Named in `specification.md`'s `## BDD Acceptance Progress` line (see *Scenario traceability*); scenario not ticked until that test passes.

**Inner loop — depends on what code is.** Test-first (red → green → refactor, per `tdd` skill) stays **mandatory** for:

- **Bug fixes** — test must have failed before fix. Reviewer finding with constructible `Failure:` is bug fix.
- **Write-safety guards** — anything that keeps `quarry` from damaging user files: overwrite / clobber refusals, symlink refusals, `os.Root` confinement (`internal/assemble/fs.go`, `internal/scaffold/fs.go`), `internal/platform/writable`.
- **Adapters with atomicity or exclusive-create claim** — `internal/platform/atomicfile`, `internal/platform/rwfs`, temp-then-rename, `O_EXCL` creates.

Everything else built as **code-first small batches**: one behaviour per batch — write code, then its unit tests in same batch, then refactor while green, then next batch. Refactor every batch, not once at end. Nothing ships untested — `test-reviewer`'s "untested change is BLOCKER" unchanged, and coverage gate still demands every added line executed. Only change: whether unit test must be *seen red* before its code exists. Who proves a test can fail, and how, is `proof.md` → *Mutation verification* — not restated here.

Architect marks each scenario's cadence in plan: `Cadence: test-first` (any mandatory item above touched — name which) or `Cadence: code-first`. Every mandatory item touched also gets its own entry on the plan's `Mutation checks:` line. Orchestrator copies it into `METRICS.md` so cadence cost can be compared later (`metrics.md`).

## Scenario traceability

Link runs **from spec to test**, never test to spec — test code carries no scenario IDs (developer fix-mode rule 8: specs archived, citations rot).

Each `## BDD Acceptance Progress` line names scenario's acceptance test:

```markdown
- [x] SCENARIO-04: Finish ticks the step in the specification — `internal/scaffold/finish_test.go` `Test_finish_ticks_the_progress_entry`
```

Test reference must be **last thing on line** — fold's "delivered by SCENARIO-NN" note goes before it, and ticked line must not wrap. `quarry`'s own tick (`tickProgressEntry`) matches by ID and keeps trailing text.

`.claude/scripts/spec-check.py <feature-slug>` checks every scenario has exactly one `When`, every **ticked** scenario names acceptance test, and test exists in that file. Developer runs it right after ticking; final gate runs it. Enforces only specs carrying `<!-- spec-check: v1 -->` marker (`intent-and-goal` template adds it); spec written before this convention reported as not opted in and passes, so resuming one never turns shipped scenarios into gate findings. `--force` checks unmarked spec by hand.

## Scenario plan files are brief step files

`docs/specifications/<feature>/SCENARIO-XX.md` read by `quarry` itself (`brief status`, `brief check`). Must start with frontmatter, then heading, then header lines, then checklist under `## Implementation Plan` (phases in architect's plan format):

```markdown
---
id: SCENARIO-XX
status: open
---

# SCENARIO-XX: <title>
```

Developer sets `status: done` when scenario complete, plus tick in `specification.md`. Every `- [ ]` under `## Implementation Plan` must be ticked by then — `brief check` reports unticked item on done step.

## Planning: coverage the gate will demand

Commonest blocking findings share one shape: fallible call in new code with no fault test, or numeric bound with no outside-the-bound test. Each costs fix pass plus re-gate for test architect could have listed up front. Every architect checklist for new or changed command, feature-package method or adapter names, in the Build batch that owns the code:

- **One fault test per fallible call** — each `Store` call, file read/write/rename, `exec`, and parse the code makes. Include call that re-reads on resume or retry branch, not just first one. Injected error has the shape the real adapter returns (`proof.md` → *Assertions that prove nothing*).
- **Every numeric bound tested just outside it**, in-bound case as control (line caps, count limits, depth limits).
- **Every fallback branch of error → exit-code mapper** — the `default:` arm, not just named sentinels.
- **One decode-fault test per decoded record kind** for adapter or parser reading files — frontmatter as well as body items. Corrupt-child-item test on a read does not cover corrupt root on same read.

## Fix passes

**Findings with `Failure:` are bug fixes → test-first** (*Build cadence*). Write test reproducing `Failure:`, run, see it fail at its assertion, then fix. Finding already covered by test failing today → cite it. Behaviour-neutral findings (docs, renames, test additions, extractions) exempt. Cannot make it red → say so with what you tried; no fix blind then backfill.

**No new behaviour.** Cheap MINOR/NIT folds = docs, renames, test additions, extractions keeping behaviour identical. **Fold adding runtime behaviour not cheap:** new branch folded in from MINOR and left untested becomes MAJOR forcing another fix pass and re-gate. New behaviour goes to STATE.md `## Open debts`, or folded with its tests planned in same brief, named per branch.

**Applies to MAJOR's fix too, not only folds.** When fix for finding adds runtime behaviour (timeout, retry, fallback, new branch), fix-pass brief names, before dispatch:

- **positive assertion** for each new branch — what DID happen, not only what did not — with control arm differing in one variable;
- for every new numeric bound, **in-bound and just-outside-bound tests** (*Planning* applies to fix passes unchanged);
- **mutation expected to redden each test**.

Test reading `ctx.Err()` from value defaulting to nil, or bound test that cannot see bound's value, passes whether or not new branch works — each such gap found at gate costs whole extra fix pass and re-gate.
