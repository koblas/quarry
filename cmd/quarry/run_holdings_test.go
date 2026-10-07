package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// holdingsRowOf is one holdings table line with the columns w wide; the table ends a line at its last non-blank cell.
func holdingsRowOf(w [8]int, account, security, shares, price, pricedOn, currency, value, in string) string {
	return strings.TrimRight(fmt.Sprintf("%-*s  %-*s  %*s  %*s  %-*s  %-*s  %*s  %*s",
		w[0], account, w[1], security, w[2], shares, w[3], price, w[4], pricedOn, w[5], currency, w[6], value, w[7], in), " ") + "\n"
}

// holdingsLine is one holdings table line, each cell as wide as the fixture's widest.
func holdingsLine(account, security, shares, price, pricedOn, currency, value, in string) string {
	return holdingsRowOf([8]int{17, 26, 6, 6, 10, 8, 9, 9}, account, security, shares, price, pricedOn, currency, value, in)
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

	stdout, stderr := mustRunHoldings(t)

	assert.Empty(t, stderr)
	assert.Equal(t, "Holdings on 2026-03-12 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35", "33,536.72")+
		holdingsLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00", "50.00")+
		holdingsLine("Total", "", "", "", "", "", "", "71,290.72"),
		stdout)
}

func Test_run_holdings_native_lists_each_currencys_own_total(t *testing.T) {
	seedHoldingsStore(t)

	stdout, stderr := mustRunHoldings(t, "--currency", "native")

	assert.Empty(t, stderr)
	assert.Equal(t, "Holdings on 2026-03-12 in all accounts; cash not included\n\n"+
		holdingsNativeLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value")+
		holdingsNativeLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00")+
		holdingsNativeLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35")+
		holdingsNativeLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00")+
		holdingsNativeLine("Total", "", "", "", "", "CAD", "37,754.00")+
		holdingsNativeLine("Total", "", "", "", "", "USD", "24,659.35"),
		stdout)
}

const chequingWarning = "quarry: warning: account \"Chequing\" is not a brokerage or retirement account, so it has no holdings\n"

