package cli_test

import (
	"io"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/platform/toolrun"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	pluginUninstalledLine  = "Uninstalled the quarry plugin from Claude Code.\n"
	pluginAbsentLine       = "The quarry plugin is not installed for all your projects.\n"
	marketplaceRemovedLine = "Removed the quarry marketplace from Claude Code.\n"
	marketplaceAbsentLine  = "The quarry marketplace is not in Claude Code.\n"
	uninstallRestartLine   = "Restart Claude Code to unload it.\n"
)

func Test_claude_uninstall_runs_both_steps_when_both_are_present(t *testing.T) {
	tool := (&toolCalls{}).lists(ourMarketplace, userPluginOn)

	stdout, stderr, err := runClaude(t, tool, "claude", "uninstall")

	require.NoError(t, err)
	assert.Equal(t, []string{marketplaceListArgv, pluginListArgv, uninstallPluginArgv, removeMarketplaceArgv}, tool.argv)
	assert.Equal(t, "Uninstalled the quarry plugin from Claude Code.\n"+
		"Removed the quarry marketplace from Claude Code.\n"+
		"Restart Claude Code to unload it.\n", stdout)
	assert.Empty(t, stderr)
}

func Test_claude_uninstall_skips_both_steps_when_nothing_is_installed(t *testing.T) {
	tool := (&toolCalls{}).lists("[]", "[]")

	stdout, stderr, err := runClaude(t, tool, "claude", "uninstall")

	require.NoError(t, err)
	assert.Equal(t, []string{marketplaceListArgv, pluginListArgv}, tool.argv)
	assert.Equal(t, pluginAbsentLine+marketplaceAbsentLine, stdout)
	assert.Empty(t, stderr)
}

func Test_claude_uninstall_reports_a_partial_uninstall_when_the_marketplace_step_fails(t *testing.T) {
	tool := (&toolCalls{}).lists(ourMarketplace, userPluginOn).
		reply(removeMarketplaceArgv, toolReply{output: "denied\n", status: 1})

	stdout, stderr, err := runClaude(t, tool, "claude", "uninstall")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, pluginUninstalledLine, stdout)
	assert.Equal(t, "denied\n"+
		"quarry: claude uninstall: uninstalled the quarry plugin, but claude plugin marketplace remove --scope user quarry "+
		"exited with status 1; see its message above, then run quarry claude uninstall again\n", stderr)
}

func Test_claude_uninstall_refuses_when_claude_is_not_on_the_path(t *testing.T) {
	tool := &toolCalls{}
	lookPath := func(string) (string, error) { return "", &exec.Error{Name: "claude", Err: exec.ErrNotFound} }

	stdout, stderr, err := runClaudeFinding(t, tool, lookPath, "claude", "uninstall")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Empty(t, tool.argv)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: claude uninstall: cannot find the claude command on your PATH; "+
		"add it to your PATH, then run quarry claude uninstall again\n", stderr)
}

func Test_claude_uninstall_refuses_json(t *testing.T) {
	tool := &toolCalls{}

	stdout, _, err := runClaude(t, tool, "claude", "uninstall", "--json")

	var usage cli.UsageError
	require.ErrorAs(t, err, &usage)
	assert.Equal(t, "claude uninstall prints no JSON; drop --json", usage.Error())
	assert.Empty(t, tool.argv)
	assert.Empty(t, stdout)
}

func Test_claude_uninstall_refuses_a_foreign_quarry_marketplace(t *testing.T) {
	tool := (&toolCalls{}).lists(foreignMarketplace, userPluginOn)

	stdout, stderr, err := runClaude(t, tool, "claude", "uninstall")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, []string{marketplaceListArgv, pluginListArgv}, tool.argv)
	assert.Empty(t, stdout)
	assert.Equal(t, `quarry: claude uninstall: Claude Code has a marketplace named "quarry" that is not koblas/quarry on GitHub, `+
		"so quarry leaves it and its plugin alone; to remove them, run claude plugin uninstall quarry@quarry, "+
		"then claude plugin marketplace remove quarry\n", stderr)
}

