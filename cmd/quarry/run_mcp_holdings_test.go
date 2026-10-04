package main

import (
	"context"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	holdingsToolRate  = 1_360_000
	holdingsLogPrefix = "quarry: mcp: holdings: "
	asOfRefusedLog    = "refused the call's as_of; details went to the client only"
)

// seedHoldingsToolStore stores holdingsRows with one USD rate, under the HOME runBothSurfaces set.
func seedHoldingsToolStore(t *testing.T, home string) {
	t.Helper()
	replaceStoreWithRates(t, home, holdingsRows(), usdRate(holdingsDay(10), holdingsToolRate))
}

func Test_run_mcp_holdings_returns_the_holdings_json_document(t *testing.T) {
	cases := []struct {
		name      string
		store     func(*testing.T, string)
		cliArgs   []string
		arguments map[string]any
	}{
		{
			name: "as_of and currency given", store: seedHoldingsToolStore,
			cliArgs:   []string{"holdings", "--as-of", "2026-03-12", "--currency", "CAD"},
			arguments: map[string]any{"as_of": "2026-03-12", "currency": "CAD"},
		},
		{name: "none given", store: seedHoldingsToolStore, cliArgs: []string{"holdings"}, arguments: map[string]any{}},
		{
			name: "a year that holds today", store: seedHoldingsToolStore,
			cliArgs: []string{"holdings", "--as-of", "2026"}, arguments: map[string]any{"as_of": "2026"},
		},
		{
			name: "no accounts on a day nothing is held", store: seedHoldingsToolStore,
			cliArgs: []string{"holdings", "--as-of", "2025"}, arguments: map[string]any{"as_of": "2025", "accounts": []string{}},
		},
		{
			name: "one account in native currencies", store: seedHoldingsToolStore,
			cliArgs:   []string{"holdings", "--as-of", "2026-03-12", "--account", "Brokerage", "--currency", "native"},
			arguments: map[string]any{"as_of": "2026-03-12", "accounts": []string{"Brokerage"}, "currency": "native"},
		},
		{
			name: "a non-investment account", store: func(t *testing.T, home string) {
				t.Helper()
				rows := holdingsRows()
				rows.Accounts = append(rows.Accounts, chequingAccount("acct-chq", 4))
				replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), holdingsToolRate))
			},
			cliArgs:   []string{"holdings", "--as-of", "2026-03-12", "--account", "Chequing"},
			arguments: map[string]any{"as_of": "2026-03-12", "accounts": []string{"Chequing"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: c.store, cliArgs: c.cliArgs, tool: "holdings", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, got.cliWarnings, got.toolWarnings)
		})
	}
}

func Test_run_mcp_holdings_refuses_an_as_of_it_cannot_use_in_mcp_words(t *testing.T) {
	cases := []struct {
		name string
		asOf string
		want string
	}{
		{name: "not a date", asOf: "2024-13", want: `as_of "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{
			name: "after today", asOf: "2027-01-01",
			want: "as_of 2027-01-01 is after today; holdings are valued up to today only, so pass an earlier as_of",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			seedHoldingsToolStore(t, home)
			ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
			defer cancel()
			peer := startClockedMCP(ctx, t)

			result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "holdings", Arguments: map[string]any{"as_of": c.asOf}})
			require.NoError(t, err)
			require.NoError(t, peer.session.Close())
			peer.waitForExit(ctx, t)

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(result))
			assert.Equal(t, holdingsLogPrefix+asOfRefusedLog+"\n", peer.stderr.String())
		})
	}
}

func Test_run_mcp_holdings_refuses_an_account_without_its_name_on_stderr(t *testing.T) {
	refuseAccountKeepingItsNameOffStderr(t, "holdings", holdingsLogPrefix)
}
