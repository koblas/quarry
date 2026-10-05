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

const (
	noRatesCADWarning = "the store has no exchange rates, so USD balances show no rate in the In CAD column; run quarry sync to fetch them"
	noRatesUSDWarning = "the store has no exchange rates, so CAD balances show no rate in the In USD column; run quarry sync to fetch them"
	futureCADWarning  = "the first exchange rate in the store, 2099-01-02, is dated after today, so USD balances show no rate in the In CAD column; check the Mac's date and time"
	futureUSDWarning  = "the first exchange rate in the store, 2099-01-02, is dated after today, so CAD balances show no rate in the In USD column; check the Mac's date and time"
)

func brokerageAccount(id string, sourceID int64, currency string) store.Account {
	return store.Account{ID: id, SourceID: sourceID, Name: "Brokerage", Type: store.AccountTypeBrokerage, Currency: currency, Active: true}
}

func closedAccount(a store.Account) store.Account {
	a.Closed, a.Active = true, false
	return a
}

// seedAccounts stores accounts under a fresh HOME with rates and no transactions.
func seedAccounts(t *testing.T, accounts []store.Account, rates ...store.Rate) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStoreWithRates(t, home, spendRows(accounts), rates...)
}

type accountsOutput struct{ text, textErr, json, jsonErr string }

// runAccountsBothForms runs accounts with args in text and then --json form, returning both runs' output.
func runAccountsBothForms(t *testing.T, args ...string) accountsOutput {
	t.Helper()
	var textOut, textErr, jsonOut, jsonErr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), append([]string{"accounts"}, args...), &textOut, &textErr), textErr.String())
	require.Equal(t, 0, run(context.Background(), append([]string{"accounts", "--json"}, args...), &jsonOut, &jsonErr), jsonErr.String())
	return accountsOutput{text: textOut.String(), textErr: textErr.String(), json: jsonOut.String(), jsonErr: jsonErr.String()}
}

func warningsOf(t *testing.T, doc string) []string {
	t.Helper()
	var parsed accountsFXDoc
	require.NoError(t, json.Unmarshal([]byte(doc), &parsed), doc)
	return parsed.Warnings
}

var (
	pastRate   = store.Rate{Date: time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC), USDCAD: money.Rate(1_250_000), Series: "FXUSDCAD"}
	futureRate = store.Rate{Date: time.Date(2099, time.January, 2, 0, 0, 0, 0, time.UTC), USDCAD: money.Rate(1_600_000), Series: "FXUSDCAD"}
)

func Test_run_accounts_warns_of_the_missing_rate_and_shows_no_rate_in_both_forms_and_currencies(t *testing.T) {
	cases := []struct {
		name  string
		rates []store.Rate
		args  []string
		want  string
	}{
		{name: "no rates in CAD", args: nil, want: noRatesCADWarning},
		{name: "no rates in USD", args: []string{"--currency", "USD"}, want: noRatesUSDWarning},
		{name: "rates only after today in CAD", rates: []store.Rate{futureRate}, args: nil, want: futureCADWarning},
		{name: "rates only after today in USD", rates: []store.Rate{futureRate}, args: []string{"--currency", "USD"}, want: futureUSDWarning},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedAccounts(t, []store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)}, c.rates...)

			got := runAccountsBothForms(t, c.args...)

			assert.Equal(t, "quarry: warning: "+c.want+"\n", got.textErr)
			assert.Contains(t, got.text, "no rate\n")
			assert.Equal(t, []string{c.want}, warningsOf(t, got.json))
			assert.Equal(t, "quarry: warning: "+c.want+"\n", got.jsonErr)
		})
	}
}

func Test_run_accounts_warns_of_the_missing_rate_for_an_investment_account_in_both_forms(t *testing.T) {
	cases := []struct {
		name     string
		accounts []store.Account
		rates    []store.Rate
		args     []string
		want     string
	}{
		{
			name:     "a USD brokerage beside CAD chequing with no rates in CAD",
			accounts: []store.Account{chequingAccount("acct-cad", 1), brokerageAccount("acct-brk", 2, "USD")},
			want:     noRatesCADWarning,
		},
		{
			name:     "a CAD brokerage beside USD chequing with no rates in USD",
			accounts: []store.Account{usdChequingAccount("acct-usd", 1), brokerageAccount("acct-brk", 2, "CAD")},
			args:     []string{"--currency", "USD"}, want: noRatesUSDWarning,
		},
		{
			name:     "a USD brokerage beside CAD chequing with rates only after today",
			accounts: []store.Account{chequingAccount("acct-cad", 1), brokerageAccount("acct-brk", 2, "USD")},
			rates:    []store.Rate{futureRate}, want: futureCADWarning,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedAccounts(t, c.accounts, c.rates...)

			got := runAccountsBothForms(t, c.args...)

			assert.Equal(t, "quarry: warning: "+c.want+"\n", got.textErr)
			assert.Contains(t, got.text, "no rate\n")
			assert.Equal(t, []string{c.want}, warningsOf(t, got.json))
			assert.Equal(t, "quarry: warning: "+c.want+"\n", got.jsonErr)
		})
	}
}

