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

// acbYearLine is one realized-gains table line, each cell as wide as the fixture's widest.
func acbYearLine(year, sales, proceeds, outlays, acb, gain string) string {
	return fmt.Sprintf("%-4s  %5s  %8s  %7s  %6s  %12s\n", year, sales, proceeds, outlays, acb, gain)
}

// acbPositionLine is one ACB position table line, each cell as wide as the fixture's widest.
func acbPositionLine(security, ticker, shares, acb, perShare string) string {
	return fmt.Sprintf("%-20s  %-6s  %6s  %6s  %13s\n", security, ticker, shares, acb, perShare)
}

// acbTrade is a buy or sell of millionths shares of security, in account on date, with amount in cents of currency.
// A sell's shares are stored negative.
func acbTrade(id string, sourceID int64, account, security, action, currency string, date time.Time, millionths, amount int64) store.InvestmentTransaction {
	return store.InvestmentTransaction{
		ID: id, SourceID: sourceID, AccountID: account, SecurityID: &security, Date: date,
		Action: action, Shares: &millionths, Amount: amount, Currency: currency,
	}
}

// acbRows is a CAD and a USD non-registered brokerage and a registered one, trading Acme (CAD), Vanguard (USD)
// and Maple (CAD, registered only) across 2024 to 2026.
func acbRows() store.Rows {
	rows := spendRows([]store.Account{
		{ID: "acct-cad", SourceID: 1, Name: "CAD Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
		{ID: "acct-usd", SourceID: 2, Name: "USD Brokerage", Type: store.AccountTypeBrokerage, Currency: "USD", Active: true},
		{ID: "acct-rrsp", SourceID: 3, Name: "RRSP", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
	})
	rows.Securities = []store.Security{
		{ID: "sec-acme", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")},
		{ID: "sec-vti", SourceID: 2, Name: "Vanguard Total Stock", Ticker: new("VTI"), Currency: new("USD")},
		{ID: "sec-maple", SourceID: 3, Name: "Maple Fund", Ticker: new("MPL"), Currency: new("CAD")},
	}
	buy, sell := store.ActionBuy, store.ActionSell
	acmeSale := acbTrade("inv-acme-sell", 4, "acct-cad", "sec-acme", sell, "CAD", day(2025, time.June, 2), -60_000_000, 90_000)
	acmeSale.Commission = new(int64(100_000))
	vtiSale := acbTrade("inv-vti-sell", 6, "acct-usd", "sec-vti", sell, "USD", day(2026, time.February, 2), -4_000_000, 50_000)
	vtiSale.Commission = new(int64(50_000))
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-acme-buy-cad", 1, "acct-cad", "sec-acme", buy, "CAD", day(2024, time.February, 1), 100_000_000, -100_000),
		acbTrade("inv-acme-buy-usd", 2, "acct-usd", "sec-acme", buy, "CAD", day(2024, time.March, 1), 50_000_000, -60_000),
		acbTrade("inv-vti-buy", 3, "acct-usd", "sec-vti", buy, "USD", day(2024, time.May, 1), 10_000_000, -100_000),
		acmeSale,
		acbTrade("inv-maple-buy", 5, "acct-rrsp", "sec-maple", buy, "CAD", day(2024, time.April, 1), 20_000_000, -20_000),
		vtiSale,
		acbTrade("inv-maple-sell", 7, "acct-rrsp", "sec-maple", sell, "CAD", day(2025, time.July, 1), -10_000_000, 12_000),
	}
	return rows
}

func Test_run_acb_prints_gains_per_tax_year_and_todays_acb_pooled_across_the_accounts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[accounts]\nnon-registered = [\"acct-cad\", \"acct-usd\"]\nregistered = [\"acct-rrsp\"]\n")
	replaceStoreWithRates(t, home, acbRows(),
		usdRate(day(2024, time.January, 2), 1_250_000), usdRate(day(2026, time.January, 2), 1_400_000))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"acb"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		acbYearLine("Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss")+
		acbYearLine("2025", "1", "910.00", "10.00", "640.00", "260.00")+
		acbYearLine("2026", "1", "707.00", "7.00", "500.00", "200.00")+
		"\nACB on 2026-03-12, in CAD\n\n"+
		acbPositionLine("Security", "Ticker", "Shares", "ACB", "ACB per share")+
		acbPositionLine("Acme Corp", "ACME", "90", "960.00", "10.6667")+
		acbPositionLine("Vanguard Total Stock", "VTI", "6", "750.00", "125.0000"),
		stdout.String())
}
