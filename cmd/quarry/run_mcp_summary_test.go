package main

import (
	"context"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedSummaryStoreAt builds the September summary store into home twice, so the second build fixes Kiosk's finding
// and finds Pharmacy's while Shell's carries.
func seedSummaryStoreAt(t *testing.T, home string) {
	t.Helper()
	replaceStore(t, home, summaryRows(false))
	replaceStore(t, home, summaryRows(true))
}

// seedSummaryNativeStoreAt builds summaryNativeRows, which holds no exchange rates, into home.
func seedSummaryNativeStoreAt(t *testing.T, home string) {
	t.Helper()
	replaceStore(t, home, summaryNativeRows())
}

// seedSummaryRatedStoreAt builds summaryNativeRows into home with a USD rate dated January 2.
func seedSummaryRatedStoreAt(t *testing.T, home string) {
	t.Helper()
	replaceStoreWithRates(t, home, summaryNativeRows(), usdRate(day(2026, time.January, 2), 1_350_000))
}

// seedSummaryFirstMonthStoreAt builds firstMonthRows into home.
func seedSummaryFirstMonthStoreAt(t *testing.T, home string) {
	t.Helper()
	replaceStore(t, home, firstMonthRows())
}

func Test_run_mcp_monthly_summary_returns_the_summary_json_document(t *testing.T) {
	cases := []struct {
		name      string
		store     func(t *testing.T, home string)
		config    string
		cliArgs   []string
		arguments map[string]any
	}{
		{name: "last month by default", store: seedSummaryStoreAt, cliArgs: []string{"summary"}, arguments: map[string]any{}},
		{
			name: "an earlier month", store: seedSummaryStoreAt,
			cliArgs: []string{"summary", "--month", "2026-08"}, arguments: map[string]any{"month": "2026-08"},
		},
		{
			name: "native currencies", store: seedSummaryNativeStoreAt,
			cliArgs: []string{"summary", "--currency", "native"}, arguments: map[string]any{"currency": "native"},
		},
		{
			name: "USD with rates", store: seedSummaryRatedStoreAt,
			cliArgs: []string{"summary", "--currency", "USD"}, arguments: map[string]any{"currency": "USD"},
		},
		{
			name: "a config key quarry ignores", store: seedSummaryStoreAt, config: "colour = \"red\"\n",
			cliArgs: []string{"summary"}, arguments: map[string]any{},
		},
		{
			name: "an unreadable config with a currency", store: seedSummaryStoreAt, config: "[snapshots\nkeep = 24\n",
			cliArgs: []string{"summary", "--currency", "CAD"}, arguments: map[string]any{"currency": "CAD"},
		},
		{
			name: "the first month of data", store: seedSummaryFirstMonthStoreAt,
			cliArgs: []string{"summary", "--month", "2026-09"}, arguments: map[string]any{"month": "2026-09"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: c.store, config: c.config, cliArgs: c.cliArgs, tool: "monthly_summary", arguments: c.arguments, now: summaryClock,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, got.cliWarnings, got.toolWarnings)
		})
	}
}

func Test_run_mcp_monthly_summary_words_the_tool_advice_not_the_flag(t *testing.T) {
	predatesSeptember := func(t *testing.T, home string) {
		t.Helper()
		replaceStore(t, home, withSnapshotTaken(summaryRows(true), time.Date(2026, time.September, 28, 14, 2, 0, 0, time.UTC)))
	}
	const predatesLead = "the store was built from a snapshot taken 2026-09-28 14:02 UTC, before September 2026 ended, " +
		"so transactions from the rest of the month are missing; open your Quicken file, run quarry sync, then "
	cases := []struct {
		name      string
		store     func(t *testing.T, home string)
		cliArgs   []string
		arguments map[string]any
		wantCLI   []string
		wantTool  []string
	}{
		{
			name: "a snapshot taken before the default month ended", store: predatesSeptember,
			cliArgs: []string{"summary"}, arguments: map[string]any{},
			wantCLI:  []string{predatesLead + "run quarry summary again"},
			wantTool: []string{predatesLead + "call monthly_summary again"},
		},
		{
			name: "the same for a month the call names", store: predatesSeptember,
			cliArgs: []string{"summary", "--month", "2026-09"}, arguments: map[string]any{"month": "2026-09"},
			wantCLI:  []string{predatesLead + "run quarry summary --month 2026-09 again"},
			wantTool: []string{predatesLead + "call monthly_summary again"},
		},
		{
			name: "a store with no exchange rates", store: seedSummaryNativeStoreAt,
			cliArgs: []string{"summary"}, arguments: map[string]any{},
			wantCLI:  []string{septemberTimeUnknownText(), noRatesLine, netWorthNoRatesLine},
			wantTool: []string{septemberTimeUnknownText(), noRatesLine, strings.Replace(netWorthNoRatesLine, "--currency", "currency", 1)},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: c.store, cliArgs: c.cliArgs, tool: "monthly_summary", arguments: c.arguments, now: summaryClock,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, c.wantCLI, got.cliWarnings)
			assert.Equal(t, c.wantTool, got.toolWarnings)
		})
	}
}

func Test_run_mcp_monthly_summary_refuses_a_month_in_mcp_words(t *testing.T) {
	cases := []struct {
		name  string
		month string
		want  string
	}{
		{
			name: "a month without its leading zero", month: "2026-9",
			want: `month "2026-9" is not a month; use YYYY-MM, such as 2026-09`,
		},
		{
			name: "the current month", month: "2026-10",
			want: "month 2026-10 has not ended; monthly_summary covers whole months, so pass 2026-09 or earlier",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			seedSummaryStoreAt(t, home)
			ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
			defer cancel()
			peer := startMCPAt(ctx, t, summaryClock)

			result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "monthly_summary", Arguments: map[string]any{"month": c.month}})
			require.NoError(t, err)
			require.NoError(t, peer.session.Close())
			peer.waitForExit(ctx, t)

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(result))
			assert.Equal(t, "quarry: mcp: monthly_summary: refused the call's month; details went to the client only\n", peer.stderr.String())
		})
	}
}
