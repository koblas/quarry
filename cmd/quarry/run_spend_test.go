// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_spend_shows_this_years_spending_by_category_in_each_currency(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, spendRows(
		[]store.Account{
			chequingAccount("acct-cad", 1),
			usdChequingAccount("acct-usd", 2),
		},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12345},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 9, 29), cents: -1000},
		spendSplit{id: "s03", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 5, 2), cents: -120450},
		spendSplit{id: "s04", account: "acct-cad", currency: "CAD", day: day(2026, 6, 1), cents: -4208},
		spendSplit{id: "s05", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 4, 1), cents: -31210},
		spendSplit{id: "s06", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2025, 12, 31), cents: -99900},
		spendSplit{id: "s07", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 9, 30), cents: -77700},
	))

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"spend"}, time.Date(2026, 9, 29, 22, 0, 0, 0, utcMinus5))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: "+noRatesLine+"\n", stderr.String())
	const row = "%-15s  %-8s  %8s\n"
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
		fmt.Sprintf(row, "Category", "Currency", "Spent")+
		fmt.Sprintf(row, "(uncategorized)", "CAD", "42.08")+
		fmt.Sprintf(row, "Auto:Fuel", "CAD", "1,204.50")+
		fmt.Sprintf(row, "Food:Groceries", "CAD", "133.45")+
		fmt.Sprintf(row, "Food:Groceries", "USD", "312.10")+
		fmt.Sprintf(row, "Total", "CAD", "1,380.03")+
		fmt.Sprintf(row, "Total", "USD", "312.10"),
		stdout.String())
}

func Test_run_spend_leaves_out_accounts_quicken_does_not_use_in_reports(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, spendRows(
		[]store.Account{
			chequingAccount("acct-in", 1),
			{ID: "acct-out", SourceID: 2, Name: "Old Card", Type: "credit_card", Currency: "CAD", Active: true, NotInReports: true},
		},
		spendSplit{id: "s01", account: "acct-in", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -2500},
		spendSplit{id: "s02", account: "acct-out", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 11), cents: -900},
	))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"spend"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	const row = "%-14s  %-8s  %5s\n"
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
		fmt.Sprintf(row, "Category", "Currency", "Spent")+
		fmt.Sprintf(row, "Food:Groceries", "CAD", "25.00")+
		fmt.Sprintf(row, "Total", "CAD", "25.00"),
		stdout.String())
}

func Test_run_spend_counts_only_the_accounts_it_is_given(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, spendRows(
		[]store.Account{
			chequingAccount("acct-chq", 1),
			{ID: "acct-visa", SourceID: 2, Name: "Visa Infinite", Type: "credit_card", Currency: "CAD", Closed: true},
			{ID: "acct-sav", SourceID: 3, Name: "Savings", Type: "savings", Currency: "CAD", Active: true},
		},
		spendSplit{id: "s01", account: "acct-chq", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -1000},
		spendSplit{id: "s02", account: "acct-visa", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 11), cents: -500},
		spendSplit{id: "s03", account: "acct-sav", category: "cat-fuel", currency: "CAD", day: day(2026, 3, 12), cents: -300},
	))

	exitCode, stdout, stderr := runSpendCapture(context.Background(),
		[]string{"spend", "--account", "chequing", "--account", "acct-visa", "--account", "Chequing"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	const row = "%-14s  %-8s  %5s\n"
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in Chequing, Visa Infinite, amounts in CAD\n\n"+
		fmt.Sprintf(row, "Category", "Currency", "Spent")+
		fmt.Sprintf(row, "Food:Groceries", "CAD", "15.00")+
		fmt.Sprintf(row, "Total", "CAD", "15.00"),
		stdout.String())
}

func Test_run_spend_warns_that_a_named_account_is_left_out_of_reports(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, spendRows(
		[]store.Account{
			{ID: "acct-old", SourceID: 1, Name: "Old Card", Type: "credit_card", Currency: "CAD", Active: true, NotInReports: true},
		},
		spendSplit{id: "s01", account: "acct-old", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -900},
	))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"spend", "--account", "Old Card"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: account \"Old Card\" is not used in reports in Quicken, so spend leaves it out; "+
		"to include it, turn on reports for it in Quicken's account settings, then run quarry sync\n",
		stderr.String())
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in Old Card, amounts in CAD\n\nCategory  Currency  Spent\n", stdout.String())
}

func Test_run_spend_warns_that_a_named_linked_tracking_account_is_left_out(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, spendRows(
		[]store.Account{
			{ID: "acct-401k", SourceID: 1, Name: "Netskope 401(k)", Type: "retirement", Currency: "USD", Active: true, NotInReports: true, LinkedTracking: true},
		},
		spendSplit{id: "s01", account: "acct-401k", category: "cat-groceries", currency: "USD", day: day(2026, 3, 10), cents: -900},
	))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"spend", "--account", "Netskope 401(k)"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: account \"Netskope 401(k)\" uses linked account tracking in Quicken, "+
		"so spend leaves it out, as Quicken's reports do\n",
		stderr.String())
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in Netskope 401(k), amounts in CAD\n\nCategory  Currency  Spent\n", stdout.String())
}

