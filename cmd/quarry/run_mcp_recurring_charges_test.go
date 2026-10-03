package main

import (
	"context"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	recurringChargesLogPrefix      = "quarry: mcp: recurring_charges: "
	linkedLineForRecurringCharges  = `account "Linked" uses linked account tracking in Quicken, so recurring_charges leaves it out, as Quicken's reports do`
	oldCardLineForRecurringCharges = `account "Old Card" is not used in reports in Quicken, so recurring_charges leaves it out; ` +
		`to include it, turn on reports for it in Quicken's account settings, then run quarry sync`
)

func Test_run_mcp_recurring_charges_returns_the_recurring_json_document(t *testing.T) {
	cases := []struct {
		name      string
		store     func(*testing.T, string)
		config    string
		cliArgs   []string
		arguments map[string]any
	}{
		{
			name: "all four given", store: populatedAnalysisStore,
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
	refuseAccountKeepingItsNameOffStderr(t, "recurring_charges", recurringChargesLogPrefix)
}

func Test_run_mcp_recurring_charges_words_its_left_out_warnings_with_the_tool_name(t *testing.T) {
	got := runBothSurfaces(t, toolDocumentRun{
		store: populatedAnalysisStore, tool: "recurring_charges",
		cliArgs:   []string{"recurring", "--account", "Linked", "--account", "Old Card"},
		arguments: map[string]any{"accounts": []string{"Linked", "Old Card"}},
	})

	assert.Equal(t, []string{linkedLineForRecurringCharges, oldCardLineForRecurringCharges}, got.toolWarnings)
}
