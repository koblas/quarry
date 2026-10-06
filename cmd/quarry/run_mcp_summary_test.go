package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// seedSummaryStoreAt builds the September summary store into home twice, so the second build fixes Kiosk's finding
// and finds Pharmacy's while Shell's carries.
func seedSummaryStoreAt(t *testing.T, home string) {
	t.Helper()
	replaceStore(t, home, summaryRows(false))
	replaceStore(t, home, summaryRows(true))
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
			name: "native currencies", store: func(t *testing.T, home string) { replaceStore(t, home, summaryNativeRows()) },
			cliArgs: []string{"summary", "--currency", "native"}, arguments: map[string]any{"currency": "native"},
		},
		{
			name: "USD with rates",
			store: func(t *testing.T, home string) {
				replaceStoreWithRates(t, home, summaryNativeRows(), usdRate(day(2026, time.January, 2), 1_350_000))
			},
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
			name: "the first month of data", store: func(t *testing.T, home string) { replaceStore(t, home, firstMonthRows()) },
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
