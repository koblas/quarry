package main

import (
	"context"
	"testing"

	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const searchLogPrefix = "quarry: mcp: search_transactions: "

func Test_run_mcp_search_transactions_returns_the_search_json_document(t *testing.T) {
	cases := []struct {
		name      string
		cliArgs   []string
		arguments map[string]any
	}{
		{name: "nothing given", cliArgs: []string{"search"}, arguments: map[string]any{}},
		{
			name: "all eight given",
			cliArgs: []string{
				"search", "bakery", "--since", "2026-02", "--until", "2026-03", "--account", "Chequing",
				"--category", "food:groceries", "--min", "5", "--max", "20", "--limit", "10",
			},
			arguments: map[string]any{
				"text": "bakery", "since": "2026-02", "until": "2026-03", "accounts": []string{"Chequing"},
				"category": "food:groceries", "min": "5", "max": "20", "limit": 10,
			},
		},
		{name: "no match", cliArgs: []string{"search", "zzz"}, arguments: map[string]any{"text": "zzz"}},
		{
			name: "no match with accounts named", cliArgs: []string{"search", "zzz", "--account", "Chequing"},
			arguments: map[string]any{"text": "zzz", "accounts": []string{"Chequing"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: replaceSearchStore, cliArgs: c.cliArgs, tool: "search_transactions", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, got.cliWarnings, got.toolWarnings)
		})
	}
}

func Test_run_mcp_search_transactions_cuts_to_its_limit_with_the_mcp_cut_line(t *testing.T) {
	cases := []struct {
		name       string
		arguments  map[string]any
		wantRows   int
		wantOldest string
		wantLine   string
	}{
		{
			name: "limit 20", arguments: map[string]any{"limit": 20}, wantRows: 20, wantOldest: "txn-n482",
			wantLine: "search_transactions lists the newest 20 of 501 matching transactions; pass a higher limit, up to 500, or narrow the search with text, since, until, accounts, category, min or max",
		},
		{
			name: "limit absent", arguments: map[string]any{}, wantRows: 500, wantOldest: "txn-n002",
			wantLine: "search_transactions lists the newest 500 of 501 matching transactions; narrow the search with text, since, until, accounts, category, min or max",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			replaceStore(t, home, searchRows([]store.Account{chequingAccount("acct-chq", 1)}, nil, manySearchTxns(501)...))
			ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
			defer cancel()
			peer := startClockedMCP(ctx, t)

			result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "search_transactions", Arguments: c.arguments})
			require.NoError(t, err)
			require.NoError(t, peer.session.Close())
			peer.waitForExit(ctx, t)

			require.False(t, result.IsError, textOf(result))
			doc := decodeSearchJSON(t, textOf(result))
			require.Len(t, doc.Transactions, c.wantRows)
			assert.Equal(t, "txn-n501", doc.Transactions[0].TransactionID)
			assert.Equal(t, c.wantOldest, doc.Transactions[c.wantRows-1].TransactionID)
			assert.Equal(t, 501, doc.Matched)
			assert.True(t, doc.Truncated)
			assert.Equal(t, []string{c.wantLine}, doc.Warnings)
			assert.Empty(t, peer.stderr.String())
		})
	}
}

func Test_run_mcp_search_transactions_refuses_an_account_without_its_name_on_stderr(t *testing.T) {
	refuseAccountKeepingItsNameOffStderr(t, "search_transactions", searchLogPrefix)
}

// replaceSearchStore seeds home with searchStore, in the shape runBothSurfaces' store hook takes.
func replaceSearchStore(t *testing.T, home string) {
	t.Helper()
	replaceStore(t, home, searchStore())
}
