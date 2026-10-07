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

const acbAdjustmentsConfig = `[accounts]
non-registered = ["acct-cad"]

[[acb.adjustment]]
security = "sec-acme"
date = 2024-06-30
reinvested-distribution = 200.00

[[acb.adjustment]]
security = "sec-acme"
date = 2024-12-31
return-of-capital = 150.00

[[acb.adjustment]]
security = "sec-acme"
date = 2025-12-31
return-of-capital = 2300.00
`

const acmeReturnOfCapitalWarning = `"Acme Corp": return of capital on 2025-12-31 is 1,250.00 more than its ACB, ` +
	"so its ACB is 0.00 and 1,250.00 is a capital gain in 2025"

// acbAdjustmentsRows is one non-registered CAD brokerage holding 10 shares of Acme Corp bought for 1,000.00 and never sold.
func acbAdjustmentsRows() store.Rows {
	rows := acbBrokerageRows()
	rows.Securities = []store.Security{{ID: "sec-acme", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")}}
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-acme", store.ActionBuy, "CAD", day(2024, time.February, 1), 10_000_000, -100_000),
	}
	return rows
}

func Test_run_acb_lowers_and_raises_the_acb_by_the_adjustments_and_counts_return_of_capital_above_it_as_a_gain(t *testing.T) {
	acbFixture(t, acbAdjustmentsConfig, acbAdjustmentsRows())

	stdout, stderr := mustRunACB(t)

	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		"Year  Sales  Proceeds  Outlays   ACB  Gain or loss\n"+
		acbYearRow(4, "2025", "0", "0.00", "0.00", "0.00", "0.00",
			"1,250.00 return of capital above ACB, a capital gain")+
		"\nACB on 2026-03-12, in CAD\n\n"+
		acbPositionRow(9, 4, "Security", "Ticker", "Shares", "ACB", "ACB per share", "")+
		acbPositionRow(9, 4, "Acme Corp", "ACME", "10", "0.00", "0.0000", ""),
		stdout)
	assert.Equal(t, "quarry: warning: "+acmeReturnOfCapitalWarning+"\n", stderr)
}

const acbAdjustmentWarningsConfig = `colour = "red"

[accounts]
non-registered = ["acct-cad"]

[[acb.adjustment]]
security = "sec-99"
date = 2024-06-30
return-of-capital = 5.00

[[acb.adjustment]]
security = "sec-acme"
date = 2025-12-31
return-of-capital = 2000.00

[[acb.adjustment]]
security = "sec-acme"
date = 2023-01-01
return-of-capital = 10.00
`

func acbAdjustmentLines(path string) []string {
	return []string{
		path + `: acb.adjustment item 1 names "sec-99", which is not a security in quarry's store; quarry skips it`,
		path + `: acb.adjustment item 3 is for "Acme Corp", which no non-registered account holds on 2023-01-01; quarry skips it`,
	}
}

const acmeLargeReturnOfCapitalWarning = `"Acme Corp": return of capital on 2025-12-31 is 720.00 more than its ACB, ` +
	"so its ACB is 0.00 and 720.00 is a capital gain in 2025"

func Test_run_acb_lists_config_then_adjustment_then_no_cost_then_removal_then_return_of_capital_warnings_in_both_forms(t *testing.T) {
	home := newHome(t)
	writeConfig(t, home, acbAdjustmentWarningsConfig)
	replaceStore(t, home, acbSharesRows())
	var textOut, textErr bytes.Buffer

	require.Equal(t, 0, runWith(context.Background(), []string{"acb"}, spendEnvAt(&textOut, &textErr, holdingsClock())), textErr.String())
	warnings, machineErr := jsonWarnings(t, "acb", "--json")

	wantStderr := stderrWarnings(append(append([]string{configShown + orderUnknownKey}, acbAdjustmentLines(configShown)...),
		acmeAddedNoCostWarning, acmeRemovalWarning, acmeLargeReturnOfCapitalWarning)...)
	assert.Equal(t, append(append([]string{configPath(home) + orderUnknownKey}, acbAdjustmentLines(configPath(home))...),
		acmeAddedNoCostWarning, acmeRemovalWarning, acmeLargeReturnOfCapitalWarning), warnings)
	assert.Equal(t, wantStderr, textErr.String())
	assert.Equal(t, wantStderr, machineErr)
}