func Test_run_spend_ranges_a_linked_and_a_reported_named_account_over_the_reported_one(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, spendRows(
		[]store.Account{
			{ID: "acct-401k", SourceID: 1, Name: "Netskope 401(k)", Type: "retirement", Currency: "USD", Active: true, LinkedTracking: true},
			chequingAccount("acct-chq", 2),
		},
		spendSplit{id: "s01", account: "acct-401k", category: "cat-groceries", currency: "USD", day: day(2003, 1, 4), cents: -900},
		spendSplit{id: "s02", account: "acct-chq", category: "cat-groceries", currency: "CAD", day: day(2019, 3, 2), cents: -100},
		spendSplit{id: "s03", account: "acct-chq", category: "cat-groceries", currency: "CAD", day: day(2019, 3, 5), cents: -100},
	))

	exitCode, _, stderr := runSpendCapture(context.Background(),
		[]string{"spend", "--account", "Netskope 401(k)", "--account", "Chequing"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: account \"Netskope 401(k)\" uses linked account tracking in Quicken, "+
		"so spend leaves it out, as Quicken's reports do\n"+
		"quarry: warning: no spending from 2026-01-01 to 2026-09-29 in the named accounts; "+
		"their transactions run 2019-03-02 to 2019-03-05\n", stderr.String())
}

func Test_run_spend_refuses_an_account_it_cannot_pick(t *testing.T) {
	cases := []struct {
		name string
		arg  string
		want string
	}{
		{
			name: "no account has the name",
			arg:  "Chequeing",
			want: "quarry: no account named \"Chequeing\"; run quarry accounts --all to list them\n",
		},
		{
			name: "the argument is empty",
			arg:  "",
			want: "quarry: no account named \"\"; run quarry accounts --all to list them\n",
		},
		{
			name: "two accounts share the name, listed by sorted id",
			arg:  "Visa",
			want: "quarry: 2 accounts are named \"Visa\"; pass one of their ids instead: acct-812, acct-977\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, spendRows([]store.Account{
				chequingAccount("acct-chq", 1),
				{ID: "acct-977", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
				{ID: "acct-812", SourceID: 3, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
			}))

			exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"spend", "--account", c.arg})

			assert.Equal(t, 1, exitCode)
			assert.Equal(t, c.want, stderr.String())
			assert.Empty(t, stdout.String())
		})
	}
}

func Test_run_spend_by_payee_groups_spending_by_payee_and_currency_biggest_first(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, spendRows(
		[]store.Account{
			chequingAccount("acct-cad", 1),
			usdChequingAccount("acct-usd", 2),
		},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", payee: "payee-costco", currency: "CAD", day: day(2026, 3, 10), cents: -30000},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-groceries", payee: "payee-bakery", currency: "CAD", day: day(2026, 3, 11), cents: -1000},
		spendSplit{id: "s03", account: "acct-cad", category: "cat-fuel", payee: "payee-bakery", currency: "CAD", day: day(2026, 5, 2), cents: -250},
		spendSplit{id: "s04", account: "acct-cad", currency: "CAD", day: day(2026, 6, 1), cents: -4208},
		spendSplit{id: "s05", account: "acct-usd", category: "cat-groceries", payee: "payee-costco", currency: "USD", day: day(2026, 4, 1), cents: -31210},
	))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"spend", "--by", "payee"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: "+noRatesLine+"\n", stderr.String())
	const row = "%-10s  %-8s  %6s\n"
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
		fmt.Sprintf(row, "Payee", "Currency", "Spent")+
		fmt.Sprintf(row, "Costco", "CAD", "300.00")+
		fmt.Sprintf(row, "(no payee)", "CAD", "42.08")+
		fmt.Sprintf(row, "Bakery", "CAD", "12.50")+
		fmt.Sprintf(row, "Costco", "USD", "312.10")+
		fmt.Sprintf(row, "Total", "CAD", "354.58")+
		fmt.Sprintf(row, "Total", "USD", "312.10"),
		stdout.String())
}

func Test_run_spend_by_tag_counts_a_two_tag_split_under_both_tags_once_in_the_total_and_warns(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, spendRows(
		[]store.Account{
			chequingAccount("acct-cad", 1),
			usdChequingAccount("acct-usd", 2),
		},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -10000, tags: []string{"tag-vacation", "tag-alpha"}},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 5, 2), cents: -2500, tags: []string{"tag-vacation"}},
		spendSplit{id: "s03", account: "acct-cad", currency: "CAD", day: day(2026, 6, 1), cents: -4208},
		spendSplit{id: "s04", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 4, 1), cents: -31210, tags: []string{"tag-alpha"}},
	))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"spend", "--by", "tag"})

	require.Equal(t, 0, exitCode, stderr.String())
	const row = "%-8s  %-8s  %6s\n"
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
		fmt.Sprintf(row, "Tag", "Currency", "Spent")+
		fmt.Sprintf(row, "(no tag)", "CAD", "42.08")+
		fmt.Sprintf(row, "alpha", "CAD", "100.00")+
		fmt.Sprintf(row, "alpha", "USD", "312.10")+
		fmt.Sprintf(row, "Vacation", "CAD", "125.00")+
		fmt.Sprintf(row, "Total", "CAD", "167.08")+
		fmt.Sprintf(row, "Total", "USD", "312.10"),
		stdout.String())
	assert.Equal(t, "quarry: warning: "+noRatesLine+"\n"+
		"quarry: warning: 1 split carries more than one tag, so the rows add up to more than the total\n",
		stderr.String())
}

func Test_run_spend_by_month_fills_empty_months_and_marks_a_cut_short_month_partial(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, spendRows(
		[]store.Account{
			chequingAccount("acct-cad", 1),
		},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 1, 10), cents: -9999},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 1, 20), cents: -5000},
		spendSplit{id: "s03", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 3, 10), cents: -12000},
	))

	exitCode, stdout, stderr := runSpendCapture(context.Background(),
		[]string{"spend", "--by", "month", "--since", "2026-01-15", "--until", "2026-03"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Spending 2026-01-15 to 2026-03-31 in all accounts, amounts in CAD\n\n"+
		"Month    Currency   Spent  Status\n"+
		"2026-01  CAD        50.00  partial\n"+
		"2026-02  CAD         0.00\n"+
		"2026-03  CAD       120.00\n"+
		"Total    CAD       170.00\n",
		stdout.String())
}

