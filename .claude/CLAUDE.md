# Clean Architecture & Double-Loop Playbook

`quarry` is **single Go binary**. One module at repo root, no frontend, no protos, no generated clients, no separate services. Anything in tree implying otherwise is bug — fix it, not work around. `plugin/` and `.claude-plugin/marketplace.json` are static files (Claude Code plugin: skill, references, MCP config) for installed binary — no program code, not second program.

## Workflow rules

The pipeline plugin (`pipeline@throughline`, enabled in `.claude/settings.json`) loads its
workflow rules — the implicit trigger, the three lanes, the two pauses, the agent-spawn authority —
into every session through its SessionStart hook. They are not restated here. No line from that
hook in this session's context (it starts `Pipeline plugin root:`) → the hook did not run: read the
plugin's `briefs/workflow.md` before acting on any request for new or changed behavior. quarry's
own notes for pipeline agents are `.claude/briefs/repo.md`.

## Delegating work to agents (pipeline or not)

- **Size each agent task to finish without context compaction.** One agent per package is default, but split package past ~100 top-level tests (or one expected to run past ~90 minutes) by command or file group. Agent that compacts loses track of own test counts and drops commit trailers.
- **Brief whole target, name what may stay behind.** Brief letting agent "descope for budget" turns one pass into several. Say which tests must move and which stay (and why) up front; ask for green, committed checkpoint only as fallback.
- **Parallel agents only through Agent tool's `isolation: "worktree"`.** Never create worktree yourself and hand path to agent: session can only write to own worktree, and agent told to work elsewhere is blocked by harness hook. Independent packages (no shared seam) can run parallel this way; merge after.
- **Model per call.** `architect` defaults to Opus; for scenario whose plan small (one package, roughly ≤15 steps) pass `model: "sonnet"` on Agent call. Keep Opus for multi-package or design-heavy scenarios.
- **Measure with repo's scripts, not ad hoc.** Counts come from `.claude/scripts/test-stats.py`; untested changes from the pipeline plugin's diff-coverage check, which `.claude/scripts/verify.sh` runs (see `.claude/rules/agent-briefs.md` → *Verification*).

## Tests

The policy is the pipeline plugin's `briefs/build.md` → *Build cadence*; its test-first set maps
onto quarry in `.claude/briefs/repo.md` → *For architect and developer*. Invoke `pipeline:tdd` +
`go-testing` when writing or modifying tests (enforced by `pipeline:test-reviewer`).

## Code quality conventions

`refactor-advisor` enforces patterns from `.claude/refactor-catalog.md` — _Comment as a missing name_, _Compose method_, _Feature envy → Move method_, others.

## Toolchain

Bash tool calls already run inside pinned nix/devenv environment, so `go` and `golangci-lint` resolve to pinned versions. Run directly:

```bash
go version
```

Always confirm `go version` matches pin in `devenv.nix` before trusting result.

Sandbox (denies worktree reads — first Bash call unsandboxed), `/nix/store` paths, and verification commands: `.claude/rules/agent-briefs.md`.