func Test_run_acb_names_two_adjustments_for_one_security_on_one_day_with_the_first_item(t *testing.T) {
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-cad\"]\n\n"+
		"[[acb.adjustment]]\nsecurity = \"sec-acme\"\ndate = 2024-06-30\nreinvested-distribution = 10.00\n\n"+
		"[[acb.adjustment]]\nsecurity = \"sec-acme\"\ndate = 2024-06-30\nreturn-of-capital = 5.00\n", acbAdjustmentsRows())

	_, stderr := mustRunACB(t)

	assert.Equal(t, stderrWarnings(configShown+`: acb.adjustment items 1 and 2 are both for "sec-acme" on 2024-06-30; quarry applies both`),
		stderr)
}

func Test_run_acb_writes_return_of_capital_gain_in_json(t *testing.T) {
	home := newHome(t)
	writeConfig(t, home, acbAdjustmentsConfig)
	rows := acbAdjustmentsRows()
	rows.InvestmentTransactions = append(rows.InvestmentTransactions,
		acbTrade("inv-sell", 2, "acct-cad", "sec-acme", store.ActionSell, "CAD", day(2024, time.September, 1), -2_000_000, 30_000))
	replaceStore(t, home, rows)

	stdout, _ := mustRunACB(t, "--json")

	var doc acbDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
	require.Len(t, doc.Years, 2)
	assert.Equal(t, 2024, doc.Years[0].Year)
	assert.Equal(t, 1, doc.Years[0].SaleCount)
	assert.Equal(t, "60.00", doc.Years[0].Gain)
	assert.Equal(t, "0.00", doc.Years[0].ReturnOfCapitalGain)
	assert.Equal(t, 2025, doc.Years[1].Year)
	assert.Equal(t, 0, doc.Years[1].SaleCount)
	assert.Equal(t, "0.00", doc.Years[1].Gain)
	assert.Equal(t, "1490.00", doc.Years[1].ReturnOfCapitalGain)
	require.Len(t, doc.Securities, 1)
	excess := doc.Securities[0].Events[len(doc.Securities[0].Events)-1]
	assert.Equal(t, "return of capital", excess.Action)
	assert.Equal(t, new("2300.00"), excess.CAD)
	assert.Nil(t, excess.Outlays)
	require.NotNil(t, excess.Gain)
	assert.Equal(t, "1490.00", *excess.Gain)
}

// acmeNoCostWarnings is what the fixture prints: the no-cost add's line, then the removal's.
var acmeNoCostWarnings = []string{acmeAddedNoCostWarning, acmeRemovalWarning}

const acmeRemovalWarning = `"Acme Corp": 4 shares left "CAD Brokerage" on 2025-03-03 without a sale; ` +
	"quarry took their share of the ACB out and reports no gain; if they went to a registered account or to someone else, " +
	"that is a disposition at market value; check it with your accountant"

// acbSharesRows is one non-registered CAD brokerage whose Acme Corp holding gains 10 bought, 5 added with a
// cost and 5 added without one, then loses 4 removed with no sale.
func acbSharesRows() store.Rows {
	rows := acbBrokerageRows()
	rows.Securities = []store.Security{{ID: "sec-acme", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")}}
	withCost := acbTrade("inv-add-cost", 2, "acct-cad", "sec-acme", store.ActionAddShares, "CAD", day(2025, time.January, 2), 5_000_000, 0)
	withCost.CostBasis = new(int64(60_000))
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-acme", store.ActionBuy, "CAD", day(2024, time.February, 1), 10_000_000, -100_000),
		withCost,
		acbTrade("inv-add-free", 3, "acct-cad", "sec-acme", store.ActionAddShares, "CAD", day(2025, time.February, 3), 5_000_000, 0),
		acbTrade("inv-remove", 4, "acct-cad", "sec-acme", store.ActionRemoveShares, "CAD", day(2025, time.March, 3), -4_000_000, 0),
	}
	return rows
}

func Test_run_acb_takes_removed_shares_out_of_the_acb_and_adds_added_shares_at_their_cost(t *testing.T) {
	acbFixture(t, acbNonRegistered, acbSharesRows())

	stdout, stderr := mustRunACB(t)

	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		"Year  Sales  Proceeds  Outlays  ACB  Gain or loss\n"+
		"\nACB on 2026-03-12, in CAD\n\n"+
		acbPositionRow(9, 8, "Security", "Ticker", "Shares", "ACB", "ACB per share", "")+
		acbPositionRow(9, 8, "Acme Corp", "ACME", "16", "1,280.00", "80.0000", "incomplete"),
		stdout)
	assert.Equal(t, stderrWarnings(acmeNoCostWarnings...), stderr)
}