func Test_claude_uninstall_reports_each_outcome(t *testing.T) {
	cases := []struct {
		name       string
		tool       *toolCalls
		wantStdout string
	}{
		{
			name:       "plugin absent, marketplace ours",
			tool:       (&toolCalls{}).lists(ourMarketplace, "[]"),
			wantStdout: pluginAbsentLine + marketplaceRemovedLine + uninstallRestartLine,
		},
		{
			name:       "plugin present, marketplace absent",
			tool:       (&toolCalls{}).lists("[]", userPluginOn),
			wantStdout: pluginUninstalledLine + marketplaceAbsentLine + uninstallRestartLine,
		},
		{
			name:       "turned-off user copy is uninstalled without a turned-off hint",
			tool:       (&toolCalls{}).lists(ourMarketplace, userPluginOff),
			wantStdout: pluginUninstalledLine + marketplaceRemovedLine + uninstallRestartLine,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, err := runClaude(t, c.tool, "claude", "uninstall")

			require.NoError(t, err)
			assert.Equal(t, c.wantStdout, stdout)
			assert.Empty(t, stderr)
		})
	}
}

func Test_claude_uninstall_reports_each_failure(t *testing.T) {
	const (
		claudeAt  = "/home/ada/bin/claude"
		cannotRun = `cannot run claude at "~/bin/claude" (permission denied); ` +
			"check that it is Claude Code and that you can run it, then run quarry claude uninstall again\n"
	)
	killed := func(output string) toolReply {
		return toolReply{output: output, status: -1, err: &toolrun.SignalError{Signal: syscall.SIGKILL}}
	}
	cases := []struct {
		name       string
		tool       *toolCalls
		wantStdout string
		wantStderr string
	}{
		{
			name: "first step, exit status, output",
			tool: (&toolCalls{}).lists(ourMarketplace, userPluginOn).reply(uninstallPluginArgv, toolReply{output: "nope\n", status: 2}),
			wantStderr: "nope\n" +
				"quarry: claude uninstall: claude plugin uninstall --scope user quarry@quarry exited with status 2; see its message above\n",
		},
		{
			name:       "partial, exit status, no output",
			tool:       (&toolCalls{}).lists(ourMarketplace, userPluginOn).reply(removeMarketplaceArgv, toolReply{status: 1}),
			wantStdout: pluginUninstalledLine,
			wantStderr: "quarry: claude uninstall: uninstalled the quarry plugin, but claude plugin marketplace remove --scope user quarry " +
				"exited with status 1, then run quarry claude uninstall again\n",
		},
		{
			name:       "partial, signal, output",
			tool:       (&toolCalls{}).lists(ourMarketplace, userPluginOn).reply(removeMarketplaceArgv, killed("lost\n")),
			wantStdout: pluginUninstalledLine,
			wantStderr: "lost\n" +
				"quarry: claude uninstall: uninstalled the quarry plugin, but claude plugin marketplace remove --scope user quarry " +
				"was stopped by signal killed; see its message above, then run quarry claude uninstall again\n",
		},
		{
			name: "second step with the first skipped, exit status, output",
			tool: (&toolCalls{}).lists(ourMarketplace, "[]").reply(removeMarketplaceArgv, toolReply{output: "nope\n", status: 2}),
			wantStderr: "nope\n" +
				"quarry: claude uninstall: claude plugin marketplace remove --scope user quarry exited with status 2; see its message above\n",
		},
		{
			name:       "marketplace list, exit status, no output",
			tool:       (&toolCalls{}).reply(marketplaceListArgv, toolReply{status: 1}),
			wantStderr: "quarry: claude uninstall: claude plugin marketplace list --json exited with status 1\n",
		},
		{
			name: "plugin list that is not JSON",
			tool: (&toolCalls{}).lists(ourMarketplace, "not json"),
			wantStderr: "quarry: claude uninstall: cannot read what claude plugin list --json printed; " +
				"update Claude Code, or run the two commands in quarry claude uninstall --help yourself\n",
		},
		{
			name: "marketplace list that is not JSON",
			tool: (&toolCalls{}).lists("not json", userPluginOn),
			wantStderr: "quarry: claude uninstall: cannot read what claude plugin marketplace list --json printed; " +
				"update Claude Code, or run the two commands in quarry claude uninstall --help yourself\n",
		},
		{
			name: "plugin list, exit status, output",
			tool: (&toolCalls{}).lists(ourMarketplace, "").reply(pluginListArgv, toolReply{output: "boom", status: 2}),
			wantStderr: "boom\n" +
				"quarry: claude uninstall: claude plugin list --json exited with status 2; see its message above\n",
		},
		{
			name: "interrupted during a step",
			tool: (&toolCalls{}).lists(ourMarketplace, userPluginOn).reply(uninstallPluginArgv, toolReply{cancel: true, ctxErr: true}),
			wantStderr: "quarry: claude uninstall: stopped before claude plugin uninstall --scope user quarry@quarry finished; " +
				"run quarry claude uninstall again\n",
		},
		{
			name:       "interrupted between the steps",
			tool:       (&toolCalls{}).lists(ourMarketplace, userPluginOn).reply(uninstallPluginArgv, toolReply{cancel: true}),
			wantStdout: pluginUninstalledLine,
			wantStderr: "quarry: claude uninstall: stopped before claude plugin marketplace remove --scope user quarry finished; " +
				"run quarry claude uninstall again\n",
		},
		{
			name: "interrupted before the command runs",
			tool: &toolCalls{cancelBefore: true},
			wantStderr: "quarry: claude uninstall: stopped before claude plugin marketplace list --json finished; " +
				"run quarry claude uninstall again\n",
		},
		{
			name:       "claude that cannot run, first step",
			tool:       (&toolCalls{}).reply(marketplaceListArgv, toolReply{err: cannotStart(claudeAt)}),
			wantStderr: "quarry: claude uninstall: " + cannotRun,
		},
		{
			name:       "claude that cannot run after the plugin uninstall",
			tool:       (&toolCalls{}).lists(ourMarketplace, userPluginOn).reply(removeMarketplaceArgv, toolReply{err: cannotStart(claudeAt)}),
			wantStdout: pluginUninstalledLine,
			wantStderr: "quarry: claude uninstall: uninstalled the quarry plugin, but " + cannotRun,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, err := runClaudeAt(t, c.tool, findsAt(claudeAt), "/home/ada", "claude", "uninstall")

			require.ErrorIs(t, err, cli.ReportedError{})
			assert.Equal(t, c.wantStdout, stdout)
			assert.Equal(t, c.wantStderr, stderr)
		})
	}
}

