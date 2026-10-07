package cli_test

import (
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const claudeGroupHelp = `Install quarry's plugin in Claude Code, or remove it. quarry does this by
running the claude command, so claude must be on your PATH.

Usage:
  quarry claude [flags]
  quarry claude [command]

Available Commands:
  install     Install quarry's plugin (skill and MCP server) in Claude Code
  uninstall   Uninstall quarry's plugin from Claude Code
`

func Test_claude_refuses_an_unknown_subcommand(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "a bare unknown name", args: []string{"claude", "bogus"}},
		{name: "an unknown name after the root --json flag", args: []string{"claude", "--json", "bogus"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tool := &toolCalls{}

			stdout, _, err := runClaude(t, tool, c.args...)

			var usage cli.UsageError
			require.ErrorAs(t, err, &usage)
			assert.Equal(t, `unknown command "bogus" for "quarry claude"; Run 'quarry claude --help' for usage.`, usage.Error())
			assert.Empty(t, stdout)
			assert.Empty(t, tool.argv)
		})
	}
}

func Test_claude_prints_its_group_help_on_stdout(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "bare", args: []string{"claude"}},
		{name: "with the root --json flag", args: []string{"claude", "--json"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tool := &toolCalls{}

			stdout, stderr, err := runClaude(t, tool, c.args...)

			require.NoError(t, err)
			assert.Empty(t, stderr)
			assert.Contains(t, stdout, claudeGroupHelp)
			assert.Empty(t, tool.argv)
		})
	}
}
