package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_sql_values_each_holding_on_a_date_from_v_holdings(t *testing.T) {
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
		{ID: "acct-cad", SourceID: 1, Name: "Brokerage CAD", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
		{ID: "acct-usd", SourceID: 2, Name: "Brokerage USD", Type: store.AccountTypeBrokerage, Currency: "USD", Active: true},
	})
	rows.Securities = []store.Security{
		{ID: "sec-cad", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")},
		{ID: "sec-usd", SourceID: 2, Name: "Globex Inc", Ticker: new("GLBX"), Currency: new("USD")},
	}
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		buy("inv-cad", 1, "acct-cad", "sec-cad", "CAD", 3_500_000),
		buy("inv-usd", 2, "acct-usd", "sec-usd", "USD", 2_000_000),
	}
	rows.Prices = []store.Price{
		{SecurityID: "sec-cad", SourceID: 1, Date: day(9), Price: 11_000_000},
		{SecurityID: "sec-cad", SourceID: 2, Date: day(11), Price: 12_345_678},
		{SecurityID: "sec-cad", SourceID: 3, Date: day(13), Price: 99_000_000},
		{SecurityID: "sec-usd", SourceID: 4, Date: day(10), Price: 19_000_000},
		{SecurityID: "sec-usd", SourceID: 5, Date: day(12), Price: 20_123_456},
		{SecurityID: "sec-usd", SourceID: 6, Date: day(13), Price: 88_000_000},
	}
	replaceStoreWithRates(t, home, rows, usdRate(day(11), 1_250_000), usdRate(day(12), 1_300_000))
	const query = `SELECT account_id, security_id, shares, price, price_date, value, value_cad, value_usd, usd_cad
		FROM v_holdings WHERE date = '2026-03-12' ORDER BY account_id`
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sql", "--csv", query}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"account_id,security_id,shares,price,price_date,value,value_cad,value_usd,usd_cad\n"+
		"acct-cad,sec-cad,3.500000,12.345678,2026-03-11,43.21,43.21,33.24,1.300000\n"+
		"acct-usd,sec-usd,2.000000,20.123456,2026-03-12,40.25,52.33,40.25,1.300000\n",
		stdout.String())
}
