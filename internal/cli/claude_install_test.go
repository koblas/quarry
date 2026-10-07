package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/claudeplugin"
	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/platform/toolrun"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	marketplaceListArgv = "plugin marketplace list --json"
	pluginListArgv      = "plugin list --json"
	addMarketplaceArgv  = "plugin marketplace add --scope user koblas/quarry"
	installPluginArgv   = "plugin install --scope user quarry@quarry"

	ourMarketplace     = `[{"name":"quarry","source":"github","repo":"koblas/quarry"}]`
	foreignMarketplace = `[{"name":"quarry","source":"github","repo":"someone/quarry"}]`
	userPluginOn       = `[{"id":"quarry@quarry","scope":"user","enabled":true}]`
	userPluginOff      = `[{"id":"quarry@quarry","scope":"user","enabled":false}]`
)

var errNoStart = errors.New("fork/exec /opt/claude: permission denied")

// toolReply is one scripted answer of the fake RunTool.
type toolReply struct {
	output string
	status int
	err    error
}

// toolCalls is a fake RunTool that records each call's arguments and answers
// from replies keyed by them; an unscripted call prints an empty JSON list and exits 0.
type toolCalls struct {
	names   []string
	argv    []string
	replies map[string]toolReply
}

func (f *toolCalls) reply(argv string, r toolReply) *toolCalls {
	if f.replies == nil {
		f.replies = map[string]toolReply{}
	}
	f.replies[argv] = r
	return f
}

func (f *toolCalls) run(_ context.Context, name string, args ...string) ([]byte, int, error) {
	argv := strings.Join(args, " ")
	f.names = append(f.names, name)
	f.argv = append(f.argv, argv)
	r, ok := f.replies[argv]
	if !ok {
		return []byte("[]"), 0, nil
	}
	return []byte(r.output), r.status, r.err
}

// lists scripts the two lists with the given JSON bodies.
func (f *toolCalls) lists(marketplaces, plugins string) *toolCalls {
	return f.reply(marketplaceListArgv, toolReply{output: marketplaces}).reply(pluginListArgv, toolReply{output: plugins})
}

// runClaude runs quarry with args over tool and returns what it printed and returned.
func runClaude(t *testing.T, tool *toolCalls, args ...string) (string, string, error) {
	t.Helper()
	return runClaudeFinding(t, tool, nil, args...)
}

// runClaudeFinding is runClaude with lookPath as the process's command lookup.
func runClaudeFinding(t *testing.T, tool *toolCalls, lookPath claudeplugin.LookPath, args ...string) (string, string, error) {
	t.Helper()
	return runClaudeAt(t, tool, lookPath, "", args...)
}

// runClaudeAt is runClaudeFinding with home as the user's home directory.
func runClaudeAt(t *testing.T, tool *toolCalls, lookPath claudeplugin.LookPath, home string, args ...string) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer

	err := cli.Execute(t.Context(), args, cli.Env{Stdout: &out, Stderr: &errOut, RunTool: tool.run, LookPath: lookPath, Home: home})

	return out.String(), errOut.String(), err
}

// findsAllBut is a LookPath that resolves every command except those named in missing.
func findsAllBut(missing ...string) claudeplugin.LookPath {
	return func(file string) (string, error) {
		if slices.Contains(missing, file) {
			return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
		}
		return "/opt/bin/" + file, nil
	}
}

func Test_claude_install_warns_when_quarry_is_not_on_the_path(t *testing.T) {
	tool := &toolCalls{}

	stdout, stderr, err := runClaudeFinding(t, tool, findsAllBut("quarry"), "claude", "install")

	require.NoError(t, err)
	assert.Equal(t, "Added the quarry marketplace to Claude Code.\n"+
		"Installed the quarry plugin (skill and MCP server) for all your projects.\n"+
		"Restart Claude Code to load it.\n", stdout)
	assert.Equal(t, `quarry: claude install: warning: the plugin starts "quarry" from your PATH, and your PATH has none; `+
		"add the directory holding quarry to your PATH\n", stderr)
}

// findsAt is a LookPath that resolves every command to path.
func findsAt(path string) claudeplugin.LookPath {
	return func(string) (string, error) { return path, nil }
}