func Test_run_acb_lists_a_removal_warning_in_json(t *testing.T) {
	acbFixture(t, acbNonRegistered, acbSharesRows())

	stdout, stderr := mustRunACB(t, "--json")

	var doc acbDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
	assert.Equal(t, acmeNoCostWarnings, doc.Warnings)
	assert.Equal(t, stderrWarnings(acmeNoCostWarnings...), stderr)
}

func Test_run_acb_leaves_a_zero_unit_removal_out_of_the_warnings(t *testing.T) {
	home := newHome(t)
	writeConfig(t, home, acbNonRegistered)
	rows := acbSharesRows()
	rows.InvestmentTransactions = append(rows.InvestmentTransactions,
		acbTrade("inv-remove-zero", 5, "acct-cad", "sec-acme", store.ActionRemoveShares, "CAD", day(2025, time.March, 10), 0, 0))
	replaceStore(t, home, rows)

	warnings, stderr := jsonWarnings(t, "acb", "--json")

	assert.Equal(t, acmeNoCostWarnings, warnings)
	assert.Equal(t, stderrWarnings(acmeNoCostWarnings...), stderr)
}

func Test_run_acb_lists_the_configs_warnings_before_a_removal_warning_in_both_forms(t *testing.T) {
	home := newHome(t)
	writeConfig(t, home, "colour = \"red\"\n[accounts]\nnon-registered = [\"acct-cad\"]\n")
	replaceStore(t, home, acbSharesRows())
	var textOut, textErr bytes.Buffer

	require.Equal(t, 0, runWith(context.Background(), []string{"acb"}, spendEnvAt(&textOut, &textErr, holdingsClock())), textErr.String())
	warnings, machineErr := jsonWarnings(t, "acb", "--json")

	wantStderr := stderrWarnings(configShown+orderUnknownKey, acmeAddedNoCostWarning, acmeRemovalWarning)
	assert.Equal(t, []string{configPath(home) + orderUnknownKey, acmeAddedNoCostWarning, acmeRemovalWarning}, warnings)
	assert.Equal(t, wantStderr, textErr.String())
	assert.Equal(t, wantStderr, machineErr)
}

const acmeAddedNoCostWarning = `"Acme Corp" has shares added with no cost, so its ACB is too low and its gains too high; ` +
	"quarry findings --type shares-without-cost lists them"

func noCostWarning(name string) string {
	return fmt.Sprintf(`"%s" has shares added with no cost, so its ACB is too low and its gains too high; `+
		"quarry findings --type shares-without-cost lists them", name)
}

// soldOutAndRebought is unknownCostRows' holding sold out for 1,500.00, then re-bought with a cost and part sold.
func soldOutAndRebought() store.Rows {
	rows := unknownCostRows()
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-acme", store.ActionBuy, "CAD", day(2024, time.February, 1), 10_000_000, -100_000),
		acbTrade("inv-add-free", 2, "acct-cad", "sec-acme", store.ActionAddShares, "CAD", day(2024, time.March, 1), 5_000_000, 0),
		acbTrade("inv-sell-out", 3, "acct-cad", "sec-acme", store.ActionSell, "CAD", day(2025, time.February, 1), -15_000_000, 150_000),
		acbTrade("inv-rebuy", 4, "acct-cad", "sec-acme", store.ActionBuy, "CAD", day(2025, time.June, 1), 10_000_000, -50_000),
		acbTrade("inv-sell-later", 5, "acct-cad", "sec-acme", store.ActionSell, "CAD", day(2025, time.July, 1), -5_000_000, 30_000),
	}
	return rows
}

