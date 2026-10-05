package main

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// acbToolTwinRows is acbToolRows plus Acme Twin, which shares Acme's ticker.
func acbToolTwinRows() store.Rows {
	rows := acbToolRows()
	rows.Securities = append(rows.Securities,
		store.Security{ID: "sec-twin", SourceID: 6, Name: "Acme Twin", Ticker: new("ACME"), Currency: new("CAD")})
	rows.InvestmentTransactions = append(rows.InvestmentTransactions,
		acbTrade("inv-twin-buy", 10, "acct-cad", "sec-twin", store.ActionBuy, "CAD", day(2025, time.April, 1), 3_000_000, -30_000))
	return rows
}

func Test_run_mcp_acb_warns_of_securities_the_security_param_leaves_out(t *testing.T) {
	const (
		twins = `"ACME" is 2 securities in Quicken (Acme Corp, Acme Twin); quarry keeps a separate ACB for each; if they are the same, merge them in Quicken`
		drip  = `"Drip Fund" has reinvested dividends with no cost, so its ACB is too low and its gains too high; ` +
			"enter their cost in Quicken; acb with security sec-drip lists them"
		gift = `"Gift Fund" has shares added with no cost, so its ACB is too low and its gains too high; ` +
			"data_quality with type shares-without-cost lists them"
		maple = `"Maple Fund" is held only in registered accounts, so it has no ACB`
	)
	cases := []struct {
		name     string
		security string
		want     []string
	}{
		{name: "one named security leaves the others' lines in", security: "sec-acme", want: []string{drip, gift, twins}},
		{name: "a registered-only security is named", security: "sec-maple", want: []string{maple, drip, gift, twins}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: func(t *testing.T, home string) {
					t.Helper()
					replaceStoreWithRates(t, home, acbToolTwinRows(), usdRate(day(2024, time.January, 2), 1_250_000))
				},
				config: acbToolConfig, cliArgs: []string{"acb", "--security", c.security}, tool: "acb",
				arguments: map[string]any{"security": []string{c.security}},
			})

			assert.Equal(t, c.want, got.toolWarnings)
		})
	}
}

func Test_run_mcp_acb_keeps_the_document_key_orders_the_cli_prints(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
	}{
		{
			name:    "a year ends with its gain, the return of capital gain, then the superficial losses",
			pattern: `"gain":"[^"]*","return_of_capital_gain":"0.00","possible_superficial_losses":\d+,"unknown_cost_sales"`,
		},
		{name: "an event ends with unknown_cost", pattern: `"gain":(null|"[^"]*"),"unknown_cost":(true|false)\}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: seedACBToolStore, config: acbToolConfig, cliArgs: []string{"acb"}, tool: "acb", arguments: map[string]any{},
			})

			assert.Regexp(t, c.pattern, got.toolBody)
		})
	}
}

func Test_run_mcp_acb_matches_the_cli_on_each_edge_row_with_and_without_year_and_security(t *testing.T) {
	closedAccount := func(t *testing.T, home string) {
		t.Helper()
		rows := acbToolRows()
		rows.Accounts[0].Closed = true
		replaceStoreWithRates(t, home, rows, usdRate(day(2024, time.January, 2), 1_250_000))
	}
	noRates := func(t *testing.T, home string) {
		t.Helper()
		replaceStoreWithRates(t, home, acbToolRows())
	}
	const returnOfCapitalOnly = acbToolConfig + `
[[acb.adjustment]]
security = "sec-acme"
date = 2024-12-01
return-of-capital = 5000.00
`
	cases := []struct {
		name      string
		store     func(t *testing.T, home string)
		config    string
		cliArgs   []string
		arguments map[string]any
	}{
		{name: "a closed account, none given", store: closedAccount, config: acbToolConfig, cliArgs: []string{"acb"}, arguments: map[string]any{}},
		{
			name: "a closed account, year given", store: closedAccount, config: acbToolConfig,
			cliArgs: []string{"acb", "--year", "2025"}, arguments: map[string]any{"year": 2025},
		},
		{
			name: "a closed account, security given", store: closedAccount, config: acbToolConfig,
			cliArgs: []string{"acb", "--security", "sec-acme"}, arguments: map[string]any{"security": []string{"sec-acme"}},
		},
		{name: "a USD trade with no rate, none given", store: noRates, config: acbToolConfig, cliArgs: []string{"acb"}, arguments: map[string]any{}},
		{
			name: "a USD trade with no rate, year given", store: noRates, config: acbToolConfig,
			cliArgs: []string{"acb", "--year", "2026"}, arguments: map[string]any{"year": 2026},
		},
		{
			name: "a USD trade with no rate, security given", store: noRates, config: acbToolConfig,
			cliArgs: []string{"acb", "--security", "sec-vti"}, arguments: map[string]any{"security": []string{"sec-vti"}},
		},
		{
			name: "a year with only a return of capital gain, none given", store: seedACBToolStore, config: returnOfCapitalOnly,
			cliArgs: []string{"acb"}, arguments: map[string]any{},
		},
		{
			name: "a year with only a return of capital gain, year given", store: seedACBToolStore, config: returnOfCapitalOnly,
			cliArgs: []string{"acb", "--year", "2024"}, arguments: map[string]any{"year": 2024},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{store: c.store, config: c.config, cliArgs: c.cliArgs, tool: "acb", arguments: c.arguments})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, acbInToolWords(got.cliWarnings), got.toolWarnings)
		})
	}
}