// cannotStart is the error toolrun returns when the file at path is not executable.
func cannotStart(path string) error {
	return &toolrun.StartError{Path: path, Err: &fs.PathError{Op: "fork/exec", Path: path, Err: syscall.EACCES}}
}

func Test_claude_install_warns_after_the_turned_off_hint(t *testing.T) {
	tool := (&toolCalls{}).lists(ourMarketplace, userPluginOff)

	stdout, stderr, err := runClaudeFinding(t, tool, findsAllBut("quarry"), "claude", "install")

	require.NoError(t, err)
	assert.Equal(t, "The quarry marketplace is already in Claude Code.\n"+
		"The quarry plugin is already installed for all your projects.\n", stdout)
	assert.Equal(t, "quarry: claude install: the quarry plugin is installed but turned off; "+
		"to turn it on, run claude plugin enable quarry@quarry\n"+
		`quarry: claude install: warning: the plugin starts "quarry" from your PATH, and your PATH has none; `+
		"add the directory holding quarry to your PATH\n", stderr)
}

func Test_claude_install_warns_when_quarry_is_not_on_the_path_and_nothing_ran(t *testing.T) {
	tool := (&toolCalls{}).lists(ourMarketplace, userPluginOn)

	stdout, stderr, err := runClaudeFinding(t, tool, findsAllBut("quarry"), "claude", "install")

	require.NoError(t, err)
	assert.Equal(t, "The quarry marketplace is already in Claude Code.\n"+
		"The quarry plugin is already installed for all your projects.\n", stdout)
	assert.Equal(t, `quarry: claude install: warning: the plugin starts "quarry" from your PATH, and your PATH has none; `+
		"add the directory holding quarry to your PATH\n", stderr)
}

func Test_claude_install_runs_claude_at_the_path_it_was_found(t *testing.T) {
	cases := []struct {
		name     string
		lookPath claudeplugin.LookPath
		want     string
	}{
		{name: "the resolved path", lookPath: findsAt("/opt/bin/claude"), want: "/opt/bin/claude"},
		{name: "the bare name when nothing resolves it", lookPath: nil, want: "claude"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tool := &toolCalls{}

			_, _, err := runClaudeFinding(t, tool, c.lookPath, "claude", "install")

			require.NoError(t, err)
			assert.Equal(t, []string{c.want, c.want, c.want, c.want}, tool.names)
		})
	}
}

func Test_claude_install_refuses_when_claude_is_not_on_the_path(t *testing.T) {
	cases := []struct {
		name string
		path string
		err  error
	}{
		{name: "not found", err: &exec.Error{Name: "claude", Err: exec.ErrNotFound}},
		{name: "found in the current directory", path: "./claude", err: &exec.Error{Name: "claude", Err: exec.ErrDot}},
		{name: "not executable", err: &exec.Error{Name: "claude", Err: fs.ErrPermission}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tool := &toolCalls{}
			lookPath := func(string) (string, error) { return c.path, c.err }

			stdout, stderr, err := runClaudeFinding(t, tool, lookPath, "claude", "install")

			require.ErrorIs(t, err, cli.ReportedError{})
			assert.Empty(t, tool.argv)
			assert.Empty(t, stdout)
			assert.Equal(t, "quarry: claude install: cannot find the claude command on your PATH; "+
				"install Claude Code, then run quarry claude install again\n", stderr)
		})
	}
}

func Test_claude_install_reports_a_claude_it_cannot_run(t *testing.T) {
	tool := (&toolCalls{}).reply(marketplaceListArgv, toolReply{err: cannotStart("/home/ada/bin/claude")})

	stdout, stderr, err := runClaudeAt(t, tool, findsAt("/home/ada/bin/claude"), "/home/ada", "claude", "install")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Empty(t, stdout)
	assert.Equal(t, `quarry: claude install: cannot run claude at "~/bin/claude" (permission denied); `+
		"check that it is Claude Code and that you can run it, then run quarry claude install again\n", stderr)
}

func Test_claude_install_reports_a_claude_it_cannot_run_after_adding_the_marketplace(t *testing.T) {
	tool := (&toolCalls{}).reply(installPluginArgv, toolReply{err: cannotStart("/home/ada/bin/claude")})

	stdout, stderr, err := runClaudeAt(t, tool, findsAt("/home/ada/bin/claude"), "/home/ada", "claude", "install")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, "Added the quarry marketplace to Claude Code.\n", stdout)
	assert.Equal(t, `quarry: claude install: added the quarry marketplace, but cannot run claude at "~/bin/claude" (permission denied); `+
		"check that it is Claude Code and that you can run it, then run quarry claude install again\n", stderr)
}