// holdingsAccountDoc reads back the holdings --json keys the account filter tests pin.
type holdingsAccountDoc struct {
	AccountFilter []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"account_filter"`
	Warnings []string `json:"warnings"`
}

// seedHoldingsAccounts stores holdingsRows with Chequing, an empty brokerage and two accounts named Visa added.
func seedHoldingsAccounts(t *testing.T) {
	t.Helper()
	home := newHome(t)
	rows := holdingsRows()
	rows.Accounts = append(rows.Accounts,
		chequingAccount("acct-chq", 4),
		store.Account{ID: "acct-empty", SourceID: 5, Name: "Empty", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
		store.Account{ID: "acct-977", SourceID: 6, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
		store.Account{ID: "acct-812", SourceID: 7, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true})
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
}

func runHoldingsAccounts(t *testing.T, args ...string) (int, string, string) {
	t.Helper()

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), append([]string{"holdings"}, args...), holdingsClock())

	return exitCode, stdout.String(), stderr.String()
}

// holdingsAccountLine is one table line sized to the Brokerage-only listing: Acme Corp (ACME) is its widest security.
func holdingsAccountLine(account, security, shares, price, pricedOn, currency, value, in string) string {
	return holdingsRowOf([8]int{9, 16, 6, 5, 10, 8, 9, 9}, account, security, shares, price, pricedOn, currency, value, in)
}

func Test_run_holdings_account_filter_lists_the_named_accounts_and_warns_for_chequing(t *testing.T) {
	home := newHome(t)
	rows := holdingsRows()
	rows.Accounts = append(rows.Accounts, chequingAccount("acct-chq", 4))
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))

	stdout, stderr := mustRunHoldings(t, "--account", "Brokerage", "--account", "Chequing")

	assert.Equal(t, "Holdings on 2026-03-12 in Brokerage, Chequing, amounts in CAD; cash not included\n\n"+
		holdingsAccountLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsAccountLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsAccountLine("Total", "", "", "", "", "", "", "37,704.00"),
		stdout)
	assert.Equal(t, chequingWarning, stderr)
}

func Test_run_holdings_refuses_an_account_it_cannot_pick_with_nothing_on_stdout(t *testing.T) {
	const (
		unknown   = "quarry: no account named \"Chequeing\"; run quarry accounts --all to list them\n"
		empty     = "quarry: no account named \"\"; run quarry accounts --all to list them\n"
		ambiguous = "quarry: 2 accounts are named \"Visa\"; pass one of their ids instead: acct-812, acct-977\n"
	)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"an unknown name", []string{"--account", "Chequeing"}, unknown},
		{"an empty argument", []string{"--account", ""}, empty},
		{"an ambiguous name", []string{"--account", "Visa"}, ambiguous},
		{"an unknown name as JSON", []string{"--account", "Chequeing", "--json"}, unknown},
		{"an empty argument as JSON", []string{"--account", "", "--json"}, empty},
		{"an ambiguous name as JSON", []string{"--account", "Visa", "--json"}, ambiguous},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedHoldingsAccounts(t)

			exitCode, stdout, stderr := runHoldingsAccounts(t, c.args...)

			assert.Equal(t, 1, exitCode)
			assert.Equal(t, c.want, stderr)
			assert.Empty(t, stdout)
		})
	}
}

func Test_run_holdings_of_only_a_non_investment_account_prints_both_warnings_in_order(t *testing.T) {
	seedHoldingsAccounts(t)

	exitCode, stdout, stderr := runHoldingsAccounts(t, "--account", "Chequing")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "Holdings on 2026-03-12 in Chequing, amounts in CAD; cash not included\n\n"+
		"Account  Security  Shares  Price  Priced on  Currency  Value  In CAD\n", stdout)
	assert.Equal(t, chequingWarning+namedNothingWarning, stderr)
}

func Test_run_holdings_of_an_investment_account_with_nothing_held_warns_only_that_nothing_is_held(t *testing.T) {
	seedHoldingsAccounts(t)

	exitCode, _, stderr := runHoldingsAccounts(t, "--account", "Empty")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, namedNothingWarning, stderr)
}

func Test_run_holdings_json_lists_the_named_accounts_and_the_warnings_stderr_prints(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"in the reporting currency", []string{"--json"}},
		{"in native amounts", []string{"--json", "--currency", "native"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedHoldingsAccounts(t)

			exitCode, stdout, stderr := runHoldingsAccounts(t, append([]string{"--account", "Chequing", "--account", "Brokerage"}, c.args...)...)

			require.Equal(t, 0, exitCode, stderr)
			var doc holdingsAccountDoc
			require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
			assert.JSONEq(t, `[{"id":"acct-chq","name":"Chequing"},{"id":"acct-cad","name":"Brokerage"}]`, mustJSON(t, doc.AccountFilter))
			assert.Equal(t, []string{"account \"Chequing\" is not a brokerage or retirement account, so it has no holdings"}, doc.Warnings)
			assert.Equal(t, chequingWarning, stderr)
		})
	}
}

func Test_run_holdings_names_an_account_given_by_name_and_by_id_once(t *testing.T) {
	seedHoldingsAccounts(t)

	exitCode, stdout, stderr := runHoldingsAccounts(t, "--account", "Brokerage", "--account", "acct-cad", "--json")

	require.Equal(t, 0, exitCode, stderr)
	var doc holdingsAccountDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	assert.JSONEq(t, `[{"id":"acct-cad","name":"Brokerage"}]`, mustJSON(t, doc.AccountFilter))
}

func Test_run_holdings_without_an_account_lists_every_account_and_an_empty_account_filter(t *testing.T) {
	seedHoldingsAccounts(t)

	exitCode, stdout, stderr := runHoldingsAccounts(t, "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Contains(t, stdout, `"account_filter": []`)
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	out, err := json.Marshal(v)
	require.NoError(t, err)
	return string(out)
}

// holdingsAsOfLine is one table line of seedSplitHoldingsStore's single holding, each cell as wide as its widest.
func holdingsAsOfLine(account, security, shares, price, pricedOn, currency, value, in string) string {
	return holdingsRowOf([8]int{9, 16, 6, 5, 10, 8, 8, 8}, account, security, shares, price, pricedOn, currency, value, in)
}

// seedSplitHoldingsStore builds the store under a temp HOME with one CAD holding whose shares change by
// trade and by a 2:1 split: 100 bought 2025-06-02, doubled 2025-09-15, 50 more bought 2026-02-10. Its
// prices are 10.00 on 2025-06-02, 12.00 on 2025-12-30 and 15.00 on 2026-01-05.
func seedSplitHoldingsStore(t *testing.T) {
	t.Helper()
	home := newHome(t)
	day := func(year int, month time.Month, d int) time.Time {
		return time.Date(year, month, d, 0, 0, 0, 0, time.UTC)
	}
	security := "sec-acme"
	txn := func(id string, sourceID int64, date time.Time, action string) store.InvestmentTransaction {
		return store.InvestmentTransaction{
			ID: id, SourceID: sourceID, AccountID: "acct-cad", SecurityID: &security, Date: date,
			Action: action, Currency: "CAD",
		}
	}
	firstBuy := txn("inv-buy-1", 1, day(2025, time.June, 2), store.ActionBuy)
	firstBuy.Shares, firstBuy.Amount = new(int64(100_000_000)), -100_000
	split := txn("inv-split", 2, day(2025, time.September, 15), store.ActionSplit)
	split.SplitNewShares, split.SplitOldShares = new(int64(2_000_000)), new(int64(1_000_000))
	secondBuy := txn("inv-buy-2", 3, day(2026, time.February, 10), store.ActionBuy)
	secondBuy.Shares, secondBuy.Amount = new(int64(50_000_000)), -75_000
	rows := spendRows([]store.Account{brokerageAccount("acct-cad", 1, "CAD")})
	rows.Securities = []store.Security{
		{ID: security, SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")},
	}
	rows.InvestmentTransactions = []store.InvestmentTransaction{firstBuy, split, secondBuy}
	rows.Prices = []store.Price{
		{SecurityID: security, SourceID: 1, Date: day(2025, time.June, 2), Price: 10_000_000},
		{SecurityID: security, SourceID: 2, Date: day(2025, time.December, 30), Price: 12_000_000},
		{SecurityID: security, SourceID: 3, Date: day(2026, time.January, 5), Price: 15_000_000},
	}
	replaceStoreWithRates(t, home, rows, usdRate(day(2025, time.June, 2), 1_360_000))
}

func Test_run_holdings_as_of_a_year_lists_the_shares_after_a_split_before_it(t *testing.T) {
	seedSplitHoldingsStore(t)

	stdout, stderr := mustRunHoldings(t, "--as-of", "2025")

	assert.Empty(t, stderr)
	assert.Equal(t, "Holdings on 2025-12-31 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsAsOfLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsAsOfLine("Brokerage", "Acme Corp (ACME)", "200", "12.00", "2025-12-30", "CAD", "2,400.00", "2,400.00")+
		holdingsAsOfLine("Total", "", "", "", "", "", "", "2,400.00"),
		stdout)
}

func Test_run_holdings_as_of_forms_pick_the_shares_of_their_day(t *testing.T) {
	tests := []struct {
		name     string
		asOf     string
		caption  string
		shares   string
		price    string
		pricedOn string
		value    string
	}{
		{"the day before the split", "2025-09-14", "2025-09-14", "100", "10.00", "2025-06-02", "1,000.00"},
		{"the split day", "2025-09-15", "2025-09-15", "200", "10.00", "2025-06-02", "2,000.00"},
		{"a month is its last day", "2025-09", "2025-09-30", "200", "10.00", "2025-06-02", "2,000.00"},
		{"a year is its last day", "2025", "2025-12-31", "200", "12.00", "2025-12-30", "2,400.00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seedSplitHoldingsStore(t)

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"holdings", "--as-of", tt.asOf}, holdingsClock())

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, "Holdings on "+tt.caption+" in all accounts, amounts in CAD; cash not included\n\n"+
				holdingsAsOfLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
				holdingsAsOfLine("Brokerage", "Acme Corp (ACME)", tt.shares, tt.price, tt.pricedOn, "CAD", tt.value, tt.value)+
				holdingsAsOfLine("Total", "", "", "", "", "", "", tt.value),
				stdout.String())
		})
	}
}

func Test_run_holdings_as_of_the_current_year_or_month_or_today_is_today(t *testing.T) {
	for _, asOf := range []string{"2026", "2026-03", "2026-03-12"} {
		t.Run(asOf, func(t *testing.T) {
			seedSplitHoldingsStore(t)

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"holdings", "--as-of", asOf}, holdingsClock())

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Empty(t, stderr.String())
			assert.Equal(t, "Holdings on 2026-03-12 in all accounts, amounts in CAD; cash not included\n\n"+
				holdingsAsOfLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
				holdingsAsOfLine("Brokerage", "Acme Corp (ACME)", "250", "15.00", "2026-01-05", "CAD", "3,750.00", "3,750.00")+
				holdingsAsOfLine("Total", "", "", "", "", "", "", "3,750.00"),
				stdout.String())
		})
	}
}

func Test_run_holdings_refuses_a_bad_as_of_before_looking_for_a_store(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"holdings", "--as-of", "2024-13"}, holdingsClock())

	assert.Equal(t, 2, exitCode)
	assert.Equal(t, `quarry: --as-of "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`+"\n", stderr.String())
	assert.Empty(t, stdout.String())
}

