// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_accounts_lists_open_accounts_with_their_balances(t *testing.T) {
	home := newHome(t)
	syncAccountsFixture(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"accounts", "--currency", "native"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"Account      Type        Currency    Balance  Status\n"+
		"Chequing     chequing    CAD       12,345.67\n"+
		"Old Savings  savings     CAD            0.00  inactive\n"+
		"RRSP         retirement  CAD        1,000.00  unclassified\n"+
		"US Chequing  chequing    USD        8,310.00\n",
		stdout.String())
}

func Test_run_accounts_all_lists_closed_accounts(t *testing.T) {
	home := newHome(t)
	syncAccountsFixture(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"accounts", "--all", "--currency", "native"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"Account        Type         Currency    Balance  Status\n"+
		"Chequing       chequing     CAD       12,345.67\n"+
		"Old Savings    savings      CAD            0.00  inactive\n"+
		"RRSP           retirement   CAD        1,000.00  unclassified\n"+
		"US Chequing    chequing     USD        8,310.00\n"+
		"Visa Infinite  credit_card  CAD       -1,204.17  closed\n",
		stdout.String())
}

func Test_run_accounts_refuses_when_home_is_unset(t *testing.T) {
	t.Setenv("HOME", "")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"accounts"})

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry accounts again\n",
		stderr.String())
}

func Test_run_accounts_reports_a_failed_stdout_write(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	syncBundle(t, bundle)
	var stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts"}, failingWriter{err: errNoSpace}, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, "quarry: cannot write the result to stdout: no space left on device\n", stderr.String())
}

func Test_run_accounts_help_describes_the_command_without_needing_home(t *testing.T) {
	t.Setenv("HOME", "")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"accounts", "--help"})

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), `List the accounts in quarry's store with each one's balance in its own
currency: the sum of its transactions dated today or earlier. Closed
accounts are left out unless --all is given.

Brokerage and retirement accounts' balance is the cash in them plus the
value of their holdings today, each at the latest price Quicken recorded
(quarry holdings lists them).

A column shows each balance in the reporting currency (--currency, else
reporting.currency in the config file, else CAD) at today's Bank of
Canada rate, or the latest earlier one; --currency native leaves it
out. quarry does not add balances together here; quarry networth does.

Status says registered for an account listed in accounts.registered in
the config file, and unclassified for a brokerage or retirement account
in neither accounts.registered nor accounts.non-registered; quarry
findings lists those.`)
	assert.Regexp(t, `(?m)^ +--all +include closed accounts$`, stdout.String())
}

// syncAccountsFixture builds the store from open CAD, USD and inactive accounts,
// a retirement account and a closed one; Chequing also holds a transaction a year ahead.
func syncAccountsFixture(t *testing.T, home string) {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequing := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	usChequing := b.Account(v9fixture.AccountRow{Name: "US Chequing", Type: "CHECKING", Currency: "USD", Active: true})
	b.Account(v9fixture.AccountRow{Name: "Old Savings", Type: "SAVINGS", Currency: "CAD", Active: false})
	rrsp := b.Account(v9fixture.AccountRow{Name: "RRSP", Type: "RETIREMENTIRA", Currency: "CAD", Active: true})
	visa := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Closed: true, Active: true})
	past := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)
	now := time.Now()
	nextYear := time.Date(now.Year()+1, now.Month(), 1, 0, 0, 0, 0, time.UTC)
	addTransaction(b, chequing, "12400.00", past)
	addTransaction(b, chequing, "-54.33", past)
	addTransaction(b, chequing, "500.00", nextYear)
	addTransaction(b, usChequing, "8310.00", past)
	addTransaction(b, rrsp, "1000.00", past)
	addTransaction(b, visa, "-1204.17", past)
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	syncBundle(t, bundle)
}

// addTransaction adds one transaction with a single split of the same amount.
func addTransaction(b *v9fixture.Builder, account int64, amount string, posted time.Time) {
	txn := b.Transaction(v9fixture.TransactionRow{Account: account, Amount: amount, PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount})
}

