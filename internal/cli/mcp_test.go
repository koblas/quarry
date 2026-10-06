package cli_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errServeFailed = errors.New("serve failed")

// serveCall records the arguments a fake ServeMCP was called with.
type serveCall struct {
	stdin          io.Reader
	stdout, stderr io.Writer
}

func Test_mcp_help_prints_the_ruled_long_text(t *testing.T) {
	const long = `Run quarry as a local MCP server for Claude and other MCP clients. The
client starts it and talks to it over stdin and stdout; quarry opens no
network port. Add it to your client's MCP config with the command
"quarry" and the argument "mcp".

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
	var stdout bytes.Buffer

	err := cli.Execute(t.Context(), []string{"mcp", "--help"}, cli.Env{Stdout: &stdout, Stderr: io.Discard})

	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(stdout.String(), long), stdout.String())
}

func Test_root_help_lists_mcp_with_its_short_line(t *testing.T) {
	var list bytes.Buffer

	err := cli.Execute(t.Context(), []string{"--help"}, cli.Env{Stdout: &list, Stderr: io.Discard})

	require.NoError(t, err)
	assert.Regexp(t, `(?m)^  mcp +Serve quarry's store to Claude over MCP \(stdio\)$`, list.String())
}

func Test_mcp_hands_the_env_streams_to_the_server(t *testing.T) {
	var got serveCall
	stdin := strings.NewReader("")
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		Stdin: stdin, Stdout: &stdout, Stderr: &stderr,
		ServeMCP: func(_ context.Context, in io.Reader, out, errOut io.Writer, _ func()) error {
			got = serveCall{stdin: in, stdout: out, stderr: errOut}
			return nil
		},
	}

	err := cli.Execute(t.Context(), []string{"mcp"}, env)

	require.NoError(t, err)
	assert.Same(t, stdin, got.stdin)
	assert.Same(t, &stdout, got.stdout)
	assert.Same(t, &stderr, got.stderr)
}

func Test_mcp_returns_a_server_error_unwrapped_not_as_usage(t *testing.T) {
	env := cli.Env{
		Stdout: io.Discard, Stderr: io.Discard,
		ServeMCP: func(context.Context, io.Reader, io.Writer, io.Writer, func()) error { return errServeFailed },
	}

	err := cli.Execute(t.Context(), []string{"mcp"}, env)

	assert.Same(t, errServeFailed, err)
}

// serveReturning is a ServeMCP that records that it ran and returns err.
func serveReturning(called *bool, err error) cli.MCPServeFunc {
	return func(context.Context, io.Reader, io.Writer, io.Writer, func()) error {
		*called = true
		return err
	}
}

func Test_mcp_ends_cleanly_when_serving_stops_normally(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "the client closed stdin", err: nil},
		{name: "the process was told to stop", err: context.Canceled},
		{name: "the stop is wrapped", err: fmt.Errorf("serving: %w", context.Canceled)},
		{name: "the client stopped reading stdout", err: syscall.EPIPE},
		{name: "the broken pipe is wrapped in a path error", err: &os.PathError{Op: "write", Path: "/dev/stdout", Err: syscall.EPIPE}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var called bool
			env := cli.Env{Stdout: io.Discard, Stderr: io.Discard, ServeMCP: serveReturning(&called, c.err)}

			err := cli.Execute(t.Context(), []string{"mcp"}, env)

			require.NoError(t, err)
			assert.True(t, called)
		})
	}
}

func Test_mcp_reports_any_other_serve_error_as_a_failure(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "a deadline passed", err: context.DeadlineExceeded},
		{name: "a closed connection", err: net.ErrClosed},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var called bool
			env := cli.Env{Stdout: io.Discard, Stderr: io.Discard, ServeMCP: serveReturning(&called, c.err)}

			err := cli.Execute(t.Context(), []string{"mcp"}, env)

			assert.ErrorIs(t, err, c.err)
		})
	}
}

func Test_mcp_refuses_arguments_and_json_without_serving(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "an argument", args: []string{"mcp", "extra"}, want: "mcp takes no arguments"},
		{name: "--json", args: []string{"mcp", "--json"}, want: "mcp always speaks JSON on stdout; drop --json"},
		{name: "an argument and --json", args: []string{"mcp", "extra", "--json"}, want: "mcp takes no arguments"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var called bool
			env := cli.Env{Stdout: io.Discard, Stderr: io.Discard, ServeMCP: serveReturning(&called, nil)}

			err := cli.Execute(t.Context(), c.args, env)

			require.ErrorAs(t, err, new(cli.UsageError))
			require.EqualError(t, err, c.want)
			assert.False(t, called)
		})
	}
}

// serveWithoutReady is a ServeMCP that returns without ever calling ready.
func serveWithoutReady(context.Context, io.Reader, io.Writer, io.Writer, func()) error { return nil }

func Test_mcp_prints_the_hint_when_serving_starts_at_a_terminal(t *testing.T) {
	const hint = "quarry: mcp: this is an MCP server for Claude and other MCP clients; it reads JSON-RPC on stdin. Press Ctrl-D to stop.\n"
	stdin := strings.NewReader("")
	var probed io.Reader
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		Stdin: stdin, Stdout: &stdout, Stderr: &stderr,
		IsTerminal: func(r io.Reader) bool { probed = r; return true },
		ServeMCP: func(_ context.Context, _ io.Reader, _, _ io.Writer, ready func()) error {
			ready()
			return nil
		},
	}

	err := cli.Execute(t.Context(), []string{"mcp"}, env)

	require.NoError(t, err)
	assert.Same(t, stdin, probed)
	assert.Equal(t, hint, stderr.String())
	assert.Empty(t, stdout.String())
}

func Test_mcp_prints_no_hint_when_stdin_is_not_a_terminal_or_unprobed(t *testing.T) {
	cases := []struct {
		name  string
		probe cli.TerminalProbe
	}{
		{name: "the probe says no", probe: func(io.Reader) bool { return false }},
		{name: "there is no probe", probe: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stderr bytes.Buffer
			env := cli.Env{
				Stderr: &stderr, Stdout: io.Discard, IsTerminal: c.probe,
				ServeMCP: func(_ context.Context, _ io.Reader, _, _ io.Writer, ready func()) error {
					ready()
					return nil
				},
			}

			err := cli.Execute(t.Context(), []string{"mcp"}, env)

			require.NoError(t, err)
			assert.Empty(t, stderr.String())
		})
	}
}

func Test_mcp_prints_no_hint_when_serve_never_reports_ready(t *testing.T) {
	var stderr bytes.Buffer
	env := cli.Env{
		Stderr: &stderr, Stdout: io.Discard,
		IsTerminal: func(io.Reader) bool { return true },
		ServeMCP:   serveWithoutReady,
	}

	err := cli.Execute(t.Context(), []string{"mcp"}, env)

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
}
