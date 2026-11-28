# How to use quarry with Claude

quarry ships a Claude Code plugin: a skill that teaches Claude to answer questions from your Quicken data with quarry, and the config for quarry's MCP server. The plugin runs the `quarry` binary from your `PATH`.

## 1. Check quarry works in a terminal

```bash
command -v quarry
```

prints the path; if it prints nothing, add `$(go env GOPATH)/bin` to your `PATH`.

```bash
quarry status
```

If there is no store yet, open your Quicken file and run `quarry sync`.

## 2. Install

```bash
quarry claude install
```

It sets up Claude Code and Claude Desktop, whichever is on this Mac, and skips the other and says so. Each step already done is skipped, so running it again is safe.

=== "Claude Code"

    `quarry claude install` runs these two commands for you; you can run them yourself instead:

    ```bash
    claude plugin marketplace add --scope user koblas/quarry
    claude plugin install --scope user quarry@quarry
    ```

    Restart Claude Code to load the plugin.

=== "Claude Desktop"

    `quarry claude install` adds quarry's MCP server, not the skill, as the `quarry` entry under `mcpServers` in `~/Library/Application Support/Claude/claude_desktop_config.json`, with the full path to quarry. Every other setting stays as it was. Whenever it changes the file, it saves the file it replaces as `claude_desktop_config.json.before-quarry` beside it.

    !!! warning "Quit Claude Desktop first"
        Desktop can rewrite that file while it runs. Quit it before you run `quarry claude install` or `quarry claude uninstall`, then reopen it.

=== "MCP server only"

    To add only the MCP server to Claude Code, without the skill:

    ```bash
    claude mcp add --scope user quarry -- quarry mcp
    ```

    In other MCP clients, add a server with the command `quarry` and the argument `mcp`. An app started outside a terminal may not find quarry on your `PATH`; give it the full path that `command -v quarry` prints.

## 3. Ask a question

Ask Claude a question such as "How did our grocery spending change since 2022?" or "Which subscriptions started this year?", or type `/quarry:quarry` to load the skill yourself. Claude checks how fresh the data is with `quarry status`, answers from quarry's output, and runs `quarry sync` only when you ask.

!!! note "What Claude sees"
    The output of the commands Claude runs becomes part of your conversation with Claude, so ask for totals rather than full transaction lists when that is all you need. See [What leaves your Mac](../concepts/how-quarry-works.md#what-leaves-your-mac).

## Get a monthly summary

`quarry summary` prints last month's unusual charges, new recurring charges, net worth change and findings. To get it every month, follow [the monthly summary recipe](https://github.com/koblas/quarry/blob/main/plugin/skills/quarry/references/monthly-summary.md).

## Update

```bash
claude plugin marketplace update quarry
```

Update the quarry binary at the same time; if Claude reports that quarry is older than the skill, update quarry.

## Remove

```bash
quarry claude uninstall
```

In Claude Code it runs the reverse of install, and leaves your quarry store alone:

```bash
claude plugin uninstall --scope user quarry@quarry
claude plugin marketplace remove --scope user quarry
```

A copy of the plugin installed only for a project, or by your organization, stays; `quarry claude uninstall` names each one, and does not remove the quarry marketplace while any remains. In Claude Desktop it removes the `quarry` entry under `mcpServers` and nothing else.