func syncClosedAccountsFixture(t *testing.T, home string, n int) {
	t.Helper()
	b := v9fixture.NewBuilder()
	for i := range n {
		b.Account(v9fixture.AccountRow{Name: fmt.Sprintf("Closed %d", i+1), Type: "CHECKING", Currency: "CAD", Closed: true, Active: true})
	}
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	syncBundle(t, bundle)
}

func Test_run_accounts_says_how_to_list_them_when_every_account_is_closed(t *testing.T) {
	home := newHome(t)
	syncClosedAccountsFixture(t, home, 3)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"accounts", "--currency", "native"})

	require.Equal(t, 0, exitCode)
	assert.Equal(t, "Account  Type  Currency  Balance  Status\n", stdout.String())
	assert.Equal(t, "quarry: warning: all 3 accounts are closed; pass --all to list them\n", stderr.String())
}

func Test_run_accounts_all_lists_closed_accounts_without_a_note(t *testing.T) {
	home := newHome(t)
	syncClosedAccountsFixture(t, home, 3)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"accounts", "--all", "--currency", "native"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"Account   Type      Currency  Balance  Status\n"+
		"Closed 1  chequing  CAD          0.00  closed\n"+
		"Closed 2  chequing  CAD          0.00  closed\n"+
		"Closed 3  chequing  CAD          0.00  closed\n",
		stdout.String())
}

// syncNotInReportsFixture builds an open, an inactive and a closed account that
// Quicken leaves out of reports, plus an open one it includes.
func syncNotInReportsFixture(t *testing.T, home string) {
	t.Helper()
	off := new(int64(0))
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	b.Account(v9fixture.AccountRow{Name: "Float", Type: "CHECKING", Currency: "CAD", Active: true, UsedInReports: off})
	b.Account(v9fixture.AccountRow{Name: "Old Savings", Type: "SAVINGS", Currency: "CAD", Active: false, UsedInReports: off})
	b.Account(v9fixture.AccountRow{Name: "Old Visa", Type: "CREDITCARD", Currency: "CAD", Closed: true, Active: true, UsedInReports: off})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	syncBundle(t, bundle)
}

func Test_run_accounts_all_marks_accounts_left_out_of_reports(t *testing.T) {
	home := newHome(t)
	syncNotInReportsFixture(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"accounts", "--all", "--currency", "native"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"Account      Type         Currency  Balance  Status\n"+
		"Chequing     chequing     CAD          0.00\n"+
		"Float        chequing     CAD          0.00  not in reports\n"+
		"Old Savings  savings      CAD          0.00  inactive, not in reports\n"+
		"Old Visa     credit_card  CAD          0.00  closed, not in reports\n",
		stdout.String())
}

func syncLinkedTrackingFixture(t *testing.T, home string) {
	t.Helper()
	on, off := new(int64(1)), new(int64(0))
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true, SimpleInvesting: off})
	b.Account(v9fixture.AccountRow{Name: "Linked", Type: "CHECKING", Currency: "CAD", Active: true, SimpleInvesting: on})
	b.Account(v9fixture.AccountRow{Name: "Old Linked", Type: "SAVINGS", Currency: "CAD", Active: false, SimpleInvesting: on})
	b.Account(v9fixture.AccountRow{Name: "Gone Linked", Type: "CREDITCARD", Currency: "CAD", Closed: true, Active: true, UsedInReports: off, SimpleInvesting: on})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	syncBundle(t, bundle)
}

func Test_run_accounts_all_marks_accounts_that_use_linked_account_tracking(t *testing.T) {
	home := newHome(t)
	syncLinkedTrackingFixture(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"accounts", "--all", "--currency", "native"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"Account      Type         Currency  Balance  Status\n"+
		"Chequing     chequing     CAD          0.00\n"+
		"Gone Linked  credit_card  CAD          0.00  closed, not in reports, linked tracking\n"+
		"Linked       chequing     CAD          0.00  linked tracking\n"+
		"Old Linked   savings      CAD          0.00  inactive, linked tracking\n",
		stdout.String())
}

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
	home := newHome(t)
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
			assert.Contains(t, got.text, "no rate  unclassified\n")
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

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"accounts", "--all"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Account      Type      Currency  Balance  In CAD  Status\nUS Chequing  chequing  USD          0.00    0.00  closed\n", stdout.String())
}