func Test_claude_install_reports_a_claude_it_cannot_run_at_a_path_outside_home(t *testing.T) {
	cases := []struct {
		name string
		home string
	}{
		{name: "home is unset", home: ""},
		{name: "the path is not under home", home: "/home/ada"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tool := (&toolCalls{}).reply(marketplaceListArgv, toolReply{err: cannotStart("/opt/claude")})

			_, stderr, err := runClaudeAt(t, tool, findsAt("/opt/claude"), c.home, "claude", "install")

			require.ErrorIs(t, err, cli.ReportedError{})
			assert.Equal(t, `quarry: claude install: cannot run claude at "/opt/claude" (permission denied); `+
				"check that it is Claude Code and that you can run it, then run quarry claude install again\n", stderr)
		})
	}
}

func Test_claude_install_runs_both_steps_when_nothing_is_installed(t *testing.T) {
	tool := &toolCalls{}
	var stdout, stderr bytes.Buffer
	env := cli.Env{Stdout: &stdout, Stderr: &stderr, RunTool: tool.run}

	err := cli.Execute(t.Context(), []string{"claude", "install"}, env)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"plugin marketplace list --json",
		"plugin list --json",
		"plugin marketplace add --scope user koblas/quarry",
		"plugin install --scope user quarry@quarry",
	}, tool.argv)
	assert.Equal(t, "Added the quarry marketplace to Claude Code.\n"+
		"Installed the quarry plugin (skill and MCP server) for all your projects.\n"+
		"Restart Claude Code to load it.\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_claude_install_skips_both_steps_when_both_are_present(t *testing.T) {
	tool := (&toolCalls{}).lists(ourMarketplace, userPluginOn)

	stdout, stderr, err := runClaude(t, tool, "claude", "install")

	require.NoError(t, err)
	assert.Equal(t, []string{marketplaceListArgv, pluginListArgv}, tool.argv)
	assert.Equal(t, "The quarry marketplace is already in Claude Code.\n"+
		"The quarry plugin is already installed for all your projects.\n", stdout)
	assert.Empty(t, stderr)
}

func Test_claude_install_installs_only_the_plugin_when_the_marketplace_is_present(t *testing.T) {
	tool := (&toolCalls{}).lists(ourMarketplace, "[]")

	stdout, stderr, err := runClaude(t, tool, "claude", "install")

	require.NoError(t, err)
	assert.Equal(t, []string{marketplaceListArgv, pluginListArgv, installPluginArgv}, tool.argv)
	assert.Equal(t, "The quarry marketplace is already in Claude Code.\n"+
		"Installed the quarry plugin (skill and MCP server) for all your projects.\n"+
		"Restart Claude Code to load it.\n", stdout)
	assert.Empty(t, stderr)
}

func Test_claude_install_hints_when_the_user_copy_is_turned_off(t *testing.T) {
	tool := (&toolCalls{}).lists(ourMarketplace, userPluginOff)

	stdout, stderr, err := runClaude(t, tool, "claude", "install")

	require.NoError(t, err)
	assert.Equal(t, "The quarry marketplace is already in Claude Code.\n"+
		"The quarry plugin is already installed for all your projects.\n", stdout)
	assert.Equal(t, "quarry: claude install: the quarry plugin is installed but turned off; "+
		"to turn it on, run claude plugin enable quarry@quarry\n", stderr)
}

func Test_claude_install_refuses_a_plugin_list_that_is_not_json(t *testing.T) {
	tool := (&toolCalls{}).lists(ourMarketplace, "not json")

	stdout, stderr, err := runClaude(t, tool, "claude", "install")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, []string{marketplaceListArgv, pluginListArgv}, tool.argv)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: claude install: cannot read what claude plugin list --json printed; "+
		"update Claude Code, or run the two commands in quarry claude install --help yourself\n", stderr)
}

