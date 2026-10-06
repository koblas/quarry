package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const acbToolConfig = "[accounts]\nnon-registered = [\"acct-cad\", \"acct-usd\"]\nregistered = [\"acct-rrsp\"]\n"

const acbToolAdjustmentConfig = acbToolConfig + `
[[acb.adjustment]]
security = "sec-acme"
date = 2025-01-15
return-of-capital = 100.00
`

const acbToolReinvestedConfig = acbToolConfig + `
[[acb.adjustment]]
security = "sec-acme"
date = 2025-01-15
reinvested-distribution = 100.00
`

// acbToolRows is acbRows plus two securities in acct-cad that took in shares with no cost: one added, one reinvested.
func acbToolRows() store.Rows {
	rows := acbRows()
	rows.Securities = append(rows.Securities,
		store.Security{ID: "sec-gift", SourceID: 4, Name: "Gift Fund", Ticker: new("GIFT"), Currency: new("CAD")},
		store.Security{ID: "sec-drip", SourceID: 5, Name: "Drip Fund", Ticker: new("DRIP"), Currency: new("CAD")},
	)
	rows.InvestmentTransactions = append(rows.InvestmentTransactions,
		acbTrade("inv-gift-add", 8, "acct-cad", "sec-gift", store.ActionAddShares, "CAD", day(2025, time.February, 3), 5_000_000, 0),
		acbTrade("inv-drip-reinvest", 9, "acct-cad", "sec-drip", store.ActionReinvestDividend, "CAD", day(2025, time.March, 1), 1_000_000, 0),
	)
	return rows
}

func seedACBToolStore(t *testing.T, home string) {
	t.Helper()
	replaceStoreWithRates(t, home, acbToolRows(),
		usdRate(day(2024, time.January, 2), 1_250_000), usdRate(day(2026, time.January, 2), 1_400_000))
}

// acbInToolWords is the CLI warnings with each command a no-cost line names replaced by the tool call that does the same.
func acbInToolWords(cliWarnings []string) []string {
	mapped := make([]string, len(cliWarnings))
	for i, line := range cliWarnings {
		line = strings.ReplaceAll(line, "quarry findings --type shares-without-cost", "data_quality with type shares-without-cost")
		mapped[i] = strings.ReplaceAll(line, "quarry acb --security ", "acb with security ")
	}
	return mapped
}

func Test_run_mcp_acb_returns_the_acb_json_document(t *testing.T) {
	cases := []struct {
		name      string
		config    string
		cliArgs   []string
		arguments map[string]any
	}{
		{name: "none given", config: acbToolConfig, cliArgs: []string{"acb"}, arguments: map[string]any{}},
		{name: "year given", config: acbToolConfig, cliArgs: []string{"acb", "--year", "2025"}, arguments: map[string]any{"year": 2025}},
		{
			name: "security given", config: acbToolConfig,
			cliArgs: []string{"acb", "--security", "sec-drip"}, arguments: map[string]any{"security": []string{"sec-drip"}},
		},
		{
			name: "year and security given", config: acbToolConfig,
			cliArgs:   []string{"acb", "--year", "2025", "--security", "sec-gift"},
			arguments: map[string]any{"year": 2025, "security": []string{"sec-gift"}},
		},
		{name: "one adjustment configured", config: acbToolAdjustmentConfig, cliArgs: []string{"acb"}, arguments: map[string]any{}},
		{name: "one reinvested distribution configured", config: acbToolReinvestedConfig, cliArgs: []string{"acb"}, arguments: map[string]any{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: seedACBToolStore, config: c.config, cliArgs: c.cliArgs, tool: "acb", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, acbInToolWords(got.cliWarnings), got.toolWarnings)
		})
	}
}

func Test_run_mcp_acb_lists_a_reinvested_distribution_as_an_event_that_raises_the_acb(t *testing.T) {
	got := runBothSurfaces(t, toolDocumentRun{
		store: seedACBToolStore, config: acbToolReinvestedConfig, cliArgs: []string{"acb", "--security", "sec-acme"}, tool: "acb",
		arguments: map[string]any{"security": []string{"sec-acme"}},
	})

	type event struct {
		Action string `json:"action"`
		CAD    string `json:"cad"`
		ACB    string `json:"acb"`
	}
	var doc struct {
		Securities []struct {
			Events []event `json:"events"`
		} `json:"securities"`
	}
	require.NoError(t, json.Unmarshal([]byte(got.toolBody), &doc), got.toolBody)
	require.Len(t, doc.Securities, 1)
	assert.Contains(t, doc.Securities[0].Events, event{Action: "reinvested distribution", CAD: "-100.00", ACB: "1700.00"})
}