func Test_run_accounts_all_pads_a_no_rate_cell_so_a_closed_Status_follows_it_in_the_column(t *testing.T) {
	seedAccounts(t, []store.Account{chequingAccount("acct-cad", 1), closedAccount(brokerageAccount("acct-brk", 2, "USD"))})

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"accounts", "--all"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, ""+
		"Account    Type       Currency  Balance   In CAD  Status\n"+
		"Brokerage  brokerage  USD          0.00  no rate  closed, unclassified\n"+
		"Chequing   chequing   CAD          0.00     0.00\n", stdout.String())
}

func Test_run_accounts_lists_the_config_warning_before_the_no_rates_warning_in_both_forms(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, spendRows([]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)}))
	writeConfig(t, home, "snapshot.keep = 3\n")
	configWarning := configShown + ": unknown key snapshot.keep; quarry ignores it"

	got := runAccountsBothForms(t)

	stderr := "quarry: warning: " + configWarning + "\nquarry: warning: " + noRatesCADWarning + "\n"
	assert.Equal(t, stderr, got.textErr)
	assert.Equal(t, stderr, got.jsonErr)
	assert.Equal(t, []string{configPath(home) + ": unknown key snapshot.keep; quarry ignores it", noRatesCADWarning}, warningsOf(t, got.json))
}

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
	home := newHome(t)
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
			exitCode, stdout, stderr := runCapture(context.Background(), append([]string{"accounts"}, c.args...))

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Empty(t, stderr.String())
			assert.Equal(t, c.text, stdout.String())
			assert.NotContains(t, stdout.String(), "Total")
		})

		t.Run(c.name+" json", func(t *testing.T) {
			exitCode, stdout, stderr := runCapture(context.Background(), append([]string{"accounts", "--json"}, c.args...))

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

func Test_run_accounts_shows_an_investment_balance_as_cash_plus_holdings_value(t *testing.T) {
	home := newHome(t)
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
		"Brokerage  brokerage  CAD        920.00  unclassified\n"+
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
	home := newHome(t)
	replaceStore(t, home, boughtOnMarginRows(brokerageAccount("acct-brokerage", 1, "CAD")))

	got := runAccountsBothForms(t, "--currency", "native")

	assert.Empty(t, got.textErr)
	assert.Equal(t, ""+
		"Account    Type       Currency  Balance  Status\n"+
		"Brokerage  brokerage  CAD        -80.00  unclassified\n",
		got.text)
	var doc accountsJSON
	require.NoError(t, json.Unmarshal([]byte(got.json), &doc))
	require.Len(t, doc.Accounts, 1)
	brokerage := doc.Accounts[0]
	assert.Equal(t, []*string{new("-80.00"), new("-100.00"), new("20.00")}, []*string{brokerage.Balance, brokerage.Cash, brokerage.HoldingsValue})
}

func Test_run_accounts_all_json_carries_a_closed_investment_accounts_cash_and_holdings_value(t *testing.T) {
	home := newHome(t)
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
	home := newHome(t)
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
				"Brokerage  brokerage  USD        920.01  1,150.01  unclassified\n",
		},
		{
			name: "USD", currency: "USD", converted: "920.01",
			text: "Account    Type       Currency  Balance  In USD  Status\n" +
				"Brokerage  brokerage  USD        920.01  920.01  unclassified\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name+" text", func(t *testing.T) {
			exitCode, stdout, stderr := runCapture(context.Background(), []string{"accounts", "--currency", c.currency})

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Empty(t, stderr.String())
			assert.Equal(t, c.text, stdout.String())
		})

		t.Run(c.name+" json", func(t *testing.T) {
			exitCode, stdout, stderr := runCapture(context.Background(), []string{"accounts", "--json", "--currency", c.currency})

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
	home := newHome(t)
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
	home := newHome(t)
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

type accountRowJSON struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Type           string  `json:"type"`
	Currency       string  `json:"currency"`
	Institution    *string `json:"institution"`
	Closed         bool    `json:"closed"`
	Active         bool    `json:"active"`
	InReports      bool    `json:"in_reports"`
	LinkedTracking bool    `json:"linked_tracking"`
	Balance        *string `json:"balance"`
	Cash           *string `json:"cash"`
	HoldingsValue  *string `json:"holdings_value"`
}

type accountsJSON struct {
	AsOf     string           `json:"as_of"`
	Accounts []accountRowJSON `json:"accounts"`
	Warnings []string         `json:"warnings"`
}

func Test_run_accounts_json_returns_accounts_as_a_document(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	bank := b.Institution(v9fixture.InstitutionRow{Name: "First Bank"})
	chequing := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Institution: bank, Active: true})
	rrsp := b.Account(v9fixture.AccountRow{Name: "RRSP", Type: "RETIREMENTIRA", Currency: "CAD", Institution: bank, Active: true})
	savings := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	past := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)
	addTransaction(b, chequing, "12400.00", past)
	addTransaction(b, chequing, "-54.33", past)
	addTransaction(b, rrsp, "1000.00", past)
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	syncBundle(t, bundle)
	var stdout, stderr bytes.Buffer
	before := time.Now()

	exitCode := run(context.Background(), []string{"accounts", "--json", "--currency", "native"}, &stdout, &stderr)

	after := time.Now()
	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	want := func(asOf time.Time) string {
		return fmt.Sprintf(`{
  "as_of": "%s",
  "currency": "native",
  "accounts": [
    {
      "id": "acct-%d",
      "name": "Chequing",
      "type": "chequing",
      "currency": "CAD",
      "institution": "First Bank",
      "closed": false,
      "active": true,
      "in_reports": true,
      "linked_tracking": false,
      "registered": null,
      "balance": "12345.67",
      "cash": "12345.67",
      "holdings_value": null,
      "converted_balance": null
    },
    {
      "id": "acct-%d",
      "name": "RRSP",
      "type": "retirement",
      "currency": "CAD",
      "institution": "First Bank",
      "closed": false,
      "active": true,
      "in_reports": true,
      "linked_tracking": false,
      "registered": null,
      "balance": "1000.00",
      "cash": "1000.00",
      "holdings_value": "0.00",
      "converted_balance": null
    },
    {
      "id": "acct-%d",
      "name": "Savings",
      "type": "savings",
      "currency": "CAD",
      "institution": null,
      "closed": false,
      "active": true,
      "in_reports": true,
      "linked_tracking": false,
      "registered": null,
      "balance": "0.00",
      "cash": "0.00",
      "holdings_value": null,
      "converted_balance": null
    }
  ],
  "warnings": []
}
`, asOf.Format("2006-01-02"), chequing, rrsp, savings)
	}
	assert.Contains(t, []string{want(before), want(after)}, stdout.String())
}

