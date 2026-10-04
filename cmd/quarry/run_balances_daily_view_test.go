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

func Test_run_sql_combines_cash_and_valued_holdings_from_v_balances_daily(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	day := func(d int) time.Time { return time.Date(2026, time.March, d, 0, 0, 0, 0, time.UTC) }
	buy := func(id string, sourceID int64, security string, shares, amount int64) store.InvestmentTransaction {
		return store.InvestmentTransaction{
			ID: id, SourceID: sourceID, AccountID: "acct-cad", SecurityID: &security, Date: day(2),
			Action: store.ActionBuy, Shares: &shares, Amount: amount, Currency: "CAD",
		}
	}
	rows := spendRows(
		[]store.Account{{ID: "acct-cad", SourceID: 1, Name: "Brokerage CAD", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true}},
		spendSplit{id: "deposit", account: "acct-cad", currency: "CAD", day: day(2), cents: 100_000},
	)
	rows.Securities = []store.Security{
		{ID: "sec-cad", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")},
		{ID: "sec-usd", SourceID: 2, Name: "Globex Inc", Ticker: new("GLBX"), Currency: new("USD")},
		{ID: "sec-unpriced", SourceID: 3, Name: "Plain Fund", Ticker: new("PLN"), Currency: new("CAD")},
		{ID: "sec-eur", SourceID: 4, Name: "Euro Fund", Ticker: new("EURF"), Currency: new("EUR")},
	}
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		buy("inv-cad", 1, "sec-cad", 2_000_000, -10_000),
		buy("inv-usd", 2, "sec-usd", 1_000_000, 0),
		buy("inv-unpriced", 3, "sec-unpriced", 1_000_000, 0),
		buy("inv-eur", 4, "sec-eur", 1_000_000, 0),
	}
	rows.Prices = []store.Price{
		{SecurityID: "sec-cad", SourceID: 1, Date: day(1), Price: 10_000_000},
		{SecurityID: "sec-usd", SourceID: 2, Date: day(1), Price: 20_000_000},
		{SecurityID: "sec-eur", SourceID: 3, Date: day(1), Price: 5_000_000},
	}
	rows.Transactions = append(rows.Transactions, store.Transaction{
		ID: "txn-inv-cad", SourceID: 2, AccountID: "acct-cad", Date: day(2), Amount: -10_000, Currency: "CAD",
		Status: "uncleared", InvestmentTransactionID: new("inv-cad"),
	})
	rows.Splits = append(rows.Splits, store.Split{ID: "split-inv-cad", SourceID: 2, TransactionID: "txn-inv-cad", Amount: -10_000})
	replaceStoreWithRates(t, home, rows, usdRate(day(5), 1_250_000), usdRate(day(12), 1_300_000))
	const query = `SELECT date, account_id, account, type, currency, cash, holdings_value, holdings_unvalued,
			balance, balance_cad, balance_usd, usd_cad
		FROM v_balances_daily WHERE date = '2026-03-10'`
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sql", "--csv", query}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"date,account_id,account,type,currency,cash,holdings_value,holdings_unvalued,balance,balance_cad,balance_usd,usd_cad\n"+
		"2026-03-10,acct-cad,Brokerage CAD,brokerage,CAD,900.00,45.00,2,945.00,945.00,756.00,1.250000\n",
		stdout.String())
}
