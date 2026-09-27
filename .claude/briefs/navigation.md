# Brief: code navigation

For agents carrying `LSP` tool: `triage`, `architect`, `developer`, `arch-reviewer`, `correctness-reviewer`. Read with `.claude/rules/agent-briefs.md` (core).

## LSP first for Go

For "who calls this", "what implements this", "where is this defined", use `LSP` tool (gopls, from `gopls-lsp` plugin enabled in `.claude/settings.json`) before `Grep`. gopls sees whole module and resolves through interfaces — answer complete where text match is not (method called through interface, renamed import, `rwfs` or `dirFS` adapter reached only via its port). Positions are 1-based line and character.

- **Implementers of `Store`, `rwfs.FS`, `dirFS` or other port method:** `goToImplementation` on interface method — returns memory and OS adapters and any fake.
- **Callers of function or method:** `findReferences`, or `incomingCalls` after `prepareCallHierarchy`.
- **Anchor symbol you know only by name:** `workspaceSymbol` with query, then work from its position.

Where LSP does not reach, grep — and say which rows of caller table came from which:

- **Strings, not symbols:** flag names, config keys, output copy, `--json` field names, file names written to disk. Grep `cmd/` and `internal/`, tests included.
- **"Same shape elsewhere" sweeps** (pattern, not symbol) are grep by nature.
- **LSP zero needs control.** gopls not started → every query returns error or nothing. Before reporting "no callers", confirm same query finds symbol you know is called.
