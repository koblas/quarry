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

	marketplaceAddedLine   = "Added the quarry marketplace to Claude Code.\n"
	marketplacePresentLine = "The quarry marketplace is already in Claude Code.\n"
	pluginInstalledLine    = "Installed the quarry plugin (skill and MCP server) for all your projects.\n"
	installRestartLine     = "Restart Claude Code to load it.\n"
)

var errNoStart = errors.New("fork/exec /opt/claude: permission denied")

// toolReply is one scripted answer of the fake RunTool.
type toolReply struct {
	output string
	status int
	err    error
	cancel bool // the call cancels the command's context before answering
	ctxErr bool // the call fails with the Err of the context it was given
}

// toolCalls is a fake RunTool that records each call's context and arguments and
// answers from script, else replies keyed by them; an unscripted call prints an
// empty JSON list and exits 0.
type toolCalls struct {
	names   []string
	argv    []string
	ctxs    []context.Context
	replies map[string]toolReply
	script  func(argv string) toolReply

	cancelBefore bool               // runClaudeAt cancels the command's context before it executes
	cancel       context.CancelFunc // set by runClaudeAt
}

func (f *toolCalls) reply(argv string, r toolReply) *toolCalls {
	if f.replies == nil {
		f.replies = map[string]toolReply{}
	}
	f.replies[argv] = r
	return f
}

func (f *toolCalls) run(ctx context.Context, name string, args ...string) ([]byte, int, error) {
	argv := strings.Join(args, " ")
	f.names = append(f.names, name)
	f.argv = append(f.argv, argv)
	f.ctxs = append(f.ctxs, ctx)
	r, ok := f.replies[argv]
	if f.script != nil {
		r, ok = f.script(argv), true
	}
	if !ok {
		return []byte("[]"), 0, nil
	}
	if r.cancel {
		f.cancel()
	}
	err := r.err
	if r.ctxErr {
		err = ctx.Err()
	}
	return []byte(r.output), r.status, err
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

// ctxMarker keys the value runClaudeAt puts on the command's context.
type ctxMarker struct{}

// runClaudeAt is runClaudeFinding with home as the user's home directory. It runs under a
// cancellable context carrying a marker, and fails the test if a call got any other context.
func runClaudeAt(t *testing.T, tool *toolCalls, lookPath claudeplugin.LookPath, home string, args ...string) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), ctxMarker{}, "command"))
	defer cancel()
	tool.cancel = cancel
	if tool.cancelBefore {
		cancel()
	}

	err := cli.Execute(ctx, args, cli.Env{Stdout: &out, Stderr: &errOut, RunTool: tool.run, LookPath: lookPath, Home: home})

	for i, got := range tool.ctxs {
		assert.Equal(t, "command", got.Value(ctxMarker{}), "call %d (%s) ran under a context that is not the command's", i, tool.argv[i])
	}
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
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, err := runClaude(t, c.tool, "claude", "install")

			require.ErrorIs(t, err, cli.ReportedError{})
			assert.Empty(t, stdout)
			assert.Equal(t, c.wantStderr, stderr)
		})
	}
}

func Test_claude_install_reports_each_step_failure(t *testing.T) {
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
			name:       "partial, exit status, no output",
			tool:       (&toolCalls{}).reply(installPluginArgv, toolReply{status: 1}),
			wantStdout: marketplaceAddedLine,
			wantStderr: "quarry: claude install: added the quarry marketplace, but claude plugin install --scope user quarry@quarry " +
				"exited with status 1, then run quarry claude install again\n",
		},
		{
			name:       "partial, signal, output",
			tool:       (&toolCalls{}).reply(installPluginArgv, killed("out of memory\n")),
			wantStdout: marketplaceAddedLine,
			wantStderr: "out of memory\n" +
				"quarry: claude install: added the quarry marketplace, but claude plugin install --scope user quarry@quarry " +
				"was stopped by signal killed; see its message above, then run quarry claude install again\n",
		},
		{
			name:       "partial, signal, no output",
			tool:       (&toolCalls{}).reply(installPluginArgv, killed("")),
			wantStdout: marketplaceAddedLine,
			wantStderr: "quarry: claude install: added the quarry marketplace, but claude plugin install --scope user quarry@quarry " +
				"was stopped by signal killed, then run quarry claude install again\n",
		},
		{
			name: "first step, signal, no output",
			tool: (&toolCalls{}).reply(addMarketplaceArgv, killed("")),
			wantStderr: "quarry: claude install: claude plugin marketplace add --scope user koblas/quarry " +
				"was stopped by signal killed\n",
		},
		{
			name: "second step with the first skipped, exit status, output",
			tool: (&toolCalls{}).lists(ourMarketplace, "[]").reply(installPluginArgv, toolReply{output: "nope\n", status: 2}),
			wantStderr: "nope\n" +
				"quarry: claude install: claude plugin install --scope user quarry@quarry exited with status 2; see its message above\n",
		},
		{
			name: "plugin list, signal, output",
			tool: (&toolCalls{}).reply(pluginListArgv, killed("lost\n")),
			wantStderr: "lost\n" +
				"quarry: claude install: claude plugin list --json was stopped by signal killed; see its message above\n",
		},
		{
			name:       "marketplace list, exit status, no output",
			tool:       (&toolCalls{}).reply(marketplaceListArgv, toolReply{status: 1}),
			wantStderr: "quarry: claude install: claude plugin marketplace list --json exited with status 1\n",
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

func Test_claude_install_reports_a_failed_first_step(t *testing.T) {
	tool := (&toolCalls{}).reply(addMarketplaceArgv, toolReply{output: "denied\n", status: 1})

	stdout, stderr, err := runClaude(t, tool, "claude", "install")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, []string{marketplaceListArgv, pluginListArgv, addMarketplaceArgv}, tool.argv)
	assert.Empty(t, stdout)
	assert.Equal(t, "denied\n"+
		"quarry: claude install: claude plugin marketplace add --scope user koblas/quarry exited with status 1; see its message above\n", stderr)
}

