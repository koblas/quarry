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

// holdingsNativeLine is holdingsLine without the In column.
func holdingsNativeLine(account, security, shares, price, pricedOn, currency, value string) string {
	return fmt.Sprintf("%-17s  %-26s  %6s  %6s  %-10s  %-8s  %9s\n",
		account, security, shares, price, pricedOn, currency, value)
}

// holdingsClock is a past date: the store's "through today" arm reads the real date, so the as-of day must not be after it.
func holdingsClock() time.Time { return time.Date(2026, time.March, 12, 12, 0, 0, 0, time.UTC) }

// holdingsDay is day d of March 2026, the month every holdings fixture dates its rows in.
func holdingsDay(d int) time.Time { return time.Date(2026, time.March, d, 0, 0, 0, 0, time.UTC) }

// holdingsBuy is a CAD or USD buy of shares millionths of security in account on day 2.
func holdingsBuy(id string, sourceID int64, account, security, currency string, shares int64) store.InvestmentTransaction {
	return store.InvestmentTransaction{
		ID: id, SourceID: sourceID, AccountID: account, SecurityID: &security, Date: holdingsDay(2),
		Action: store.ActionBuy, Shares: &shares, Amount: -10_000, Currency: currency,
	}
}

// seedHoldingsStore builds the store under a temp HOME with a CAD brokerage, a USD account and a closed
// account, each holding one priced security.
func seedHoldingsStore(t *testing.T) {
	t.Helper()
	home := newHome(t)
	replaceStoreWithRates(t, home, holdingsRows(), usdRate(holdingsDay(10), 1_360_000))
}

// holdingsRows is the rows seedHoldingsStore stores: Acme (CAD), Vanguard (USD) and Maple (CAD, closed account).
func holdingsRows() store.Rows {
	day, buy := holdingsDay, holdingsBuy
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
	return rows
}

func Test_run_holdings_lists_todays_holdings_in_the_reporting_currency(t *testing.T) {
	seedHoldingsStore(t)

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"holdings"}, holdingsClock())

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

func Test_run_holdings_native_lists_each_currencys_own_total(t *testing.T) {
	seedHoldingsStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings", "--currency", "native"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Holdings on 2026-03-12 in all accounts; cash not included\n\n"+
		holdingsNativeLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value")+
		holdingsNativeLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00")+
		holdingsNativeLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35")+
		holdingsNativeLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00")+
		holdingsNativeLine("Total", "", "", "", "", "CAD", "37,754.00")+
		holdingsNativeLine("Total", "", "", "", "", "USD", "24,659.35"),
		stdout.String())
}
