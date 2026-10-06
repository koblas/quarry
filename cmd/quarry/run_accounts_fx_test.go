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

// accountsFXDoc is the part of accounts's --json document the currency tests read.
type accountsFXDoc struct {
	AsOf     string          `json:"as_of"`
	Currency string          `json:"currency"`
	Accounts []accountFXJSON `json:"accounts"`
	Warnings []string        `json:"warnings"`
}

// accountFXJSON is one account of an accounts document: its own balance and its balance in the reporting currency.
type accountFXJSON struct {
	Name             string  `json:"name"`
	Currency         string  `json:"currency"`
	Balance          *string `json:"balance"`
	ConvertedBalance *string `json:"converted_balance"`
}

// accountsFXStore holds, under a fresh HOME, a CAD chequing account with 12,345.67, a USD one with 8,310.00 and a
// USD brokerage account with no transactions. USD/CAD is 1.25 from 2026-01-02 and 1.60 from 2099, which never applies.
func accountsFXStore(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStoreWithRates(t, home, spendRows(
		[]store.Account{
			chequingAccount("acct-cad", 1),
			usdChequingAccount("acct-usd", 2),
			{ID: "acct-brk", SourceID: 3, Name: "Brokerage", Type: store.AccountTypeBrokerage, Currency: "USD", Active: true},
		},
		spendSplit{id: "c1", account: "acct-cad", currency: "CAD", day: day(2025, time.June, 2), cents: 1_240_000},
		spendSplit{id: "c2", account: "acct-cad", currency: "CAD", day: day(2025, time.June, 2), cents: -5_433},
		spendSplit{id: "u1", account: "acct-usd", currency: "USD", day: day(2025, time.June, 2), cents: 831_000},
	),
		store.Rate{Date: day(2026, time.January, 2), USDCAD: money.Rate(1_250_000), Series: "FXUSDCAD"},
		store.Rate{Date: day(2099, time.January, 2), USDCAD: money.Rate(1_600_000), Series: "FXUSDCAD"},
	)
}

func Test_run_accounts_shows_each_balance_in_the_reporting_currency(t *testing.T) {
	accountsFXStore(t)
	cadText := "" +
		"Account      Type       Currency    Balance     In CAD  Status\n" +
		"Brokerage    brokerage  USD            0.00       0.00  unclassified\n" +
		"Chequing     chequing   CAD       12,345.67  12,345.67\n" +
		"US Chequing  chequing   USD        8,310.00  10,387.50\n"
	usdText := "" +
		"Account      Type       Currency    Balance    In USD  Status\n" +
		"Brokerage    brokerage  USD            0.00      0.00  unclassified\n" +
		"Chequing     chequing   CAD       12,345.67  9,876.54\n" +
		"US Chequing  chequing   USD        8,310.00  8,310.00\n"

	cases := []struct {
		name     string
		args     []string
		text     string
		currency string
		accounts []accountFXJSON
	}{
		{
			name: "CAD by default", args: nil, text: cadText, currency: "CAD",
			accounts: []accountFXJSON{
				{Name: "Brokerage", Currency: "USD", Balance: new("0.00"), ConvertedBalance: new("0.00")},
				{Name: "Chequing", Currency: "CAD", Balance: new("12345.67"), ConvertedBalance: new("12345.67")},
				{Name: "US Chequing", Currency: "USD", Balance: new("8310.00"), ConvertedBalance: new("10387.50")},
			},
		},
		{
			name: "USD when asked", args: []string{"--currency", "USD"}, text: usdText, currency: "USD",
			accounts: []accountFXJSON{
				{Name: "Brokerage", Currency: "USD", Balance: new("0.00"), ConvertedBalance: new("0.00")},
				{Name: "Chequing", Currency: "CAD", Balance: new("12345.67"), ConvertedBalance: new("9876.54")},
				{Name: "US Chequing", Currency: "USD", Balance: new("8310.00"), ConvertedBalance: new("8310.00")},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name+" text", func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), append([]string{"accounts"}, c.args...), &stdout, &stderr)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Empty(t, stderr.String())
			assert.Equal(t, c.text, stdout.String())
			assert.NotContains(t, stdout.String(), "Total")
		})

		t.Run(c.name+" json", func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), append([]string{"accounts", "--json"}, c.args...), &stdout, &stderr)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Empty(t, stderr.String())
			var doc accountsFXDoc
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
			assert.Equal(t, c.currency, doc.Currency)
			assert.Equal(t, c.accounts, doc.Accounts)
			assert.Equal(t, []string{}, doc.Warnings)
		})
	}
}
