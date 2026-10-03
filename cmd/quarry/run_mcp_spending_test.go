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
	windowRefusedLog            = "refused the call's since or until; details went to the client only"
)

func Test_run_mcp_spending_returns_the_spend_json_document(t *testing.T) {
	cases := []struct {
		name      string
		store     func(*testing.T, string)
		cliArgs   []string
		arguments map[string]any
	}{
		{
			name: "all five given", store: populatedAnalysisStore,
			cliArgs: []string{
				"spend", "--since", "2026-01", "--until", "2026-08", "--by", "payee", "--currency", "CAD",
				"--account", "Chequing", "--account", "US Chequing", "--account", "Linked", "--account", "Old Card",
			},
			arguments: map[string]any{
				"since": "2026-01", "until": "2026-08", "by": "payee", "currency": "CAD",
				"accounts": []string{"Chequing", "US Chequing", "Linked", "Old Card"},
			},
		},
		{name: "none given", store: populatedAnalysisStore, cliArgs: []string{"spend"}, arguments: map[string]any{}},
		{
			name: "native currency", store: populatedAnalysisStore,
			cliArgs: []string{"spend", "--currency", "native"}, arguments: map[string]any{"currency": "native"},
		},
		{
			name: "grouped by tag", store: multiTagAnalysisStore,
			cliArgs: []string{"spend", "--by", "tag"}, arguments: map[string]any{"by": "tag"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: c.store, cliArgs: c.cliArgs, tool: "spending", arguments: c.arguments,
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

func Test_run_mcp_spending_refuses_a_bad_window_in_mcp_words(t *testing.T) {
	cases := []struct {
		name      string
		arguments map[string]any
		want      string
	}{
		{
			name: "since is not a date", arguments: map[string]any{"since": "2024-13"},
			want: `since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`,
		},
		{
			name: "since is empty", arguments: map[string]any{"since": ""},
			want: `since "" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`,
		},
		{
			name: "until is not a date", arguments: map[string]any{"until": "2024-13"},
			want: `until "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`,
		},
		{
			name: "since is after until", arguments: map[string]any{"since": "2025", "until": "2024"},
			want: "since 2025 is after until 2024",
		},
		{
			name: "until is before the default since", arguments: map[string]any{"until": "2025-03"},
			want: "until 2025-03 is before the default since 2026-01-01; pass since too",
		},
		{
			name: "since is in the future with no until", arguments: map[string]any{"since": "2099"},
			want: "since 2099 is after today; pass until to include future-dated transactions",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			populatedAnalysisStore(t, home)
			ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
			defer cancel()
			peer := startClockedMCP(ctx, t)

			result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "spending", Arguments: c.arguments})
			require.NoError(t, err)
			require.NoError(t, peer.session.Close())
			peer.waitForExit(ctx, t)

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(result))
			assert.Equal(t, spendingLogPrefix+windowRefusedLog+"\n", peer.stderr.String())
		})
	}
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
