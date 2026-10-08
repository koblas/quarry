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
	marketplaceKeptLine    = "Kept the quarry marketplace: the quarry plugin is still installed elsewhere and needs it.\n"
	uninstallRestartLine   = "Restart Claude Code to unload it.\n"
)

func Test_claude_uninstall_runs_both_steps_when_both_are_present(t *testing.T) {
	tool := (&toolCalls{}).lists(ourMarketplace, userPluginOn)

	stdout, stderr, err := runClaude(t, tool, "claude", "uninstall")

	require.NoError(t, err)
	assert.Equal(t, []string{marketplaceListArgv, pluginListArgv, uninstallPluginArgv, removeMarketplaceArgv}, tool.argv)
	assert.Equal(t, "Uninstalled the quarry plugin from Claude Code.\n"+
		"Removed the quarry marketplace from Claude Code.\n"+
		"Restart Claude Code to unload it.\n"+desktopSkippedLine, stdout)
	assert.Empty(t, stderr)
}

func Test_claude_uninstall_keeps_the_marketplace_for_a_project_copy(t *testing.T) {
	home := t.TempDir()
	tool := (&toolCalls{}).lists(ourMarketplace, `[{"id":"quarry@quarry","scope":"user","enabled":true},`+projectPluginUnder(home)+`]`)

	stdout, stderr, err := runClaudeAt(t, tool, nil, home, "claude", "uninstall")

	require.NoError(t, err)
	assert.Equal(t, []string{marketplaceListArgv, pluginListArgv, uninstallPluginArgv}, tool.argv)
	assert.Equal(t, pluginUninstalledLine+marketplaceKeptLine+uninstallRestartLine+desktopSkippedLine, stdout)
	assert.Equal(t, `quarry: claude uninstall: the quarry plugin is still installed for project "~/repos/foo"; `+
		"to remove it, run claude plugin uninstall --scope project quarry@quarry in that directory\n", stderr)
}

func Test_claude_uninstall_shows_a_project_path_outside_home_unabbreviated(t *testing.T) {
	tool := (&toolCalls{}).lists(ourMarketplace, `[{"id":"quarry@quarry","scope":"project","projectPath":"/srv/x"}]`)

	_, stderr, err := runClaudeAt(t, tool, nil, "", "claude", "uninstall")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, `quarry: claude uninstall: the quarry plugin is still installed for project "/srv/x"; `+
		"to remove it, run claude plugin uninstall --scope project quarry@quarry in that directory\n"+desktopNoHomeUninstallLine, stderr)
}