// unknownCostRows is one non-registered CAD brokerage whose Acme Corp holding buys 10 for 1,000.00, gains 5 with
// no cost, then sells 5 for 900.00.
func unknownCostRows() store.Rows {
	rows := acbBrokerageRows()
	rows.Securities = []store.Security{{ID: "sec-acme", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")}}
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-acme", store.ActionBuy, "CAD", day(2024, time.February, 1), 10_000_000, -100_000),
		acbTrade("inv-add-free", 2, "acct-cad", "sec-acme", store.ActionAddShares, "CAD", day(2025, time.February, 3), 5_000_000, 0),
		acbTrade("inv-sell", 3, "acct-cad", "sec-acme", store.ActionSell, "CAD", day(2025, time.March, 3), -5_000_000, 90_000),
	}
	return rows
}

func Test_run_acb_marks_an_incomplete_security_and_its_sales_after_shares_added_with_no_cost(t *testing.T) {
	acbFixture(t, acbNonRegistered, unknownCostRows())

	stdout, stderr := mustRunACB(t)

	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		acbYearLine("Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss")+
		acbYearRow(6, "2025", "1", "900.00", "0.00", "333.33", "566.67", "1 sale of shares with unknown cost")+
		"\nACB on 2026-03-12, in CAD\n\n"+
		acbPositionRow(9, 6, "Security", "Ticker", "Shares", "ACB", "ACB per share", "")+
		acbPositionRow(9, 6, "Acme Corp", "ACME", "10", "666.67", "66.6670", "incomplete"),
		stdout)
	assert.Equal(t, "quarry: warning: "+acmeAddedNoCostWarning+"\n", stderr)
}

func Test_run_acb_writes_the_unknown_cost_marks_in_json(t *testing.T) {
	acbFixture(t, acbNonRegistered, unknownCostRows())

	stdout, _ := mustRunACB(t, "--json")

	var doc acbDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
	require.Len(t, doc.Years, 1)
	assert.Equal(t, 1, doc.Years[0].UnknownCostSales)
	require.Len(t, doc.Years[0].Sales, 1)
	assert.True(t, doc.Years[0].Sales[0].UnknownCost)
	require.Len(t, doc.Securities, 1)
	assert.True(t, doc.Securities[0].Incomplete)
	assert.Equal(t, []string{acmeAddedNoCostWarning}, doc.Warnings)
}

// noCostOrderRows holds three securities, Beta first and the later alpha with the lower id, each bought with a
// cost and then given 5 shares with none.
func noCostOrderRows() store.Rows {
	rows := unknownCostRows()
	rows.Securities = []store.Security{
		{ID: "sec-b", SourceID: 1, Name: "Beta", Ticker: new("BET"), Currency: new("CAD")},
		{ID: "sec-a2", SourceID: 2, Name: "alpha", Ticker: new("AL2"), Currency: new("CAD")},
		{ID: "sec-a1", SourceID: 3, Name: "alpha", Ticker: new("AL1"), Currency: new("CAD")},
	}
	rows.InvestmentTransactions = nil
	for i, id := range []string{"sec-b", "sec-a2", "sec-a1"} {
		source := int64(2*i + 1)
		rows.InvestmentTransactions = append(rows.InvestmentTransactions,
			acbTrade("inv-buy-"+id, source, "acct-cad", id, store.ActionBuy, "CAD", day(2024, time.February, 1), 10_000_000, -100_000),
			acbTrade("inv-add-"+id, source+1, "acct-cad", id, store.ActionAddShares, "CAD", day(2025, time.February, 3), 5_000_000, 0))
	}
	return rows
}

func Test_run_acb_warns_for_no_cost_securities_by_name_ignoring_case_then_id(t *testing.T) {
	acbFixture(t, acbNonRegistered, noCostOrderRows())

	stdout, _ := mustRunACB(t, "--json")

	var doc acbDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
	assert.Equal(t, []string{noCostWarning("alpha"), noCostWarning("alpha"), noCostWarning("Beta")}, doc.Warnings)
	require.Len(t, doc.Securities, 3)
	assert.Equal(t, []string{"sec-a1", "sec-a2", "sec-b"}, []string{doc.Securities[0].ID, doc.Securities[1].ID, doc.Securities[2].ID})
}

