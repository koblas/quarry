package main

import (
	"bytes"
	"context"
	"fmt"
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
	return fmt.Sprintf("%-9s  %-6s  %6s  %6s  %13s  %s\n", security, ticker, shares, acb, perShare, suffix)
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
