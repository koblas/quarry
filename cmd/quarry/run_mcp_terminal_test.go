package main

import (
	"context"
	"io"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mcpTerminalHint = "quarry: mcp: this is an MCP server for Claude and other MCP clients; it reads JSON-RPC on stdin. Press Ctrl-D to stop.\n"

func Test_run_mcp_at_a_terminal_prints_the_hint_and_keeps_serving(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	peer := startMCP(ctx, t, func(env *cli.Env) {
		env.IsTerminal = func(io.Reader) bool { return true }
	})

	_, listErr := peer.session.ListTools(ctx, nil)
	require.NoError(t, peer.session.Close())
	code := peer.waitForExit(ctx, t)

	require.NoError(t, listErr)
	assert.Equal(t, 0, code)
	assert.Equal(t, mcpTerminalHint, peer.stderr.String())
}