func Test_claude_install_refuses_a_plugin_list_entry_without_an_id(t *testing.T) {
	tool := (&toolCalls{}).lists(ourMarketplace, `[{"scope":"user"}]`)

	stdout, stderr, err := runClaude(t, tool, "claude", "install")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, []string{marketplaceListArgv, pluginListArgv}, tool.argv)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: claude install: cannot read what claude plugin list --json printed; "+
		"update Claude Code, or run the two commands in quarry claude install --help yourself\n", stderr)
}

func Test_claude_install_replays_a_failed_marketplace_list(t *testing.T) {
	tool := (&toolCalls{}).reply(marketplaceListArgv, toolReply{output: "Error: An unknown error occurred (Unexpected)\n", status: 1})

	stdout, stderr, err := runClaude(t, tool, "claude", "install")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, []string{marketplaceListArgv}, tool.argv)
	assert.Empty(t, stdout)
	assert.Equal(t, "Error: An unknown error occurred (Unexpected)\n"+
		"quarry: claude install: claude plugin marketplace list --json exited with status 1; see its message above\n", stderr)
}

func Test_claude_install_refuses_a_foreign_quarry_marketplace(t *testing.T) {
	tool := (&toolCalls{}).lists(foreignMarketplace, "[]")

	stdout, stderr, err := runClaude(t, tool, "claude", "install")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, []string{marketplaceListArgv, pluginListArgv}, tool.argv)
	assert.Empty(t, stdout)
	assert.Equal(t, `quarry: claude install: Claude Code has a marketplace named "quarry" that is not koblas/quarry on GitHub; `+
		"remove it with claude plugin marketplace remove quarry, then run quarry claude install again\n", stderr)
}

func Test_claude_install_reports_each_list_failure(t *testing.T) {
	cases := []struct {
		name       string
		tool       *toolCalls
		wantStdout string
		wantStderr string
	}{
		{
			name: "a marketplace list that is not an array",
			tool: (&toolCalls{}).lists(`{"name":"quarry"}`, "[]"),
			wantStderr: "quarry: claude install: cannot read what claude plugin marketplace list --json printed; " +
				"update Claude Code, or run the two commands in quarry claude install --help yourself\n",
		},
		{
			name: "a plugin list that exits 2 without a final newline",
			tool: (&toolCalls{}).lists(ourMarketplace, "").reply(pluginListArgv, toolReply{output: "boom", status: 2}),
			wantStderr: "boom\n" +
				"quarry: claude install: claude plugin list --json exited with status 2; see its message above\n",
		},
		{
			name:       "a marketplace list that exits 1 with no output",
			tool:       (&toolCalls{}).reply(marketplaceListArgv, toolReply{status: 1}),
			wantStderr: "quarry: claude install: claude plugin marketplace list --json exited with status 1; see its message above\n",
		},
		{
			name:       "an install step that exits 1 after the marketplace was added",
			tool:       (&toolCalls{}).reply(installPluginArgv, toolReply{output: "denied\n", status: 1}),
			wantStdout: "Added the quarry marketplace to Claude Code.\n",
			wantStderr: "denied\n" +
				"quarry: claude install: claude plugin install --scope user quarry@quarry exited with status 1; see its message above\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, err := runClaude(t, c.tool, "claude", "install")

			require.ErrorIs(t, err, cli.ReportedError{})
			assert.Equal(t, c.wantStdout, stdout)
			assert.Equal(t, c.wantStderr, stderr)
		})
	}
}

func Test_claude_install_reports_when_the_user_copy_is_on_but_the_marketplace_is_missing(t *testing.T) {
	tool := (&toolCalls{}).lists("[]", userPluginOn)

	stdout, stderr, err := runClaude(t, tool, "claude", "install")

	require.NoError(t, err)
	assert.Equal(t, "Added the quarry marketplace to Claude Code.\n"+
		"The quarry plugin is already installed for all your projects.\n"+
		"Restart Claude Code to load it.\n", stdout)
	assert.Empty(t, stderr)
}

func Test_claude_install_hints_after_adding_the_marketplace_when_the_user_copy_is_turned_off(t *testing.T) {
	tool := (&toolCalls{}).lists("[]", userPluginOff)

	stdout, stderr, err := runClaude(t, tool, "claude", "install")

	require.NoError(t, err)
	assert.Equal(t, "Added the quarry marketplace to Claude Code.\n"+
		"The quarry plugin is already installed for all your projects.\n"+
		"Restart Claude Code to load it.\n", stdout)
	assert.Equal(t, "quarry: claude install: the quarry plugin is installed but turned off; "+
		"to turn it on, run claude plugin enable quarry@quarry\n", stderr)
}

