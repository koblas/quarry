package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
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
without a restart. SQL runs read-only and returns at most 500 rows.

Payee names, memos and category names reach the client as Quicken holds
them: quarry does not yet mask account or card numbers written in them.

Tools: describe_schema, query, sync_status, data_quality.
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
		ServeMCP: func(_ context.Context, in io.Reader, out, errOut io.Writer) error {
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
		ServeMCP: func(context.Context, io.Reader, io.Writer, io.Writer) error { return errServeFailed },
	}

	err := cli.Execute(t.Context(), []string{"mcp"}, env)

	assert.Same(t, errServeFailed, err)
}
