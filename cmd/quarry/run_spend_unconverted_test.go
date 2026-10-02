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

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStoreWithRates(t, home, spendRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12345},
		spendSplit{id: "s02", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 1, 1), cents: -1000},
		spendSplit{id: "s03", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 1, 1), cents: -2000},
		spendSplit{id: "s04", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 5, 2), cents: -8000},
	), rateOnJan2)

	t.Run("text", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := runWith(context.Background(), []string{"spend"}, spendEnv(&stdout, &stderr))

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
		home := t.TempDir()
		t.Setenv("HOME", home)
		replaceStore(t, home, spendRows(
			[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
			spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12345},
			spendSplit{id: "s02", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 4, 1), cents: -1000},
		))
		var stdout, stderr bytes.Buffer

		exitCode := runWith(context.Background(), []string{"spend"}, spendEnv(&stdout, &stderr))

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
		home := t.TempDir()
		t.Setenv("HOME", home)
		replaceStore(t, home, spendRows(
			[]store.Account{chequingAccount("acct-cad", 1)},
			spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12345},
		))
		var stdout, stderr bytes.Buffer

		exitCode := runWith(context.Background(), []string{"spend"}, spendEnv(&stdout, &stderr))

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

// runSpendUnconverted runs spend --json and returns its document and stderr.
func runSpendUnconverted(t *testing.T, args ...string) (unconvertedDoc, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	exitCode := runWith(context.Background(), append([]string{"spend", "--json"}, args...), spendEnv(&stdout, &stderr))
	require.Equal(t, 0, exitCode, stderr.String())
	var doc unconvertedDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	return doc, stderr.String()
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
			home := t.TempDir()
			t.Setenv("HOME", home)
			replaceStore(t, home, spendRows([]store.Account{c.account}, c.split))
			args := []string{"--currency", "USD"}
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), append([]string{"spend"}, args...), spendEnv(&stdout, &stderr))
			doc, echoedStderr := runSpendUnconverted(t, args...)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, warningLines(c.want), stderr.String())
			assert.Equal(t, warningLines(c.want), echoedStderr)
			assert.ElementsMatch(t, c.want, doc.Warnings)
		})
	}
}

func Test_run_spend_warns_once_per_report_with_the_count_of_its_own_accounts_and_currency(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
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
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), append([]string{"spend"}, c.args...), spendEnv(&stdout, &stderr))
			doc, echoedStderr := runSpendUnconverted(t, c.args...)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, warningLines(c.want), stderr.String())
			assert.Equal(t, warningLines(c.want), echoedStderr)
			assert.ElementsMatch(t, c.want, doc.Warnings)
		})
	}
}

func Test_run_spend_by_month_lists_the_other_currency_only_in_the_month_that_holds_it(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStoreWithRates(t, home, spendRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 1, 1), cents: -1000},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 2, 10), cents: -5000},
		spendSplit{id: "s03", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 3, 10), cents: -2000},
	), rateOnJan2)
	args := []string{"--by", "month", "--since", "2026-01", "--until", "2026-03"}
	wantRows := []string{"2026-01 CAD", "2026-01 USD", "2026-02 CAD", "2026-03 CAD"}

	t.Run("text", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := runWith(context.Background(), append([]string{"spend"}, args...), spendEnv(&stdout, &stderr))

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
		var stdout, stderr bytes.Buffer

		exitCode := runWith(context.Background(), append([]string{"spend", "--json"}, args...), spendEnv(&stdout, &stderr))

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
		var stdout, stderr bytes.Buffer

		exitCode := runWith(context.Background(), append([]string{"spend", "--currency", "native"}, args...), spendEnv(&stdout, &stderr))

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Equal(t, 6, strings.Count(stdout.String(), "\n2026-0"), stdout.String())
	})
}

func Test_run_spend_of_an_unrated_empty_window_gives_only_the_empty_window_note(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, spendRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 3, 10), cents: -1000},
	))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"spend", "--since", "2020-01-01", "--until", "2020-12-31"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	const note = "no spending from 2020-01-01 to 2020-12-31; the store's transactions run 2026-03-10 to 2026-03-10"
	assert.Equal(t, warningLines([]string{note}), stderr.String())
	doc, echoedStderr := runSpendUnconverted(t, "--since", "2020-01-01", "--until", "2020-12-31")
	assert.Equal(t, stderr.String(), echoedStderr)
	assert.Equal(t, []string{note}, doc.Warnings)
}

func Test_run_spend_by_month_of_an_empty_window_lists_no_rows_beside_the_empty_window_note(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStoreWithRates(t, home, spendRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 3, 10), cents: -1000},
	), rateOnJan2)
	args := []string{"--by", "month", "--since", "2020-01", "--until", "2020-03"}
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), append([]string{"spend"}, args...), spendEnv(&stdout, &stderr))
	doc, _ := runSpendUnconverted(t, args...)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Spending 2020-01-01 to 2020-03-31 in all accounts, amounts in CAD\n\n"+
		"Month  Currency  Spent  Status\n", stdout.String())
	assert.Equal(t, []spendMoney{}, doc.Rows)
	assert.Equal(t, []spendMoney{}, doc.Totals)
	assert.Equal(t, []string{"no spending from 2020-01-01 to 2020-03-31; the store's transactions run 2026-03-10 to 2026-03-10"}, doc.Warnings)
}

func Test_run_spend_by_month_of_a_window_holding_only_unconverted_rows_still_lists_the_report_currency_zero_rows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStoreWithRates(t, home, spendRows(
		[]store.Account{usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2025, 12, 20), cents: -1000},
	), rateOnJan2)
	args := []string{"--by", "month", "--since", "2025-11", "--until", "2025-12"}
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), append([]string{"spend"}, args...), spendEnv(&stdout, &stderr))
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