func Test_run_acb_counts_only_the_sales_before_a_no_cost_holding_sold_out_and_leaves_a_rebought_one_complete(t *testing.T) {
	acbFixture(t, acbNonRegistered, soldOutAndRebought())

	stdout, stderr := mustRunACB(t)

	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		acbYearRow(8, "Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss", "")+
		acbYearRow(8, "2025", "2", "1,800.00", "0.00", "1,250.00", "550.00", "1 sale of shares with unknown cost")+
		"\nACB on 2026-03-12, in CAD\n\n"+
		acbPositionRow(9, 6, "Security", "Ticker", "Shares", "ACB", "ACB per share", "")+
		acbPositionRow(9, 6, "Acme Corp", "ACME", "5", "250.00", "50.0000", ""),
		stdout)
	assert.Equal(t, stderrWarnings(acmeAddedNoCostWarning), stderr)
}

const moneyFundOversoldWarning = `"Money Fund": the sale on 2017-01-12 in "CAD Brokerage" sold 10 more shares than the ` +
	"non-registered accounts held; quarry counts them at no cost, so the sale's gain is too high by what they cost, " +
	"and the next 10 shares acquired only bring the holding back to 0; correct the shares in Quicken if they are wrong"

// oversoldRows is one non-registered brokerage whose Money Fund holding buys 100 for 100.00, sells 110 for
// 110.00, then buys 15 for 15.00.
func oversoldRows() store.Rows {
	rows := acbBrokerageRows()
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
	acbFixture(t, acbNonRegistered, oversoldRows())

	stdout, stderr := mustRunACB(t, "--json")

	var doc shortDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
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
	assert.Equal(t, stderrWarnings(moneyFundOversoldWarning), stderr)
}

func Test_run_acb_shows_a_buy_that_covers_a_short_at_the_pro_rata_cost_of_the_units_beyond_it(t *testing.T) {
	acbFixture(t, acbNonRegistered, oversoldRows())
	w := [10]int{10, 13, 6, 6, 11, 4, 7, 11, 6, 12}

	positions, _ := mustRunACB(t)
	history, stderr := mustRunACB(t, "--security", "MNY")

	assert.Contains(t, positions, acbPositionRow(10, 4, "Money Fund", "MNY", "5", "5.00", "1.0000", ""))
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
	acbFixture(t, acbNonRegistered, shortOpenRows())

	stdout, stderr := mustRunACB(t)

	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		acbYearLine("Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss")+
		acbYearRow(6, "2017", "1", "1,222.84", "0.00", "100.00", "1,122.84", "1 sale of shares with unknown cost")+
		"\nACB on 2026-03-12, in CAD\n\n"+
		"Security  Ticker  Shares  ACB  ACB per share\n", //nolint:dupword // the ACB column sits beside the ACB per share column
		stdout)
	assert.Equal(t, stderrWarnings(moneyFundShortWarning), stderr)
}

func Test_run_acb_json_writes_a_short_position_with_negative_shares_and_no_acb_per_share(t *testing.T) {
	acbFixture(t, acbNonRegistered, shortOpenRows())

	stdout, stderr := mustRunACB(t, "--json")

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
	acbFixture(t, acbNonRegistered, shortOpenRows())
	w := [7]int{10, 8, 8, 8, 7, 6, 12}

	stdout, stderr := mustRunACB(t, "--year", "2017")

	assert.Equal(t, "Sales in 2017, in CAD\n\n"+
		acbSaleRowOf(w, [8]string{"Date", "Security", "Shares", "Proceeds", "Outlays", "ACB", "Gain or loss"})+
		acbSaleRowOf(w, [8]string{"2017-01-12", "MNY", "1,222.84", "1,222.84", "0.00", "100.00", "1,122.84", "unknown cost"})+
		acbSaleRowOf(w, [8]string{"Total", "", "", "1,222.84", "0.00", "100.00", "1,122.84"}),
		stdout)
	assert.Equal(t, stderrWarnings(moneyFundShortWarning), stderr)
}

func Test_run_acb_year_without_a_sale_still_warns_of_an_oversold_sale_in_another_year(t *testing.T) {
	acbFixture(t, acbNonRegistered, shortOpenRows())

	_, stderr := mustRunACB(t, "--year", "2018")

	assert.True(t, strings.HasSuffix(stderr, stderrWarnings(moneyFundShortWarning)), stderr)
}