func Test_accounts_warn_of_no_rates_only_when_a_balance_needed_one(t *testing.T) {
	cases := []struct {
		name     string
		accounts []store.Account
		rates    []store.Rate
		args     []string
	}{
		{name: "an all-CAD store with no rates in CAD", accounts: []store.Account{chequingAccount("acct-cad", 1)}},
		{name: "an all-USD store with no rates in USD", accounts: []store.Account{usdChequingAccount("acct-usd", 1)}, args: []string{"--currency", "USD"}},
		{name: "native with no rates", accounts: []store.Account{usdChequingAccount("acct-usd", 1)}, args: []string{"--currency", "native"}},
		{
			name:     "rates on or before today, in CAD",
			accounts: []store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
			rates:    []store.Rate{pastRate},
		},
		{
			name:     "rates on or before today, in USD",
			accounts: []store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
			rates:    []store.Rate{pastRate}, args: []string{"--currency", "USD"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedAccounts(t, c.accounts, c.rates...)

			got := runAccountsBothForms(t, c.args...)

			assert.Empty(t, got.textErr)
			assert.Empty(t, got.jsonErr)
			assert.Equal(t, []string{}, warningsOf(t, got.json))
			assert.NotContains(t, got.text, "no rate")
		})
	}
}

func Test_run_accounts_header_only_keeps_the_column_and_prints_only_the_all_closed_note(t *testing.T) {
	cases := []struct {
		name string
		args []string
		text string
	}{
		{name: "CAD", args: nil, text: "Account  Type  Currency  Balance  In CAD  Status\n"},
		{name: "USD", args: []string{"--currency", "USD"}, text: "Account  Type  Currency  Balance  In USD  Status\n"},
	}
	const note = "the only account is closed; pass --all to list it"

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedAccounts(t, []store.Account{closedAccount(usdChequingAccount("acct-usd", 1))})

			got := runAccountsBothForms(t, c.args...)

			assert.Equal(t, c.text, got.text)
			assert.Equal(t, "quarry: warning: "+note+"\n", got.textErr)
			assert.Equal(t, []string{note}, warningsOf(t, got.json))
		})
	}
}

func Test_run_accounts_all_converts_a_closed_account_like_any_other(t *testing.T) {
	seedAccounts(t, []store.Account{closedAccount(usdChequingAccount("acct-usd", 1))}, pastRate)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts", "--all"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Account      Type      Currency  Balance  In CAD  Status\nUS Chequing  chequing  USD          0.00    0.00  closed\n", stdout.String())
}

func Test_run_accounts_all_pads_a_no_rate_cell_so_a_closed_Status_follows_it_in_the_column(t *testing.T) {
	seedAccounts(t, []store.Account{chequingAccount("acct-cad", 1), closedAccount(brokerageAccount("acct-brk", 2, "USD"))})
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts", "--all"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, ""+
		"Account    Type       Currency  Balance   In CAD  Status\n"+
		"Brokerage  brokerage  USD          0.00  no rate  closed\n"+
		"Chequing   chequing   CAD          0.00     0.00\n", stdout.String())
}

func Test_run_accounts_lists_the_config_warning_before_the_no_rates_warning_in_both_forms(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStoreWithRates(t, home, spendRows([]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)}))
	writeConfig(t, home, "snapshot.keep = 3\n")
	configWarning := configShown + ": unknown key snapshot.keep; quarry ignores it"

	got := runAccountsBothForms(t)

	stderr := "quarry: warning: " + configWarning + "\nquarry: warning: " + noRatesCADWarning + "\n"
	assert.Equal(t, stderr, got.textErr)
	assert.Equal(t, stderr, got.jsonErr)
	assert.Equal(t, []string{configPath(home) + ": unknown key snapshot.keep; quarry ignores it", noRatesCADWarning}, warningsOf(t, got.json))
}