func Test_run_spend_says_when_the_period_holds_nothing(t *testing.T) {
	const emptyTable = "Spending 2026-01-01 to 2026-02-28 in all accounts, amounts in CAD\n\nCategory  Currency  Spent\n"
	cases := []struct {
		name       string
		splits     []spendSplit
		wantStderr string
	}{
		{
			name: "the store has transactions, none in the period",
			splits: []spendSplit{
				{id: "s01", account: "acct-chq", category: "cat-groceries", currency: "CAD", day: day(2003, 1, 4), cents: -1000},
				{id: "s02", account: "acct-chq", category: "cat-groceries", currency: "CAD", day: day(2025, 12, 31), cents: -500},
			},
			wantStderr: "quarry: warning: no spending from 2026-01-01 to 2026-02-28; " +
				"the store's transactions run 2003-01-04 to 2025-12-31\n",
		},
		{
			name:       "the store has no transactions",
			wantStderr: "quarry: warning: no spending from 2026-01-01 to 2026-02-28; the store has no transactions\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, spendRows([]store.Account{
				chequingAccount("acct-chq", 1),
			}, c.splits...))

			exitCode, stdout, stderr := runSpendCapture(context.Background(),
				[]string{"spend", "--since", "2026-01", "--until", "2026-02"})

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, emptyTable, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

// spendReport is the part of spend's --json document the currency tests read.
type spendReport struct {
	Currency string       `json:"currency"`
	Rows     []spendMoney `json:"rows"`
	Totals   []spendMoney `json:"totals"`
}

// spendMoney is one amount of a spend document: a row's category and a total alike.
type spendMoney struct {
	Category *string `json:"category"`
	Currency string  `json:"currency"`
	Spent    string  `json:"spent"`
}

// rateOnJan2 is 1.25 CAD per USD, dated before every split the fixtures below hold.
var rateOnJan2 = store.Rate{Date: day(2026, 1, 2), USDCAD: money.Rate(1_250_000), Series: "FXUSDCAD"}

// runSpendDoc runs spend with args plus --json, requires it to succeed, and decodes the part of its document that
// T names, returning that and stderr.
func runSpendDoc[T any](tb testing.TB, args ...string) (T, string) {
	tb.Helper()
	exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"spend", "--json"}, args...))
	require.Equal(tb, 0, exitCode, stderr.String())
	var doc T
	require.NoError(tb, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	return doc, stderr.String()
}

// runSpendJSON is runSpendDoc over the currency tests' spendReport.
func runSpendJSON(tb testing.TB, args ...string) (spendReport, string) {
	tb.Helper()
	return runSpendDoc[spendReport](tb, args...)
}

func Test_run_spend_converts_every_split_to_cad_by_default(t *testing.T) {
	// Two 0.10 USD splits at 1.25 make 0.26 rounded each, 0.25 summed first.
	home := newHome(t)
	replaceStoreWithRates(t, home, spendRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12345},
		spendSplit{id: "s02", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 4, 1), cents: -10},
		spendSplit{id: "s03", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 4, 2), cents: -10},
		spendSplit{id: "s04", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 5, 2), cents: -8000},
		spendSplit{id: "s05", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 5, 3), cents: -1000},
	), rateOnJan2)

	t.Run("text", func(t *testing.T) {
		exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"spend"})

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Empty(t, stderr.String())
		const row = "%-14s  %-8s  %6s\n"
		assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
			fmt.Sprintf(row, "Category", "Currency", "Spent")+
			fmt.Sprintf(row, "Auto:Fuel", "CAD", "110.00")+
			fmt.Sprintf(row, "Food:Groceries", "CAD", "123.71")+
			fmt.Sprintf(row, "Total", "CAD", "233.71"),
			stdout.String())
	})

	t.Run("json", func(t *testing.T) {
		doc, stderr := runSpendJSON(t)

		assert.Empty(t, stderr)
		assert.Equal(t, spendReport{
			Currency: "CAD",
			Rows: []spendMoney{
				{Category: new("Auto:Fuel"), Currency: "CAD", Spent: "110.00"},
				{Category: new("Food:Groceries"), Currency: "CAD", Spent: "123.71"},
			},
			Totals: []spendMoney{{Currency: "CAD", Spent: "233.71"}},
		}, doc)
	})
}