func Test_run_accounts_json_reports_the_all_closed_note_in_both_streams(t *testing.T) {
	home := newHome(t)
	syncClosedAccountsFixture(t, home, 3)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"accounts", "--json", "--currency", "native"})

	require.Equal(t, 0, exitCode, stderr.String())
	var got accountsJSON
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	assert.Empty(t, got.Accounts)
	assert.Contains(t, stdout.String(), "  \"accounts\": [],\n")
	assert.Equal(t, []string{"all 3 accounts are closed; pass --all to list them"}, got.Warnings)
	assert.Equal(t, "quarry: warning: all 3 accounts are closed; pass --all to list them\n", stderr.String())
}

func Test_run_accounts_json_carries_in_reports_per_account(t *testing.T) {
	home := newHome(t)
	syncNotInReportsFixture(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"accounts", "--all", "--json", "--currency", "native"})

	require.Equal(t, 0, exitCode, stderr.String())
	var got accountsJSON
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	inReports := map[string]bool{}
	for _, a := range got.Accounts {
		inReports[a.Name] = a.InReports
	}
	assert.Equal(t, map[string]bool{"Chequing": true, "Float": false, "Old Savings": false, "Old Visa": false}, inReports)
	assert.Contains(t, stdout.String(), "      \"active\": true,\n      \"in_reports\": false,\n")
}

