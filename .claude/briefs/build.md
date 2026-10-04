# Brief: building a scenario

For `architect`, `developer`, step-5a checkpoint reviewers, and orchestrator writing fix-pass brief. Read with `.claude/rules/agent-briefs.md` (core).

## Build cadence: the double loop

One statement of when test must fail before code exists. `CLAUDE.md`, `tdd` skill, `architect`, `developer` and `test-reviewer` cite it; none restates it.

**Outer loop — test-first, always.** Each scenario has exactly one **acceptance test** at its observable boundary: command slice through `cli.Run`, or feature-package `Server` method. Written before any production code, must fail **at its assertion, for expected reason**. Failing output goes in developer's report. Named in `specification.md`'s `## BDD Acceptance Progress` line (see *Scenario traceability*); scenario not ticked until that test passes.

**Inner loop — depends on what code is.** Test-first (red → green → refactor, per `tdd` skill) stays **mandatory** for:

- **Bug fixes** — test must have failed before fix. Reviewer finding with constructible `Failure:` is bug fix.
- **Write-safety guards** — anything that keeps `quarry` from damaging user files: overwrite / clobber / delete refusals and identity guards on deletion, symlink refusals, `os.Root` confinement (`internal/assemble/fs.go`, `internal/scaffold/fs.go`), `internal/platform/writable`.
- **Adapters with atomicity or exclusive-create claim** — `internal/platform/atomicfile`, `internal/platform/rwfs`, temp-then-rename, `O_EXCL` creates.

Everything else built as **code-first small batches**: one behaviour per batch — write code, then its unit tests in same batch, then refactor while green, then next batch. Refactor every batch, not once at end. Nothing ships untested — `test-reviewer`'s "untested change is BLOCKER" unchanged, and coverage gate still demands every added line executed. Only change: whether unit test must be *seen red* before its code exists. Who proves a test can fail, and how, is `proof.md` → *Mutation verification* — not restated here.

Architect marks each scenario's cadence in plan: `Cadence: test-first` (any mandatory item above touched — name which) or `Cadence: code-first`. Every mandatory item touched also gets its own entry on the plan's `Mutation checks:` line. Orchestrator copies it into `METRICS.md` so cadence cost can be compared later (`metrics.md`).

## Edit hook

After every `Edit`/`Write` on a `.go` file, `.claude/scripts/post-edit-check.sh` runs `gofmt -l` and `go build` on that package and feeds back only failures (≤50 lines). Silence = both passed. No `go build` or `gofmt` turns of your own between edits; fix what hook reports. Tests and `golangci-lint` stay yours, at phase boundaries — hook never runs tests (red phases red by design). Hook silent because toolchain unreachable is indistinguishable from pass, so Verify block still runs `go build ./...` once. Mid-batch compile error naming symbol you are about to add in next edit is expected — keep going, don't stub around it.

## Scenario traceability

Link runs **from spec to test**, never test to spec — test code carries no scenario IDs (developer fix-mode rule 8: specs archived, citations rot).

Each `## BDD Acceptance Progress` line names scenario's acceptance test:

```markdown
- [x] SCENARIO-04: Finish ticks the step in the specification — `internal/scaffold/finish_test.go` `Test_finish_ticks_the_progress_entry`
```

Test reference must be **last thing on line** — fold's "delivered by SCENARIO-NN" note goes before it, and ticked line must not wrap.

`.claude/scripts/spec-check.py <feature-slug>` checks every scenario has exactly one `When`, every **ticked** scenario names acceptance test, and test exists in that file. Developer runs it right after ticking; final gate runs it with `--run`, which also executes each ticked acceptance test (anchored `-run`) and fails the round if one fails or matches nothing — a tick can't outlive its test. Enforces only specs carrying `<!-- spec-check: v1 -->` marker (`intent-and-goal` template adds it); spec written before this convention reported as not opted in and passes, so resuming one never turns shipped scenarios into gate findings. `--force` checks unmarked spec by hand.

## Scenario plan files are brief step files

