# Clean Architecture & Double-Loop Playbook

`quarry` is **single Go binary**. One module at repo root, no frontend, no protos, no generated clients, no separate services. Anything in tree implying otherwise is bug — fix it, not work around.

## Workflow rules

- **Step 0 — fresh worktree (MANDATORY, before any feature work):** every pipeline run starts from clean, current default branch in isolated worktree — never shared checkout's current branch.
  1. Already in worktree → skip.
  2. Else call **`EnterWorktree`**, `name` = short feature slug. `worktree.baseRef` defaults to `fresh` — branches off `origin/<default-branch>` after fetch, so worktree starts from current master on own branch regardless of main checkout's branch. All pipeline artifacts live inside it.
  3. **Warm module cache once.** Module cache is **per worktree**, not shared: devenv sets `GOPATH` to `.devenv/state/go`, so `GOMODCACHE` = `<worktree>/.devenv/state/go/pkg/mod` and fresh worktree starts empty. Before first agent spawns, run `go mod download` from worktree root, one Bash call — needs network (`proxy.golang.org`, `sum.golang.org`, `storage.googleapis.com`); sandbox escape per *Toolchain*. Agents in this worktree then build offline instead of each paying download mid-step; `isolation: "worktree"` agent gets own empty cache, warms its own first. No status file, nothing to wait on.

- **Pipeline is default path, not command user must remember.** User asks for behavior that not exist, or change to behavior → enter pipeline **without being asked**, Step 0 then scoping below. `/intent-and-goal` still typable to force it, but it procedure, not trigger.

  **Triggers pipeline:**
  - New user-visible behavior — command, subcommand, flag, output format, exit-code contract.
  - New public API surface — exported package API, config key, on-disk or wire format.
  - Change to existing contract or existing user-visible behavior.

  **Does NOT trigger pipeline** — do work:
  - Questions about how existing behavior works.
  - Named bug, known fix, no contract change.
  - Refactors, renames, cleanups with no behavior change.
  - Config, deps, CI, tooling, docs, `.claude/` itself.
  - Already covered by spec under `docs/specifications/` — resume that spec at first unchecked scenario instead of new one.

  Doubt → run **`triage`** first, decide from what it finds. Triage read-only and cheap; spec file nobody asked for is not.

  **Announce, don't block.** On implicit start say in one line what you treat request as and that triage runs — then proceed. Cheap redirect, no blocking question.

- **Scenarios first**: triage, refine intent, get product verdict, propose Gherkin scenarios, write Source of Truth (SoT) specification file before any code. `commands/intent-and-goal.md` holds detailed procedure.