func Test_claude_install_drops_see_above_when_the_failed_step_printed_nothing(t *testing.T) {
	tool := (&toolCalls{}).reply(addMarketplaceArgv, toolReply{status: 1})

	stdout, stderr, err := runClaude(t, tool, "claude", "install")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: claude install: claude plugin marketplace add --scope user koblas/quarry exited with status 1\n", stderr)
}

func Test_claude_install_reports_a_step_stopped_by_a_signal(t *testing.T) {
	tool := (&toolCalls{}).lists(ourMarketplace, "[]").
		reply(installPluginArgv, toolReply{output: "Killed\n", status: -1, err: &toolrun.SignalError{Signal: syscall.SIGKILL}})

	stdout, stderr, err := runClaude(t, tool, "claude", "install")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Empty(t, stdout)
	assert.Equal(t, "Killed\n"+
		"quarry: claude install: claude plugin install --scope user quarry@quarry was stopped by signal killed; see its message above\n", stderr)
}

func Test_claude_install_reports_an_interrupt_while_a_step_runs(t *testing.T) {
	tool := (&toolCalls{}).reply(installPluginArgv, toolReply{output: "installing...\n", cancel: true, ctxErr: true})

	stdout, stderr, err := runClaude(t, tool, "claude", "install")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, marketplaceAddedLine, stdout)
	assert.Equal(t, "quarry: claude install: stopped before claude plugin install --scope user quarry@quarry finished; "+
		"run quarry claude install again\n", stderr)
}

func Test_claude_install_reports_an_interrupt_before_or_between_children(t *testing.T) {
	cases := []struct {
		name      string
		tool      *toolCalls
		wantArgv  []string
		wantAfter string
	}{
		{
			name:      "cancelled before the command runs",
			tool:      &toolCalls{cancelBefore: true},
			wantAfter: "claude plugin marketplace list --json",
		},
		{
			name:      "cancelled after the plugin list",
			tool:      (&toolCalls{}).reply(pluginListArgv, toolReply{output: "[]", cancel: true}),
			wantArgv:  []string{marketplaceListArgv, pluginListArgv},
			wantAfter: "claude plugin marketplace add --scope user koblas/quarry",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, err := runClaude(t, c.tool, "claude", "install")

			require.ErrorIs(t, err, cli.ReportedError{})
			assert.Equal(t, c.wantArgv, c.tool.argv)
			assert.Empty(t, stdout)
			assert.Equal(t, "quarry: claude install: stopped before "+c.wantAfter+" finished; run quarry claude install again\n", stderr)
		})
	}
}

func Test_claude_install_finishes_on_a_rerun_after_a_partial_install(t *testing.T) {
	marketplaceAdded, pluginInstalled, installFailures := false, false, 1
	tool := &toolCalls{}
	tool.script = func(argv string) toolReply {
		switch argv {
		case marketplaceListArgv:
			if marketplaceAdded {
				return toolReply{output: ourMarketplace}
			}
			return toolReply{output: "[]"}
		case pluginListArgv:
			if pluginInstalled {
				return toolReply{output: userPluginOn}
			}
			return toolReply{output: "[]"}
		case addMarketplaceArgv:
			marketplaceAdded = true
		case installPluginArgv:
			if installFailures > 0 {
				installFailures--
				return toolReply{output: "denied\n", status: 1}
			}
			pluginInstalled = true
		}
		return toolReply{}
	}

	_, _, firstErr := runClaude(t, tool, "claude", "install")
	callsAfterFirst := len(tool.argv)
	stdout, stderr, secondErr := runClaude(t, tool, "claude", "install")

	require.ErrorIs(t, firstErr, cli.ReportedError{})
	require.NoError(t, secondErr)
	assert.Equal(t, []string{marketplaceListArgv, pluginListArgv, installPluginArgv}, tool.argv[callsAfterFirst:])
	assert.Equal(t, marketplacePresentLine+pluginInstalledLine+installRestartLine, stdout)
	assert.Empty(t, stderr)
}

func Test_claude_install_reports_a_partial_install_when_the_plugin_step_fails(t *testing.T) {
	tool := (&toolCalls{}).reply(installPluginArgv, toolReply{output: "denied\n", status: 1})

	stdout, stderr, err := runClaude(t, tool, "claude", "install")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, "Added the quarry marketplace to Claude Code.\n", stdout)
	assert.Equal(t, "denied\n"+
		"quarry: claude install: added the quarry marketplace, but claude plugin install --scope user quarry@quarry "+
		"exited with status 1; see its message above, then run quarry claude install again\n", stderr)
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