`docs/specifications/<feature>/SCENARIO-XX.md` keeps one fixed shape so orchestrator, developer runs and reviewers find the same anchors. Must start with frontmatter, then heading, then header lines, then checklist under `## Implementation Plan` (phases in architect's plan format), then `## Handoff`, then `## Phase report`:

```markdown
---
id: SCENARIO-XX
status: open
---

# SCENARIO-XX: <title>
```

Developer sets `status: done` when scenario complete, plus tick in `specification.md`. Every `- [ ]` under `## Implementation Plan` must be ticked by then — checkpoint question 2 checks it.

## Developer runs

One scenario = several short `developer` runs, never one long one. Every turn re-reads whole context, so long run pays for its early turns again on each late one; in baseline features one developer session carried up to 32% of feature's tokens.

Architect writes run groups on plan's `Runs:` header line; orchestrator spawns **fresh** `developer` per group, in order:

| Run | Plan phases | Ends with | Skills to load |
| --- | --- | --- | --- |
| `A` | Acceptance (red) | acceptance test failing at its assertion | `tdd`, `go-testing`, `clean-architecture` |
| `B1`, `B2`, … | Build, **≤3 behaviour batches** per run | its batches green on `Narrow loop:` | `go-testing`, `clean-architecture`; `tdd` only under `test-first` |
| `L` | writes plan (*Light lane*), then Acceptance (red) + Build | plan written, its batches green | `tdd`, `go-testing`, `clean-architecture` |
| `V` | Sweep + Verify, spec tick, `spec-check.py`, `STATE.md` rewrite, `status: done` | full verification block green | none beyond briefs |

- Run gets: tag line with `unit:`, `Run: <group>` line, plan (with ticks so far), `STATE.md`, plan's `## Phase report`. **Never** previous run's transcript or report pasted into prompt — it is in the file.
- Each run ends by **rewriting** `## Phase report` (last section of plan, ≤30 lines, replaces previous): files changed (`file:line`), what is red / green now (run `A` quotes failing assertion), anything next run must not redo or undo. Then returns.
- Run past ~40 tool calls: finish current batch, write phase report, return `PARTIAL: <steps left>`; orchestrator spawns next run for remainder. Long run is the cost this section exists to stop.
- Plan with ≤1 Build batch may merge `A` and `B1` into one run (`Runs: A+B1 (1-4) | V (5-6)`). Nothing else merges.
- Mutation checks on plan's `Mutation checks:` line belong to `B` run that builds that guard; `V` adds none.
- `<start>` (CLAUDE.md step 5) recorded before run `A`; checkpoint runs after `V`, over whole scenario's range.
- Plan without `Runs:` line (older plan) → orchestrator derives groups by these rules before spawning.
- Fix passes stay single runs — findings list is their plan.

## Light lane

Scenario the sizing pass rated **LIGHT** (architect → *Size verdict*) skips its per-scenario `architect` run. Two developer runs instead:

- `L` — writes `SCENARIO-XX.md` itself (no plan exists yet; read spec's scenario and `STATE.md` instead), same shape as architect plan, **≤15 lines** under `## Implementation Plan`, header `Size: LIGHT — <n> steps, <package>` and `Runs: L | V`, no `## Handoff` beyond anything a later scenario must not contradict. Then executes Acceptance (red) and Build, ends with phase report. Plan names every `## Surface & Copy` line the scenario delivers (help Long, warning, refusal, status line) with the test asserting it; Long text asserted at wrap width.
- `V` — as above.

Checkpoint still runs after `V`. Plan touching mandatory test-first item, growing past 3 Build steps while being written, or delivering more than 3 ruled `## Surface & Copy` lines → `L` stops, returns `PARTIAL: needs architect`; orchestrator runs `architect` and continues normally.

## Planning: coverage the gate will demand

Commonest blocking findings share one shape: fallible call in new code with no fault test, or numeric bound with no outside-the-bound test. Each costs fix pass plus re-gate for test architect could have listed up front. Every architect checklist for new or changed command, feature-package method or adapter names, in the Build batch that owns the code:

- **One fault test per fallible call** — each `Store` call, file read/write/rename, `exec`, and parse the code makes. Include call that re-reads on resume or retry branch, not just first one. Injected error has the shape the real adapter returns (`proof.md` → *Assertions that prove nothing*).
- **Every numeric bound tested just outside it**, in-bound case as control (line caps, count limits, depth limits). Interval derived from data (`First..Last` computed from rows or clock) gets an empty row and an inverted (`First > Last`) row.
- **One fault test per classified reason, not only per call.** Error mapped to one of several user-visible reasons → each reason its own row, reached through the real decode path, not a fake returning the sentinel.
- **Every fallback branch of error → exit-code mapper** — the `default:` arm, not just named sentinels.
- **One decode-fault test per decoded record kind** for adapter or parser reading files — frontmatter as well as body items. Corrupt-child-item test on a read does not cover corrupt root on same read.
- **Destructive guard or destructive advice = identity invariant, not a name rule.** Scenario that deletes or overwrites user files, or tells user to delete or merge something (a finding whose fix is "delete it in Quicken"), states its safety property as invariant holding without any name comparison ("prune never deletes an entry that is the same file as the one the store recorded"); name, case and extension matching may only choose what to display. Guard that selects by name (first match, newest match, name fallback) → list every way two names reach one object (letter case, hard link, symlink, extension) and say which the invariant covers. Mutation checks: see *Build cadence*; the invariant-breaking one is among them.
- **Stored coverage claim = state invariant plus exhaustive small grid.** Scenario that records "this range / key set is done" (fetched span, floor, watermark, cache validity) and later skips work because of that record states, before any span arithmetic, the invariant making a wrong claim impossible ("stored ranges are one interval; asked ∪ have = one interval covering hull(need ∪ have)"). Owning batch lists a grid test over every operand pair on a small index range (zero, inverted, every First≤Last pair, ≥7 points) through the public method with a recording fake; assertions derived from the points, never copied from the planner; head, tail, interior, wholly-before and wholly-after each a row so a mirror case cannot be fixed one side at a time. Plus a two-operation property on the real store (after any second run the stored claim still holds).
- **Injected adapters: one seam, one wiring pin.** Adapter the binary builds itself (network client, clock, filesystem) is injected through a constructor option or run/env parameter — never a package-level `var` tests reassign. Plan names injection point (`file:line`), test-wide fake default, and a test that fails if the shipped wiring line is deleted (e.g. entry point with a recording `http.DefaultTransport`). Mutation check: delete the wiring line.
- **One pin per ruled string and edge row the scenario owns** — every `## Surface & Copy` line it delivers (help Long, Example, flag help, refusal, warning, sort order) asserted verbatim, and every edge-case row it reaches (future-dated, other currency, closed account, empty period) as its own case. Scenario's `Then` rarely covers them. Each edge row is crossed with flags and formats (next-but-one bullet).
- **One pin per arm of a ruled rule, each differing from its control in one variable.** Every sort-key tier (final tiebreak included), every SQL `WHERE`/`JOIN`/`CASE` clause, every normalisation step (case-fold, trim, Unicode class, cut-at), every both-orders case (A-then-B and B-then-A, reversed dates) is own row; checklist names arms in Build batch that owns code. Rule over hierarchy or graph pinned at depth ≥ 2 (grandchild, not only child), in both directions (blocked by ancestor, blocked by descendant).
- **One out-of-domain row per enumerated domain.** Every fallback, passthrough and `default:`/`ELSE` arm is own row; every enumerated domain (flag value, SQL param, `CASE` on a value) gets one out-of-domain row asserting what it yields — an `ELSE` that silently picks a branch is the bug this row finds.
- **Siblings and mirrors.** New command, tool, reference file or recipe joining a family: plan greps siblings for the rules and edge rows they state (refusal tables, empty-string rows, prose caveats such as partial periods) and lists each to copy or deliberately drop, by `file:line`. Test-side set that mirrors files on disk (registry of recipes, list of references) is derived from the directory or pinned equal to it, with a non-empty guard.
- **Cross each edge row with each flag and output format.** For every edge row the scenario reaches (closed account, no payee, other currency, future-dated, empty window) and every output arm it owns (text, `--json`, `--account`, sort tier, tally/footer), checklist names the cell's test or says "n/a: <reason>". Row asserted in text only, or arm asserted without `--account`, is the unpinned cell checkpoint will find.
- **One read per command.** Command rendering two views of store (status plus counts, list plus totals) takes both from one port call or one open; plan names that call and a fake-store test whose second call returns different data and cannot produce mixed output.
- **Cross-surface rule gets one pin per site**, or one chokepoint test that fails if a site bypasses shared helper — rule in `## Surface & Copy` saying "every X" (every text cell, every path in `--json`). Copy reused "as in <earlier phase>" counts as an "every X" rule: for a new command the plan greps the test tree for each all-commands table (refusal, usage-hint, root-help, registration pins) and lists the row to add per command by file:line.
- **Shared helper rewritten or extracted in the scenario gets its behaviour classes pinned in its own package** (width unit, escaping, ordering), not only through command oracles.
- **Machine formats get read-back test.** Output others parse (CSV, JSON) read back with standard-library parser; record count and values asserted equal to input, including one-column result and NULL versus empty-string cell.

## Fix passes

**Findings with `Failure:` are bug fixes → test-first** (*Build cadence*). Brief naming mutation checks cites `proof.md` → *Mutation verification*. Write test reproducing `Failure:`, run, see it fail at its assertion, then fix. Finding already covered by test failing today → cite it. Behaviour-neutral findings (docs, renames, test additions, extractions) exempt. Cannot make it red → say so with what you tried; no fix blind then backfill.

**No new behaviour.** Cheap MINOR/NIT folds = docs, renames, test additions, extractions keeping behaviour identical. **Fold adding runtime behaviour not cheap:** new branch folded in from MINOR and left untested becomes MAJOR forcing another fix pass and re-gate. New behaviour goes to STATE.md `## Open debts`, or folded with its tests planned in same brief, named per branch.

**Applies to MAJOR's fix too, not only folds.** When fix for finding adds runtime behaviour (timeout, retry, fallback, new branch), fix-pass brief names, before dispatch:

- **positive assertion** for each new branch — what DID happen, not only what did not — with control arm differing in one variable;
- for every new numeric bound, **in-bound and just-outside-bound tests** (*Planning* applies to fix passes unchanged);
- **mutation expected to redden each test**.

Test reading `ctx.Err()` from value defaulting to nil, or bound test that cannot see bound's value, passes whether or not new branch works — each such gap found at gate costs whole extra fix pass and re-gate.