func Test_run_spend_takes_its_currency_from_the_config_unless_the_flag_names_one(t *testing.T) {
	const (
		unset     = ""
		malformed = "[snapshots\nkeep = 24\n"
		caption   = "Spending 2026-01-01 to 2026-09-29 in all accounts"
		row       = "%-14s  %-8s  %6s\n"
	)
	cadText := caption + ", amounts in CAD\n\n" +
		fmt.Sprintf(row, "Category", "Currency", "Spent") +
		fmt.Sprintf(row, "Food:Groceries", "CAD", "200.00") +
		fmt.Sprintf(row, "Total", "CAD", "200.00")
	usdText := caption + ", amounts in USD\n\n" +
		fmt.Sprintf(row, "Category", "Currency", "Spent") +
		fmt.Sprintf(row, "Food:Groceries", "USD", "160.00") +
		fmt.Sprintf(row, "Total", "USD", "160.00")
	nativeText := caption + "\n\n" +
		fmt.Sprintf(row, "Category", "Currency", "Spent") +
		fmt.Sprintf(row, "Food:Groceries", "CAD", "100.00") +
		fmt.Sprintf(row, "Food:Groceries", "USD", "80.00") +
		fmt.Sprintf(row, "Total", "CAD", "100.00") +
		fmt.Sprintf(row, "Total", "USD", "80.00")
	groceries := new("Food:Groceries")
	cadDoc := spendReport{
		Currency: "CAD",
		Rows:     []spendMoney{{Category: groceries, Currency: "CAD", Spent: "200.00"}},
		Totals:   []spendMoney{{Currency: "CAD", Spent: "200.00"}},
	}
	usdDoc := spendReport{
		Currency: "USD",
		Rows:     []spendMoney{{Category: groceries, Currency: "USD", Spent: "160.00"}},
		Totals:   []spendMoney{{Currency: "USD", Spent: "160.00"}},
	}
	nativeDoc := spendReport{
		Currency: "native",
		Rows: []spendMoney{
			{Category: groceries, Currency: "CAD", Spent: "100.00"},
			{Category: groceries, Currency: "USD", Spent: "80.00"},
		},
		Totals: []spendMoney{{Currency: "CAD", Spent: "100.00"}, {Currency: "USD", Spent: "80.00"}},
	}
	cases := []struct {
		name   string
		config string
		flag   []string
		text   string
		doc    spendReport
	}{
		{name: "unset gives CAD", config: unset, text: cadText, doc: cadDoc},
		{name: "USD in the config", config: "reporting.currency = \"USD\"\n", text: usdText, doc: usdDoc},
		{name: "native in the config", config: "reporting.currency = \"native\"\n", text: nativeText, doc: nativeDoc},
		{name: "lower case in the config", config: "reporting.currency = \"usd\"\n", text: usdText, doc: usdDoc},
		{name: "the flag beats the config", config: "reporting.currency = \"USD\"\n", flag: []string{"--currency", "CAD"}, text: cadText, doc: cadDoc},
		{name: "the flag excuses a malformed config", config: malformed, flag: []string{"--currency", "CAD"}, text: cadText, doc: cadDoc},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStoreWithRates(t, home, spendRows(
				[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
				spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -10000},
				spendSplit{id: "s02", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 3, 11), cents: -8000},
			), rateOnJan2)
			if c.config != unset {
				writeConfig(t, home, c.config)
			}

			t.Run("text", func(t *testing.T) {
				exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"spend"}, c.flag...))

				require.Equal(t, 0, exitCode, stderr.String())
				assert.Empty(t, stderr.String())
				assert.Equal(t, c.text, stdout.String())
			})

			t.Run("json", func(t *testing.T) {
				doc, stderr := runSpendJSON(t, c.flag...)

				assert.Empty(t, stderr)
				assert.Equal(t, c.doc, doc)
			})
		})
	}
}

// spendTextView is the first line of a spend table and the cells of each of its Total rows.
func spendTextView(out string) (string, [][]string) {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	var totals [][]string
	for _, line := range lines[1:] {
		if strings.HasPrefix(line, "Total") {
			totals = append(totals, strings.Fields(line))
		}
	}
	return lines[0], totals
}

// totalCells are the Total rows of a text table for totals, as spendTextView reads them.
func totalCells(totals []spendMoney) [][]string {
	cells := make([][]string, 0, len(totals))
	for _, t := range totals {
		cells = append(cells, []string{"Total", t.Currency, t.Spent})
	}
	return cells
}

func Test_run_spend_converts_the_edge_cases_in_each_reporting_currency(t *testing.T) {
	const thisYear = "Spending 2026-01-01 to 2026-09-29 in "
	cad100 := []spendMoney{{Currency: "CAD", Spent: "100.00"}}
	usd80 := []spendMoney{{Currency: "USD", Spent: "80.00"}}
	fridayRate := store.Rate{Date: day(2026, 3, 13), USDCAD: money.Rate(1_250_000), Series: "FXUSDCAD"}
	closedUSD := store.Account{ID: "acct-usd", SourceID: 2, Name: "US Chequing", Type: "chequing", Currency: "USD", Closed: true}
	usdSplit := func(when time.Time) spendSplit {
		return spendSplit{id: "s1", account: "acct-usd", category: "cat-groceries", currency: "USD", day: when, cents: -8000}
	}
	cadSplit := spendSplit{id: "s2", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -10000}
	cases := []struct {
		name       string
		accounts   []store.Account
		splits     []spendSplit
		rate       store.Rate
		args       []string
		caption    string
		currencies []string
		want       map[string][]spendMoney
	}{
		{
			name:     "a closed USD account converts",
			accounts: []store.Account{closedUSD}, splits: []spendSplit{usdSplit(day(2026, 3, 11))}, rate: rateOnJan2,
			caption: thisYear + "all accounts", currencies: []string{"CAD", "USD", "native"},
			want: map[string][]spendMoney{"CAD": cad100, "USD": usd80, "native": usd80},
		},
		{
			name:     "one USD account converts to a CAD total",
			accounts: []store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
			splits:   []spendSplit{usdSplit(day(2026, 3, 11)), cadSplit}, rate: rateOnJan2,
			args: []string{"--account", "US Chequing"}, caption: thisYear + "US Chequing", currencies: []string{"CAD", "USD", "native"},
			want: map[string][]spendMoney{"CAD": cad100, "USD": usd80, "native": usd80},
		},
		{
			name:     "a split dated after the last rate converts at the latest rate",
			accounts: []store.Account{usdChequingAccount("acct-usd", 2)}, splits: []spendSplit{usdSplit(day(2099, 6, 1))}, rate: rateOnJan2,
			args: []string{"--until", "2099-12-31"}, caption: "Spending 2026-01-01 to 2099-12-31 in all accounts", currencies: []string{"CAD", "USD"},
			want: map[string][]spendMoney{"CAD": cad100, "USD": usd80},
		},
		{
			name:     "a weekend split converts at the Friday rate",
			accounts: []store.Account{usdChequingAccount("acct-usd", 2)}, splits: []spendSplit{usdSplit(day(2026, 3, 14))}, rate: fridayRate,
			caption: thisYear + "all accounts", currencies: []string{"CAD", "USD"},
			want: map[string][]spendMoney{"CAD": cad100, "USD": usd80},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStoreWithRates(t, home, spendRows(c.accounts, c.splits...), c.rate)

			for _, currency := range c.currencies {
				args := append([]string{"--currency", currency}, c.args...)
				wantCaption := c.caption + map[string]string{"CAD": ", amounts in CAD", "USD": ", amounts in USD", "native": ""}[currency]

				exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"spend"}, args...))
				require.Equal(t, 0, exitCode, stderr.String())
				gotCaption, gotTotals := spendTextView(stdout.String())
				doc, jsonStderr := runSpendJSON(t, args...)

				assert.Empty(t, stderr.String(), currency)
				assert.Empty(t, jsonStderr, currency)
				assert.Equal(t, wantCaption, gotCaption, currency)
				assert.Equal(t, totalCells(c.want[currency]), gotTotals, currency)
				assert.Equal(t, currency, doc.Currency)
				assert.Equal(t, c.want[currency], doc.Totals, currency)
			}
		})
	}
}

