# quarry verification rules for pipeline agents

This repo's half of the standing brief: how to verify quarry by hand. The generic half — batching,
IDE diagnostics, verification greps, reporting — is the pipeline plugin's `briefs/core.md`; read
it first. quarry's layout, conventions and role notes are in `.claude/briefs/repo.md`; its surface
and gate commands in `.claude/pipeline.json`.

## Verification

`quarry` is one Go module at repo root. No build-graph tool, no codegen step, no second workspace. Run from repo root:

```bash
.claude/scripts/verify.sh <start> ./<touched package>/...
```

One plain call: `go build`, full suite once with coverage into a unique `$TMPDIR` dir, failures grepped from its log, the pipeline plugin's diff-coverage check on that profile, `go test -race` on the packages you name, `golangci-lint run ./...`, `test-stats.py --base <start> --changed`. Prints `<step> rc=<n>` per step, exits 1 if any failed. `<start>` = commit your scenario or fix pass started from. Never hand-roll the steps: shell variables do not survive between calls, and a fixed name like `$TMPDIR/cover.out` is shared by every agent in session. `go test rc≠0` → read the log path it prints, never re-run. The check is read from a koblas/throughline checkout — `$THROUGHLINE`, else `.throughline`, else `~/.claude/plugins/marketplaces/throughline`; `diff-coverage rc=2` means none was found.

**Narrow loop while working, full run once.** During scenario Acceptance and Build phases run only packages and tests in play — plan's `Narrow loop:` line, e.g. `go test ./internal/setup/ -run 'Skill|Init'`. Run `verify.sh` once, in `### Verify` phase (and at end of every fix pass). Full suite after every edit = most expensive habit, proves nothing final run does not. Its one suite run is both full suite and coverage data — its printed `go test rc=` is the evidence; do not run suite second time, for gate or "unpiped". Narrow loops run without `-count=1` so Go test cache skips unchanged packages; `-count=1` belongs only to the Verify run.

**Lint gate before handing off.** `golangci-lint run ./...` reads repo-root `.golangci.yaml` and must exit 0 and print `0 issues` with no `level=error` line — part of the linting step, same standing as `go build`; judge exit code, not grepped text (a typecheck/config error can still end in `0 issues`). Run `golangci-lint fmt ./...` (and `golangci-lint run --fix ./...` for mechanical rewrites) first, then fix rest by hand; review `--fix` output — it can rewrite asserted copy. Default output caps repeats per linter, so hidden findings surface once visible ones fixed: rerun until zero, or pass `--max-issues-per-linter=0 --max-same-issues=0` to see all at once. `//nolint` only when fixing would change behaviour or user-visible copy, always as `//nolint:<linter> // <reason>` naming one linter. Never edit `.golangci.yaml` to silence a finding — stop and report the config change you would make.

**Coverage gate before handing off.** Every production block you added or changed must be executed by test. The diff-coverage check lists each changed Go block no test executes (`file:start-end`), exits 1 if any left. Reach zero, or mark genuinely unreachable defensive branch in code with `// uncovered: <reason>` (reason states how unreachability was established — *Unreachable claims* below) on the line directly above the block's first statement, on that statement, or on the block's opening line when the statement follows it directly — a marker two or more lines up is not seen; a reason too long for one line runs in the comment block above and the statement carries `// uncovered: see the reason above` — then it stops failing gate every later pass, and reviewer judges reason. Untested branch added by fix pass becomes next round's pipeline:test-reviewer MAJOR.

**Counts come from `test-stats.py --base <start> --changed`**: every package whose tests changed, with `now (±delta)` for top-level tests, `t.TempDir()` sites and disk-touching tests, read from git at `<start>` — never from archive or checkout you build yourself.

Rules:

- **Never pipe verification command through `head`/`tail`.** Hides failures below cut, and `$?` become pipe status — `go build ./nonexistent 2>&1 | tail -2` reports **exit 0** for failed build. Output too long → redirect to log file and grep it, as full-suite line above does; report the printed `rc=`. If must pipe, prefix `set -o pipefail`.
- **Report exact test count and delta, from `.claude/scripts/test-stats.py`** — "green" not result, and hand-rolled counts drift between agents on same commit. Quote its rows as printed. Never write own counting script. Count that moved without explanation = finding, not rounding error.
- Green summary not mean everything ran. `test-stats.py --run <pkgdir>` counts leaf pass/fail/skip in one parallel `go test -json`; check skips before leaning on package.
- Write scratch files only under `$TMPDIR` or session scratchpad — never `/tmp`, never path outside worktree you got.
- **Sandbox denies reads of worktree too**, not only writes: this repo's worktrees sit under read-denied home dir. So first Bash call that reads worktree goes with `dangerouslyDisableSandbox: true` — known denial, no probe-then-retry. Other `operation not permitted` (e.g. write to `.claude/settings.json`) = diagnose before escaping.


## Unreachable claims

`// uncovered: <reason>` (*Coverage gate* above) is a claim like any other. The reason, on one
line (the marker line is the one the check reads), states **how** unreachability was
established — a caller that validates first (`file:line`), a type that cannot hold the value, an
`LSP` `findReferences` showing no other entry — not just why the branch exists. "Defensive" alone
is no reason; `pipeline:test-reviewer` judges it.

## Fault tests

A fault test injects the error chain the real adapter returns — the driver's own error type
included, wrapped as the adapter wraps it — not a convenient stdlib stand-in; one row per shape
the adapter can surface. A disk-full test injecting `*fs.PathError{Err: ENOSPC}` passed while the
real SQLite backup returned `sqlite3.Error{Code: SQLITE_FULL}` with no errno, so the classifier
sent a real disk-full to the wrong refusal. Read the adapter's wrap sites before writing the fake.
