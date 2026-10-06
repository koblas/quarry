package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const moneyFundOversoldWarning = `"Money Fund": the sale on 2017-01-12 in "CAD Brokerage" sold 10 more shares than the ` +
	"non-registered accounts held; quarry counts them at no cost, so the sale's gain is too high by what they cost, " +
	"and the next 10 shares acquired only bring the holding back to 0; correct the shares in Quicken if they are wrong"

// oversoldRows is one non-registered brokerage whose Money Fund holding buys 100 for 100.00, sells 110 for
// 110.00, then buys 15 for 15.00.
func oversoldRows() store.Rows {
	rows := spendRows([]store.Account{
		{ID: "acct-cad", SourceID: 1, Name: "CAD Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
	})
	rows.Securities = []store.Security{{ID: "sec-fund", SourceID: 1, Name: "Money Fund", Ticker: new("MNY"), Currency: new("CAD")}}
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-fund", store.ActionBuy, "CAD", day(2017, time.January, 3), 100_000_000, -10_000),
		acbTrade("inv-sell", 2, "acct-cad", "sec-fund", store.ActionSell, "CAD", day(2017, time.January, 12), -110_000_000, 11_000),
		acbTrade("inv-cover", 3, "acct-cad", "sec-fund", store.ActionBuy, "CAD", day(2017, time.January, 30), 15_000_000, -1_500),
	}

	return rows
}

// shortDoc is the part of acb's --json document the oversold sale reads.
type shortDoc struct {
	Years []struct {
		Year             int `json:"year"`
		UnknownCostSales int `json:"unknown_cost_sales"`
		Sales            []struct {
			ACB         string `json:"acb"`
			Gain        string `json:"gain"`
			UnknownCost bool   `json:"unknown_cost"`
		} `json:"sales"`
	} `json:"years"`
	Securities []struct {
		Security    string  `json:"security"`
		Shares      string  `json:"shares"`
		ACB         string  `json:"acb"`
		ACBPerShare *string `json:"acb_per_share"`
		Incomplete  bool    `json:"incomplete"`
		Events      []struct {
			Action     string `json:"action"`
			SharesHeld string `json:"shares_held"`
			ACB        string `json:"acb"`
		} `json:"events"`
	} `json:"securities"`
}

func Test_run_acb_counts_shares_sold_beyond_the_pool_at_no_cost_and_lets_the_next_buy_cover_the_short(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[accounts]\nnon-registered = [\"acct-cad\"]\n")
	replaceStore(t, home, oversoldRows())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"acb", "--json"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	var doc shortDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	require.Len(t, doc.Years, 1)
	assert.Equal(t, 1, doc.Years[0].UnknownCostSales)
	require.Len(t, doc.Years[0].Sales, 1)
	assert.Equal(t, "100.00", doc.Years[0].Sales[0].ACB)
	assert.Equal(t, "10.00", doc.Years[0].Sales[0].Gain)
	assert.True(t, doc.Years[0].Sales[0].UnknownCost)
	require.Len(t, doc.Securities, 1)
	events := doc.Securities[0].Events
	require.Len(t, events, 3)
	assert.Equal(t, []string{"sell", "-10.000000", "0.00"}, []string{events[1].Action, events[1].SharesHeld, events[1].ACB})
	assert.Equal(t, []string{"buy", "5.000000", "5.00"}, []string{events[2].Action, events[2].SharesHeld, events[2].ACB})
	assert.Equal(t, []string{"Money Fund", "5.000000", "5.00"},
		[]string{doc.Securities[0].Security, doc.Securities[0].Shares, doc.Securities[0].ACB})
	assert.False(t, doc.Securities[0].Incomplete)
	assert.Equal(t, stderrWarnings(moneyFundOversoldWarning), stderr.String())
}

func Test_run_acb_shows_a_buy_that_covers_a_short_at_the_pro_rata_cost_of_the_units_beyond_it(t *testing.T) {
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-cad\"]\n", oversoldRows())
	w := [10]int{10, 13, 6, 6, 11, 4, 7, 11, 6, 12}

	positions, _ := runACB(t)
	history, stderr := runACB(t, "--security", "MNY")

	assert.Contains(t, positions, fmt.Sprintf("%-10s  %-6s  %6s  %4s  %13s\n", "Money Fund", "MNY", "5", "5.00", "1.0000"))
	assert.Contains(t, history, acbHistoryRowOf(w, [11]string{"2017-01-12", "CAD Brokerage", "sell", "110", "110.00 CAD", "", "110.00", "-10", "0.00", "10.00", "unknown cost"}))
	assert.Contains(t, history, acbHistoryRowOf(w, [11]string{"2017-01-30", "CAD Brokerage", "buy", "15", "-15.00 CAD", "", "-15.00", "5", "5.00", ""}))
	assert.Equal(t, stderrWarnings(moneyFundOversoldWarning), stderr)
}