func Test_run_spend_of_an_empty_window_names_the_currency_and_lists_no_rows_beside_the_empty_window_note(t *testing.T) {
	const emptyWindowWarning = "quarry: warning: no spending from 2020-01-01 to 2020-12-31; the store's transactions run 2026-03-11 to 2026-03-11\n"
	const caption = "Spending 2020-01-01 to 2020-12-31 in all accounts"
	suffixes := map[string]string{"CAD": ", amounts in CAD", "USD": ", amounts in USD", "native": ""}
	for currency, suffix := range suffixes {
		t.Run(currency, func(t *testing.T) {
			home := newHome(t)
			replaceStoreWithRates(t, home, spendRows(
				[]store.Account{usdChequingAccount("acct-usd", 2)},
				spendSplit{id: "s1", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 3, 11), cents: -8000},
			), rateOnJan2)
			window := []string{"--currency", currency, "--since", "2020-01-01", "--until", "2020-12-31"}

			t.Run("text", func(t *testing.T) {
				exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"spend"}, window...))

				require.Equal(t, 0, exitCode, stderr.String())
				assert.Equal(t, caption+suffix+"\n\nCategory  Currency  Spent\n", stdout.String())
				assert.Equal(t, emptyWindowWarning, stderr.String())
			})

			t.Run("json", func(t *testing.T) {
				doc, stderr := runSpendJSON(t, window...)

				assert.Equal(t, currency, doc.Currency)
				assert.Equal(t, []spendMoney{}, doc.Rows)
				assert.Equal(t, []spendMoney{}, doc.Totals)
				assert.Equal(t, emptyWindowWarning, stderr)
			})
		})
	}
}

func Test_run_spend_json_reads_back_with_every_amount_in_the_reporting_currency(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, spendRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s1", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12345},
		spendSplit{id: "s2", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 3, 11), cents: -8001},
		spendSplit{id: "s3", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 3, 12), cents: -3333},
	), rateOnJan2)

	doc, _ := runSpendJSON(t, "--by", "category")

	require.Len(t, doc.Totals, 1)
	var rowCents int64
	for _, r := range append(append([]spendMoney{}, doc.Rows...), doc.Totals...) {
		assert.Equal(t, doc.Currency, r.Currency)
	}
	for _, r := range doc.Rows {
		rowCents += centsOf(t, r.Spent)
	}
	assert.Equal(t, centsOf(t, doc.Totals[0].Spent), rowCents)
}

// centsOf is a spend document's amount, "123.71", in cents.
func centsOf(t *testing.T, amount string) int64 {
	t.Helper()
	cents, err := strconv.ParseInt(strings.Replace(amount, ".", "", 1), 10, 64)
	require.NoError(t, err)
	return cents
}

func Test_run_spend_json_returns_spending_as_a_document(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, spendRows(
		[]store.Account{
			chequingAccount("acct-cad", 1),
			usdChequingAccount("acct-usd", 2),
		},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12345},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 5, 2), cents: -120450},
		spendSplit{id: "s03", account: "acct-cad", currency: "CAD", day: day(2026, 6, 1), cents: -4208},
		spendSplit{id: "s04", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 4, 1), cents: -31210},
	))

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"spend", "--json"}, time.Date(2026, 9, 29, 22, 0, 0, 0, utcMinus5))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: "+noRatesLine+"\n", stderr.String())
	//nolint:testifylint // bytes are the contract
	assert.Equal(t, `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "by": "category",
  "currency": "CAD",
  "account_filter": [],
  "rows": [
    {
      "category": null,
      "currency": "CAD",
      "spent": "42.08"
    },
    {
      "category": "Auto:Fuel",
      "currency": "CAD",
      "spent": "1204.50"
    },
    {
      "category": "Food:Groceries",
      "currency": "CAD",
      "spent": "123.45"
    },
    {
      "category": "Food:Groceries",
      "currency": "USD",
      "spent": "312.10"
    }
  ],
  "totals": [
    {
      "currency": "CAD",
      "spent": "1370.03"
    },
    {
      "currency": "USD",
      "spent": "312.10"
    }
  ],
  "warnings": [
    "`+noRatesLine+`"
  ]
}
`, stdout.String())
}

