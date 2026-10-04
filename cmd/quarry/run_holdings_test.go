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

// holdingsLine is one holdings table line, each cell as wide as the fixture's widest.
func holdingsLine(account, security, shares, price, pricedOn, currency, value, in string) string {
	return fmt.Sprintf("%-17s  %-26s  %6s  %6s  %-10s  %-8s  %9s  %9s\n",
		account, security, shares, price, pricedOn, currency, value, in)
}

// A past clock date: the store's "through today" arm reads the real date, so the as-of day must not be after it.
func Test_run_holdings_lists_todays_holdings_in_the_reporting_currency(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	day := func(d int) time.Time { return time.Date(2026, time.March, d, 0, 0, 0, 0, time.UTC) }
	buy := func(id string, sourceID int64, account, security, currency string, shares int64) store.InvestmentTransaction {
		return store.InvestmentTransaction{
			ID: id, SourceID: sourceID, AccountID: account, SecurityID: &security, Date: day(2),
			Action: store.ActionBuy, Shares: &shares, Amount: -10_000, Currency: currency,
		}
	}
	rows := spendRows([]store.Account{
		brokerageAccount("acct-cad", 1, "CAD"),
		{ID: "acct-usd", SourceID: 2, Name: "IRA", Type: store.AccountTypeBrokerage, Currency: "USD", Active: true},
		closedAccount(store.Account{ID: "acct-old", SourceID: 3, Name: "Old RRSP", Type: store.AccountTypeBrokerage, Currency: "CAD"}),
	})
	rows.Securities = []store.Security{
		{ID: "sec-acme", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")},
		{ID: "sec-vti", SourceID: 2, Name: "Vanguard Total Stock", Ticker: new("VTI"), Currency: new("USD")},
		{ID: "sec-maple", SourceID: 3, Name: "Maple Fund", Ticker: new("Maple Fund"), Currency: new("CAD")},
	}
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		buy("inv-acme", 1, "acct-cad", "sec-acme", "CAD", 1_200_000_000),
		buy("inv-vti", 2, "acct-usd", "sec-vti", "USD", 85_000_000),
		buy("inv-maple", 3, "acct-old", "sec-maple", "CAD", 10_000_000),
	}
	rows.Prices = []store.Price{
		{SecurityID: "sec-acme", SourceID: 1, Date: day(9), Price: 31_420_000},
		{SecurityID: "sec-vti", SourceID: 2, Date: day(9), Price: 290_110_000},
		{SecurityID: "sec-maple", SourceID: 3, Date: day(5), Price: 5_000_000},
	}
	replaceStoreWithRates(t, home, rows, usdRate(day(10), 1_360_000))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings"},
		spendEnvAt(&stdout, &stderr, time.Date(2026, time.March, 12, 12, 0, 0, 0, time.UTC)))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Holdings on 2026-03-12 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35", "33,536.72")+
		holdingsLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00", "50.00")+
		holdingsLine("Total", "", "", "", "", "", "", "71,290.72"),
		stdout.String())
}