const moneyFundShortWarning = `"Money Fund": the sale on 2017-01-12 in "CAD Brokerage" sold 1,122.84 more shares than the ` +
	"non-registered accounts held; quarry counts them at no cost, so the sale's gain is too high by what they cost, " +
	"and the next 1,122.84 shares acquired only bring the holding back to 0; correct the shares in Quicken if they are wrong"

const (
	moneyFundRemovalWarning = `"Money Fund": 110 shares left "CAD Brokerage" on 2017-01-12 without a sale; quarry took their share of the ACB out and reports no gain; ` +
		"if they went to a registered account or to someone else, that is a disposition at market value; check it with your accountant"
	moneyFundRemovalOversoldWarning = `"Money Fund": 10 more shares left "CAD Brokerage" on 2017-01-12 than the non-registered accounts held; ` +
		"the next 10 shares acquired only bring the holding back to 0; correct the shares in Quicken if they are wrong"
)

// shortOpenRows is one non-registered brokerage whose Money Fund holding buys 100 for 100.00, then sells 1,222.84 for
// 1,222.84, which leaves it short 1,122.84 with no buy to cover it.
func shortOpenRows() store.Rows {
	rows := oversoldRows()
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-fund", store.ActionBuy, "CAD", day(2017, time.January, 3), 100_000_000, -10_000),
		acbTrade("inv-sell", 2, "acct-cad", "sec-fund", store.ActionSell, "CAD", day(2017, time.January, 12), -1_222_840_000, 122_284),
	}

	return rows
}

func Test_run_acb_text_marks_the_oversold_sale_and_lists_no_short_position(t *testing.T) {
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-cad\"]\n", shortOpenRows())

	stdout, stderr := runACB(t)

	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		acbYearLine("Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss")+
		unknownCostYearLine("2017", "1", "1,222.84", "0.00", "100.00", "1,122.84", "1 sale of shares with unknown cost")+
		"\nACB on 2026-03-12, in CAD\n\n"+
		"Security  Ticker  Shares  ACB  ACB per share\n", //nolint:dupword // the ACB column sits beside the ACB per share column
		stdout)
	assert.Equal(t, stderrWarnings(moneyFundShortWarning), stderr)
}

func Test_run_acb_json_writes_a_short_position_with_negative_shares_and_no_acb_per_share(t *testing.T) {
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-cad\"]\n", shortOpenRows())

	stdout, stderr := runACB(t, "--json")

	var doc shortDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
	require.Len(t, doc.Securities, 1)
	position := doc.Securities[0]
	assert.Equal(t, []string{"-1122.840000", "0.00"}, []string{position.Shares, position.ACB})
	assert.Nil(t, position.ACBPerShare)
	assert.True(t, position.Incomplete)
	require.Len(t, position.Events, 2)
	assert.Equal(t, []string{"sell", "-1122.840000", "0.00"},
		[]string{position.Events[1].Action, position.Events[1].SharesHeld, position.Events[1].ACB})
	assert.Equal(t, stderrWarnings(moneyFundShortWarning), stderr)
}

func Test_run_acb_year_marks_the_oversold_sale_unknown_cost(t *testing.T) {
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-cad\"]\n", shortOpenRows())
	w := [7]int{10, 8, 8, 8, 7, 6, 12}

	stdout, stderr := runACB(t, "--year", "2017")

	assert.Equal(t, "Sales in 2017, in CAD\n\n"+
		acbSaleRowOf(w, [8]string{"Date", "Security", "Shares", "Proceeds", "Outlays", "ACB", "Gain or loss"})+
		acbSaleRowOf(w, [8]string{"2017-01-12", "MNY", "1,222.84", "1,222.84", "0.00", "100.00", "1,122.84", "unknown cost"})+
		acbSaleRowOf(w, [8]string{"Total", "", "", "1,222.84", "0.00", "100.00", "1,122.84"}),
		stdout)
	assert.Equal(t, stderrWarnings(moneyFundShortWarning), stderr)
}

func Test_run_acb_year_without_a_sale_still_warns_of_an_oversold_sale_in_another_year(t *testing.T) {
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-cad\"]\n", shortOpenRows())

	_, stderr := runACB(t, "--year", "2018")

	assert.True(t, strings.HasSuffix(stderr, stderrWarnings(moneyFundShortWarning)), stderr)
}