func Test_run_acb_security_prints_negative_shares_held_grouped(t *testing.T) {
	acbFixture(t, acbNonRegistered, shortOpenRows())
	w := [10]int{10, 13, 6, 8, 12, 4, 8, 11, 6, 12}

	stdout, stderr := mustRunACB(t, "--security", "MNY")

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
	acbFixture(t, acbNonRegistered, rows)

	_, stderr := mustRunACB(t)

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

	stdout, stderr := mustRunACB(t)

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
	acbFixture(t, acbNonRegistered, splitSoldOutRows(1_000_000_000, 7_000_000, 142_857_143))

	stdout, stderr := mustRunACB(t, "--json")

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

	stdout, stderr := mustRunACB(t)

	assert.Equal(t, stderrWarnings(
		configShown+`: acb.adjustment item 1 is for "Money Fund", which no non-registered account holds on 2017-02-01; quarry skips it`,
		moneyFundShortWarning), stderr)
	assert.Contains(t, stdout, "1 sale of shares with unknown cost")
	assert.NotContains(t, stdout, "return of capital")
}

const (
	noRateWarningVTI = `"Vanguard Total Stock" has a USD trade on 2023-12-01, before 2024-01-02, the first exchange rate in the store, ` +
		"so its ACB is incomplete and its gains are left out of the year totals"
	sharedTickerWarningVTI = `"VTI" is 2 securities in Quicken ("Vanguard Total Stock", "Vanguard Total Stock CAD"); ` +
		"quarry keeps a separate ACB for each; if they are the same, merge them in Quicken"
	returnOfCapitalWarningVTI = `"Vanguard Total Stock": return of capital on 2025-12-01 is 1,000.00 more than its ACB, so its ACB is 0.00 ` +
		"and 1,000.00 is a capital gain in 2025"
	decemberSaleWarning2025 = "1 sale dated December 24–31, 2025: a sale settles a day or two after its trade date and counts " +
		"for tax in the year it settles; check its date on your T5008"
)

// noRateSharedTickerRows is a USD and a CAD brokerage, each holding one of two securities that share ticker VTI;
// the USD one is bought before the first rate, and both are part-sold in December 2025.
func noRateSharedTickerRows() store.Rows {
	rows := spendRows([]store.Account{
		{ID: "acct-usd", SourceID: 1, Name: "USD Brokerage", Type: store.AccountTypeBrokerage, Currency: "USD", Active: true},
		{ID: "acct-cad", SourceID: 2, Name: "CAD Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
	})
	rows.Securities = []store.Security{
		{ID: "sec-vti", SourceID: 1, Name: "Vanguard Total Stock", Ticker: new("VTI"), Currency: new("USD")},
		{ID: "sec-vti-cad", SourceID: 2, Name: "Vanguard Total Stock CAD", Ticker: new("VTI"), Currency: new("CAD")},
	}
	buy, sell := store.ActionBuy, store.ActionSell
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-vti-buy", 1, "acct-usd", "sec-vti", buy, "USD", day(2023, time.December, 1), 10_000_000, -100_000),
		acbTrade("inv-vti-cad-buy", 2, "acct-cad", "sec-vti-cad", buy, "CAD", day(2025, time.March, 3), 10_000_000, -100_000),
		acbTrade("inv-vti-cad-sell", 3, "acct-cad", "sec-vti-cad", sell, "CAD", day(2025, time.December, 28), -4_000_000, 60_000),
		acbTrade("inv-vti-sell", 4, "acct-usd", "sec-vti", sell, "USD", day(2025, time.December, 29), -4_000_000, 50_000),
	}
	return rows
}

func Test_run_acb_warns_of_a_no_rate_trade_a_shared_ticker_and_a_december_sale(t *testing.T) {
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-usd\", \"acct-cad\"]\n", noRateSharedTickerRows(), usdRate(day(2024, time.January, 2), 1_250_000))

	stdout, stderr := mustRunACB(t)

	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		acbYearLine("Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss")+
		acbYearLine("2025", "1", "600.00", "0.00", "400.00", "200.00")+
		"\nACB on 2026-03-12, in CAD\n\n"+
		acbPositionRow(24, 6, "Security", "Ticker", "Shares", "ACB", "ACB per share", "")+
		acbPositionRow(24, 6, "Vanguard Total Stock", "VTI", "6", "0.00", "0.0000", "incomplete")+
		acbPositionRow(24, 6, "Vanguard Total Stock CAD", "VTI", "6", "600.00", "100.0000", ""),
		stdout)
	assert.Equal(t, "quarry: warning: "+noRateWarningVTI+"\n"+
		"quarry: warning: "+sharedTickerWarningVTI+"\n"+
		"quarry: warning: "+decemberSaleWarning2025+"\n",
		stderr)
}

