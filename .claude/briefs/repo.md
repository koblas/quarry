# Brief: quarry for pipeline agents

The repo's notes for the pipeline plugin's agents (the plugin's `briefs/core.md` → *The repo's
own briefs*). Everything here is quarry's, not the pipeline's; read the section for your role.
Surfaces and the commands that check and gate them are in `.claude/pipeline.json`.

## The repo

`quarry` is a **single Go binary** (cgo, DuckDB linked in) that snapshots a Quicken Classic for
Mac file into a local DuckDB store and serves it through a CLI, a local MCP server and a Claude
skill. One module at the repo root (`github.com/koblas/quarry`): no `go.work`, no nested `go/`,
no frontend, no protos, no generated code, no build-graph tool. Anything in the tree implying
otherwise is a bug — fix it, do not work around it.

- `cmd/quarry/` — thin wiring: config, `run()`, the error → exit-code mapping. Command-slice
  tests live here (`run_*_test.go`).
- `internal/cli/` — one file per command: parses input, formats output. No business logic.
- `internal/<feature>/` — the behaviour: `snapshot`, `importer`, `quicken` (the only package
  that knows Quicken's schema), `store` (+ `duckstore`), `report`, `finding`, `fx`, `mcp`,
  `config`, `claudeplugin`, `claudedesktop`.
- `internal/platform/*` — shared helpers (`atomicfile`, `replacefile`, `lockfile`, `money`,
  `accountmask`, `duckdb`, `sqlite`, …).
- `plugin/` and `.claude-plugin/marketplace.json` — static files of quarry's own Claude Code
  plugin (skill, references, MCP config). Not program code.
- `docs/product/` — the product authority (index `README.md`, settled rules in `decisions.md`). `docs/adr/` — decisions.
- Specs: new features under `docs/specs/<slug>.md` (the plugin's layout). Features built before
  the plugin are under `docs/specifications/<slug>/` (spec, `SCENARIO-XX.md`, `REVIEW-NN.md`,
  `STATE.md`, `METRICS.md`, `REFERENCE-CHECK.md`) — read-only history, prior art for scoping.

**Where commands run:** every Go command from the repo root.

**Toolchain:** devenv via direnv; Go pinned in `devenv.nix` (`go version` must match before a
result counts). The module cache is **per worktree** (devenv sets `GOPATH` to
`.devenv/state/go`), so a fresh worktree starts empty: run `go mod download` once from its root
before the first agent spawns (needs network, outside the sandbox).

## Briefs, skills, rules and scripts here

| File | When |
| --- | --- |
| `.claude/briefs/product.md` | product-vision: what quarry is, invariants Q1–Q8, DON'T BUILD list, consumers, the copy a scoping pass owes |
| `.claude/briefs/navigation.md` | Go navigation with `LSP` — triage, architect, developer, arch / correctness reviewers |
| `.claude/skills/clean-architecture/SKILL.md` | Go production code: cmd/internal layout, dependency rule, feature-package shape |
| `.claude/skills/go-testing/SKILL.md` | Go tests (the repo's Go test skill): structure, naming, fakes, testify |
| `.claude/rules/go-code.md` | any `.go` edit: doc-comment budget, testify, synctest, toolchain |
| `.claude/refactor-catalog.md` | `refactor-advisor` |
| `.claude/scripts/verify.sh <start> [race-pkg …]` | a hand-run Verify: build, covered full suite, coverage gate, lint, counts |
| `.claude/scripts/test-stats.py --base <start> --changed` | test counts and deltas per changed package |

The repo's own reviewers: `arch-reviewer`, `correctness-reviewer`, `refactor-advisor`,
`api-reviewer` (dormant: quarry has no HTTP surface; its triggers do not match a CLI diff).

## For triage

Blast radius map:

- `internal/cli/**` — which command or flag owns the surface today.
- `internal/<feature>/**` — which package owns the behaviour, what its `Store` / port interface
  and adapters (memory and production) already hold.
- `cmd/quarry/**` — how it is wired, which options and config keys exist.
- `internal/platform/**` — which shared helper already does part of this.
- `plugin/skills/quarry/**` and `internal/mcp/**` — whether the skill or an MCP tool states or
  depends on the rule being changed.

When a command, flag, output shape, exported API or on-disk format changes shape, return a
`## Callers` table across `cmd/**` and `internal/**`, each row tagged `LSP` or `grep`
(`navigation.md`), and a `## Becomes dead if this ships` list. product-vision's cost claims
cite them.

## For architect and developer

- Business logic and its tests live in the feature package (`internal/<feature>`). `cmd/quarry`
  stays thin; `internal/cli` only parses input and formats output.
- The acceptance test sits at the scenario's boundary: a command slice through `cli.Execute`
  (`internal/cli/run.go`; in `cmd/quarry` tests the `run()` helper, `main_test.go`), or a
  feature-package `Server` method. For a command surface, the build batches are the subcommand
  in `internal/cli`, the feature-package decision func, the output renderer, and the
  `cmd/quarry` wiring.
- Test behaviour through `cli.Execute` or the `Server` method against the in-memory adapter.
  Dependencies are injected by functional options and wired in `cmd/quarry`; a consumer declares
  the interface it needs.
- **The mandatory test-first set** (the plugin's `briefs/build.md` → *Build cadence*) maps here:
  - *Security guards* = **write-safety guards** — anything that keeps quarry from damaging
    user files or touching Quicken: overwrite / clobber / delete refusals, identity guards on
    deletion, symlink refusals, the read-only access to the live
    Quicken file (`internal/snapshot`), and edits to the user's Claude config
    (`internal/claudedesktop`).
  - *Atomic store adapters* = `internal/platform/atomicfile`, `internal/platform/replacefile`,
    `internal/platform/lockfile`, temp-then-rename and `O_EXCL` creates, the store's atomic swap.
- Money is `DECIMAL(18,2)` / `internal/platform/money`, never floats. Only `internal/quicken`
  and the importer know Quicken's schema.
- Lint is a gate, not advice: `golangci-lint run ./...` must exit 0. Run `golangci-lint fmt`
  first (gofumpt, gci, goimports are on). `//nolint` only as `//nolint:<linter> // <reason>`.
  Never edit `.golangci.yaml` to silence a finding — stop and report the change you would make.
- Every added production line is executed by a test; a defensive branch that cannot be reached
  carries `// unreachable: <reason>` stating how that was established.
- `testing/synctest` for concurrency tests, not sleeps and polling.

## For product-vision

Read `.claude/briefs/product.md` in full: it is quarry's product model — the three consumers
(person at the terminal, a script or scheduled job, Claude via the skill or MCP), the invariants
Q1–Q8 that are rejection criteria, the standing DON'T BUILD list, the exit-code contract
(`0` success, `1` failure, `2` usage) and the literal copy a scoping pass owes. Where the
plugin agent speaks of a UI or a generated client, quarry's consumers are those three.

## For the orchestrator

- **Reference check before the gate.** A spec that compares against an outside reference
  (Quicken's own reports, a copy of the real store) lists that comparison as its last scenario —
  a case-table row only, no acceptance test. The orchestrator runs it itself after the other scenarios,
  before the gate; results (counts only, never names or amounts from the user's file) go in the pull request description. A rule gap it finds is a
  new scenario, announced in one line and built before the gate. After each such fix, re-run
  the **whole** comparison against the old binary and the new one.
- **Rule copy when a new failure mode appears.** A scenario, plan or finding that introduces an
  outcome the spec has no copy for (new refusal, exit code, hint) gets a scoped product-vision
  copy ruling before the developer implements it. A ruling that changes a shared format lists
  the earlier pins it affects, each filed as a `debt` issue.
