package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	noRateWarningVTI = `"Vanguard Total Stock" has a USD trade on 2023-12-01, before 2024-01-02, the first exchange rate in the store, ` +
		"so its ACB is incomplete and its gains are left out of the year totals"
	sharedTickerWarningVTI = `"VTI" is 2 securities in Quicken (Vanguard Total Stock, Vanguard Total Stock CAD); ` +
		"quarry keeps a separate ACB for each; if they are the same, merge them in Quicken"
	decemberSaleWarning2025 = "1 sale dated December 24–31, 2025: a sale settles a day or two after its trade date and counts " +
		"for tax in the year it settles; check its date on your T5008"
)

// warningsPositionLine is one position table line, each cell as wide as this fixture's widest.
func warningsPositionLine(security, ticker, shares, acb, perShare, suffix string) string {
	return strings.TrimRight(fmt.Sprintf("%-24s  %-6s  %6s  %6s  %13s  %s", security, ticker, shares, acb, perShare, suffix), " ") + "\n"
}

// noRateSharedTickerRows is a non-registered USD and CAD brokerage holding two securities that share ticker VTI.
// The USD one is bought on 2023-12-01, before the first rate, and part-sold on December 29; the CAD one is bought
// and part-sold on December 28.
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
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[accounts]\nnon-registered = [\"acct-usd\", \"acct-cad\"]\n")
	replaceStoreWithRates(t, home, noRateSharedTickerRows(), usdRate(day(2024, time.January, 2), 1_250_000))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"acb"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		acbYearLine("Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss")+
		acbYearLine("2025", "1", "600.00", "0.00", "400.00", "200.00")+
		"\nACB on 2026-03-12, in CAD\n\n"+
		fmt.Sprintf("%-24s  %-6s  %6s  %6s  %13s\n", "Security", "Ticker", "Shares", "ACB", "ACB per share")+
		warningsPositionLine("Vanguard Total Stock", "VTI", "6", "0.00", "0.0000", "incomplete")+
		warningsPositionLine("Vanguard Total Stock CAD", "VTI", "6", "600.00", "100.0000", ""),
		stdout.String())
	assert.Equal(t, "quarry: warning: "+noRateWarningVTI+"\n"+
		"quarry: warning: "+sharedTickerWarningVTI+"\n"+
		"quarry: warning: "+decemberSaleWarning2025+"\n",
		stderr.String())
}