func Test_run_holdings_refuses_a_date_it_cannot_use(t *testing.T) {
	tests := []struct {
		name   string
		asOf   string
		stderr string
	}{
		{
			"not a date", "2024-13",
			`quarry: --as-of "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD` + "\n",
		},
		{
			"a day after today", "2099-01-01",
			"quarry: --as-of 2099-01-01 is after today; holdings are valued up to today only, so pass an earlier --as-of\n",
		},
		{
			"a year after today, quoted as typed", "2099",
			"quarry: --as-of 2099 is after today; holdings are valued up to today only, so pass an earlier --as-of\n",
		},
		{
			"a month after today, quoted as typed", "2026-11",
			"quarry: --as-of 2026-11 is after today; holdings are valued up to today only, so pass an earlier --as-of\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seedHoldingsStore(t)

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"holdings", "--as-of", tt.asOf}, holdingsClock())

			assert.Equal(t, 2, exitCode)
			assert.Equal(t, tt.stderr, stderr.String())
			assert.Empty(t, stdout.String())
		})
	}
}

const (
	emptyCaption        = "Holdings on 2026-03-01 in all accounts, amounts in CAD; cash not included\n\n"
	emptyHeader         = "Account  Security  Shares  Price  Priced on  Currency  Value  In CAD\n"
	spanWarning         = "quarry: warning: no holdings on 2026-03-01; the store's investment transactions run 2026-03-02 to 2026-03-05\n"
	noDataWarning       = "quarry: warning: no holdings on 2026-03-01; the store has no investment transactions\n"
	namedNothingWarning = "quarry: warning: no holdings on 2026-03-12 in the named accounts; they have no investment transactions\n"
)