func Test_claude_install_returns_the_stdout_write_error_when_the_result_cannot_be_printed(t *testing.T) {
	env := cli.Env{Stdout: failingWriter{err: errNoSpace}, Stderr: io.Discard, RunTool: (&toolCalls{}).run}

	err := cli.Execute(t.Context(), []string{"claude", "install"}, env)

	require.ErrorIs(t, err, errNoSpace)
	assert.NotErrorAs(t, err, new(cli.UsageError))
}

func Test_claude_install_reports_a_refusal_when_stdout_cannot_be_written(t *testing.T) {
	tool := (&toolCalls{}).lists(foreignMarketplace, "[]")
	var stderr bytes.Buffer
	env := cli.Env{Stdout: failingWriter{err: errNoSpace}, Stderr: &stderr, RunTool: tool.run}

	err := cli.Execute(t.Context(), []string{"claude", "install"}, env)

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, `quarry: claude install: Claude Code has a marketplace named "quarry" that is not koblas/quarry on GitHub; `+
		"remove it with claude plugin marketplace remove quarry, then run quarry claude install again\n", stderr.String())
}

func Test_claude_install_returns_the_stdout_write_error_when_a_step_line_cannot_be_printed(t *testing.T) {
	tool := (&toolCalls{}).reply(installPluginArgv, toolReply{output: "denied\n", status: 1})
	env := cli.Env{Stdout: failingWriter{err: errNoSpace}, Stderr: io.Discard, RunTool: tool.run}

	err := cli.Execute(t.Context(), []string{"claude", "install"}, env)

	require.ErrorIs(t, err, errNoSpace)
	assert.NotErrorIs(t, err, cli.ReportedError{})
}

func Test_claude_install_returns_the_runners_own_error_as_a_runtime_failure(t *testing.T) {
	tool := (&toolCalls{}).reply(marketplaceListArgv, toolReply{err: errNoStart})

	_, _, err := runClaude(t, tool, "claude", "install")

	require.ErrorIs(t, err, errNoStart)
	assert.NotErrorAs(t, err, new(cli.UsageError))
}

func Test_claude_install_refuses_json(t *testing.T) {
	tool := &toolCalls{}

	stdout, _, err := runClaude(t, tool, "claude", "install", "--json")

	var usage cli.UsageError
	require.ErrorAs(t, err, &usage)
	assert.Equal(t, "claude install prints no JSON; drop --json", usage.Error())
	assert.Empty(t, tool.argv)
	assert.Empty(t, stdout)
}

func Test_claude_install_refuses_arguments(t *testing.T) {
	tool := &toolCalls{}

	_, _, err := runClaude(t, tool, "claude", "install", "extra")

	var usage cli.UsageError
	require.ErrorAs(t, err, &usage)
	assert.Equal(t, "install takes no arguments; Run 'quarry claude install --help' for usage.", usage.Error())
	assert.Empty(t, tool.argv)
}

func Test_claude_install_help_prints_the_ruled_text(t *testing.T) {
	const long = `Install quarry's Claude Code plugin for all your projects. The plugin adds
the quarry skill, which teaches Claude to answer from quarry, and quarry's
MCP server. install runs:

  claude plugin marketplace add --scope user koblas/quarry
  claude plugin install --scope user quarry@quarry

skipping each step that is already done, so running it again is safe.
Claude Code downloads the plugin from github.com/koblas/quarry; quarry
itself sends nothing and opens none of Claude Code's files. The plugin
starts "quarry" from your PATH. Restart Claude Code to load it.
`
	tool := &toolCalls{}

	stdout, _, err := runClaude(t, tool, "claude", "install", "--help")
	require.NoError(t, err)
	group, _, groupErr := runClaude(t, tool, "claude", "--help")

	require.NoError(t, groupErr)
	assert.True(t, strings.HasPrefix(stdout, long), stdout)
	assert.Contains(t, group, "  install     Install quarry's plugin (skill and MCP server) in Claude Code\n")
	assert.Empty(t, tool.argv)
}
