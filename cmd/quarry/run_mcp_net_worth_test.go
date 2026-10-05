package main

import (
	"context"
	"testing"

	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const netWorthLogPrefix = "quarry: mcp: net_worth: "

// netWorthSeeder is seed as the store hook of a toolDocumentRun; seed sets the HOME both surfaces read.
func netWorthSeeder(seed func(*testing.T)) func(*testing.T, string) {
	return func(t *testing.T, _ string) {
		t.Helper()
		seed(t)
	}
}

func Test_run_mcp_net_worth_returns_the_networth_json_document(t *testing.T) {
	cases := []struct {
		name      string
		seed      func(*testing.T)
		cliArgs   []string
		arguments map[string]any
	}{
		{name: "today by default", seed: seedNetWorthStore, cliArgs: []string{"networth"}, arguments: map[string]any{}},
		{
			name: "month ends from since to until", seed: seedNetWorthHistoryStore,
			cliArgs:   []string{"networth", "--since", "2026-01", "--until", "2026-03"},
			arguments: map[string]any{"since": "2026-01", "until": "2026-03"},
		},
		{
			name: "month ends from since to today", seed: seedNetWorthHistoryStore,
			cliArgs: []string{"networth", "--since", "2026-01"}, arguments: map[string]any{"since": "2026-01"},
		},
		{
			name: "one day in native currencies", seed: seedNetWorthStore,
			cliArgs:   []string{"networth", "--as-of", "2026-03-12", "--currency", "native"},
			arguments: map[string]any{"as_of": "2026-03-12", "currency": "native"},
		},
		{
			name: "month ends in native currencies", seed: seedNetWorthHistoryStore,
			cliArgs:   []string{"networth", "--since", "2026-01", "--until", "2026-03", "--currency", "native"},
			arguments: map[string]any{"since": "2026-01", "until": "2026-03", "currency": "native"},
		},
		{
			name: "month ends before the first balance", seed: seedNetWorthStore,
			cliArgs:   []string{"networth", "--since", "2026-01", "--until", "2026-02"},
			arguments: map[string]any{"since": "2026-01", "until": "2026-02"},
		},
		{
			name: "one day before the first balance", seed: seedNetWorthStore,
			cliArgs: []string{"networth", "--as-of", "2026-03-01"}, arguments: map[string]any{"as_of": "2026-03-01"},
		},
		{
			name: "no account in the reports has a balance", seed: seedUncountedOnlyStore,
			cliArgs: []string{"networth", "--as-of", "2026-03-01"}, arguments: map[string]any{"as_of": "2026-03-01"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: netWorthSeeder(c.seed), cliArgs: c.cliArgs, tool: "net_worth", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, got.cliWarnings, got.toolWarnings)
		})
	}
}

func Test_run_mcp_net_worth_words_the_rate_advice_for_a_tool_parameter_not_a_flag(t *testing.T) {
	cases := []struct {
		name      string
		seed      func(*testing.T)
		cliArgs   []string
		arguments map[string]any
		wantCLI   string
		wantTool  string
	}{
		{
			name: "one day with a USD balance before the first rate", seed: seedNetWorthStore,
			cliArgs: []string{"networth", "--as-of", "2026-03-05"}, arguments: map[string]any{"as_of": "2026-03-05"},
			wantCLI:  netWorthBeforeFirstRateLine,
			wantTool: "USD balances on 2026-03-05, before 2026-03-10, the first exchange rate in the store, are not converted to CAD and are left out of the CAD total; pass currency native to list them",
		},
		{
			name: "CAD balances in USD with no exchange rate", seed: seedNetWorthCADOnlyStore,
			cliArgs:   []string{"networth", "--as-of", "2026-01-20", "--currency", "USD"},
			arguments: map[string]any{"as_of": "2026-01-20", "currency": "USD"},
			wantCLI:   "the store has no exchange rates, so CAD balances are not converted to USD and are left out of the USD total; pass --currency native to list them, or run quarry sync to fetch rates",
			wantTool:  "the store has no exchange rates, so CAD balances are not converted to USD and are left out of the USD total; pass currency native to list them, or run quarry sync to fetch rates",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: netWorthSeeder(c.seed), cliArgs: c.cliArgs, tool: "net_worth", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, []string{c.wantCLI}, got.cliWarnings)
			assert.Equal(t, []string{c.wantTool}, got.toolWarnings)
		})
	}
}

func Test_run_mcp_net_worth_returns_the_left_out_holding_warnings_the_cli_prints(t *testing.T) {
	seed := func(t *testing.T, home string) {
		t.Helper()
		rows := noRateHoldingRows()
		rows.Securities = append(rows.Securities,
			store.Security{ID: "sec-bare", SourceID: 5, Name: "Bare Fund", Ticker: new("BARE"), Currency: new("CAD")})
		rows.InvestmentTransactions = append(rows.InvestmentTransactions,
			holdingsBuy("inv-bare", 5, "acct-cad", "sec-bare", "CAD", 40_000_000))
		replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
	}

	got := runBothSurfaces(t, toolDocumentRun{
		store: seed, cliArgs: []string{"networth", "--as-of", "2026-03-05"}, tool: "net_worth",
		arguments: map[string]any{"as_of": "2026-03-05"},
	})

	assert.Equal(t, got.cliBody, got.toolBody)
	assert.Equal(t, got.cliWarnings, got.toolWarnings)
	assert.Equal(t, []string{
		`"Brokerage" holds 2 securities with no price on or before 2026-03-05, so its balance leaves them out; enter prices in Quicken, then run quarry sync`,
		`"IRA" holds 1 security with no price on or before 2026-03-05, so its balance leaves it out; enter a price in Quicken, then run quarry sync`,
		`"Brokerage" holds 1 USD security valued on 2026-03-05, before 2026-03-10, the first exchange rate in the store, so its CAD balance leaves it out`,
	}, got.toolWarnings)
}

func Test_run_mcp_net_worth_refuses_a_call_it_cannot_value_in_mcp_words(t *testing.T) {
	cases := []struct {
		name      string
		arguments map[string]any
		want      string
		wantLog   string
	}{
		{
			name: "as_of after today", arguments: map[string]any{"as_of": "2027-01-01"},
			want:    "as_of 2027-01-01 is after today; net worth is valued up to today only, so pass an earlier as_of",
			wantLog: asOfRefusedLog,
		},
		{
			name: "since after today", arguments: map[string]any{"since": "2027-01-01"},
			want:    "since 2027-01-01 is after today; net worth is valued up to today only, so pass an earlier since",
			wantLog: windowRefusedLog,
		},
		{
			name: "as_of beside since", arguments: map[string]any{"as_of": "2026-03-12", "since": "2026-01"},
			want:    "as_of cannot be combined with since or until; pass as_of for one day, or since and until for month ends",
			wantLog: asOfRefusedLog,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedNetWorthStore(t)
			ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
			defer cancel()
			peer := startClockedMCP(ctx, t)

			result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "net_worth", Arguments: c.arguments})
			require.NoError(t, err)
			require.NoError(t, peer.session.Close())
			peer.waitForExit(ctx, t)

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(result))
			assert.Equal(t, netWorthLogPrefix+c.wantLog+"\n", peer.stderr.String())
		})
	}
}