// holdingsRowsFromDayFive is holdingsRows with Maple's buy on day 5, so the store's transactions run day 2 to day 5.
func holdingsRowsFromDayFive() store.Rows {
	rows := holdingsRows()
	rows.InvestmentTransactions[2].Date = holdingsDay(5)
	return rows
}

// mustRunHoldings runs holdings with args at holdingsClock, which must exit 0, and returns its stdout and stderr.
func mustRunHoldings(t *testing.T, args ...string) (string, string) {
	t.Helper()

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), append([]string{"holdings"}, args...), holdingsClock())

	require.Equal(t, 0, exitCode, stderr.String())
	return stdout.String(), stderr.String()
}

// runHoldingsOn stores rows and runs holdings with args at holdingsClock.
func runHoldingsOn(t *testing.T, rows store.Rows, args ...string) (int, string, string) {
	t.Helper()
	home := newHome(t)
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), append([]string{"holdings"}, args...), holdingsClock())

	return exitCode, stdout.String(), stderr.String()
}

func Test_run_holdings_before_the_first_investment_transaction_warns_where_they_start_and_prints_no_total(t *testing.T) {
	home := newHome(t)
	rows := holdingsRows()
	rows.InvestmentTransactions[2].Date = holdingsDay(5)
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))

	stdout, stderr := mustRunHoldings(t, "--as-of", "2026-03-01")

	assert.Equal(t, "Holdings on 2026-03-01 in all accounts, amounts in CAD; cash not included\n\n"+
		"Account  Security  Shares  Price  Priced on  Currency  Value  In CAD\n", stdout)
	assert.Equal(t, "quarry: warning: no holdings on 2026-03-01; the store's investment transactions run 2026-03-02 to 2026-03-05\n",
		stderr)
}

func Test_run_holdings_of_a_store_with_no_investment_transactions_warns_there_are_none(t *testing.T) {
	rows := holdingsRows()
	rows.InvestmentTransactions = nil

	exitCode, stdout, stderr := runHoldingsOn(t, rows, "--as-of", "2026-03-01")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, emptyCaption+emptyHeader, stdout)
	assert.Equal(t, noDataWarning, stderr)
}

func Test_run_holdings_of_a_named_account_warns_where_only_its_own_transactions_run(t *testing.T) {
	exitCode, stdout, stderr := runHoldingsOn(t, holdingsRowsFromDayFive(), "--account", "Brokerage", "--as-of", "2026-03-01")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "Holdings on 2026-03-01 in Brokerage, amounts in CAD; cash not included\n\n"+emptyHeader, stdout)
	assert.Equal(t, "quarry: warning: no holdings on 2026-03-01 in the named accounts; "+
		"their investment transactions run 2026-03-02 to 2026-03-02\n", stderr)
}

func Test_run_holdings_native_before_the_first_investment_transaction_has_no_in_column_and_the_same_warning(t *testing.T) {
	exitCode, stdout, stderr := runHoldingsOn(t, holdingsRowsFromDayFive(), "--as-of", "2026-03-01", "--currency", "native")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "Holdings on 2026-03-01 in all accounts; cash not included\n\n"+
		"Account  Security  Shares  Price  Priced on  Currency  Value\n", stdout)
	assert.Equal(t, spanWarning, stderr)
}

