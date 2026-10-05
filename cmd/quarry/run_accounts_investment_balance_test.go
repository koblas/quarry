// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// boughtOnMarginRows is a CAD account holding 2 shares priced 10.00, paid for by a -100.00 cash row alone: no deposit.
func boughtOnMarginRows(account store.Account) store.Rows {
	when := day(2026, time.March, 2)
	rows := spendRows([]store.Account{account})
	rows.Securities = []store.Security{{ID: "sec-cad", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")}}
	rows.InvestmentTransactions = []store.InvestmentTransaction{{
		ID: "inv-cad", SourceID: 1, AccountID: account.ID, SecurityID: new("sec-cad"), Date: when,
		Action: store.ActionBuy, Shares: new(int64(2_000_000)), Amount: -10_000, Currency: "CAD",
	}}
	rows.Prices = []store.Price{{SecurityID: "sec-cad", SourceID: 1, Date: day(2026, time.March, 1), Price: 10_000_000}}
	rows.Transactions = []store.Transaction{{
		ID: "txn-inv-cad", SourceID: 3, AccountID: account.ID, Date: when, Amount: -10_000, Currency: "CAD",
		Status: "uncleared", InvestmentTransactionID: new("inv-cad"),
	}}
	rows.Splits = []store.Split{{ID: "split-inv-cad", SourceID: 3, TransactionID: "txn-inv-cad", Amount: -10_000}}
	return rows
}

func Test_run_accounts_shows_an_overdrawn_investment_balance_with_its_minus_sign(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, boughtOnMarginRows(brokerageAccount("acct-brokerage", 1, "CAD")))

	got := runAccountsBothForms(t, "--currency", "native")

	assert.Empty(t, got.textErr)
	assert.Equal(t, ""+
		"Account    Type       Currency  Balance  Status\n"+
		"Brokerage  brokerage  CAD        -80.00\n",
		got.text)
	var doc accountsJSON
	require.NoError(t, json.Unmarshal([]byte(got.json), &doc))
	require.Len(t, doc.Accounts, 1)
	brokerage := doc.Accounts[0]
	assert.Equal(t, []*string{new("-80.00"), new("-100.00"), new("20.00")}, []*string{brokerage.Balance, brokerage.Cash, brokerage.HoldingsValue})
}

func Test_run_accounts_all_json_carries_a_closed_investment_accounts_cash_and_holdings_value(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, boughtOnMarginRows(closedAccount(brokerageAccount("acct-brokerage", 1, "CAD"))))

	got := runAccountsBothForms(t, "--all", "--currency", "native")

	var doc accountsJSON
	require.NoError(t, json.Unmarshal([]byte(got.json), &doc))
	require.Len(t, doc.Accounts, 1)
	brokerage := doc.Accounts[0]
	assert.True(t, brokerage.Closed)
	assert.Equal(t, []*string{new("-80.00"), new("-100.00"), new("20.00")}, []*string{brokerage.Balance, brokerage.Cash, brokerage.HoldingsValue})
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

// unpricedHoldingRows is rows of accounts in which held has 2 shares of a security in currency that never had a price.
func unpricedHoldingRows(accounts []store.Account, held store.Account, currency string) store.Rows {
	rows := spendRows(accounts)
	rows.Securities = []store.Security{{ID: "sec-acme", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: &currency}}
	rows.InvestmentTransactions = []store.InvestmentTransaction{{
		ID: "inv-acme", SourceID: 1, AccountID: held.ID, SecurityID: new("sec-acme"), Date: day(2026, time.March, 2),
		Action: store.ActionBuy, Shares: new(int64(2_000_000)), Amount: -10_000, Currency: currency,
	}}
	return rows
}

// accounts reads today from the store's own clock, so the as-of date is matched, not pinned.
const unpricedHoldingPattern = `"%s" holds 1 security with no price on or before \d{4}-\d{2}-\d{2}, ` +
	`so its balance leaves it out; enter a price in Quicken, then run quarry sync`

func Test_run_accounts_lists_the_unpriced_holding_warning_after_the_config_warning_and_before_the_no_rates_warning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	brokerage := store.Account{ID: "acct-brokerage", SourceID: 1, Name: "Brokerage", Type: store.AccountTypeBrokerage, Currency: "USD", Active: true}
	replaceStore(t, home, unpricedHoldingRows([]store.Account{brokerage}, brokerage, "USD"))
	writeConfig(t, home, "snapshot.keep = 3\n")
	configWarning := configShown + ": unknown key snapshot.keep; quarry ignores it"
	holdingPattern := fmt.Sprintf(unpricedHoldingPattern, "Brokerage")

	got := runAccountsBothForms(t)

	lines := strings.Split(strings.TrimSuffix(got.textErr, "\n"), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, "quarry: warning: "+configWarning, lines[0])
	assert.Regexp(t, "^quarry: warning: "+holdingPattern+"$", lines[1])
	assert.Equal(t, "quarry: warning: "+noRatesCADWarning, lines[2])
	assert.Equal(t, got.textErr, got.jsonErr)
	warnings := warningsOf(t, got.json)
	require.Len(t, warnings, 3)
	assert.Equal(t, configPath(home)+": unknown key snapshot.keep; quarry ignores it", warnings[0])
	assert.Regexp(t, "^"+holdingPattern+"$", warnings[1])
	assert.Equal(t, noRatesCADWarning, warnings[2])
}

func Test_run_accounts_warns_about_a_closed_accounts_unpriced_holding_only_with_all(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldRRSP := closedAccount(store.Account{ID: "acct-old", SourceID: 2, Name: "Old RRSP", Type: store.AccountTypeBrokerage, Currency: "CAD"})
	replaceStore(t, home, unpricedHoldingRows([]store.Account{chequingAccount("acct-chequing", 1), oldRRSP}, oldRRSP, "CAD"))

	listed := runAccountsBothForms(t)
	withAll := runAccountsBothForms(t, "--all")

	assert.Empty(t, listed.textErr)
	assert.Equal(t, []string{}, warningsOf(t, listed.json))
	assert.Regexp(t, "^quarry: warning: "+fmt.Sprintf(unpricedHoldingPattern, "Old RRSP")+"\n$", withAll.textErr)
	require.Len(t, warningsOf(t, withAll.json), 1)
	assert.Regexp(t, "^"+fmt.Sprintf(unpricedHoldingPattern, "Old RRSP")+"$", warningsOf(t, withAll.json)[0])
}
