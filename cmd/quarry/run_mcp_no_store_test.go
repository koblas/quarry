package main

import (
	"context"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_mcp_every_tool_refuses_before_the_first_sync(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	peer := startMCP(ctx, t, func(*cli.Env) {})
	wantLine := "no store at " + abbreviated(t, storePathUnder(home), home) + " yet; run quarry sync to build it"
	calls := []struct {
		tool      string
		arguments map[string]any
	}{
		{tool: "query", arguments: map[string]any{"sql": "SELECT 1"}},
		{tool: "describe_schema"},
		{tool: "sync_status"},
		{tool: "data_quality"},
		{tool: "spending"},
		{tool: "cash_flow"},
		{tool: "recurring_charges"},
		{tool: "anomalies"},
		{tool: "search_transactions"},
		{tool: "holdings"},
		{tool: "net_worth"},
		{tool: "acb"},
		{tool: "monthly_summary"},
	}

	for _, c := range calls {
		t.Run(c.tool, func(t *testing.T) {
			loggedBefore := peer.stderr.Len()

			result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: c.tool, Arguments: c.arguments})

			require.NoError(t, err)
			assert.True(t, result.IsError)
			assert.Equal(t, wantLine, textOf(result))
			assert.Equal(t, "quarry: mcp: "+c.tool+": "+wantLine+"\n", peer.stderr.String()[loggedBefore:])
		})
	}
}