func Test_run_holdings_json_before_the_first_investment_transaction_has_empty_lists_and_the_warning_stderr_prints(t *testing.T) {
	exitCode, stdout, stderr := runHoldingsOn(t, holdingsRowsFromDayFive(), "--as-of", "2026-03-01", "--json")

	require.Equal(t, 0, exitCode, stderr)
	var doc struct {
		Holdings      []any    `json:"holdings"`
		Totals        []any    `json:"totals"`
		AccountFilter []any    `json:"account_filter"`
		Warnings      []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	assert.Equal(t, []any{}, doc.Holdings)
	assert.Equal(t, []any{}, doc.Totals)
	assert.Equal(t, []any{}, doc.AccountFilter)
	assert.Equal(t, []string{"no holdings on 2026-03-01; the store's investment transactions run 2026-03-02 to 2026-03-05"}, doc.Warnings)
	assert.Equal(t, spanWarning, stderr)
}

const holdingsNoPriceLine = "1 holding has no price on or before 2026-03-12, so it has no value and is left out of the total; " +
	"enter a price for it in Quicken, then run quarry sync"

// holdingsNoPriceTableLine is one table line wide enough for the "no price" cell.
func holdingsNoPriceTableLine(account, security, shares, price, pricedOn, currency, value, in string) string {
	return holdingsRowOf([8]int{17, 26, 6, 8, 10, 8, 9, 9}, account, security, shares, price, pricedOn, currency, value, in)
}

// seedHoldingsStoreWithUnpricedHolding is seedHoldingsStore plus 40 shares of Bare Fund in the brokerage,
// whose only price is dated the day after the test clock's day.
func seedHoldingsStoreWithUnpricedHolding(t *testing.T) {
	t.Helper()
	home := newHome(t)
	rows := holdingsRows()
	rows.Securities = append(rows.Securities,
		store.Security{ID: "sec-bare", SourceID: 4, Name: "Bare Fund", Ticker: new("BARE"), Currency: new("CAD")})
	rows.InvestmentTransactions = append(rows.InvestmentTransactions,
		holdingsBuy("inv-bare", 4, "acct-cad", "sec-bare", "CAD", 40_000_000))
	rows.Prices = append(rows.Prices,
		store.Price{SecurityID: "sec-bare", SourceID: 4, Date: holdingsDay(13), Price: 7_000_000})
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
}

func Test_run_holdings_lists_a_holding_with_no_price_without_value(t *testing.T) {
	seedHoldingsStoreWithUnpricedHolding(t)

	stdout, stderr := mustRunHoldings(t)

	assert.Equal(t, "quarry: warning: "+holdingsNoPriceLine+"\n", stderr)
	assert.Equal(t, "Holdings on 2026-03-12 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsNoPriceTableLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsNoPriceTableLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsNoPriceTableLine("Brokerage", "Bare Fund (BARE)", "40", "no price", "", "CAD", "", "")+
		holdingsNoPriceTableLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35", "33,536.72")+
		holdingsNoPriceTableLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00", "50.00")+
		holdingsNoPriceTableLine("Total", "", "", "", "", "", "", "71,290.72"),
		stdout)
}

// holdingsJSONRow is the fields of one holdings entry the no-price tests read back.
type holdingsJSONRow struct {
	Security       *string `json:"security"`
	Shares         string  `json:"shares"`
	Price          *string `json:"price"`
	PriceDate      *string `json:"price_date"`
	Currency       *string `json:"currency"`
	Value          *string `json:"value"`
	ConvertedValue *string `json:"converted_value"`
}

type holdingsJSONDoc struct {
	Holdings []holdingsJSONRow `json:"holdings"`
	Totals   []struct {
		Currency string `json:"currency"`
		Value    string `json:"value"`
	} `json:"totals"`
	AccountFilter json.RawMessage `json:"account_filter"`
	Warnings      []string        `json:"warnings"`
}

func Test_run_holdings_json_lists_the_unpriced_holding_with_nulls_and_a_warning(t *testing.T) {
	seedHoldingsStoreWithUnpricedHolding(t)

	stdout, _ := mustRunHoldings(t, "--json")

	var doc holdingsJSONDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	require.Len(t, doc.Holdings, 4)
	assert.Equal(t, holdingsJSONRow{
		Security: new("Acme Corp"), Shares: "1200.000000", Price: new("31.420000"), PriceDate: new("2026-03-09"),
		Currency: new("CAD"), Value: new("37704.00"), ConvertedValue: new("37704.00"),
	}, doc.Holdings[0])
	assert.Equal(t, holdingsJSONRow{
		Security: new("Bare Fund"), Shares: "40.000000", Currency: new("CAD"),
	}, doc.Holdings[1])
	assert.Equal(t, "71290.72", doc.Totals[0].Value)
	assert.JSONEq(t, "[]", string(doc.AccountFilter))
	assert.Equal(t, []string{holdingsNoPriceLine}, doc.Warnings)
}

const (
	holdingsBeforeFirstRateLine = "1 holding valued on 2026-03-09, before 2026-03-10, the first exchange rate in the store, " +
		"is not converted to CAD and is totalled in USD"
	holdingsBeforeFirstRateUSDLine = "2 holdings valued on 2026-03-09, before 2026-03-10, the first exchange rate in the store, " +
		"are not converted to USD and are totalled in CAD"
	holdingsNoRatesWarningLine = "the store has no exchange rates, so values are listed in each security's own currency; " +
		"run quarry sync to fetch them"
)

func Test_run_holdings_before_the_first_rate_shows_no_rate_and_totals_usd_separately(t *testing.T) {
	seedHoldingsStore(t)

	stdout, stderr := mustRunHoldings(t, "--as-of", "2026-03-09")

	assert.Equal(t, "quarry: warning: "+holdingsBeforeFirstRateLine+"\n", stderr)
	assert.Equal(t, "Holdings on 2026-03-09 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35", "no rate")+
		holdingsLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00", "50.00")+
		holdingsLine("Total", "", "", "", "", "", "", "37,754.00")+
		holdingsLine("Total", "", "", "", "", "USD", "24,659.35", ""),
		stdout)
}

func Test_run_holdings_json_before_the_first_rate_lists_the_converted_total_then_the_usd_one_and_the_stderr_warning(t *testing.T) {
	seedHoldingsStore(t)

	stdout, stderr := mustRunHoldings(t, "--json", "--as-of", "2026-03-09")

	var doc holdingsJSONDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	require.Len(t, doc.Holdings, 3)
	assert.Equal(t, "24659.35", *doc.Holdings[1].Value)
	assert.Nil(t, doc.Holdings[1].ConvertedValue)
	assert.Equal(t, "37704.00", *doc.Holdings[0].ConvertedValue)
	require.Len(t, doc.Totals, 2)
	assert.Equal(t, "CAD", doc.Totals[0].Currency)
	assert.Equal(t, "37754.00", doc.Totals[0].Value)
	assert.Equal(t, "USD", doc.Totals[1].Currency)
	assert.Equal(t, "24659.35", doc.Totals[1].Value)
	assert.Equal(t, []string{holdingsBeforeFirstRateLine}, doc.Warnings)
	assert.Equal(t, "quarry: warning: "+doc.Warnings[0]+"\n", stderr)
}