// HOME holds no store: exit 2 (not the missing-store 1) shows each check runs first.
func Test_run_spend_rejects_a_period_it_cannot_use(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name:       "a since that is not a date",
			args:       []string{"spend", "--since", "2024-13"},
			wantStderr: "quarry: --since \"2024-13\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n",
		},
		{
			name:       "an until that is not a date",
			args:       []string{"spend", "--until", "yesterday"},
			wantStderr: "quarry: --until \"yesterday\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n",
		},
		{
			name:       "a since after until",
			args:       []string{"spend", "--since", "2025", "--until", "2024"},
			wantStderr: "quarry: --since 2025 is after --until 2024\n",
		},
		{
			name:       "a since after today",
			args:       []string{"spend", "--since", "2099"},
			wantStderr: "quarry: --since 2099 is after today; pass --until to include future-dated transactions\n",
		},
		{
			name:       "a grouping that does not exist",
			args:       []string{"spend", "--by", "vendor"},
			wantStderr: "quarry: --by must be category, payee, tag or month\n",
		},
		{
			name:       "a positional argument",
			args:       []string{"spend", "extra"},
			wantStderr: "quarry: spend takes no arguments\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())

			exitCode, stdout, stderr := runSpendCapture(context.Background(), c.args)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

func Test_run_report_commands_refuse_when_home_is_unset(t *testing.T) {
	cases := []struct {
		command    string
		wantStderr string
	}{
		{command: "spend", wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry spend again\n"},
		{
			command:    "cashflow",
			wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry cashflow again\n",
		},
		{
			command:    "recurring",
			wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry recurring again\n",
		},
		{
			command:    "anomalies",
			wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry anomalies again\n",
		},
		{
			command:    "search",
			wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry search again\n",
		},
		{
			command:    "holdings",
			wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry holdings again\n",
		},
		{
			command:    "networth",
			wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry networth again\n",
		},
		{
			command:    "acb",
			wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry acb again\n",
		},
		{
			command:    "summary",
			wantStderr: "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry summary again\n",
		},
	}

	for _, c := range cases {
		t.Run(c.command, func(t *testing.T) {
			t.Setenv("HOME", "")

			exitCode, stdout, stderr := runCapture(context.Background(), []string{c.command})

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

const (
	beforeFirstRateLine = "2 transactions dated before 2026-01-02, the first exchange rate in the store, " +
		"are listed in USD, not converted to CAD"
	noRatesLine = "the store has no exchange rates, so amounts are listed in each account's own currency; " +
		"run quarry sync to fetch them"
)

// unconvertedDoc is the part of spend's --json document these tests read.
type unconvertedDoc struct {
	Rows     []spendMoney `json:"rows"`
	Totals   []spendMoney `json:"totals"`
	Warnings []string     `json:"warnings"`
}

func Test_run_spend_lists_a_split_before_the_first_rate_in_its_own_currency_and_warns(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, spendRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12345},
		spendSplit{id: "s02", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 1, 1), cents: -1000},
		spendSplit{id: "s03", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 1, 1), cents: -2000},
		spendSplit{id: "s04", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 5, 2), cents: -8000},
	), rateOnJan2)

	t.Run("text", func(t *testing.T) {
		exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"spend"})

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Equal(t, "quarry: warning: "+beforeFirstRateLine+"\n", stderr.String())
		const row = "%-14s  %-8s  %6s\n"
		assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
			fmt.Sprintf(row, "Category", "Currency", "Spent")+
			fmt.Sprintf(row, "Auto:Fuel", "CAD", "100.00")+
			fmt.Sprintf(row, "Auto:Fuel", "USD", "20.00")+
			fmt.Sprintf(row, "Food:Groceries", "CAD", "123.45")+
			fmt.Sprintf(row, "Food:Groceries", "USD", "10.00")+
			fmt.Sprintf(row, "Total", "CAD", "223.45")+
			fmt.Sprintf(row, "Total", "USD", "30.00"),
			stdout.String())
	})

	t.Run("json", func(t *testing.T) {
		doc, stderr := runSpendUnconverted(t)

		assert.Equal(t, "quarry: warning: "+beforeFirstRateLine+"\n", stderr)
		assert.Equal(t, []string{beforeFirstRateLine}, doc.Warnings)
		assert.Equal(t, []spendMoney{
			{Currency: "CAD", Spent: "223.45"},
			{Currency: "USD", Spent: "30.00"},
		}, doc.Totals)
	})
}

