package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shortCoveredRows is Money Fund bought for 100.00, 15 sold for 20.00 (a loss, short 5), then cover units bought.
func shortCoveredRows(cover int64) store.Rows {
	rows := oversoldRows()
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-fund", store.ActionBuy, "CAD", day(2017, time.January, 3), 10_000_000, -10_000),
		acbTrade("inv-sell", 2, "acct-cad", "sec-fund", store.ActionSell, "CAD", day(2017, time.January, 12), -15_000_000, 2_000),
		acbTrade("inv-cover", 3, "acct-cad", "sec-fund", store.ActionBuy, "CAD", day(2017, time.January, 20), cover, -500),
	}

	return rows
}

func Test_run_acb_json_marks_an_oversold_loss_superficial_only_when_the_cover_leaves_shares_held(t *testing.T) {
	cases := []struct {
		name  string
		cover int64
		want  bool
	}{
		{name: "a buy that only covers the short", cover: 5_000_000, want: false},
		{name: "a buy that covers the short and holds 3 more", cover: 8_000_000, want: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			acbFixture(t, "[accounts]\nnon-registered = [\"acct-cad\"]\n", shortCoveredRows(c.cover))

			stdout, _ := runACB(t, "--json")

			doc := decodeACBYear(t, stdout)
			require.Len(t, doc.Years, 1)
			require.Len(t, doc.Years[0].Sales, 1)
			assert.Equal(t, c.want, doc.Years[0].Sales[0].PossibleSuperficialLoss)
		})
	}
}

// shortSplitRows is Money Fund bought for 100.00, 120 sold for 120.00 (short 20), then split 2 for 1.
func shortSplitRows() store.Rows {
	rows := oversoldRows()
	split := acbTrade("inv-split", 3, "acct-cad", "sec-fund", store.ActionSplit, "CAD", day(2017, time.February, 1), 0, 0)
	split.Shares, split.SplitNewShares, split.SplitOldShares = nil, new(int64(2_000_000)), new(int64(1_000_000))
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-fund", store.ActionBuy, "CAD", day(2017, time.January, 3), 100_000_000, -10_000),
		acbTrade("inv-sell", 2, "acct-cad", "sec-fund", store.ActionSell, "CAD", day(2017, time.January, 12), -120_000_000, 12_000),
		split,
	}

	return rows
}

func Test_run_acb_json_doubles_a_short_position_a_split_multiplies(t *testing.T) {
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-cad\"]\n", shortSplitRows())

	stdout, _ := runACB(t, "--json")

	var doc shortDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
	require.Len(t, doc.Securities, 1)
	position := doc.Securities[0]
	assert.Equal(t, []string{"-40.000000", "0.00"}, []string{position.Shares, position.ACB})
	assert.Nil(t, position.ACBPerShare)
	assert.True(t, position.Incomplete)
	require.Len(t, position.Events, 3)
	assert.Equal(t, []string{"split", "-40.000000", "0.00"},
		[]string{position.Events[2].Action, position.Events[2].SharesHeld, position.Events[2].ACB})
}

func Test_run_acb_security_prints_the_split_of_a_short_position_with_its_negative_shares_held(t *testing.T) {
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-cad\"]\n", shortSplitRows())
	w := [10]int{10, 13, 6, 6, 11, 4, 7, 11, 6, 12}

	stdout, _ := runACB(t, "--security", "MNY")

	assert.Equal(t, "ACB history of \"Money Fund\" (MNY), in CAD\n\n"+
		acbHistoryRowOf(w, acbHistoryHeaderCells)+
		acbHistoryRowOf(w, [11]string{"2017-01-03", "CAD Brokerage", "buy", "100", "-100.00 CAD", "", "-100.00", "100", "100.00", ""})+
		acbHistoryRowOf(w, [11]string{"2017-01-12", "CAD Brokerage", "sell", "120", "120.00 CAD", "", "120.00", "-20", "0.00", "20.00", "unknown cost"})+
		acbHistoryRowOf(w, [11]string{"2017-02-01", "CAD Brokerage", "split", "0", "0.00 CAD", "", "0.00", "-40", "0.00", ""}),
		stdout)
}

