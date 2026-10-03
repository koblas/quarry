package main

import (
	"context"
	"testing"

	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	recurringChargesLogPrefix     = "quarry: mcp: recurring_charges: "
	linkedLineForRecurringCharges = `account "Linked" uses linked account tracking in Quicken, so recurring_charges leaves it out, as Quicken's reports do`
)

func Test_run_mcp_recurring_charges_returns_the_recurring_json_document(t *testing.T) {
	cases := []struct {
		name        string
		store       func(*testing.T, string)
		config      string
		cliArgs     []string
		arguments   map[string]any
		wantWarning string
	}{
		{
			name: "all four given", store: populatedAnalysisStore, wantWarning: linkedLineForRecurringCharges,
			cliArgs: []string{
				"recurring", "--since", "2026-01", "--until", "2026-08", "--currency", "CAD",
				"--account", "Chequing", "--account", "US Chequing", "--account", "Linked", "--account", "Old Card",
			},
			arguments: map[string]any{
				"since": "2026-01", "until": "2026-08", "currency": "CAD",
				"accounts": []string{"Chequing", "US Chequing", "Linked", "Old Card"},
			},
		},
		{name: "none given", store: populatedAnalysisStore, cliArgs: []string{"recurring"}, arguments: map[string]any{}},
		{
			name: "native currency", store: populatedAnalysisStore,
			cliArgs: []string{"recurring", "--currency", "native"}, arguments: map[string]any{"currency": "native"},
		},
		{name: "empty window", store: emptyWindowAnalysisStore, cliArgs: []string{"recurring"}, arguments: map[string]any{}},
		{
			name: "currency absent, config with an unknown key", store: populatedAnalysisStore, config: unknownKeyConfig,
			cliArgs: []string{"recurring"}, arguments: map[string]any{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: c.store, config: c.config, cliArgs: c.cliArgs, tool: "recurring_charges", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, inToolWords(got.cliWarnings, "recurring", "recurring_charges"), got.toolWarnings)
			if c.wantWarning != "" {
				assert.Contains(t, got.toolWarnings, c.wantWarning)
			}
		})
	}
}

func Test_run_mcp_recurring_charges_refuses_a_future_since_in_its_own_words(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	populatedAnalysisStore(t, home)
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	peer := startClockedMCP(ctx, t)

	result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{
		Name: "recurring_charges", Arguments: map[string]any{"since": "2099"},
	})
	require.NoError(t, err)
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	assert.True(t, result.IsError)
	assert.Equal(t, "since 2099 is after today; recurring_charges lists charges up to today only, so pass an earlier since", textOf(result))
	assert.Equal(t, recurringChargesLogPrefix+windowRefusedLog+"\n", peer.stderr.String())
}

func Test_run_mcp_recurring_charges_refuses_an_account_without_its_name_on_stderr(t *testing.T) {
	cases := []struct {
		name       string
		arg        string
		want       string
		wantStderr string
		absent     []string
	}{
		{
			name: "no account has the name", arg: "Nope",
			want:       `no account named "Nope"; call describe_schema to list the accounts`,
			wantStderr: recurringChargesLogPrefix + unknownAccountLog + "\n",
			absent:     []string{"Nope"},
		},
		{
			name: "the name is empty", arg: "",
			want:       `no account named ""; call describe_schema to list the accounts`,
			wantStderr: recurringChargesLogPrefix + unknownAccountLog + "\n",
			absent:     []string{`""`},
		},
		{
			name: "two accounts share the name", arg: "Visa",
			want:       `2 accounts are named "Visa"; pass one of their ids instead: acct-812, acct-977`,
			wantStderr: recurringChargesLogPrefix + ambiguousAccountLog + "\n",
			absent:     []string{"Visa", "acct-812", "acct-977"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			replaceStore(t, home, spendRows([]store.Account{
				chequingAccount("acct-chq", 1),
				{ID: "acct-977", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
				{ID: "acct-812", SourceID: 3, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
			}))
			ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
			defer cancel()
			peer := startClockedMCP(ctx, t)

			result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{
				Name: "recurring_charges", Arguments: map[string]any{"accounts": []string{c.arg}},
			})
			require.NoError(t, err)
			require.NoError(t, peer.session.Close())
			peer.waitForExit(ctx, t)

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(result))
			assert.Equal(t, c.wantStderr, peer.stderr.String())
			for _, text := range c.absent {
				assert.NotContains(t, peer.stderr.String(), text)
			}
		})
	}
}