func Test_run_acb_security_prints_negative_shares_held_grouped(t *testing.T) {
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-cad\"]\n", shortOpenRows())
	w := [10]int{10, 13, 6, 8, 12, 4, 8, 11, 6, 12}

	stdout, stderr := runACB(t, "--security", "MNY")

	assert.Equal(t, "ACB history of \"Money Fund\" (MNY), in CAD\n\n"+
		acbHistoryRowOf(w, acbHistoryHeaderCells)+
		acbHistoryRowOf(w, [11]string{"2017-01-03", "CAD Brokerage", "buy", "100", "-100.00 CAD", "", "-100.00", "100", "100.00", ""})+
		acbHistoryRowOf(w, [11]string{"2017-01-12", "CAD Brokerage", "sell", "1,222.84", "1,222.84 CAD", "", "1,222.84", "-1,122.84", "0.00", "1,122.84", "unknown cost"}),
		stdout)
	assert.Equal(t, stderrWarnings(moneyFundShortWarning), stderr)
}

func Test_run_acb_names_a_removal_beyond_the_pool_after_its_removal_line(t *testing.T) {
	rows := shortOpenRows()
	rows.InvestmentTransactions[1] = acbTrade("inv-remove", 2, "acct-cad", "sec-fund", store.ActionRemoveShares, "CAD",
		day(2017, time.January, 12), -110_000_000, 0)
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-cad\"]\n", rows)

	_, stderr := runACB(t)

	assert.Equal(t, stderrWarnings(moneyFundRemovalWarning, moneyFundRemovalOversoldWarning), stderr)
}

// splitSoldOutRows is Money Fund bought for 3,000.00, split 1 for splitOld, then sold for 1,000.00: bought and
// sold are in millionths of shares.
func splitSoldOutRows(bought, splitOld, sold int64) store.Rows {
	rows := oversoldRows()
	split := acbTrade("inv-split", 2, "acct-cad", "sec-fund", store.ActionSplit, "CAD", day(2017, time.February, 1), 0, 0)
	split.Shares, split.SplitNewShares, split.SplitOldShares = nil, new(int64(1_000_000)), new(splitOld)
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-fund", store.ActionBuy, "CAD", day(2017, time.January, 3), bought, -300_000),
		split,
		acbTrade("inv-sell", 3, "acct-cad", "sec-fund", store.ActionSell, "CAD", day(2017, time.March, 1), -sold, 100_000),
	}

	return rows
}

func Test_run_acb_text_lists_no_position_and_skips_a_return_of_capital_once_a_split_pool_is_sold_out(t *testing.T) {
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-cad\"]\n\n"+
		"[[acb.adjustment]]\nsecurity = \"sec-fund\"\ndate = 2017-06-01\nreturn-of-capital = 10.00\n",
		splitSoldOutRows(100_000_000, 3_000_000, 33_333_333))

	stdout, stderr := runACB(t)

	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		"Year  Sales  Proceeds  Outlays       ACB  Gain or loss\n"+
		"2017      1  1,000.00     0.00  3,000.00     -2,000.00\n"+
		"\nACB on 2026-03-12, in CAD\n\n"+
		"Security  Ticker  Shares  ACB  ACB per share\n", //nolint:dupword // the ACB column sits beside the ACB per share column
		stdout)
	assert.Equal(t, stderrWarnings(
		configShown+`: acb.adjustment item 1 is for "Money Fund", which no non-registered account holds on 2017-06-01; quarry skips it`),
		stderr)
}

func Test_run_acb_json_counts_a_sale_of_every_share_of_a_split_pool_as_neither_oversold_nor_unknown_cost(t *testing.T) {
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-cad\"]\n", splitSoldOutRows(1_000_000_000, 7_000_000, 142_857_143))

	stdout, stderr := runACB(t, "--json")

	var doc shortDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
	require.Len(t, doc.Years, 1)
	assert.Equal(t, 0, doc.Years[0].UnknownCostSales)
	require.Len(t, doc.Years[0].Sales, 1)
	assert.False(t, doc.Years[0].Sales[0].UnknownCost)
	require.Len(t, doc.Securities, 1)
	assert.False(t, doc.Securities[0].Incomplete)
	assert.Empty(t, stderr)
}

func Test_run_acb_skips_a_return_of_capital_while_the_pool_is_short_as_not_held(t *testing.T) {
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-cad\"]\n\n"+
		"[[acb.adjustment]]\nsecurity = \"sec-fund\"\ndate = 2017-02-01\nreturn-of-capital = 5.00\n", shortOpenRows())

	stdout, stderr := runACB(t)

	assert.Equal(t, stderrWarnings(
		configShown+`: acb.adjustment item 1 is for "Money Fund", which no non-registered account holds on 2017-02-01; quarry skips it`,
		moneyFundShortWarning), stderr)
	assert.Contains(t, stdout, "1 sale of shares with unknown cost")
	assert.NotContains(t, stdout, "return of capital")
}