func Test_run_holdings_in_usd_before_the_first_rate_shows_no_rate_for_the_cad_rows_and_totals_cad_separately(t *testing.T) {
	seedHoldingsStore(t)

	stdout, stderr := mustRunHoldings(t, "--currency", "USD", "--as-of", "2026-03-09")

	assert.Equal(t, "quarry: warning: "+holdingsBeforeFirstRateUSDLine+"\n", stderr)
	assert.Equal(t, "Holdings on 2026-03-09 in all accounts, amounts in USD; cash not included\n\n"+
		holdingsLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In USD")+
		holdingsLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "no rate")+
		holdingsLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35", "24,659.35")+
		holdingsLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00", "no rate")+
		holdingsLine("Total", "", "", "", "", "", "", "24,659.35")+
		holdingsLine("Total", "", "", "", "", "CAD", "37,754.00", ""),
		stdout)
}

func Test_run_holdings_native_before_the_first_rate_has_no_no_rate_cell_and_no_warning(t *testing.T) {
	seedHoldingsStore(t)

	stdout, stderr := mustRunHoldings(t, "--currency", "native", "--as-of", "2026-03-09")

	assert.Empty(t, stderr)
	assert.Equal(t, "Holdings on 2026-03-09 in all accounts; cash not included\n\n"+
		holdingsNativeLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value")+
		holdingsNativeLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00")+
		holdingsNativeLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35")+
		holdingsNativeLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00")+
		holdingsNativeLine("Total", "", "", "", "", "CAD", "37,754.00")+
		holdingsNativeLine("Total", "", "", "", "", "USD", "24,659.35"),
		stdout)
}

func Test_run_holdings_in_a_store_with_no_rates_says_so_and_shows_no_rate_for_the_usd_row(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, holdingsRows())

	stdout, stderr := mustRunHoldings(t)

	assert.Equal(t, "quarry: warning: "+holdingsNoRatesWarningLine+"\n", stderr)
	assert.Equal(t, "Holdings on 2026-03-12 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35", "no rate")+
		holdingsLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00", "50.00")+
		holdingsLine("Total", "", "", "", "", "", "", "37,754.00")+
		holdingsLine("Total", "", "", "", "", "USD", "24,659.35", ""),
		stdout)
}

func Test_run_holdings_on_a_day_inside_a_rate_gap_converts_at_the_earlier_rate_and_is_silent(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, holdingsRows(), usdRate(holdingsDay(10), 1_360_000), usdRate(holdingsDay(12), 1_400_000))

	stdout, stderr := mustRunHoldings(t, "--as-of", "2026-03-11")

	assert.Empty(t, stderr)
	assert.Equal(t, "Holdings on 2026-03-11 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35", "33,536.72")+
		holdingsLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00", "50.00")+
		holdingsLine("Total", "", "", "", "", "", "", "71,290.72"),
		stdout)
}

func Test_run_holdings_of_cad_holdings_only_before_the_first_rate_in_cad_needs_no_rate(t *testing.T) {
	home := newHome(t)
	rows := holdingsRows()
	rows.InvestmentTransactions, rows.Prices = rows.InvestmentTransactions[:1], rows.Prices[:1]
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))

	stdout, stderr := mustRunHoldings(t, "--as-of", "2026-03-09")

	assert.Empty(t, stderr)
	assert.NotContains(t, stdout, "no rate")
	assert.Regexp(t, `(?m)^Total +37,704\.00$`, stdout)
}

const (
	holdingsNoCurrencyLine = `"Mystery Fund" has no currency in Quicken, so quarry leaves its value out of the total; ` +
		`set its currency in Quicken, then run quarry sync`
	holdingsOtherCurrencyLine = `"Euro Fund" is priced in EUR, which quarry does not convert, so its value is left out of the total`
)

// holdingsNotConvertedLine is one table line wide enough for the "not converted" cell.
func holdingsNotConvertedLine(account, security, shares, price, pricedOn, currency, value, in string) string {
	return holdingsRowOf([8]int{17, 26, 6, 6, 10, 8, 9, 13}, account, security, shares, price, pricedOn, currency, value, in)
}

