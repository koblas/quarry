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

const acmeAddedNoCostWarning = `"Acme Corp" has shares added with no cost, so its ACB is too low and its gains too high; ` +
	"quarry findings --type shares-without-cost lists them"

// unknownCostYearLine is the year table's row for 2025, each cell as wide as this fixture's widest.
func unknownCostYearLine(year, sales, proceeds, outlays, acb, gain, suffix string) string {
	return fmt.Sprintf("%-4s  %5s  %8s  %7s  %6s  %12s  %s\n", year, sales, proceeds, outlays, acb, gain, suffix)
}

// unknownCostPositionLine is one position table line, each cell as wide as this fixture's widest.
func unknownCostPositionLine(security, ticker, shares, acb, perShare, suffix string) string {
	return strings.TrimRight(fmt.Sprintf("%-9s  %-6s  %6s  %6s  %13s  %s", security, ticker, shares, acb, perShare, suffix), " ") + "\n"
}

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
	rows := spendRows([]store.Account{
		{ID: "acct-cad", SourceID: 1, Name: "CAD Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
	})
	rows.Securities = []store.Security{{ID: "sec-acme", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")}}
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-acme", store.ActionBuy, "CAD", day(2024, time.February, 1), 10_000_000, -100_000),
		acbTrade("inv-add-free", 2, "acct-cad", "sec-acme", store.ActionAddShares, "CAD", day(2025, time.February, 3), 5_000_000, 0),
		acbTrade("inv-sell", 3, "acct-cad", "sec-acme", store.ActionSell, "CAD", day(2025, time.March, 3), -5_000_000, 90_000),
	}
	return rows
}

func Test_run_acb_marks_an_incomplete_security_and_its_sales_after_shares_added_with_no_cost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[accounts]\nnon-registered = [\"acct-cad\"]\n")
	replaceStore(t, home, unknownCostRows())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"acb"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		acbYearLine("Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss")+
		unknownCostYearLine("2025", "1", "900.00", "0.00", "333.33", "566.67", "1 sale of shares with unknown cost")+
		"\nACB on 2026-03-12, in CAD\n\n"+
		fmt.Sprintf("%-9s  %-6s  %6s  %6s  %13s\n", "Security", "Ticker", "Shares", "ACB", "ACB per share")+
		unknownCostPositionLine("Acme Corp", "ACME", "10", "666.67", "66.6670", "incomplete"),
		stdout.String())
	assert.Equal(t, "quarry: warning: "+acmeAddedNoCostWarning+"\n", stderr.String())
}

func Test_run_acb_writes_the_unknown_cost_marks_in_json(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[accounts]\nnon-registered = [\"acct-cad\"]\n")
	replaceStore(t, home, unknownCostRows())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"acb", "--json"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	var doc acbDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
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
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[accounts]\nnon-registered = [\"acct-cad\"]\n")
	replaceStore(t, home, noCostOrderRows())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"acb", "--json"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	var doc acbDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	assert.Equal(t, []string{noCostWarning("alpha"), noCostWarning("alpha"), noCostWarning("Beta")}, doc.Warnings)
	require.Len(t, doc.Securities, 3)
	assert.Equal(t, []string{"sec-a1", "sec-a2", "sec-b"}, []string{doc.Securities[0].ID, doc.Securities[1].ID, doc.Securities[2].ID})
}

func Test_run_acb_counts_only_the_sales_before_a_no_cost_holding_sold_out_and_leaves_a_rebought_one_complete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[accounts]\nnon-registered = [\"acct-cad\"]\n")
	replaceStore(t, home, soldOutAndRebought())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"acb"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		fmt.Sprintf("%-4s  %5s  %8s  %7s  %8s  %12s\n", "Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss")+
		fmt.Sprintf("%-4s  %5s  %8s  %7s  %8s  %12s  %s\n", "2025", "2", "1,800.00", "0.00", "1,250.00", "550.00", "1 sale of shares with unknown cost")+
		"\nACB on 2026-03-12, in CAD\n\n"+
		fmt.Sprintf("%-9s  %-6s  %6s  %6s  %13s\n", "Security", "Ticker", "Shares", "ACB", "ACB per share")+
		unknownCostPositionLine("Acme Corp", "ACME", "5", "250.00", "50.0000", ""),
		stdout.String())
	assert.Equal(t, stderrWarnings(acmeAddedNoCostWarning), stderr.String())
}
