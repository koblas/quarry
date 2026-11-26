# Security

quarry reads one person's Quicken Classic for Mac file and serves it to a terminal, to scripts and
to Claude. What it holds is personal financial history: every account, balance, payee, memo and
investment the user has recorded, often over decades. This file lists what is worth attacking and
the rules that defend it. It is for security reviews; build rules live in `.claude/`.

## Reporting a vulnerability

Do not open a public issue. Use GitHub's private vulnerability reporting on this repository
(Security → Report a vulnerability). Include the commit or release you ran, the command or
MCP call, and what an attacker gains.

## What is worth protecting

| Asset | Where it lives | Rule |
| --- | --- | --- |
| The live Quicken file | `<name>.quicken/data` | Never written. Touched only by SQLite's online backup API, read-only, during `quarry sync`. |
| Snapshots | `~/Library/Application Support/quarry/snapshots/` | Created `0600` in a `0700` folder, exclusively created and moved into place atomically; integrity-checked and rejected (deleted at once) when empty or corrupt; capped by count. |
| The store | `~/Library/Application Support/quarry/quarry.duckdb` | Rebuilt from a snapshot, swapped in atomically; every read session opens it read-only with outside access off. |
| The config file | `~/Library/Application Support/quarry/config.toml` | quarry reads it and never writes it. It may hold account numbers the user typed. |
| Claude Desktop's config | `~/Library/Application Support/Claude/claude_desktop_config.json` | `quarry claude install` / `uninstall` change only the `mcpServers.quarry` entry, keep a backup of the file they replace, and refuse a file they cannot parse or an entry they do not own. |

## Account-number masking

quarry imports no account-number field from Quicken. An account number the user writes into
quarry's config is shown masked to its last four digits in everything quarry prints or serves, CLI
and MCP alike (`internal/platform/accountmask`). quarry's own ids (`acct-<n>`) are labels, not
numbers, and are not masked. Free text (payees, memos, notes) is passed through as Quicken holds
it, because the institution's download and Quicken already mask card and account numbers there;
that is a recorded decision, not a gap (see `docs/product/decisions.md`, Redaction).

## The MCP surface

- Local stdio only: `quarry mcp` opens no network listener.
- `query` runs SQL on a read-only connection with external access disabled, a row cap (default
  500) and a timeout, so a stray `SELECT *` cannot dump the whole history into a conversation.
- The server never runs `sync` and never prunes snapshots.
- Everything a tool returns comes from the user's own Quicken file, which can hold any text a bank
  or payee put there. The skill and tool descriptions tell Claude to treat it as data, never as
  instructions. A payee or memo that tries to steer Claude is the injection path to review.
- Names from the Quicken file are Go-quoted in warnings, so a quote, newline or escape character
  cannot split a line or forge a `quarry:` prefix.

## Network

The only outbound call is `quarry sync` fetching USD/CAD rates from the Bank of Canada. The
request carries only the series and date range, never user data. No telemetry.

## Sensitive paths

A change under any of these paths gets a security review before it merges. The list is the
`security_paths` pattern in `.claude/pipeline.json`; keep the two in step.

- `internal/platform/atomicfile/`, `internal/platform/replacefile/`, `internal/platform/lockfile/`
- `internal/platform/accountmask/`
- `internal/snapshot/`
- `internal/claudedesktop/`

Also review any change to the read-only DuckDB connection (`internal/platform/duckdb/`), the MCP
`query` tool (`internal/mcp/`), or the Bank of Canada fetch (`internal/fx/`).

## Reviews

A security review runs before each release and on any pull request that touches a sensitive path.
It is not part of the feature gate. Each confirmed finding becomes an issue labelled `security`.
Each finding reasoned to be a non-issue goes in `docs/security/known-non-issues.md` so it is not
reported again.

## Not in scope

FileVault and the Mac's own account security; Quicken's own encryption of its file; a user who
gives Claude shell access and asks it to read the store directly.