func Test_run_spend_without_rates_lists_each_currency_natively_and_warns_only_when_a_conversion_is_needed(t *testing.T) {
	const row = "%-14s  %-8s  %6s\n"
	t.Run("CAD and USD data warns that there are no rates", func(t *testing.T) {
		home := newHome(t)
		replaceStore(t, home, spendRows(
			[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
			spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12345},
			spendSplit{id: "s02", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 4, 1), cents: -1000},
		))

		exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"spend"})

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Equal(t, "quarry: warning: "+noRatesLine+"\n", stderr.String())
		assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
			fmt.Sprintf(row, "Category", "Currency", "Spent")+
			fmt.Sprintf(row, "Food:Groceries", "CAD", "123.45")+
			fmt.Sprintf(row, "Food:Groceries", "USD", "10.00")+
			fmt.Sprintf(row, "Total", "CAD", "123.45")+
			fmt.Sprintf(row, "Total", "USD", "10.00"),
			stdout.String())
		doc, _ := runSpendUnconverted(t)
		assert.Equal(t, []string{noRatesLine}, doc.Warnings)
	})

	t.Run("all-CAD data in CAD needs no conversion and stays quiet", func(t *testing.T) {
		home := newHome(t)
		replaceStore(t, home, spendRows(
			[]store.Account{chequingAccount("acct-cad", 1)},
			spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12345},
		))

		exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"spend"})

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Empty(t, stderr.String())
		assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
			fmt.Sprintf(row, "Category", "Currency", "Spent")+
			fmt.Sprintf(row, "Food:Groceries", "CAD", "123.45")+
			fmt.Sprintf(row, "Total", "CAD", "123.45"),
			stdout.String())
		doc, _ := runSpendUnconverted(t)
		assert.Equal(t, []string{}, doc.Warnings)
	})
}

// runSpendUnconverted is runSpendDoc over the warnings tests' unconvertedDoc.
func runSpendUnconverted(tb testing.TB, args ...string) (unconvertedDoc, string) {
	tb.Helper()
	return runSpendDoc[unconvertedDoc](tb, args...)
}

// unconvertedCell is one report on a store, in text and --json, with the warnings it must give.
type unconvertedCell struct {
	name string
	args []string
	want []string
}

const (
	usdPreRateLine = "2 transactions dated before 2026-01-02, the first exchange rate in the store, " +
		"are listed in USD, not converted to CAD"
	cadPreRateLine = "1 transaction dated before 2026-01-02, the first exchange rate in the store, " +
		"is listed in CAD, not converted to USD"
)

func Test_run_spend_in_usd_without_rates_warns_only_when_a_conversion_is_needed(t *testing.T) {
	cases := []struct {
		name    string
		account store.Account
		split   spendSplit
		want    []string
	}{
		{
			name:    "all-USD data in USD",
			account: usdChequingAccount("acct-usd", 2),
			split:   spendSplit{id: "s01", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 3, 10), cents: -1000},
		},
		{
			name:    "all-CAD data in USD",
			account: chequingAccount("acct-cad", 1),
			split:   spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12345},
			want:    []string{noRatesLine},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, spendRows([]store.Account{c.account}, c.split))
			args := []string{"--currency", "USD"}

			exitCode, _, stderr := runSpendCapture(context.Background(), append([]string{"spend"}, args...))
			doc, echoedStderr := runSpendUnconverted(t, args...)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, warningLines(c.want), stderr.String())
			assert.Equal(t, warningLines(c.want), echoedStderr)
			assert.ElementsMatch(t, c.want, doc.Warnings)
		})
	}
}

func Test_run_spend_warns_once_per_report_with_the_count_of_its_own_accounts_and_currency(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, spendRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12345},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 1, 1), cents: -500},
		spendSplit{id: "s03", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 1, 1), cents: -1000},
		spendSplit{id: "s04", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 1, 1), cents: -2000},
		spendSplit{id: "s05", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 5, 2), cents: -8000},
	), rateOnJan2)
	cells := []unconvertedCell{
		{name: "CAD mode counts the USD transactions", want: []string{usdPreRateLine}},
		{name: "USD mode counts the CAD transactions", args: []string{"--currency", "USD"}, want: []string{cadPreRateLine}},
		{name: "the USD account alone warns with its own count", args: []string{"--account", "acct-usd"}, want: []string{usdPreRateLine}},
		{name: "the CAD account alone stays silent in CAD", args: []string{"--account", "acct-cad"}},
		{name: "native converts nothing", args: []string{"--currency", "native"}},
		{name: "by payee warns once", args: []string{"--by", "payee"}, want: []string{usdPreRateLine}},
		{name: "by tag warns once", args: []string{"--by", "tag"}, want: []string{usdPreRateLine}},
		{name: "by month warns once", args: []string{"--by", "month"}, want: []string{usdPreRateLine}},
	}

	for _, c := range cells {
		t.Run(c.name, func(t *testing.T) {
			exitCode, _, stderr := runSpendCapture(context.Background(), append([]string{"spend"}, c.args...))
			doc, echoedStderr := runSpendUnconverted(t, c.args...)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, warningLines(c.want), stderr.String())
			assert.Equal(t, warningLines(c.want), echoedStderr)
			assert.ElementsMatch(t, c.want, doc.Warnings)
		})
	}
}