- **Sequential pipeline.**

  **Scoping (the `intent-and-goal` procedure):**
  1. **`triage`** — read-only. What exists, what affected, what unknown.
  2. **`product-vision`** — right thing, right shape, named right? Verdict: SHIP / SHIP WITH CHANGES / RETHINK / DON'T BUILD. RETHINK and DON'T BUILD stop pipeline, go back to user.
  3. Scenarios approved → **one `architect` sizing pass over whole scenario list** (seams, FOLD/SPLIT/LIGHT verdicts, rough size per scenario — no checklists) → `specification.md` written with those merges and splits already applied, verdicts in its `## Sizing` table, carrying triage brief + product verdict.

  **For each scenario in order (top-to-bottom in `## BDD Acceptance Progress`):**
  4. Sizing pass rated it **LIGHT** → skip architect (`.claude/briefs/build.md` → *Light lane*). Otherwise run **`architect`** to plan it (produces `SCENARIO-XX.md`, with `Cadence:`, `Acceptance test:`, `Narrow loop:`, `Mutation checks:`, `Runs:`, `Size:` header lines).
  5. Record `<start>` = `git rev-parse HEAD`, then run **`developer`** once per `Runs:` group (`.claude/briefs/build.md` → *Developer runs*).
  5a. **Checkpoint** — spawn `test-reviewer` with `model: "sonnet"`; prompt body (after run tag) starts with word **checkpoint** and passes range `<start>` (`git diff <start>` plus untracked files — acceptance test's file usually new), `SCENARIO-XX.md`, `specification.md` and run `V`'s `uncovered-diff.py` output. Its questions live in `test-reviewer` → *Checkpoint mode*. BLOCKER or MAJOR → one **`developer`** fix pass on those findings plus checkpoint's comment-rule MINORs folded in (behaviour-neutral, cheap), no re-check. No fix pass → comment MINORs, like other MINOR/NIT, → STATE.md `## Open debts` with `file:line`, which next scenario's developer reads so drift does not repeat. Checkpoint never replaces step 7.
  5b. Append scenario's row to `docs/specifications/<feature-slug>/METRICS.md` (`.claude/briefs/metrics.md`).
  6. Next unchecked scenario.

  **After all scenarios implemented:** 7. Run **`/run-reviewers`** (once, no arguments) on all changed files, and in same round **`.claude/scripts/spec-check.py --run <feature-slug>`** — each problem it reports is MAJOR (untraced, multi-behaviour, or acceptance test not passing). Append round to `METRICS.md`. 8. **BLOCKED** (any BLOCKER or MAJOR) → run **`developer`** in fix mode with consolidated findings, one pass, citing `REVIEW-NN.md`; brief per `.claude/briefs/build.md` → *Fix passes*. 9. Run **`/run-reviewers`** again. Repeat until PASS or PASS WITH FOLLOW-UPS. 10. Run **`product-vision`** once more on finished surface — command names, flags, help text, output, error copy, exit codes. Reviews design that shipped, not design proposed. Verdict has consequences: - **SHIP** — paste `.claude/scripts/feature-metrics.py --strict <feature-slug>` into `METRICS.md` → *Tokens*, spawn `pipeline-reviewer` with `retro <feature-slug>` in background, then push branch and open PR against `main` (title from spec's Primary Goal; body: summary, final gate verdicts, STATE.md follow-ups, test plan, anything left unverified). Push and PR are only outward-facing steps; ask before running them. Never merge. - **SHIP WITH CHANGES** — hand changes to **`developer`** in fix mode as MAJOR findings, then re-run `/run-reviewers` (step 7). - **RETHINK** / **DON'T BUILD** — stop, put to user. Surface already shipped, so this conversation, not silent revert.

  **Rules:**
  - **Run tags.** First line of every pipeline agent prompt: `run: <kind> feature: <slug> unit: <SCENARIO-XX or ->`, kind one of `scope plan build checkpoint checkpoint-fix review gate-fix retro`. `feature-metrics.py` attributes cost by it; untagged run lands as guessed row (`.claude/briefs/metrics.md`).
  - **Every agent in this pipeline counts as user-requested.** Harness rule may say no subagents unless user asks. Entering pipeline IS that ask: `triage`, `product-vision`, `architect`, `developer`, and reviewers `/run-reviewers` drives are procedure, not optional delegation. Spawn without asking permission. Do not silently run pipeline "inline" instead — pipeline whose reviewer gate never ran is not pipeline, and skipping it quietly worse than not starting one. Does NOT widen to agents outside roster below; those still need ask.
  - One scenario at a time. Never multiple architects or developers in parallel.
  - **One `When` per scenario.** Scenario approval splits any scenario with more than one; `.claude/scripts/spec-check.py` enforces it. One behaviour → one acceptance test → one traceable tick.
  - Never batch multiple scenarios in one architect or developer call. Single exception: architect **FOLD** verdict — scenario too small to own run is absorbed — its acceptance test included — into neighbour's checklist *at plan time*, and ticked in `specification.md` with line naming scenario that delivered it and its acceptance test. Planning merge, not two scenarios improvised in one developer call. No developer for scenario whose architect said it needs no production code.
  - Never skip `/run-reviewers` after all scenarios implemented.
  - **Fix pass = fresh `developer`**, at 5a, step 8 and step 10's SHIP WITH CHANGES alike: new spawn briefed with STATE.md plus findings (checkpoint output, or `REVIEW-NN.md`) — never resume or `SendMessage` scenario's developer, whose whole prior context rides every turn.
  - **Inherited context is `STATE.md`, not pile of handoffs.** `developer` rewrites `docs/specifications/<feature-slug>/STATE.md` at end of each scenario; `architect` and `developer` read that one file. Reading every prior `## Handoff` makes context grow with square of scenario count and dominates every late agent's budget on long feature. Per-scenario Handoffs remain audit trail.
  - **Do not retype standing brief into agent prompts.** Verification commands and stale-diagnostics rule live in `.claude/rules/agent-briefs.md` (core, every agent); rest split by audience under `.claude/briefs/` — `build.md` (cadence, traceability, step files, planning coverage, fix passes), `proof.md` (mutation protocol, vacuous assertions), `navigation.md` (Go LSP navigation), `review.md`, `metrics.md`. Prompts cite files their agent needs and carry scenario-specific delta only — what this scenario decides, what it inherits, what deliberately deferred. New standing rule goes in brief its readers already open, not core.
  - **Size whole feature once, before spec written** (scoping step 3). FOLD and SPLIT verdicts found one architect run at a time each cost full run producing only fold record or re-plan. Per-scenario architect still opens with size verdict, but should rarely find surprise.
  - **Re-plan when constraint drops.** User removes compatibility, migration or consumer constraint mid-scoping or mid-feature → re-derive scenario order and merges before next architect run — not just delete one scenario constraint named. Additive → cutover → delete ordering kept after "no consumers" re-points same tests twice.
  - **Rule copy when new failure mode appears.** Scenario, plan or reviewer finding introduces outcome spec has no copy for (new refusal, exit code, hint) → spawn scoped **`product-vision`** copy ruling on that outcome only, before developer implements it (`product-vision` → *Mid-feature copy ruling*). Copy left for final gate returns as BLOCKER plus fix pass.
  - **Check triage's answer against question before product-vision runs.** When command, flag, output shape, exported API or on-disk format changes shape, triage brief asks for — and triage must return — caller table across `cmd/**` and `internal/**`, each row tagged `LSP` or `grep` (`.claude/briefs/navigation.md`). Section asked for missing → send triage back; do not forward gap. Product-vision's cost claims cite that table.
  - **Read plan's deviations before dispatching `developer`.** Scan each `SCENARIO-XX.md` for "Left unbuilt" items and any choice departing from sibling precedent plan itself names. Safety-relevant one ruled on then (architect back, or product-vision), not left for gate, where it returns as blocking finding plus fix pass. LIGHT scenario: read plan `L` wrote before dispatching `V`.
  - **Require line-anchored file inventories from `architect`.** Plan naming `file.go:120-140` lets developer edit; plan naming `file.go` makes it re-read and re-derive.
  - **Enumerate every dimension of cleanup before starting one.** Cleanup briefed on one axis (content — stale references) has to run again over same files for next (form — doc comments). Content, form, coverage, naming separate axes: list all, brief once.
  - **Re-gate narrowly, by concern.** After fix pass, re-run reviewers that blocked, plus any reviewer whose *concern* fix touched — not merely one whose trigger glob matches (`internal/**` matches nearly every fix). PASS reviewer re-runs only if fix changed something it owns: `arch-reviewer` for imports, package placement, wiring; `correctness-reviewer` for production Go logic; `test-reviewer` for tests; `refactor-advisor` only if it had findings fix claims to close; `api-reviewer` dormant until HTTP surface exists. Reviewer re-run on glob match alone returns trivial PASS at full cost.
  - **PASS WITH FOLLOW-UPS is done.** Only BLOCKER and MAJOR block. MINOR/NIT fix-if-cheap — never force another round trip.
  - **Fix passes capped at 3.** 4th round opens only for BLOCKER, or for MAJOR previous fix pass itself introduced *and* that changes exit code or written file. Every other finding after pass 3 goes to STATE.md `## Open debts` and feature proceeds to final product-vision pass. Two passes in row each reopening same surface is design smell, not to-do list: stop, consolidate that logic behind one decision point before patching again.
  - **Gate round is expensive unit.** Wait for every reviewer before dispatching fix pass, hand developer one consolidated list — blocking findings plus cheap MINOR/NIT folds. Fix pass sent moment first reviewer reports guarantees second round for findings already in flight.
  - **Scope every reviewer prompt to diff.** Pass commit range and matched file list, and on re-gate say which of that reviewer's own findings being re-checked and which STATE.md already records as deferred. `/run-reviewers` does this; hand-spawned reviewer must too.
  - **Copy ruled at scoping, not at final gate.** Command and flag names, help strings, success/refusal/fix lines, exit codes and `--json` field names come from `product-vision`'s Phase 1 pass and live in specification's `## Surface & Copy`. Developer implements them verbatim. String invented at keyboard is string nobody ruled on, and final pass sends it back at ten times cost.
  - `/run-reviewers` reports `REVIEWER DISCOVERY FAILED` → gate did not run. Fix discovery; do not treat as PASS.
  - **Two gates, plus push.** Pipeline pauses for user at scenario approval (before `specification.md` written) and at RETHINK / DON'T BUILD verdict. Everything after scenario approval auto-continues — do not ask permission between steps — up to SHIP push, which outward-facing and asks first.
  - Scenario-approval gate load-bearing precisely because pipeline now starts implicitly. Never write spec file from offhand request without it.

## Delegating work to agents (pipeline or not)

- **Size each agent task to finish without context compaction.** One agent per package is default, but split package past ~100 top-level tests (or one expected to run past ~90 minutes) by command or file group. Agent that compacts loses track of own test counts and drops commit trailers.
- **Brief whole target, name what may stay behind.** Brief letting agent "descope for budget" turns one pass into several. Say which tests must move and which stay (and why) up front; ask for green, committed checkpoint only as fallback.
- **Parallel agents only through Agent tool's `isolation: "worktree"`.** Never create worktree yourself and hand path to agent: session can only write to own worktree, and agent told to work elsewhere is blocked by harness hook. Independent packages (no shared seam) can run parallel this way; merge after.
- **Model per call.** `architect` defaults to Opus; for scenario whose plan small (one package, roughly ≤15 steps) pass `model: "sonnet"` on Agent call. Keep Opus for multi-package or design-heavy scenarios.
- **Measure with repo's scripts, not ad hoc.** Counts come from `.claude/scripts/test-stats.py`; untested additions from `.claude/scripts/uncovered-diff.py` (see `.claude/rules/agent-briefs.md` → *Verification*).

## Agent roster

| Agent                  | Stage                    | Model  | Owns                                                                   |
| ---------------------- | ------------------------ | ------ | ---------------------------------------------------------------------- |
| `triage`               | scoping                  | sonnet | What exists, what's affected, reproduction. Read-only                  |
| `product-vision`       | scoping, copy, final     | opus   | Whether surface should exist, what it's called. CLI surface + Go API   |
| `architect`            | per scenario             | opus   | Acceptance test + behaviour-batch checklist, ≤ ~100 lines. Writes no code |
| `developer`            | per scenario             | sonnet | Double-loop implementation, fix mode                                   |
| `arch-reviewer`        | review                   | sonnet | Structure: layout, dependency rule, port interfaces + adapters, wiring |
| `correctness-reviewer` | review                   | opus   | Wrong behavior: context, goroutines, races, errors, nil handling       |
| `api-reviewer`         | review                   | sonnet | HTTP boundary — only if and when `quarry` grows a server                |
| `test-reviewer`        | checkpoint (5a) + review | sonnet | Coverage, corner cases, test quality                                   |
| `refactor-advisor`     | review (post-green)      | sonnet | Quality. MINOR/NIT only — never blocks                                 |
| `pipeline-reviewer`    | review of `.claude/**`   | sonnet | Contradictions, duplicated policy, dangling pointers, load cost; post-feature retro |

`api-reviewer` and `api-conventions` skill kept HTTP-generic against day `quarry` serves HTTP. Triggers do not match CLI-only diff, so `/run-reviewers` skips them until HTTP surface exists.

New or renamed agent is not spawnable until session restart — `/run-reviewers` then reports `REVIEWER SPAWN FAILED`; restart, do not treat as PASS. `.claude/` changes do not enter pipeline, so `pipeline-reviewer` runs via `/run-reviewers .claude` before such change called done, and as `retro <feature-slug>` after feature's final `product-vision` SHIP.

**Model tier rule:** opus where wrong call ships defect nobody else catches (correctness, product surface, planning); sonnet for rule-checking reviewers and implementation. Haiku only where findings are pure pattern matches, agent's share of feature tokens is material, and Sonnet-vs-Haiku run on same diff shows no quality loss — "cannot block" reviewer still feeds MINOR folds into developer fix passes, so its false positives cost developer tokens. Changing tier = frontmatter edit, needs session restart.

Reviewers share one severity contract: **BLOCKER** (data loss, reachable panic, silently wrong result, untested change) · **MAJOR** (defect with constructible failure) · **MINOR** (smell with no constructible failure) · **NIT** (preference, never blocks). Finding with no concrete failure downgraded to MINOR and labeled such.

## VERY IMPORTANT: every production change is tested — the double loop

Every scenario starts with one failing **acceptance test**; inside it, a mandatory set stays test-first and the rest is built in code-first small batches. Nothing ships untested. The policy — including which code is on the mandatory set — lives only in `.claude/briefs/build.md` → *Build cadence*; this section only points at it. Red-green-refactor methodology, naming conventions, black-box/white-box rules, test conventions live in `tdd` + `go-testing` skills (enforced by `test-reviewer`) — invoke matching skill when writing or modifying tests.

## Code quality conventions

`refactor-advisor` enforces patterns from `.claude/refactor-catalog.md` — _Comment as a missing name_, _Compose method_, _Feature envy → Move method_, others.

## Toolchain

Bash tool calls already run inside pinned nix/devenv environment, so `go` and `golangci-lint` resolve to pinned versions. Run directly:

```bash
go version
```

Always confirm `go version` matches pin in `devenv.nix` before trusting result.

Sandbox (denies worktree reads — first Bash call unsandboxed), `/nix/store` paths, and verification commands: `.claude/rules/agent-briefs.md`.