// tempHome is a hints-table home: a fresh directory holding no Claude Desktop folder.
func tempHome(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func unsetHome(*testing.T) string { return "" }

func Test_claude_uninstall_hints_each_remaining_copy(t *testing.T) {
	namedHint := func(quotedPath, scope string) string {
		return "quarry: claude uninstall: the quarry plugin is still installed for project " + quotedPath +
			"; to remove it, run claude plugin uninstall --scope " + scope + " quarry@quarry in that directory\n"
	}
	unnamedHint := func(scope string) string {
		return "quarry: claude uninstall: the quarry plugin is still installed for a project Claude Code did not name; " +
			"to remove it, run claude plugin uninstall --scope " + scope + " quarry@quarry in that project's directory\n"
	}
	otherScopeHint := func(scope string) string {
		return "quarry: claude uninstall: the quarry plugin is still installed at scope " + scope +
			", which claude plugin uninstall cannot remove from; it stays until whoever manages that scope removes it\n"
	}
	cases := []struct {
		name       string
		tool       func(home string) *toolCalls
		home       func(t *testing.T) string
		wantArgv   []string
		wantStdout string
		wantStderr string
		wantErr    error
	}{
		{
			name: "project copy alone keeps the marketplace and prints no restart line",
			tool: func(home string) *toolCalls {
				return (&toolCalls{}).lists(ourMarketplace, "["+projectPluginUnder(home)+"]")
			},
			home:       tempHome,
			wantArgv:   []string{marketplaceListArgv, pluginListArgv},
			wantStdout: pluginAbsentLine + marketplaceKeptLine + desktopSkippedLine,
			wantStderr: namedHint(`"~/repos/foo"`, "project"),
		},
		{
			name:       "project copy without our marketplace prints no kept line",
			tool:       func(home string) *toolCalls { return (&toolCalls{}).lists("[]", "["+projectPluginUnder(home)+"]") },
			home:       tempHome,
			wantArgv:   []string{marketplaceListArgv, pluginListArgv},
			wantStdout: pluginAbsentLine + marketplaceAbsentLine + desktopSkippedLine,
			wantStderr: namedHint(`"~/repos/foo"`, "project"),
		},
		{
			name: "a user copy beside a project copy without our marketplace uninstalls the plugin and prints no kept line",
			tool: func(home string) *toolCalls {
				return (&toolCalls{}).lists("[]", `[{"id":"quarry@quarry","scope":"user","enabled":true},`+projectPluginUnder(home)+`]`)
			},
			home:       tempHome,
			wantArgv:   []string{marketplaceListArgv, pluginListArgv, uninstallPluginArgv},
			wantStdout: pluginUninstalledLine + marketplaceAbsentLine + uninstallRestartLine + desktopSkippedLine,
			wantStderr: namedHint(`"~/repos/foo"`, "project"),
		},
		{
			name: "copies are named in the order claude listed them",
			tool: func(home string) *toolCalls {
				return (&toolCalls{}).lists(ourMarketplace, `[`+
					`{"id":"quarry@quarry","scope":"project","projectPath":"`+home+`/b"},`+
					`{"id":"quarry@quarry","scope":"local"},`+
					`{"id":"quarry@quarry","scope":"project","projectPath":"`+home+`/a"}]`)
			},
			home:       tempHome,
			wantArgv:   []string{marketplaceListArgv, pluginListArgv},
			wantStdout: pluginAbsentLine + marketplaceKeptLine + desktopSkippedLine,
			wantStderr: namedHint(`"~/b"`, "project") + unnamedHint("local") + namedHint(`"~/a"`, "project"),
		},
		{
			name: "a copy with no projectPath key is a project claude did not name",
			tool: func(string) *toolCalls {
				return (&toolCalls{}).lists(ourMarketplace, `[{"id":"quarry@quarry","scope":"local"}]`)
			},
			home:       tempHome,
			wantArgv:   []string{marketplaceListArgv, pluginListArgv},
			wantStdout: pluginAbsentLine + marketplaceKeptLine + desktopSkippedLine,
			wantStderr: unnamedHint("local"),
		},
		{
			name: "a copy with an empty projectPath is a project claude did not name",
			tool: func(string) *toolCalls {
				return (&toolCalls{}).lists(ourMarketplace, `[{"id":"quarry@quarry","scope":"project","projectPath":""}]`)
			},
			home:       tempHome,
			wantArgv:   []string{marketplaceListArgv, pluginListArgv},
			wantStdout: pluginAbsentLine + marketplaceKeptLine + desktopSkippedLine,
			wantStderr: unnamedHint("project"),
		},
		{
			name: "the scope claude reported is printed verbatim",
			tool: func(home string) *toolCalls {
				return (&toolCalls{}).lists(ourMarketplace, `[{"id":"quarry@quarry","scope":"local","projectPath":"`+home+`/x"}]`)
			},
			home:       tempHome,
			wantArgv:   []string{marketplaceListArgv, pluginListArgv},
			wantStdout: pluginAbsentLine + marketplaceKeptLine + desktopSkippedLine,
			wantStderr: namedHint(`"~/x"`, "local"),
		},
		{
			name: "a project at home itself is shown as ~",
			tool: func(home string) *toolCalls {
				return (&toolCalls{}).lists(ourMarketplace, `[{"id":"quarry@quarry","scope":"project","projectPath":"`+home+`"}]`)
			},
			home:       tempHome,
			wantArgv:   []string{marketplaceListArgv, pluginListArgv},
			wantStdout: pluginAbsentLine + marketplaceKeptLine + desktopSkippedLine,
			wantStderr: namedHint(`"~"`, "project"),
		},
		{
			name: "a project outside home is shown as claude printed it",
			tool: func(string) *toolCalls {
				return (&toolCalls{}).lists(ourMarketplace, `[{"id":"quarry@quarry","scope":"project","projectPath":"/srv/x"}]`)
			},
			home:       tempHome,
			wantArgv:   []string{marketplaceListArgv, pluginListArgv},
			wantStdout: pluginAbsentLine + marketplaceKeptLine + desktopSkippedLine,
			wantStderr: namedHint(`"/srv/x"`, "project"),
		},
		{
			name: "an unset home shows a project under it unabbreviated",
			tool: func(string) *toolCalls {
				return (&toolCalls{}).lists(ourMarketplace, "["+projectPluginUnder("/home/ada")+"]")
			},
			home:       unsetHome,
			wantArgv:   []string{marketplaceListArgv, pluginListArgv},
			wantStdout: pluginAbsentLine + marketplaceKeptLine,
			wantStderr: namedHint(`"/home/ada/repos/foo"`, "project") + desktopNoHomeUninstallLine,
			wantErr:    cli.ReportedError{},
		},
		{
			name: "a path with quotes is abbreviated, then quoted",
			tool: func(home string) *toolCalls {
				return (&toolCalls{}).lists(ourMarketplace, `[{"id":"quarry@quarry","scope":"project","projectPath":"`+home+`/my \"dir\""}]`)
			},
			home:       tempHome,
			wantArgv:   []string{marketplaceListArgv, pluginListArgv},
			wantStdout: pluginAbsentLine + marketplaceKeptLine + desktopSkippedLine,
			wantStderr: namedHint(`"~/my \"dir\""`, "project"),
		},
		{
			name: "a managed copy gets the scope hint, not the project one",
			tool: func(string) *toolCalls {
				return (&toolCalls{}).lists(ourMarketplace, `[{"id":"quarry@quarry","scope":"managed"}]`)
			},
			home:       tempHome,
			wantArgv:   []string{marketplaceListArgv, pluginListArgv},
			wantStdout: pluginAbsentLine + marketplaceKeptLine + desktopSkippedLine,
			wantStderr: otherScopeHint(`"managed"`),
		},
		{
			name: "a scope hint ignores the projectPath claude reported",
			tool: func(home string) *toolCalls {
				return (&toolCalls{}).lists(ourMarketplace, `[{"id":"quarry@quarry","scope":"enterprise","projectPath":"`+home+`/x"}]`)
			},
			home:       tempHome,
			wantArgv:   []string{marketplaceListArgv, pluginListArgv},
			wantStdout: pluginAbsentLine + marketplaceKeptLine + desktopSkippedLine,
			wantStderr: otherScopeHint(`"enterprise"`),
		},
		{
			name: "a managed copy among project and local copies is named in list order",
			tool: func(home string) *toolCalls {
				return (&toolCalls{}).lists(ourMarketplace, `[`+
					`{"id":"quarry@quarry","scope":"project","projectPath":"`+home+`/a"},`+
					`{"id":"quarry@quarry","scope":"managed"},`+
					`{"id":"quarry@quarry","scope":"local"}]`)
			},
			home:       tempHome,
			wantArgv:   []string{marketplaceListArgv, pluginListArgv},
			wantStdout: pluginAbsentLine + marketplaceKeptLine + desktopSkippedLine,
			wantStderr: namedHint(`"~/a"`, "project") + otherScopeHint(`"managed"`) + unnamedHint("local"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := c.home(t)
			tool := c.tool(home)

			stdout, stderr, err := runClaudeAt(t, tool, nil, home, "claude", "uninstall")

			require.ErrorIs(t, err, c.wantErr)
			assert.Equal(t, c.wantArgv, tool.argv)
			assert.Equal(t, c.wantStdout, stdout)
			assert.Equal(t, c.wantStderr, stderr)
		})
	}
}

func Test_claude_uninstall_skips_both_steps_when_nothing_is_installed(t *testing.T) {
	tool := (&toolCalls{}).lists("[]", "[]")

	stdout, stderr, err := runClaude(t, tool, "claude", "uninstall")

	require.NoError(t, err)
	assert.Equal(t, []string{marketplaceListArgv, pluginListArgv}, tool.argv)
	assert.Equal(t, pluginAbsentLine+marketplaceAbsentLine+desktopSkippedLine, stdout)
	assert.Empty(t, stderr)
}

func Test_claude_uninstall_reports_a_partial_uninstall_when_the_marketplace_step_fails(t *testing.T) {
	tool := (&toolCalls{}).lists(ourMarketplace, userPluginOn).
		reply(removeMarketplaceArgv, toolReply{output: "denied\n", status: 1})

	stdout, stderr, err := runClaude(t, tool, "claude", "uninstall")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, pluginUninstalledLine+desktopSkippedLine, stdout)
	assert.Equal(t, "denied\n"+
		"quarry: claude uninstall: uninstalled the quarry plugin, but claude plugin marketplace remove --scope user quarry "+
		"exited with status 1; see its message above, then run quarry claude uninstall again\n", stderr)
}