const noRateReturnOfCapitalConfig = `[accounts]
non-registered = ["acct-usd", "acct-cad"]

[[acb.adjustment]]
security = "sec-vti"
date = 2025-12-01
return-of-capital = 1000.00
`

func Test_run_acb_json_leaves_a_no_rate_security_out_of_the_years_and_warns(t *testing.T) {
	acbFixture(t, noRateReturnOfCapitalConfig, noRateSharedTickerRows(), usdRate(day(2024, time.January, 2), 1_250_000))

	stdout, _ := mustRunACB(t, "--json")

	var doc acbDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
	require.Len(t, doc.Years, 1)
	assert.Equal(t, 1, doc.Years[0].SaleCount)
	assert.Equal(t, "600.00", doc.Years[0].Proceeds)
	assert.Equal(t, "0.00", doc.Years[0].ReturnOfCapitalGain)
	noRate := doc.Securities[0]
	assert.True(t, noRate.Incomplete)
	assert.Nil(t, noRate.Events[0].CAD)
	assert.Nil(t, noRate.Events[0].Gain)
	assert.Equal(t, new("1000.00"), noRate.Events[1].Gain)
	assert.Equal(t, []string{noRateWarningVTI, sharedTickerWarningVTI, returnOfCapitalWarningVTI, decemberSaleWarning2025}, doc.Warnings)
}

const acbSuperficialConfig = "[accounts]\nnon-registered = [\"acct-cad\"]\nregistered = [\"acct-rrsp\"]\n"

const superficialLossWarning = "1 possible superficial loss in 2025: the same security was acquired within 30 days before or after the sale, " +
	"in any account, and still held 30 days after; quarry does not deny or adjust these losses; review them with your accountant"

// acbSuperficialRows is 20 Acme shares bought for 2,000.00 in a non-registered account on 2025-01-10, 10 sold there
// for 600.00 on 2025-03-01, and 5 bought for 300.00 in a registered account on 2025-03-15. Every date is long past holdingsClock.
func acbSuperficialRows() store.Rows {
	rows := spendRows([]store.Account{
		{ID: "acct-cad", SourceID: 1, Name: "CAD Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
		{ID: "acct-rrsp", SourceID: 2, Name: "RRSP", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
	})
	rows.Securities = []store.Security{{ID: "sec-acme", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")}}
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-acme", store.ActionBuy, "CAD", day(2025, time.January, 10), 20_000_000, -200_000),
		acbTrade("inv-sell", 2, "acct-cad", "sec-acme", store.ActionSell, "CAD", day(2025, time.March, 1), -10_000_000, 60_000),
		acbTrade("inv-rebuy", 3, "acct-rrsp", "sec-acme", store.ActionBuy, "CAD", day(2025, time.March, 15), 5_000_000, -30_000),
	}
	return rows
}

func Test_run_acb_marks_a_loss_sale_rebought_in_a_registered_account_within_30_days(t *testing.T) {
	acbFixture(t, acbSuperficialConfig, acbSuperficialRows())
	var stdout, stderr, jsonOut, jsonErr bytes.Buffer

	textExit := runWith(context.Background(), []string{"acb"}, spendEnvAt(&stdout, &stderr, holdingsClock()))
	jsonExit := runWith(context.Background(), []string{"acb", "--json"}, spendEnvAt(&jsonOut, &jsonErr, holdingsClock()))

	require.Equal(t, 0, textExit, stderr.String())
	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		acbYearRow(8, "Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss", "")+
		acbYearRow(8, "2025", "1", "600.00", "0.00", "1,000.00", "-400.00", "1 possible superficial loss")+
		"\nACB on 2026-03-12, in CAD\n\n"+
		acbPositionRow(9, 8, "Security", "Ticker", "Shares", "ACB", "ACB per share", "")+
		acbPositionRow(9, 8, "Acme Corp", "ACME", "10", "1,000.00", "100.0000", ""),
		stdout.String())
	assert.Equal(t, stderrWarnings(superficialLossWarning), stderr.String())
	require.Equal(t, 0, jsonExit, jsonErr.String())
	var doc struct {
		Years []struct {
			Year                      int    `json:"year"`
			Gain                      string `json:"gain"`
			PossibleSuperficialLosses int    `json:"possible_superficial_losses"`
			Sales                     []struct {
				Gain                    string `json:"gain"`
				PossibleSuperficialLoss bool   `json:"possible_superficial_loss"`
			} `json:"sales"`
		} `json:"years"`
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &doc), jsonOut.String())
	require.Len(t, doc.Years, 1)
	assert.Equal(t, 1, doc.Years[0].PossibleSuperficialLosses)
	assert.Equal(t, "-400.00", doc.Years[0].Gain)
	require.Len(t, doc.Years[0].Sales, 1)
	assert.True(t, doc.Years[0].Sales[0].PossibleSuperficialLoss)
	assert.Equal(t, "-400.00", doc.Years[0].Sales[0].Gain)
	assert.Equal(t, []string{superficialLossWarning}, doc.Warnings)
	assert.Equal(t, stderr.String(), jsonErr.String())
}

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
			acbFixture(t, acbNonRegistered, shortCoveredRows(c.cover))

			stdout, _ := mustRunACB(t, "--json")

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
	acbFixture(t, acbNonRegistered, shortSplitRows())

	stdout, _ := mustRunACB(t, "--json")

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
	acbFixture(t, acbNonRegistered, shortSplitRows())
	w := [10]int{10, 13, 6, 6, 11, 4, 7, 11, 6, 12}

	stdout, _ := mustRunACB(t, "--security", "MNY")

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
	acbFixture(t, acbNonRegistered, rows)

	stdout, stderr := mustRunACB(t, "--json")

	doc := decodeACBYear(t, stdout)
	assert.Equal(t, []string{moneyFundRemovalWarning, moneyFundRemovalOversoldWarning}, doc.Warnings)
	assert.Equal(t, stderrWarnings(moneyFundRemovalWarning, moneyFundRemovalOversoldWarning), stderr)
}

