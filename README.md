# quarry

`quarry` turns a Quicken Classic for Mac data file into a clean, local, queryable financial database. See `docs/initial-prd.md`.

## Use quarry with Claude Code

quarry ships a Claude Code plugin: a skill that teaches Claude to answer questions from your Quicken data with quarry, and the config for quarry's MCP server. The plugin runs the `quarry` binary from your PATH, so install quarry first and check it works in a terminal:

```
command -v quarry      # prints the path; if empty, add $(go env GOPATH)/bin to your PATH
quarry status          # if there is no store yet, open your Quicken file and run: quarry sync
```

Then install the plugin:

```
quarry claude install
```

This runs `claude plugin marketplace add koblas/quarry` and `claude plugin install quarry@quarry` for you; you can run those two yourself instead.

Ask Claude a question such as "How did our grocery spending change since 2022?" or "Which subscriptions started this year?", or type `/quarry:quarry` to load the skill yourself. Claude checks how fresh the data is with `quarry status`, answers from quarry's output, and runs `quarry sync` only when you ask.

quarry's only network request is the exchange-rate fetch during `quarry sync`, which carries nothing but dates, back to the date of your earliest transaction; `quarry claude install` has Claude Code download the plugin from GitHub, and quarry itself sends nothing. The output of the commands Claude runs becomes part of your conversation with Claude, so ask for totals rather than full transaction lists when that is all you need.

To update the plugin: `claude plugin marketplace update quarry`. Update the quarry binary at the same time; if Claude reports that quarry is older than the skill, update quarry.

## Install or remove the plugin

`quarry claude install` runs two commands for you, skipping any that is already done, so running it again is safe:

```
claude plugin marketplace add --scope user koblas/quarry
claude plugin install --scope user quarry@quarry
```

Restart Claude Code to load the plugin. `quarry claude uninstall` runs the reverse, and leaves your quarry store alone:

```
claude plugin uninstall --scope user quarry@quarry
claude plugin marketplace remove --scope user quarry
```

If you installed the plugin in a project, that copy stays, and `quarry claude uninstall` names it so you can remove it yourself.

To add only the MCP server, without the skill, run:

```
claude mcp add --scope user quarry -- quarry mcp
```

## Run a monthly summary

`quarry summary` prints last month's unusual charges, new recurring charges, net worth change and findings. To get it every month, follow [plugin/skills/quarry/references/monthly-summary.md](plugin/skills/quarry/references/monthly-summary.md).

## Credits

`quarry` builds on the schema knowledge and SQL of two MIT-licensed projects, [dweekly/quicken-mac-mcp](https://github.com/dweekly/quicken-mac-mcp) and [hardkoded/quicken-skills](https://github.com/hardkoded/quicken-skills). Frozen copies live in `docs/prior-art/`; license text is in `THIRD_PARTY_NOTICES`.
