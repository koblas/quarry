// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Rows are built by hand: the price needs no sync, and the cash row carries InvestmentTransactionID.
func Test_run_accounts_shows_an_investment_balance_as_cash_plus_holdings_value(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	day := func(d int) time.Time { return time.Date(2026, time.March, d, 0, 0, 0, 0, time.UTC) }
	rows := spendRows(
		[]store.Account{
			{ID: "acct-brokerage", SourceID: 1, Name: "Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
			chequingAccount("acct-chequing", 2),
		},
		spendSplit{id: "deposit", account: "acct-brokerage", currency: "CAD", day: day(2), cents: 100_000},
		spendSplit{id: "paycheque", account: "acct-chequing", currency: "CAD", day: day(2), cents: 10_000},
	)
	rows.Securities = []store.Security{{ID: "sec-cad", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")}}
	rows.InvestmentTransactions = []store.InvestmentTransaction{{
		ID: "inv-cad", SourceID: 1, AccountID: "acct-brokerage", SecurityID: new("sec-cad"), Date: day(2),
		Action: store.ActionBuy, Shares: new(int64(2_000_000)), Amount: -10_000, Currency: "CAD",
	}}
	rows.Prices = []store.Price{{SecurityID: "sec-cad", SourceID: 1, Date: day(1), Price: 10_000_000}}
	rows.Transactions = append(rows.Transactions, store.Transaction{
		ID: "txn-inv-cad", SourceID: 3, AccountID: "acct-brokerage", Date: day(2), Amount: -10_000, Currency: "CAD",
		Status: "uncleared", InvestmentTransactionID: new("inv-cad"),
	})
	rows.Splits = append(rows.Splits, store.Split{ID: "split-inv-cad", SourceID: 3, TransactionID: "txn-inv-cad", Amount: -10_000})
	replaceStore(t, home, rows)
	var stdout, stderr, jsonOut bytes.Buffer

	textCode := run(context.Background(), []string{"accounts", "--currency", "native"}, &stdout, &stderr)
	jsonCode := run(context.Background(), []string{"accounts", "--json", "--currency", "native"}, &jsonOut, &stderr)

	require.Equal(t, 0, textCode, stderr.String())
	require.Equal(t, 0, jsonCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"Account    Type       Currency  Balance  Status\n"+
		"Brokerage  brokerage  CAD        920.00\n"+
		"Chequing   chequing   CAD        100.00\n",
		stdout.String())
	var doc accountsJSON
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &doc))
	require.Len(t, doc.Accounts, 2)
	brokerage, chequing := doc.Accounts[0], doc.Accounts[1]
	assert.Equal(t, "Brokerage", brokerage.Name)
	assert.Equal(t, new("920.00"), brokerage.Balance)
	assert.Equal(t, new("900.00"), brokerage.Cash)
	assert.Equal(t, new("20.00"), brokerage.HoldingsValue)
	assert.Equal(t, new("100.00"), chequing.Cash)
	assert.Nil(t, chequing.HoldingsValue)
}

func Test_run_accounts_converts_an_investment_balance_as_cash_plus_holdings_value(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	when := day(2026, time.March, 2)
	rows := spendRows(
		[]store.Account{{ID: "acct-brokerage", SourceID: 1, Name: "Brokerage", Type: store.AccountTypeBrokerage, Currency: "USD", Active: true}},
		spendSplit{id: "deposit", account: "acct-brokerage", currency: "USD", day: when, cents: 100_001},
	)
	rows.Securities = []store.Security{{ID: "sec-usd", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("USD")}}
	rows.InvestmentTransactions = []store.InvestmentTransaction{{
		ID: "inv-usd", SourceID: 1, AccountID: "acct-brokerage", SecurityID: new("sec-usd"), Date: when,
		Action: store.ActionBuy, Shares: new(int64(2_000_000)), Amount: -10_000, Currency: "USD",
	}}
	rows.Prices = []store.Price{{SecurityID: "sec-usd", SourceID: 1, Date: day(2026, time.March, 1), Price: 10_000_000}}
	rows.Transactions = append(rows.Transactions, store.Transaction{
		ID: "txn-inv-usd", SourceID: 3, AccountID: "acct-brokerage", Date: when, Amount: -10_000, Currency: "USD",
		Status: "uncleared", InvestmentTransactionID: new("inv-usd"),
	})
	rows.Splits = append(rows.Splits, store.Split{ID: "split-inv-usd", SourceID: 3, TransactionID: "txn-inv-usd", Amount: -10_000})
	replaceStoreWithRates(t, home, rows, store.Rate{Date: day(2026, time.January, 2), USDCAD: money.Rate(1_250_000), Series: "FXUSDCAD"})

	cases := []struct {
		name      string
		currency  string
		text      string
		converted string
	}{
		{
			name: "CAD", currency: "CAD", converted: "1150.01",
			text: "Account    Type       Currency  Balance    In CAD  Status\n" +
				"Brokerage  brokerage  USD        920.01  1,150.01\n",
		},
		{
			name: "USD", currency: "USD", converted: "920.01",
			text: "Account    Type       Currency  Balance  In USD  Status\n" +
				"Brokerage  brokerage  USD        920.01  920.01\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name+" text", func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), []string{"accounts", "--currency", c.currency}, &stdout, &stderr)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Empty(t, stderr.String())
			assert.Equal(t, c.text, stdout.String())
		})

		t.Run(c.name+" json", func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), []string{"accounts", "--json", "--currency", c.currency}, &stdout, &stderr)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Empty(t, stderr.String())
			var doc accountsFXDoc
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
			assert.Equal(t, []accountFXJSON{
				{Name: "Brokerage", Currency: "USD", Balance: new("920.01"), ConvertedBalance: new(c.converted)},
			}, doc.Accounts)
		})
	}
}