func Test_run_acb_security_marks_a_sale_that_is_both_a_possible_superficial_loss_and_of_unknown_cost(t *testing.T) {
	acbFixture(t, acbRegisteredConfig, acbStackedRows())
	w := [10]int{10, 13, 10, 6, 13, 4, 9, 11, 8, 12}

	stdout, stderr := mustRunACB(t, "--security", "ACME")

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
		want      string
	}{
		{
			name: "a possible superficial loss", store: seed(acbSuperficialRows), config: acbSuperficialConfig,
			cliArgs: []string{"acb"}, arguments: map[string]any{}, want: superficialLossWarning,
		},
		{
			name: "a possible superficial loss, year given", store: seed(acbSuperficialRows), config: acbSuperficialConfig,
			cliArgs: []string{"acb", "--year", "2025"}, arguments: map[string]any{"year": 2025}, want: superficialLossWarning,
		},
		{
			name: "a removal with no sale", store: seed(acbSharesRows), config: fund,
			cliArgs: []string{"acb"}, arguments: map[string]any{}, want: acmeRemovalWarning,
		},
		{
			name: "a December sale", store: seed(noRateSharedTickerRows), config: pair,
			cliArgs: []string{"acb"}, arguments: map[string]any{}, want: decemberSaleWarning2025,
		},
		{
			name: "a removal beyond the pool", store: seed(oversoldRemoval), config: fund,
			cliArgs: []string{"acb"}, arguments: map[string]any{}, want: moneyFundRemovalOversoldWarning,
		},
		{
			name: "a short, year given", store: seed(shortOpenRows), config: fund,
			cliArgs: []string{"acb", "--year", "2017"}, arguments: map[string]any{"year": 2017}, want: moneyFundShortWarning,
		},
		{
			name: "a short, security given", store: seed(shortOpenRows), config: fund,
			cliArgs: []string{"acb", "--security", "MNY"}, arguments: map[string]any{"security": []string{"MNY"}}, want: moneyFundShortWarning,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{store: c.store, config: c.config, cliArgs: c.cliArgs, tool: "acb", arguments: c.arguments})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, acbInToolWords(got.cliWarnings), got.toolWarnings)
			assert.Contains(t, got.toolWarnings, c.want)
		})
	}
}
