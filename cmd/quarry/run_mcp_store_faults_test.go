package main

import (
	"context"
	"os"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// directoryStore makes the store path a directory, so the store exists but cannot be opened.
func directoryStore(t *testing.T, home string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(storePathUnder(home), 0o750))
}

// brokenStore syncs a store, then runs each statement against it.
func brokenStore(statements ...string) func(*testing.T, string) {
	return func(t *testing.T, home string) {
		t.Helper()
		syncAccountsFixture(t, home)
		for _, stmt := range statements {
			editStore(t, home, stmt)
		}
	}
}

func Test_run_mcp_logs_only_the_withheld_line_for_a_store_read_fault(t *testing.T) {
	const (
		rebuild    = "; run quarry sync to rebuild it"
		storeToken = "{store}"
	)
	cases := []struct {
		name      string
		tool      string
		arguments map[string]any
		damage    func(*testing.T, string)
		reason    string
	}{
		{
			name: "query, store cannot be opened", tool: "query", arguments: map[string]any{"sql": "SELECT 1"},
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "describe_schema, store cannot be opened", tool: "describe_schema",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "sync_status, store cannot be opened", tool: "sync_status",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "data_quality, store cannot be opened", tool: "data_quality",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "spending, store cannot be opened", tool: "spending",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "cash_flow, store cannot be opened", tool: "cash_flow",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "recurring_charges, store cannot be opened", tool: "recurring_charges",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "anomalies, store cannot be opened", tool: "anomalies",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "search_transactions, store cannot be opened", tool: "search_transactions",
			damage: directoryStore, reason: `Could not read from file "{store}": Is a directory`,
		},
		{
			name: "describe_schema, a table dropped", tool: "describe_schema",
			damage: brokenStore("DROP TABLE categories CASCADE"), reason: "Table with name categories does not exist!",
		},
		{
			name: "sync_status, import history gone", tool: "sync_status",
			damage: brokenStore("DELETE FROM import_runs"), reason: "the store has no import history",
		},
		{
			name: "data_quality, findings tables dropped", tool: "data_quality",
			damage: brokenStore("DROP TABLE finding_items", "DROP TABLE findings"), reason: "Table with name findings does not exist!",
		},
		{
			name: "spending, its view dropped", tool: "spending",
			damage: brokenStore("DROP VIEW v_spending"), reason: "Table with name v_spending does not exist!",
		},
		{
			name: "cash_flow, its view dropped", tool: "cash_flow",
			damage: brokenStore("DROP VIEW v_cash_flow"), reason: "Table with name v_cash_flow does not exist!",
		},
		{
			name: "recurring_charges, its view dropped", tool: "recurring_charges",
			damage: brokenStore("DROP VIEW v_spending"), reason: "Table with name v_spending does not exist!",
		},
		{
			name: "anomalies, its view dropped", tool: "anomalies",
			damage: brokenStore("DROP VIEW v_spending"), reason: "Table with name v_spending does not exist!",
		},
		{
			name: "search_transactions, a table dropped", tool: "search_transactions",
			damage: brokenStore("DROP TABLE categories CASCADE"), reason: "Table with name categories does not exist!",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			c.damage(t, home)
			at := abbreviated(t, storePathUnder(home), home)
			ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
			defer cancel()
			peer := startClockedMCP(ctx, t)

			result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: c.tool, Arguments: c.arguments})
			require.NoError(t, err)
			require.NoError(t, peer.session.Close())
			peer.waitForExit(ctx, t)

			assert.True(t, result.IsError)
			assert.Equal(t, "cannot read the store at "+at+": "+strings.ReplaceAll(c.reason, storeToken, at)+rebuild, textOf(result))
			assert.Equal(t, "quarry: mcp: "+c.tool+": cannot read the store at "+at+"; details went to the client only\n", peer.stderr.String())
		})
	}
}