func Test_run_spend_by_month_lists_the_other_currency_only_in_the_month_that_holds_it(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, spendRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 1, 1), cents: -1000},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 2, 10), cents: -5000},
		spendSplit{id: "s03", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 3, 10), cents: -2000},
	), rateOnJan2)
	args := []string{"--by", "month", "--since", "2026-01", "--until", "2026-03"}
	wantRows := []string{"2026-01 CAD", "2026-01 USD", "2026-02 CAD", "2026-03 CAD"}

	t.Run("text", func(t *testing.T) {
		exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"spend"}, args...))

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Equal(t, "Spending 2026-01-01 to 2026-03-31 in all accounts, amounts in CAD\n\n"+
			"Month    Currency  Spent  Status\n"+
			"2026-01  CAD        0.00\n"+
			"2026-01  USD       10.00\n"+
			"2026-02  CAD       50.00\n"+
			"2026-03  CAD       20.00\n"+
			"Total    CAD       70.00\n"+
			"Total    USD       10.00\n", stdout.String())
	})

	t.Run("json", func(t *testing.T) {
		exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"spend", "--json"}, args...))

		require.Equal(t, 0, exitCode, stderr.String())
		var doc struct {
			Rows []struct {
				Month    string `json:"month"`
				Currency string `json:"currency"`
			} `json:"rows"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
		gotRows := make([]string, len(doc.Rows))
		for i, r := range doc.Rows {
			gotRows[i] = r.Month + " " + r.Currency
		}
		assert.Equal(t, wantRows, gotRows)
	})

	t.Run("native fills every currency in every month", func(t *testing.T) {
		exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"spend", "--currency", "native"}, args...))

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Equal(t, 6, strings.Count(stdout.String(), "\n2026-0"), stdout.String())
	})
}

func Test_run_spend_of_an_unrated_empty_window_gives_only_the_empty_window_note(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, spendRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 3, 10), cents: -1000},
	))

	exitCode, _, stderr := runSpendCapture(context.Background(), []string{"spend", "--since", "2020-01-01", "--until", "2020-12-31"})

	require.Equal(t, 0, exitCode, stderr.String())
	const note = "no spending from 2020-01-01 to 2020-12-31; the store's transactions run 2026-03-10 to 2026-03-10"
	assert.Equal(t, warningLines([]string{note}), stderr.String())
	doc, echoedStderr := runSpendUnconverted(t, "--since", "2020-01-01", "--until", "2020-12-31")
	assert.Equal(t, stderr.String(), echoedStderr)
	assert.Equal(t, []string{note}, doc.Warnings)
}

func Test_run_spend_by_month_of_an_empty_window_lists_no_rows_beside_the_empty_window_note(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, spendRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 3, 10), cents: -1000},
	), rateOnJan2)
	args := []string{"--by", "month", "--since", "2020-01", "--until", "2020-03"}

	exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"spend"}, args...))
	doc, _ := runSpendUnconverted(t, args...)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Spending 2020-01-01 to 2020-03-31 in all accounts, amounts in CAD\n\n"+
		"Month  Currency  Spent  Status\n", stdout.String())
	assert.Equal(t, []spendMoney{}, doc.Rows)
	assert.Equal(t, []spendMoney{}, doc.Totals)
	assert.Equal(t, []string{"no spending from 2020-01-01 to 2020-03-31; the store's transactions run 2026-03-10 to 2026-03-10"}, doc.Warnings)
}

func Test_run_spend_by_month_of_a_window_holding_only_unconverted_rows_still_lists_the_report_currency_zero_rows(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, spendRows(
		[]store.Account{usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2025, 12, 20), cents: -1000},
	), rateOnJan2)
	args := []string{"--by", "month", "--since", "2025-11", "--until", "2025-12"}

	exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"spend"}, args...))
	doc, _ := runSpendUnconverted(t, args...)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Spending 2025-11-01 to 2025-12-31 in all accounts, amounts in CAD\n\n"+
		"Month    Currency  Spent  Status\n"+
		"2025-11  CAD        0.00\n"+
		"2025-12  CAD        0.00\n"+
		"2025-12  USD       10.00\n"+
		"Total    USD       10.00\n", stdout.String())
	assert.Equal(t, []string{"CAD", "CAD", "USD"}, spendCurrencies(doc.Rows))
}

// spendCurrencies is the currency of each row, in order.
func spendCurrencies(rows []spendMoney) []string {
	currencies := make([]string, len(rows))
	for i, r := range rows {
		currencies[i] = r.Currency
	}
	return currencies
}

func Test_run_spend_counts_the_whole_period_it_is_given(t *testing.T) {
	cases := []struct {
		name         string
		since, until string
		caption      string
		total        string
	}{
		{
			name:  "a bare year covers January 1 to December 31",
			since: "2024", until: "2024",
			caption: "Spending 2024-01-01 to 2024-12-31 in all accounts, amounts in CAD",
			total:   "30.00",
		},
		{
			name:  "a bare month covers its last day",
			since: "2024-12", until: "2025-01",
			caption: "Spending 2024-12-01 to 2025-01-31 in all accounts, amounts in CAD",
			total:   "60.00",
		},
		{
			name:  "one day is a period of its own",
			since: "2024-12-31", until: "2024-12-31",
			caption: "Spending 2024-12-31 to 2024-12-31 in all accounts, amounts in CAD",
			total:   "20.00",
		},
		{
			name:  "a period after today counts what is dated in it",
			since: "2099", until: "2099",
			caption: "Spending 2099-01-01 to 2099-12-31 in all accounts, amounts in CAD",
			total:   "80.00",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, spendRows(
				[]store.Account{chequingAccount("acct-cad", 1)},
				spendSplit{id: "s01", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2024, 1, 1), cents: -1000},
				spendSplit{id: "s02", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2024, 12, 31), cents: -2000},
				spendSplit{id: "s03", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2025, 1, 1), cents: -4000},
				spendSplit{id: "s04", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2099, 6, 1), cents: -8000},
			))

			exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"spend", "--since", c.since, "--until", c.until})

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Empty(t, stderr.String())
			const row = "%-9s  %-8s  %5s\n"
			assert.Equal(t, c.caption+"\n\n"+
				fmt.Sprintf(row, "Category", "Currency", "Spent")+
				fmt.Sprintf(row, "Auto:Fuel", "CAD", c.total)+
				fmt.Sprintf(row, "Total", "CAD", c.total),
				stdout.String())
		})
	}
}
