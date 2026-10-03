package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	spendingLogPrefix      = "quarry: mcp: spending: "
	spendingConfigLog      = "cannot read quarry's config file; run quarry spend to see why"
	unparseableConfig      = "snapshots.keep = 0\n"
	linkedLineForSpending  = `account "Linked" uses linked account tracking in Quicken, so spending leaves it out, as Quicken's reports do`
	oldCardLineForSpending = `account "Old Card" is not used in reports in Quicken, so spending leaves it out; ` +
		`to include it, turn on reports for it in Quicken's account settings, then run quarry sync`
	spendingBeforeFirstRateLine = "3 transactions dated before 2026-03-01, the first exchange rate in the store, are listed in USD, not converted to CAD"
	unknownKeyConfig            = "bogus = 1\n"
)

func Test_run_mcp_spending_returns_the_spend_json_document(t *testing.T) {
	cases := []struct {
		name      string
		cliArgs   []string
		arguments map[string]any
	}{
		{
			name: "all five given",
			cliArgs: []string{
				"spend", "--since", "2026-01", "--until", "2026-08", "--by", "payee", "--currency", "CAD",
				"--account", "Chequing", "--account", "US Chequing", "--account", "Linked", "--account", "Old Card",
			},
			arguments: map[string]any{
				"since": "2026-01", "until": "2026-08", "by": "payee", "currency": "CAD",
				"accounts": []string{"Chequing", "US Chequing", "Linked", "Old Card"},
			},
		},
		{name: "none given", cliArgs: []string{"spend"}, arguments: map[string]any{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: populatedAnalysisStore, cliArgs: c.cliArgs, tool: "spending", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, inToolWords(got.cliWarnings, "spend", "spending"), got.toolWarnings)
		})
	}
}

func Test_run_mcp_spending_reads_the_config_only_when_currency_is_absent(t *testing.T) {
	cases := []struct {
		name         string
		config       string
		cliArgs      []string
		arguments    map[string]any
		wantCurrency string
	}{
		{name: "currency absent, config says USD", config: `reporting.currency = "USD"` + "\n", cliArgs: []string{"spend"}, arguments: map[string]any{}, wantCurrency: "USD"},
		{name: "currency absent, no config", cliArgs: []string{"spend"}, arguments: map[string]any{}, wantCurrency: "CAD"},
		{
			name: "currency given, config unparseable", config: unparseableConfig,
			cliArgs: []string{"spend", "--currency", "CAD"}, arguments: map[string]any{"currency": "CAD"}, wantCurrency: "CAD",
		},
		{name: "currency absent, config with an unknown key", config: unknownKeyConfig, cliArgs: []string{"spend"}, arguments: map[string]any{}, wantCurrency: "CAD"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: populatedAnalysisStore, config: c.config, cliArgs: c.cliArgs, tool: "spending", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, inToolWords(got.cliWarnings, "spend", "spending"), got.toolWarnings)
			assert.Contains(t, got.toolBody, `"currency":"`+c.wantCurrency+`"`)
		})
	}

	t.Run("currency absent, config unparseable", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		populatedAnalysisStore(t, home)
		writeConfig(t, home, unparseableConfig)
		ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
		defer cancel()
		var cliStdout, cliStderr bytes.Buffer
		require.Equal(t, 1, runWith(ctx, []string{"spend"}, spendEnvAt(&cliStdout, &cliStderr, toolClock)))
		refusal := strings.TrimSuffix(strings.TrimPrefix(cliStderr.String(), "quarry: "), "\n")
		peer := startClockedMCP(ctx, t)

		result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "spending", Arguments: map[string]any{}})
		require.NoError(t, err)
		require.NoError(t, peer.session.Close())
		peer.waitForExit(ctx, t)

		assert.True(t, result.IsError)
		assert.Equal(t, refusal, textOf(result))
		assert.Contains(t, refusal, configShown)
		assert.Equal(t, spendingLogPrefix+spendingConfigLog+"\n", peer.stderr.String())
	})
}

func Test_run_mcp_spending_words_its_warnings_with_the_tool_name(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	populatedAnalysisStore(t, home)
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	peer := startClockedMCP(ctx, t)

	result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{
		Name: "spending", Arguments: map[string]any{"accounts": []string{"Linked", "Old Card", "US Chequing"}},
	})
	require.NoError(t, err)
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	require.False(t, result.IsError, textOf(result))
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &doc))
	assert.Equal(t, []string{linkedLineForSpending, oldCardLineForSpending, spendingBeforeFirstRateLine}, doc.Warnings)
}

func Test_tool_warning_mapping_rewrites_only_the_left_out_template(t *testing.T) {
	cliWarnings := []string{
		`account "Visa" uses linked account tracking in Quicken, so spend leaves it out, as Quicken's reports do`,
		`account "spend" is not used in reports in Quicken, so spend leaves it out; to include it, turn on reports for it in Quicken's account settings, then run quarry sync`,
		"3 transactions dated before 2017-01-03, the first exchange rate in the store, are listed in USD, not converted to CAD",
		"no spending from 2026-01-01 to 2026-10-03; the store's transactions run 2003-01-02 to 2026-09-30",
	}

	mapped := inToolWords(cliWarnings, "spend", "spending")

	assert.Equal(t, []string{
		`account "Visa" uses linked account tracking in Quicken, so spending leaves it out, as Quicken's reports do`,
		`account "spend" is not used in reports in Quicken, so spending leaves it out; to include it, turn on reports for it in Quicken's account settings, then run quarry sync`,
		cliWarnings[2],
		cliWarnings[3],
	}, mapped)
	assert.NotEqual(t, cliWarnings, mapped)
}
