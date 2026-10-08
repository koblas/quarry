package cli_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const claudeInstallLong = `Install quarry in Claude Code and in Claude Desktop, whichever of the two
is on this Mac; install skips the other and says so.

Claude Code counts as present when the claude command is on your PATH.
install adds quarry's plugin for all your projects: the quarry skill,
which teaches Claude to answer from quarry, and quarry's MCP server. It
runs:

  claude plugin marketplace add --scope user koblas/quarry
  claude plugin install --scope user quarry@quarry

Claude Code downloads the plugin from github.com/koblas/quarry; quarry
itself sends nothing and opens none of Claude Code's files. The plugin
starts "quarry" from your PATH.

Claude Desktop counts as present when ~/Library/Application Support/Claude
exists. install adds quarry's MCP server, not the skill, as the "quarry"
entry under mcpServers in claude_desktop_config.json in that folder,
with the full path to quarry. Every other setting stays as it was, though
the file's layout may change, and the file it replaces is saved as
claude_desktop_config.json.before-quarry. Quit Claude Desktop first: it
can rewrite the file while it runs.

Each step already done is skipped, so running install again is safe.
Restart Claude Code, or quit and reopen Claude Desktop, to load quarry.
`

const claudeUninstallLong = `Uninstall quarry from Claude Code and from Claude Desktop, whichever of
the two is on this Mac; uninstall skips the other and says so.

In Claude Code, uninstall removes quarry's plugin from your user scope
and the quarry marketplace. It runs:

  claude plugin uninstall --scope user quarry@quarry
  claude plugin marketplace remove --scope user quarry

A copy of the plugin installed for a single project stays, and uninstall
names each one.

In Claude Desktop, uninstall removes the "quarry" entry under mcpServers
in ~/Library/Application Support/Claude/claude_desktop_config.json when
that entry runs quarry mcp. Every other setting stays as it was, though
the file's layout may change, and the file it replaces is saved as
claude_desktop_config.json.before-quarry. Quit Claude Desktop first.

Each step with nothing to remove is skipped. Your quarry store and
snapshots are not touched.
`

const mcpHelpLong = `Run quarry as a local MCP server for Claude and other MCP clients. The
client starts it and talks to it over stdin and stdout; quarry opens no
network port.

In Claude Code or Claude Desktop, run quarry claude install: in Claude
Code it installs quarry's plugin, which adds this server and the quarry
skill; in Claude Desktop it adds this server with quarry's full path. In
other MCP clients, add a server with the command "quarry" and the
argument "mcp". An app started outside a terminal may not find quarry on
your PATH; give it the full path that "command -v quarry" prints.

The server reads quarry's store; it never runs quarry sync, never prunes
snapshots and never touches Quicken. Each request reads the store as it
is then, so after you run quarry sync the client sees the new data
without a restart. SQL runs read-only, and every list a tool returns
stops at 500 entries.

Payee names, memos, account names and category names reach the client
as Quicken holds them; quarry does not rewrite or mask them.

Tools: describe_schema, query, sync_status, data_quality, spending,
cash_flow, recurring_charges, anomalies, search_transactions, holdings,
net_worth, acb, monthly_summary.
`

func Test_help_names_both_claude_targets(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "claude", args: []string{"claude", "--help"}, want: claudeGroupHelp},
		{name: "claude install", args: []string{"claude", "install", "--help"}, want: claudeInstallLong},
		{name: "claude uninstall", args: []string{"claude", "uninstall", "--help"}, want: claudeUninstallLong},
		{name: "mcp", args: []string{"mcp", "--help"}, want: mcpHelpLong},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tool := &toolCalls{}

			stdout, stderr, err := runClaude(t, tool, c.args...)

			require.NoError(t, err)
			assert.Empty(t, stderr)
			assert.True(t, strings.HasPrefix(stdout, c.want), stdout)
			assert.Empty(t, tool.argv)
		})
	}
}
