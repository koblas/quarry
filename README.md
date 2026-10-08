# quarry

`quarry` turns a Quicken Classic for Mac data file into a clean, local, queryable financial database. See `docs/initial-prd.md`.

## Use quarry with Claude Code or Claude Desktop

quarry ships a Claude Code plugin: a skill that teaches Claude to answer questions from your Quicken data with quarry, and the config for quarry's MCP server. The plugin runs the `quarry` binary from your PATH, so install quarry first and check it works in a terminal:

```
command -v quarry      # prints the path; if empty, add $(go env GOPATH)/bin to your PATH
quarry status          # if there is no store yet, open your Quicken file and run: quarry sync
```

Then add quarry to Claude Code and Claude Desktop, whichever you have:

```
quarry claude install
```

In Claude Code this runs `claude plugin marketplace add koblas/quarry` and `claude plugin install quarry@quarry` for you; you can run those two yourself instead. In Claude Desktop it adds quarry's MCP server; see below.

Ask Claude a question such as "How did our grocery spending change since 2022?" or "Which subscriptions started this year?", or type `/quarry:quarry` to load the skill yourself. Claude checks how fresh the data is with `quarry status`, answers from quarry's output, and runs `quarry sync` only when you ask.

quarry's only network request is the exchange-rate fetch during `quarry sync`, which carries nothing but dates, back to the date of your earliest transaction; `quarry claude install` has Claude Code download the plugin from GitHub, and quarry itself sends nothing. The output of the commands Claude runs becomes part of your conversation with Claude, so ask for totals rather than full transaction lists when that is all you need.

To update the plugin: `claude plugin marketplace update quarry`. Update the quarry binary at the same time; if Claude reports that quarry is older than the skill, update quarry.

## Install or remove quarry

In Claude Code, `quarry claude install` runs two commands for you, skipping each one that is already done, so running it again is safe:

```
claude plugin marketplace add --scope user koblas/quarry
claude plugin install --scope user quarry@quarry
```

Restart Claude Code to load the plugin. `quarry claude uninstall` runs the reverse, and leaves your quarry store alone:

```
claude plugin uninstall --scope user quarry@quarry
claude plugin marketplace remove --scope user quarry
```

A copy of the plugin installed only for a project, or by your organization, stays; `quarry claude uninstall` names each one, and does not remove the quarry marketplace while any remains.

In Claude Desktop, `quarry claude install` adds quarry's MCP server, not the skill, as the `quarry` entry under `mcpServers` in `~/Library/Application Support/Claude/claude_desktop_config.json`, with the full path to quarry, and saves the file it replaces as `claude_desktop_config.json.before-quarry` beside it. Every other setting stays as it was. Quit Claude Desktop before you run it, since Desktop can rewrite that file while it runs, then reopen Desktop to load quarry. `quarry claude uninstall` removes that entry and nothing else. If Claude Code or Claude Desktop is not on this Mac, both commands skip it and say so.

To add only the MCP server to Claude Code, without the skill, run:

```
claude mcp add --scope user quarry -- quarry mcp
```

## Run a monthly summary

`quarry summary` prints last month's unusual charges, new recurring charges, net worth change and findings. To get it every month, follow [plugin/skills/quarry/references/monthly-summary.md](plugin/skills/quarry/references/monthly-summary.md).

## Credits

`quarry` builds on the schema knowledge and SQL of two MIT-licensed projects, [dweekly/quicken-mac-mcp](https://github.com/dweekly/quicken-mac-mcp) and [hardkoded/quicken-skills](https://github.com/hardkoded/quicken-skills). Frozen copies live in `docs/prior-art/`; license text is in `THIRD_PARTY_NOTICES`.