// seedHoldingsStoreWithUnconvertible is seedHoldingsStore plus, in the brokerage, 5 shares of Mystery Fund
// (no currency) at 10.00 and 20 shares of Euro Fund (EUR) at 12.50, both priced on day 9.
func seedHoldingsStoreWithUnconvertible(t *testing.T) {
	t.Helper()
	home := newHome(t)
	rows := holdingsRows()
	rows.Securities = append(rows.Securities,
		store.Security{ID: "sec-null", SourceID: 4, Name: "Mystery Fund", Ticker: new("MYST")},
		store.Security{ID: "sec-eur", SourceID: 5, Name: "Euro Fund", Ticker: new("EURO"), Currency: new("EUR")})
	rows.InvestmentTransactions = append(rows.InvestmentTransactions,
		holdingsBuy("inv-null", 4, "acct-cad", "sec-null", "CAD", 5_000_000),
		holdingsBuy("inv-eur", 5, "acct-cad", "sec-eur", "CAD", 20_000_000))
	rows.Prices = append(rows.Prices,
		store.Price{SecurityID: "sec-null", SourceID: 4, Date: holdingsDay(9), Price: 10_000_000},
		store.Price{SecurityID: "sec-eur", SourceID: 5, Date: holdingsDay(9), Price: 12_500_000})
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
}

func Test_run_holdings_leaves_a_security_it_cannot_convert_out_of_the_total(t *testing.T) {
	seedHoldingsStoreWithUnconvertible(t)

	stdout, stderr := mustRunHoldings(t)

	assert.Equal(t, "quarry: warning: "+holdingsNoCurrencyLine+"\nquarry: warning: "+holdingsOtherCurrencyLine+"\n", stderr)
	assert.Equal(t, "Holdings on 2026-03-12 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsNotConvertedLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsNotConvertedLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsNotConvertedLine("Brokerage", "Euro Fund (EURO)", "20", "12.50", "2026-03-09", "EUR", "250.00", "not converted")+
		holdingsNotConvertedLine("Brokerage", "Mystery Fund (MYST)", "5", "10.00", "2026-03-09", "none", "50.00", "not converted")+
		holdingsNotConvertedLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35", "33,536.72")+
		holdingsNotConvertedLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00", "50.00")+
		holdingsNotConvertedLine("Total", "", "", "", "", "", "", "71,290.72"),
		stdout)
}

func Test_run_holdings_json_lists_an_unconvertible_security_with_a_null_converted_value(t *testing.T) {
	seedHoldingsStoreWithUnconvertible(t)

	stdout, _ := mustRunHoldings(t, "--json")

	var doc holdingsJSONDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	require.Len(t, doc.Holdings, 5)
	assert.Equal(t, holdingsJSONRow{
		Security: new("Euro Fund"), Shares: "20.000000", Price: new("12.500000"), PriceDate: new("2026-03-09"),
		Currency: new("EUR"), Value: new("250.00"),
	}, doc.Holdings[1])
	assert.Equal(t, holdingsJSONRow{
		Security: new("Mystery Fund"), Shares: "5.000000", Price: new("10.000000"), PriceDate: new("2026-03-09"),
		Value: new("50.00"),
	}, doc.Holdings[2])
	require.Len(t, doc.Totals, 1)
	assert.Equal(t, "CAD", doc.Totals[0].Currency)
	assert.Equal(t, "71290.72", doc.Totals[0].Value)
	assert.Equal(t, []string{holdingsNoCurrencyLine, holdingsOtherCurrencyLine}, doc.Warnings)
}

func Test_run_holdings_native_totals_a_security_priced_in_another_currency_and_never_one_with_none(t *testing.T) {
	seedHoldingsStoreWithUnconvertible(t)

	stdout, stderr := mustRunHoldings(t, "--currency", "native")

	assert.Equal(t, "quarry: warning: "+holdingsNoCurrencyLine+"\n", stderr)
	assert.Equal(t, "Holdings on 2026-03-12 in all accounts; cash not included\n\n"+
		holdingsNativeLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value")+
		holdingsNativeLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00")+
		holdingsNativeLine("Brokerage", "Euro Fund (EURO)", "20", "12.50", "2026-03-09", "EUR", "250.00")+
		holdingsNativeLine("Brokerage", "Mystery Fund (MYST)", "5", "10.00", "2026-03-09", "none", "50.00")+
		holdingsNativeLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35")+
		holdingsNativeLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00")+
		holdingsNativeLine("Total", "", "", "", "", "CAD", "37,754.00")+
		holdingsNativeLine("Total", "", "", "", "", "USD", "24,659.35")+
		holdingsNativeLine("Total", "", "", "", "", "EUR", "250.00"),
		stdout)
}

