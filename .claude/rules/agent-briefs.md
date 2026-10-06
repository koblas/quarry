# Standing brief for pipeline agents — core

Standing brief every `architect`/`developer`/reviewer prompt would otherwise retype. This file = what **every** agent needs; it auto-loads. Rest split by audience under `.claude/briefs/` — read only what your prompt or agent file cites:

| File | Read by |
| --- | --- |
| `.claude/briefs/build.md` — build cadence (double loop), edit hook, scenario traceability, step-file format, developer runs and light lane, planning coverage, fix passes | architect, developer, checkpoint reviewers, orchestrator writing fix-pass brief |
| `.claude/briefs/proof.md` — mutation verification, assertions that prove nothing | developer, test / correctness reviewers; architect when plan names mutation check |
| `.claude/briefs/navigation.md` — Go navigation with `LSP` tool (gopls) | triage, architect, developer, arch / correctness reviewers |
| `.claude/briefs/review.md` — reviewing: scope and completeness | every reviewer |
| `.claude/briefs/metrics.md` — feature cost ledger | orchestrator only |

Read once; no ask for repeat. New standing rule goes in brief its readers already open, not here.

## Batch tool calls

Turn count × context size is agent's wall-clock; each extra turn re-reads whole context. Independent reads, greps and `LSP` queries go in **one turn** as parallel tool calls. Read whole relevant range once with `Read` `offset`/`limit` — not successive `sed -n`/`grep` probes widening by few lines. One multi-pattern grep (`-e A -e B`) beats several.

## Verification

`quarry` is one Go module at repo root. No build-graph tool, no codegen step, no second workspace. Run from repo root:

```bash
.claude/scripts/verify.sh <start> ./<touched package>/...
```

One plain call: `go build`, full suite once with coverage into a unique `$TMPDIR` dir, failures grepped from its log, `uncovered-diff.py` on that profile, `go test -race` on the packages you name, `golangci-lint run ./...`, `test-stats.py --base <start> --changed`. Prints `<step> rc=<n>` per step, exits 1 if any failed. `<start>` = commit your scenario or fix pass started from. Never hand-roll the steps: shell variables do not survive between calls, a fixed name like `$TMPDIR/cover.out` is shared by every agent in session, and the worktree guard refuses compound `$(...)`/`$VAR`/`;` forms. `go test rc≠0` → read the log path it prints, never re-run. `uncovered-diff.py` refuses (exit 2) profile older than a changed file, and lists changed file with no coverage block at all as open row.

**Narrow loop while working, full run once.** During scenario Acceptance and Build phases run only packages and tests in play — plan's `Narrow loop:` line, e.g. `go test ./internal/setup/ -run 'Skill|Init'`. Run `verify.sh` once, in `### Verify` phase (and at end of every fix pass). Full suite after every edit = most expensive habit, proves nothing final run does not. Its one suite run is both full suite and coverage data — its printed `go test rc=` is the evidence; do not run suite second time, for gate or "unpiped". Narrow loops run without `-count=1` so Go test cache skips unchanged packages; `-count=1` belongs only to the Verify run.

**Lint gate before handing off.** `golangci-lint run ./...` reads repo-root `.golangci.yaml` and must exit 0 and print `0 issues` with no `level=error` line — part of the linting step, same standing as `go build`; judge exit code, not grepped text (a typecheck/config error can still end in `0 issues`). Run `golangci-lint fmt ./...` (and `golangci-lint run --fix ./...` for mechanical rewrites) first, then fix rest by hand; review `--fix` output — it can rewrite asserted copy. Default output caps repeats per linter, so hidden findings surface once visible ones fixed: rerun until zero, or pass `--max-issues-per-linter=0 --max-same-issues=0` to see all at once. `//nolint` only when fixing would change behaviour or user-visible copy, always as `//nolint:<linter> // <reason>` naming one linter. Never edit `.golangci.yaml` to silence a finding — stop and report the config change you would make.

**Coverage gate before handing off.** Every production line you added must be executed by test. `uncovered-diff.py` lists each added non-test line no test executes, grouped into runs with enclosing function, exits 1 if any left. Reach zero, or mark genuinely unreachable defensive branch in code with `// unreachable: <reason>` (reason states how unreachability was established — `proof.md` → *Unreachable claims*) on the line, or anywhere in the contiguous `//` comment block directly above the run — then it move to "declared unreachable" section reviewer judge, and stop failing gate every later pass. Untested branch added by fix pass becomes next round's test-reviewer MAJOR.

**Counts come from `test-stats.py --base <start> --changed`**: every package whose tests changed, with `now (±delta)` for top-level tests, `t.TempDir()` sites and disk-touching tests, read from git at `<start>` — never from archive or checkout you build yourself.

Rules:

- **Never pipe verification command through `head`/`tail`.** Hides failures below cut, and `$?` become pipe status — `go build ./nonexistent 2>&1 | tail -2` reports **exit 0** for failed build. Output too long → redirect to log file and grep it, as full-suite line above does; report the printed `rc=`. If must pipe, prefix `set -o pipefail`.
- **Report exact test count and delta, from `.claude/scripts/test-stats.py`** — "green" not result, and hand-rolled counts drift between agents on same commit. Quote its rows as printed. Never write own counting script. Count that moved without explanation = finding, not rounding error.
- Green summary not mean everything ran. `test-stats.py --run <pkgdir>` counts leaf pass/fail/skip in one parallel `go test -json`; check skips before leaning on package.
- Write scratch files only under `$TMPDIR` or session scratchpad — never `/tmp`, never path outside worktree you got.
- **Sandbox denies reads of worktree too**, not only writes: this repo's worktrees sit under read-denied home dir. So first Bash call that reads worktree goes with `dangerouslyDisableSandbox: true` — known denial, no probe-then-retry. Other `operation not permitted` (e.g. write to `.claude/settings.json`) = diagnose before escaping.

## IDE diagnostics are advisory

IDE indexes mid-edit, and during mutation windows. Routinely reports compile errors `go build` does not, and indexes deleted files.

No chase them. No re-verify on their account. Authority is `go build`. One exception: diagnostic that **contradicts claim you just made** worth single targeted check — it can be live mutation left behind by crashed run.

## Verification greps

Grep is evidence only if it can see what it looks for.

- **Comment sweeps must be multiline-aware.** `//` blocks wrap, so phrase splits across lines and line-based `grep` silently reports zero. Flatten continuations first (strip leading `//`) before matching. "0 hits" from line-based grep over prose = untested claim, not clean sweep.
- **Run positive control before believing zero.** Grep for symbol you KNOW is present with same flags and scope. Control not found → sweep cannot see target, its zero means nothing.
- State what grep would MISS, not just what it found.

## Reporting

- Step comes out **green on arrival** → say so and say why. No manufacture red.
- Disagree with instruction or finding → say so **with evidence**, no silent skip.
- Control arm not behave as its plan predicts → **stop and report** — no proceed to green on claim whose control proved nothing.
- Deferred items stay deferred. No opportunistic fix outside brief; list them instead.
- Never write `/nix/store/...` path into plan, prompt, or command. They go stale every rebuild.
