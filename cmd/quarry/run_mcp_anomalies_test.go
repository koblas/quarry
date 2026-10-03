package main

import (
	"context"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const anomaliesLogPrefix = "quarry: mcp: anomalies: "

func Test_run_mcp_anomalies_returns_the_anomalies_json_document(t *testing.T) {
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
				"anomalies", "--since", "2026-01", "--until", "2026-08", "--currency", "CAD",
				"--account", "Chequing", "--account", "US Chequing", "--account", "Linked", "--account", "Old Card",
			},
			arguments: map[string]any{
				"since": "2026-01", "until": "2026-08", "currency": "CAD",
				"accounts": []string{"Chequing", "US Chequing", "Linked", "Old Card"},
			},
		},
		{name: "none given", store: populatedAnalysisStore, cliArgs: []string{"anomalies"}, arguments: map[string]any{}},
		{
			name: "native currency", store: populatedAnalysisStore,
			cliArgs: []string{"anomalies", "--currency", "native"}, arguments: map[string]any{"currency": "native"},
		},
		{name: "empty window", store: emptyWindowAnalysisStore, cliArgs: []string{"anomalies"}, arguments: map[string]any{}},
		{
			name: "currency absent, config with an unknown key", store: populatedAnalysisStore, config: unknownKeyConfig,
			cliArgs: []string{"anomalies"}, arguments: map[string]any{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: c.store, config: c.config, cliArgs: c.cliArgs, tool: "anomalies", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, inToolWords(got.cliWarnings, "anomalies", "anomalies"), got.toolWarnings)
		})
	}
}

func Test_run_mcp_anomalies_refuses_a_future_since_in_its_own_words(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	populatedAnalysisStore(t, home)
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	peer := startClockedMCP(ctx, t)

	result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{
		Name: "anomalies", Arguments: map[string]any{"since": "2099"},
	})
	require.NoError(t, err)
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	assert.True(t, result.IsError)
	assert.Equal(t, "since 2099 is after today; anomalies lists charges up to today only, so pass an earlier since", textOf(result))
	assert.Equal(t, anomaliesLogPrefix+windowRefusedLog+"\n", peer.stderr.String())
}