// The helper's home has no Desktop folder, so neither target is present.
func Test_claude_uninstall_refuses_when_claude_is_not_on_the_path(t *testing.T) {
	tool := &toolCalls{}
	lookPath := func(string) (string, error) { return "", &exec.Error{Name: "claude", Err: exec.ErrNotFound} }

	stdout, stderr, err := runClaudeFinding(t, tool, lookPath, "claude", "uninstall")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Empty(t, tool.argv)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: claude uninstall: "+neitherPresentHead+desktopFolderShown+" does not exist"+neitherUninstallTail, stderr)
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
	assert.Equal(t, desktopSkippedLine, stdout)
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
			wantStdout: pluginAbsentLine + marketplaceRemovedLine + uninstallRestartLine + desktopSkippedLine,
		},
		{
			name:       "plugin present, marketplace absent",
			tool:       (&toolCalls{}).lists("[]", userPluginOn),
			wantStdout: pluginUninstalledLine + marketplaceAbsentLine + uninstallRestartLine + desktopSkippedLine,
		},
		{
			name:       "turned-off user copy is uninstalled without a turned-off hint",
			tool:       (&toolCalls{}).lists(ourMarketplace, userPluginOff),
			wantStdout: pluginUninstalledLine + marketplaceRemovedLine + uninstallRestartLine + desktopSkippedLine,
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
		claudeAt  = "/opt/bin/claude"
		cannotRun = `cannot run claude at "/opt/bin/claude" (permission denied); ` +
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
			name:       "first step, exit status, output",
			tool:       (&toolCalls{}).lists(ourMarketplace, userPluginOn).reply(uninstallPluginArgv, toolReply{output: "nope\n", status: 2}),
			wantStdout: desktopSkippedLine,
			wantStderr: "nope\n" +
				"quarry: claude uninstall: claude plugin uninstall --scope user quarry@quarry exited with status 2; see its message above\n",
		},
		{
			name:       "partial, exit status, no output",
			tool:       (&toolCalls{}).lists(ourMarketplace, userPluginOn).reply(removeMarketplaceArgv, toolReply{status: 1}),
			wantStdout: pluginUninstalledLine + desktopSkippedLine,
			wantStderr: "quarry: claude uninstall: uninstalled the quarry plugin, but claude plugin marketplace remove --scope user quarry " +
				"exited with status 1, then run quarry claude uninstall again\n",
		},
		{
			name:       "partial, signal, output",
			tool:       (&toolCalls{}).lists(ourMarketplace, userPluginOn).reply(removeMarketplaceArgv, killed("lost\n")),
			wantStdout: pluginUninstalledLine + desktopSkippedLine,
			wantStderr: "lost\n" +
				"quarry: claude uninstall: uninstalled the quarry plugin, but claude plugin marketplace remove --scope user quarry " +
				"was stopped by signal killed; see its message above, then run quarry claude uninstall again\n",
		},
		{
			name:       "second step with the first skipped, exit status, output",
			tool:       (&toolCalls{}).lists(ourMarketplace, "[]").reply(removeMarketplaceArgv, toolReply{output: "nope\n", status: 2}),
			wantStdout: desktopSkippedLine,
			wantStderr: "nope\n" +
				"quarry: claude uninstall: claude plugin marketplace remove --scope user quarry exited with status 2; see its message above\n",
		},
		{
			name:       "marketplace list, exit status, no output",
			tool:       (&toolCalls{}).reply(marketplaceListArgv, toolReply{status: 1}),
			wantStdout: desktopSkippedLine,
			wantStderr: "quarry: claude uninstall: claude plugin marketplace list --json exited with status 1\n",
		},
		{
			name:       "plugin list that is not JSON",
			tool:       (&toolCalls{}).lists(ourMarketplace, "not json"),
			wantStdout: desktopSkippedLine,
			wantStderr: "quarry: claude uninstall: cannot read what claude plugin list --json printed; " +
				"update Claude Code, or run the two commands in quarry claude uninstall --help yourself\n",
		},
		{
			name:       "marketplace list that is not JSON",
			tool:       (&toolCalls{}).lists("not json", userPluginOn),
			wantStdout: desktopSkippedLine,
			wantStderr: "quarry: claude uninstall: cannot read what claude plugin marketplace list --json printed; " +
				"update Claude Code, or run the two commands in quarry claude uninstall --help yourself\n",
		},
		{
			name:       "plugin list, exit status, output",
			tool:       (&toolCalls{}).lists(ourMarketplace, "").reply(pluginListArgv, toolReply{output: "boom", status: 2}),
			wantStdout: desktopSkippedLine,
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
			wantStdout: desktopSkippedLine,
			wantStderr: "quarry: claude uninstall: " + cannotRun,
		},
		{
			name:       "claude that cannot run after the plugin uninstall",
			tool:       (&toolCalls{}).lists(ourMarketplace, userPluginOn).reply(removeMarketplaceArgv, toolReply{err: cannotStart(claudeAt)}),
			wantStdout: pluginUninstalledLine + desktopSkippedLine,
			wantStderr: "quarry: claude uninstall: uninstalled the quarry plugin, but " + cannotRun,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()

			stdout, stderr, err := runClaudeAt(t, c.tool, findsAt(claudeAt), home, "claude", "uninstall")

			require.ErrorIs(t, err, cli.ReportedError{})
			assert.Equal(t, c.wantStdout, stdout)
			assert.Equal(t, c.wantStderr, stderr)
		})
	}
}