func Test_claude_uninstall_returns_the_runners_own_error_as_a_runtime_failure(t *testing.T) {
	tool := (&toolCalls{}).reply(marketplaceListArgv, toolReply{err: errNoStart})

	_, _, err := runClaude(t, tool, "claude", "uninstall")

	require.ErrorIs(t, err, errNoStart)
	assert.NotErrorAs(t, err, new(cli.UsageError))
}

func Test_claude_uninstall_returns_the_stdout_write_error_when_the_result_cannot_be_printed(t *testing.T) {
	env := cli.Env{Stdout: failingWriter{err: errNoSpace}, Stderr: io.Discard, RunTool: (&toolCalls{}).run}

	err := cli.Execute(t.Context(), []string{"claude", "uninstall"}, env)

	require.ErrorIs(t, err, errNoSpace)
	assert.NotErrorAs(t, err, new(cli.UsageError))
}

func Test_claude_uninstall_refuses_arguments(t *testing.T) {
	tool := &toolCalls{}

	_, _, err := runClaude(t, tool, "claude", "uninstall", "extra")

	var usage cli.UsageError
	require.ErrorAs(t, err, &usage)
	assert.Equal(t, "uninstall takes no arguments; Run 'quarry claude uninstall --help' for usage.", usage.Error())
	assert.Empty(t, tool.argv)
}

func Test_claude_uninstall_help_prints_the_ruled_text(t *testing.T) {
	const long = `Uninstall quarry's Claude Code plugin from your user scope and remove the
quarry marketplace. uninstall runs:

  claude plugin uninstall --scope user quarry@quarry
  claude plugin marketplace remove --scope user quarry

skipping each step with nothing to remove. A copy of the plugin installed
for a single project stays, and uninstall names each one. Your quarry
store and snapshots are not touched.
`
	tool := &toolCalls{}

	stdout, _, err := runClaude(t, tool, "claude", "uninstall", "--help")
	require.NoError(t, err)
	group, _, groupErr := runClaude(t, tool, "claude", "--help")

	require.NoError(t, groupErr)
	assert.True(t, strings.HasPrefix(stdout, long), stdout)
	assert.Contains(t, group, "  uninstall   Uninstall quarry's plugin from Claude Code\n")
	assert.Empty(t, tool.argv)
}