func Test_run_accounts_json_carries_linked_tracking_per_account(t *testing.T) {
	home := newHome(t)
	syncLinkedTrackingFixture(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"accounts", "--all", "--json", "--currency", "native"})

	require.Equal(t, 0, exitCode, stderr.String())
	var got accountsJSON
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	linked := map[string]bool{}
	for _, a := range got.Accounts {
		linked[a.Name] = a.LinkedTracking
	}
	assert.Equal(t, map[string]bool{"Chequing": false, "Linked": true, "Old Linked": true, "Gone Linked": true}, linked)
}

const (
	unmatchedConfig = "[accounts]\nregistered = [\"acct-99\"]\nnon-registered = [\"RBC 12345678\"]\n"
	unmatchedTail   = ", which is not an account in quarry's store; quarry skips it"
)

// unmatchedWarningsAt is the two warnings unmatchedConfig draws, naming the config file as shown.
func unmatchedWarningsAt(shown string) []string {
	return []string{
		shown + `: accounts.registered lists "acct-99"` + unmatchedTail,
		shown + `: accounts.non-registered lists "RBC ****5678"` + unmatchedTail,
	}
}

// unmatchedStderr is the stderr lines unmatchedConfig draws from accounts and findings.
const unmatchedStderr = "quarry: warning: " + configShown + `: accounts.registered lists "acct-99"` + unmatchedTail + "\n" +
	"quarry: warning: " + configShown + `: accounts.non-registered lists "RBC ****5678"` + unmatchedTail + "\n"

// syncUnmatchedFixture syncs one chequing account under home and writes unmatchedConfig.
func syncUnmatchedFixture(t *testing.T, home string) {
	t.Helper()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	writeConfig(t, home, unmatchedConfig)
}

// jsonWarnings runs args and returns the warnings array of its --json document and its stderr.
func jsonWarnings(t *testing.T, args ...string) ([]string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), args, &stdout, &stderr), stderr.String())
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	return doc.Warnings, stderr.String()
}

func Test_run_accounts_json_names_the_config_by_absolute_path_in_a_no_account_warning(t *testing.T) {
	home := newHome(t)
	syncUnmatchedFixture(t, home)

	warnings, stderr := jsonWarnings(t, "accounts", "--json")

	assert.Equal(t, unmatchedWarningsAt(configPath(home)), warnings)
	assert.Equal(t, unmatchedStderr, stderr)
}

func Test_run_findings_json_names_the_config_by_absolute_path_in_a_no_account_warning(t *testing.T) {
	home := newHome(t)
	syncUnmatchedFixture(t, home)

	warnings, stderr := jsonWarnings(t, "findings", "--json")

	assert.Equal(t, unmatchedWarningsAt(configPath(home)), warnings)
	assert.Equal(t, unmatchedStderr, stderr)
}

func Test_run_accounts_warns_nothing_about_a_listed_closed_account_without_all(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	closed := b.Account(v9fixture.AccountRow{Name: "Old RRSP", Type: "RETIREMENTIRA", Currency: "CAD", Closed: true})
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	writeConfig(t, home, fmt.Sprintf("[accounts]\nregistered = [\"acct-%d\"]\n", closed))

	warnings, stderr := jsonWarnings(t, "accounts", "--json")

	assert.Empty(t, warnings)
	assert.NotContains(t, stderr, "which is not an account")
}