func Test_claude_uninstall_prints_an_unclassified_claude_failure_in_place(t *testing.T) {
	cases := []struct {
		name       string
		tool       *toolCalls
		wantStdout string
		wantStderr string
	}{
		{
			name:       "before any step ran",
			tool:       (&toolCalls{}).reply(marketplaceListArgv, toolReply{err: errNoStart}),
			wantStdout: desktopSkippedLine,
			wantStderr: "quarry: claude uninstall: fork/exec /opt/claude: permission denied\n",
		},
		{
			name:       "after the plugin uninstall",
			tool:       (&toolCalls{}).lists(ourMarketplace, userPluginOn).reply(removeMarketplaceArgv, toolReply{err: errNoStart}),
			wantStdout: pluginUninstalledLine + desktopSkippedLine,
			wantStderr: "quarry: claude uninstall: uninstalled the quarry plugin, but fork/exec /opt/claude: permission denied\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, err := runClaude(t, c.tool, "claude", "uninstall")

			require.ErrorIs(t, err, cli.ReportedError{})
			assert.Equal(t, c.wantStdout, stdout)
			assert.Equal(t, c.wantStderr, stderr)
		})
	}
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
	assert.True(t, strings.HasPrefix(stdout, long), stdout)
	assert.Empty(t, tool.argv)
}
