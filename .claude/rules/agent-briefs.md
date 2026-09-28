# Standing brief for pipeline agents — core

Standing brief every `architect`/`developer`/reviewer prompt would otherwise retype. This file = what **every** agent needs; it auto-loads. Rest split by audience under `.claude/briefs/` — read only what your prompt or agent file cites:

| File | Read by |
| --- | --- |
| `.claude/briefs/build.md` — build cadence (double loop), scenario traceability, step-file format, planning coverage, fix passes | architect, developer, checkpoint reviewers, orchestrator writing fix-pass brief |
| `.claude/briefs/proof.md` — mutation verification, assertions that prove nothing | developer, test / correctness reviewers; architect when plan names mutation check |
| `.claude/briefs/navigation.md` — Go navigation with `LSP` tool (gopls) | triage, architect, developer, arch / correctness reviewers |
| `.claude/briefs/review.md` — reviewing: scope and completeness | every reviewer |
| `.claude/briefs/metrics.md` — feature cost ledger | orchestrator only |

Read once; no ask for repeat. New standing rule goes in brief its readers already open, not here.

## Verification

`quarry` is one Go module at repo root. No build-graph tool, no codegen step, no second workspace. Run from repo root:

```bash
go build ./...
COVER="$(mktemp "$TMPDIR/cover.XXXXXX")"                                   # unique per run
go test -count=1 -coverpkg=./... -coverprofile="$COVER" ./... &&          # the full suite, once
  .claude/scripts/uncovered-diff.py --profile "$COVER" <start>              # coverage gate, no re-run
go test -race ./<touched package>/...
golangci-lint run ./...
.claude/scripts/test-stats.py --base <start> --changed                     # counts and deltas
```

`<start>` = commit your scenario or fix pass started from. `mktemp` line, `go test` and `uncovered-diff.py` go in **one Bash call** — shell variables do not survive between calls, and fixed name like `$TMPDIR/cover.out` is shared by every agent in session (same hazard as `proof.md` → *Unique backup name*). `uncovered-diff.py` refuses (exit 2) profile older than a changed file, and lists changed file with no coverage block at all as open row.

**Narrow loop while working, full run once.** During scenario Acceptance and Build phases run only packages and tests in play — plan's `Narrow loop:` line, e.g. `go test ./internal/setup/ -run 'Skill|Init'`. Run block above once, in `### Verify` phase (and at end of every fix pass). Full suite after every edit = most expensive habit, proves nothing final run does not. That one `go test` line is both full suite and coverage data — do not run suite second time for gate.

**Coverage gate before handing off.** Every production line you added must be executed by test. `uncovered-diff.py` lists each added non-test line no test executes, grouped into runs with enclosing function, exits 1 if any left. Reach zero, or mark genuinely unreachable defensive branch in code with `// unreachable: <reason>` on the line, or anywhere in the contiguous `//` comment block directly above the run — then it move to "declared unreachable" section reviewer judge, and stop failing gate every later pass. Untested branch added by fix pass becomes next round's test-reviewer MAJOR.

**Counts come from `test-stats.py --base <start> --changed`**: every package whose tests changed, with `now (±delta)` for top-level tests, `t.TempDir()` sites and disk-touching tests, read from git at `<start>` — never from archive or checkout you build yourself.

Rules:

- **Never pipe verification command through `head`/`tail`.** Hides failures below cut, and `$?` become pipe status — `go build ./nonexistent 2>&1 | tail -2` reports **exit 0** for failed build. If must pipe, prefix `set -o pipefail`.
- **Report exact test count and delta, from `.claude/scripts/test-stats.py`** — "green" not result, and hand-rolled counts drift between agents on same commit. Quote its rows as printed. Never write own counting script. Count that moved without explanation = finding, not rounding error.
- Green summary not mean everything ran. `test-stats.py --run <pkgdir>` counts leaf pass/fail/skip in one parallel `go test -json`; check skips before leaning on package.
- Before declaring scenario done, run `go test ./...` from repo root once, unpiped. Exit code of unpiped command = evidence.
- Write scratch files only under `$TMPDIR` or session scratchpad — never `/tmp`, never path outside worktree you got.
- Bash call failing with `operation not permitted` mean shell was **sandboxed**. Re-run with `dangerouslyDisableSandbox: true`.

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