func Test_run_acb_json_lists_the_oversold_removal_warning_after_the_removal_warning(t *testing.T) {
	rows := shortOpenRows()
	rows.InvestmentTransactions[1] = acbTrade("inv-remove", 2, "acct-cad", "sec-fund", store.ActionRemoveShares, "CAD",
		day(2017, time.January, 12), -110_000_000, 0)
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-cad\"]\n", rows)

	stdout, stderr := runACB(t, "--json")

	doc := decodeACBYear(t, stdout)
	assert.Equal(t, []string{moneyFundRemovalWarning, moneyFundRemovalOversoldWarning}, doc.Warnings)
	assert.Equal(t, stderrWarnings(moneyFundRemovalWarning, moneyFundRemovalOversoldWarning), stderr)
}

func Test_run_acb_security_marks_a_sale_that_is_both_a_possible_superficial_loss_and_of_unknown_cost(t *testing.T) {
	acbFixture(t, acbRegisteredConfig, acbStackedRows())
	w := [10]int{10, 13, 10, 6, 13, 4, 9, 11, 8, 12}

	stdout, stderr := runACB(t, "--security", "ACME")

	assert.Equal(t, "ACB history of \"Acme Corp\" (ACME), in CAD\n\n"+
		acbHistoryRowOf(w, acbHistoryHeaderCells)+
		acbHistoryRowOf(w, [11]string{"2025-01-10", "CAD Brokerage", "buy", "10", "-1,000.00 CAD", "", "-1,000.00", "10", "1,000.00", ""})+
		acbHistoryRowOf(w, [11]string{"2025-01-20", "CAD Brokerage", "add_shares", "10", "0.00 CAD", "", "0.00", "20", "1,000.00", "", "unknown cost"})+
		acbHistoryRowOf(w, [11]string{"2025-03-01", "CAD Brokerage", "sell", "10", "300.00 CAD", "", "300.00", "10", "500.00", "-200.00", "possible superficial loss, unknown cost"}),
		stdout)
	assert.Equal(t, stderrWarnings(superficialLossWarning, acmeAddedNoCostWarning), stderr)
}

func Test_run_mcp_acb_matches_the_cli_on_the_superficial_removal_december_and_short_rows(t *testing.T) {
	seed := func(rows func() store.Rows) func(t *testing.T, home string) {
		return func(t *testing.T, home string) {
			t.Helper()
			replaceStoreWithRates(t, home, rows(), usdRate(day(2024, time.January, 2), 1_250_000))
		}
	}
	oversoldRemoval := func() store.Rows {
		rows := shortOpenRows()
		rows.InvestmentTransactions[1] = acbTrade("inv-remove", 2, "acct-cad", "sec-fund", store.ActionRemoveShares, "CAD",
			day(2017, time.January, 12), -110_000_000, 0)
		return rows
	}
	const (
		pair = "[accounts]\nnon-registered = [\"acct-usd\", \"acct-cad\"]\n"
		fund = "[accounts]\nnon-registered = [\"acct-cad\"]\n"
	)
	cases := []struct {
		name      string
		store     func(t *testing.T, home string)
		config    string
		cliArgs   []string
		arguments map[string]any
	}{
		{name: "a possible superficial loss", store: seed(acbSuperficialRows), config: acbSuperficialConfig, cliArgs: []string{"acb"}, arguments: map[string]any{}},
		{
			name: "a possible superficial loss, year given", store: seed(acbSuperficialRows), config: acbSuperficialConfig,
			cliArgs: []string{"acb", "--year", "2025"}, arguments: map[string]any{"year": 2025},
		},
		{name: "a removal with no sale", store: seed(acbSharesRows), config: fund, cliArgs: []string{"acb"}, arguments: map[string]any{}},
		{name: "a December sale", store: seed(noRateSharedTickerRows), config: pair, cliArgs: []string{"acb"}, arguments: map[string]any{}},
		{name: "a removal beyond the pool", store: seed(oversoldRemoval), config: fund, cliArgs: []string{"acb"}, arguments: map[string]any{}},
		{
			name: "a short, year given", store: seed(shortOpenRows), config: fund,
			cliArgs: []string{"acb", "--year", "2017"}, arguments: map[string]any{"year": 2017},
		},
		{
			name: "a short, security given", store: seed(shortOpenRows), config: fund,
			cliArgs: []string{"acb", "--security", "MNY"}, arguments: map[string]any{"security": []string{"MNY"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{store: c.store, config: c.config, cliArgs: c.cliArgs, tool: "acb", arguments: c.arguments})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, acbInToolWords(got.cliWarnings), got.toolWarnings)
			assert.NotEmpty(t, got.toolWarnings)
		})
	}
}