func Test_run_holdings_json_carries_account_and_security_names_as_stored(t *testing.T) {
	home := newHome(t)
	rows := holdingsRows()
	rows.Accounts[0].Name = "Broker\nage"
	rows.Securities[0].Name = "Acme\tCorp\r\n"
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))

	stdout, _ := mustRunHoldings(t, "--json", "--account", "acct-cad")

	var doc struct {
		Holdings []struct {
			Account  string `json:"account"`
			Security string `json:"security"`
		} `json:"holdings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	require.Len(t, doc.Holdings, 1)
	assert.Equal(t, "Broker\nage", doc.Holdings[0].Account)
	assert.Equal(t, "Acme\tCorp\r\n", doc.Holdings[0].Security)
}

// seedLowerCaseAccountHoldingsStore is seedHoldingsStore with its first account named in lower case, so
// plain byte order would put IRA and Old RRSP before it.
func seedLowerCaseAccountHoldingsStore(t *testing.T) {
	t.Helper()
	home := newHome(t)
	rows := holdingsRows()
	rows.Accounts[0].Name = "brokerage"
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
}

func Test_run_holdings_sorts_accounts_ignoring_case_in_the_table(t *testing.T) {
	seedLowerCaseAccountHoldingsStore(t)

	stdout, _ := mustRunHoldings(t)

	assert.Equal(t, "Holdings on 2026-03-12 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsLine("brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35", "33,536.72")+
		holdingsLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00", "50.00")+
		holdingsLine("Total", "", "", "", "", "", "", "71,290.72"),
		stdout)
}

func Test_run_holdings_json_sorts_accounts_ignoring_case(t *testing.T) {
	seedLowerCaseAccountHoldingsStore(t)

	stdout, _ := mustRunHoldings(t, "--json")

	var doc struct {
		Holdings []struct {
			Account string `json:"account"`
		} `json:"holdings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	accounts := make([]string, len(doc.Holdings))
	for i, h := range doc.Holdings {
		accounts[i] = h.Account
	}
	assert.Equal(t, []string{"brokerage", "IRA", "Old RRSP"}, accounts)
}

// Needs run(): the help text is assembled with the flags, not by calling a command directly.
// The expected blocks are hand copies at the help's wrap width, so a re-wrap fails here.
func Test_run_accounts_and_sql_help_carry_the_holdings_copy(t *testing.T) {
	t.Setenv("HOME", "")
	var accountsOut, accountsErr, sqlOut, sqlErr bytes.Buffer

	accountsCode := run(context.Background(), []string{"accounts", "--help"}, &accountsOut, &accountsErr)
	sqlCode := run(context.Background(), []string{"sql", "--help"}, &sqlOut, &sqlErr)

	require.Equal(t, 0, accountsCode)
	require.Equal(t, 0, sqlCode)
	assert.Contains(t, accountsOut.String(), `Brokerage and retirement accounts' balance is the cash in them plus the
value of their holdings today, each at the latest price Quicken recorded
(quarry holdings lists them).`)
	assert.Contains(t, sqlOut.String(), `records none).
holding_shares holds each account's count of each security, one row per span
of days it is unchanged and not zero (from_date through to_date, NULL while
still held), splits applied; these are the counts quarry sync checks against
Quicken. v_holdings has one row per holding per day held, through today:
price is the latest on or before date and price_date its day (NULL when
none), value is shares times price rounded to the cent, value_cad and
value_usd convert it at the rate for date, as quarry holdings does; filter
it by date. Neither includes cash in investment accounts.`)
	assert.Contains(t, sqlOut.String(), `Neither includes cash in investment accounts. action is one of
add_shares, buy, capital_gain_long, capital_gain_short, dividend, interest,
margin_interest, misc_expense, misc_income, reinvest_dividend,
remove_shares, sell, split.`)
}

// Needs run(): the help text is assembled with the flags, not by calling a command directly.
func Test_run_holdings_help_ends_by_pointing_at_accounts_for_the_balance_with_cash(t *testing.T) {
	t.Setenv("HOME", "")

	exitCode, stdout, _ := runCapture(context.Background(), []string{"holdings", "--help"})

	require.Equal(t, 0, exitCode)
	assert.Contains(t, stdout.String(), `The total is the value of the securities only, without the cash held in
investment accounts; quarry accounts shows each account's balance, cash
included.`)
}

func Test_run_sql_values_each_holding_on_a_date_from_v_holdings(t *testing.T) {
	home := newHome(t)
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

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sql", "--csv", query})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"account_id,security_id,shares,price,price_date,value,value_cad,value_usd,usd_cad\n"+
		"acct-cad,sec-cad,3.500000,12.345678,2026-03-11,43.21,43.21,33.24,1.300000\n"+
		"acct-usd,sec-usd,2.000000,20.123456,2026-03-12,40.25,52.33,40.25,1.300000\n",
		stdout.String())
}

func Test_run_holdings_lists_a_holding_priced_at_zero_with_a_value_of_zero_and_no_warning(t *testing.T) {
	home := newHome(t)
	rows := holdingsRows()
	rows.Securities = append(rows.Securities,
		store.Security{ID: "sec-zero", SourceID: 4, Name: "Zero Fund", Ticker: new("ZERO"), Currency: new("CAD")})
	rows.InvestmentTransactions = append(rows.InvestmentTransactions,
		holdingsBuy("inv-zero", 4, "acct-cad", "sec-zero", "CAD", 40_000_000))
	rows.Prices = append(rows.Prices, store.Price{SecurityID: "sec-zero", SourceID: 4, Date: holdingsDay(9), Price: 0})
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))

	stdout, stderr := mustRunHoldings(t)

	assert.Empty(t, stderr)
	assert.Equal(t, "Holdings on 2026-03-12 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsLine("Brokerage", "Zero Fund (ZERO)", "40", "0.00", "2026-03-09", "CAD", "0.00", "0.00")+
		holdingsLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35", "33,536.72")+
		holdingsLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00", "50.00")+
		holdingsLine("Total", "", "", "", "", "", "", "71,290.72"),
		stdout)
}