func Test_run_mcp_data_quality_names_the_config_by_absolute_path_in_a_no_account_warning(t *testing.T) {
	var home string
	ctx, peer := newStatusPeer(t, func(t *testing.T, h string) {
		t.Helper()
		home = h
		syncUnmatchedFixture(t, h)
	})

	result := callDataQuality(ctx, t, peer, map[string]any{})
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	require.False(t, result.IsError, textOf(result))
	var doc dataQualityDocument
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &doc))
	assert.Equal(t, unmatchedWarningsAt(configPath(home)), doc.Warnings)
}

func Test_run_status_and_sync_stay_silent_about_a_listed_id_that_names_no_account(t *testing.T) {
	home := newHome(t)
	syncUnmatchedFixture(t, home)
	control, _ := jsonWarnings(t, "accounts", "--json")
	require.Equal(t, unmatchedWarningsAt(configPath(home)), control)

	statusWarnings, statusStderr := jsonWarnings(t, "status", "--json")
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	exitCode, _, syncStderr := syncNewBundle(t, home, "DocumentsB", b)

	assert.Empty(t, statusWarnings)
	assert.Empty(t, statusStderr)
	assert.Equal(t, 0, exitCode)
	assert.Empty(t, syncStderr)
}

func Test_run_mcp_sync_status_stays_silent_about_a_listed_id_that_names_no_account(t *testing.T) {
	var home string
	ctx, peer := newStatusPeer(t, func(t *testing.T, h string) {
		t.Helper()
		home = h
		syncUnmatchedFixture(t, h)
	})
	control, _ := jsonWarnings(t, "accounts", "--json")
	require.Equal(t, unmatchedWarningsAt(configPath(home)), control)

	doc := readSyncStatus(ctx, t, peer)
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	assert.Empty(t, doc.Warnings)
	assert.Empty(t, peer.stderr.String())
}

func Test_run_accounts_and_findings_warn_a_listed_id_that_names_no_account(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	writeConfig(t, home, "[accounts]\nregistered = [\"acct-99\"]\nnon-registered = [\"RBC 12345678\"]\n")
	const warningLead = "quarry: warning: " + configShown + ": accounts."
	const warningTail = ", which is not an account in quarry's store; quarry skips it\n"

	for _, command := range []string{"accounts", "findings"} {
		t.Run(command, func(t *testing.T) {
			exitCode, _, stderr := runCapture(context.Background(), []string{command})

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Contains(t, stderr.String(), warningLead+`registered lists "acct-99"`+warningTail)
			assert.Contains(t, stderr.String(), warningLead+`non-registered lists "RBC ****5678"`+warningTail)
		})
	}
}

// seedWideBrokerageStore stores a CAD brokerage holding 999,999,999,999.999999 shares priced at the same, so its
// balance passes 64 bits of cents.
func seedWideBrokerageStore(t *testing.T) {
	t.Helper()
	home := newHome(t)
	const widest = 999_999_999_999_999_999
	rows := spendRows([]store.Account{brokerageAccount("acct-cad", 1, "CAD")})
	rows.Securities = []store.Security{{ID: "sec-wide", SourceID: 1, Name: "Wide Fund", Ticker: new("WIDE"), Currency: new("CAD")}}
	rows.InvestmentTransactions = []store.InvestmentTransaction{holdingsBuy("inv-wide", 1, "acct-cad", "sec-wide", "CAD", widest)}
	rows.Prices = []store.Price{{SecurityID: "sec-wide", SourceID: 1, Date: holdingsDay(2), Price: widest}}
	replaceStore(t, home, rows)
}

func Test_run_accounts_lists_a_balance_past_64_bits_in_exact_cents(t *testing.T) {
	seedWideBrokerageStore(t)

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"accounts"}, holdingsClock())

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "999,999,999,999,999,998,000,000.00")
}

func Test_run_holdings_of_a_named_account_resolves_an_account_whose_balance_passes_64_bits(t *testing.T) {
	seedWideBrokerageStore(t)

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"holdings", "--account", "Brokerage"}, holdingsClock())

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "Wide Fund (WIDE)")
}
