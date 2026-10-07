// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"context"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_cashflow_warns_with_the_count_of_its_own_accounts_and_currency(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, cashFlowRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 3, 10), cents: 100000},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 1, 1), cents: -500},
		spendSplit{id: "s03", account: "acct-usd", category: "cat-salary", currency: "USD", day: day(2026, 1, 1), cents: 8000},
		spendSplit{id: "s04", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 1, 1), cents: -2000},
	), rateOnJan2)
	cells := []unconvertedCell{
		{name: "CAD mode counts income and expense in USD", want: []string{usdPreRateLine}},
		{name: "USD mode counts the CAD transaction", args: []string{"--currency", "USD"}, want: []string{cadPreRateLine}},
		{name: "the USD account alone warns with its own count", args: []string{"--account", "acct-usd"}, want: []string{usdPreRateLine}},
		{name: "the CAD account alone stays silent in CAD", args: []string{"--account", "acct-cad"}},
		{name: "by year warns once", args: []string{"--by", "year"}, want: []string{usdPreRateLine}},
		{name: "native converts nothing", args: []string{"--currency", "native"}},
	}

	for _, c := range cells {
		t.Run(c.name, func(t *testing.T) {
			exitCode, _, stderr := runSpendCapture(context.Background(), append([]string{"cashflow"}, c.args...))
			doc, echoedStderr := runCashFlowJSON(t, c.args...)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, warningLines(c.want), stderr.String())
			assert.Equal(t, warningLines(c.want), echoedStderr)
			assert.ElementsMatch(t, c.want, doc.Warnings)
		})
	}
}

func Test_run_cashflow_without_rates_warns_only_when_a_conversion_is_needed(t *testing.T) {
	cases := []struct {
		name     string
		accounts []store.Account
		splits   []spendSplit
		want     []string
	}{
		{
			name:     "CAD and USD data in CAD",
			accounts: []store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
			splits: []spendSplit{
				{id: "s01", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 3, 10), cents: 100000},
				{id: "s02", account: "acct-usd", category: "cat-salary", currency: "USD", day: day(2026, 3, 11), cents: 8000},
			},
			want: []string{noRatesLine},
		},
		{
			name:     "all-CAD data in CAD",
			accounts: []store.Account{chequingAccount("acct-cad", 1)},
			splits:   []spendSplit{{id: "s01", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 3, 10), cents: 100000}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, cashFlowRows(c.accounts, c.splits...))

			exitCode, _, stderr := runSpendCapture(context.Background(), []string{"cashflow", "--since", "2026-03", "--until", "2026-03"})
			doc, echoedStderr := runCashFlowJSON(t, "--since", "2026-03", "--until", "2026-03")

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, warningLines(c.want), stderr.String())
			assert.Equal(t, warningLines(c.want), echoedStderr)
			assert.ElementsMatch(t, c.want, doc.Warnings)
		})
	}
}

func Test_run_cashflow_in_usd_without_rates_warns_only_when_a_conversion_is_needed(t *testing.T) {
	cases := []struct {
		name    string
		account store.Account
		split   spendSplit
		want    []string
	}{
		{
			name:    "all-USD data in USD",
			account: usdChequingAccount("acct-usd", 2),
			split:   spendSplit{id: "s01", account: "acct-usd", category: "cat-salary", currency: "USD", day: day(2026, 3, 10), cents: 8000},
		},
		{
			name:    "all-CAD data in USD",
			account: chequingAccount("acct-cad", 1),
			split:   spendSplit{id: "s01", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 3, 10), cents: 100000},
			want:    []string{noRatesLine},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, cashFlowRows([]store.Account{c.account}, c.split))
			args := []string{"--currency", "USD", "--since", "2026-03", "--until", "2026-03"}

			exitCode, _, stderr := runSpendCapture(context.Background(), append([]string{"cashflow"}, args...))
			doc, echoedStderr := runCashFlowJSON(t, args...)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, warningLines(c.want), stderr.String())
			assert.Equal(t, warningLines(c.want), echoedStderr)
			assert.ElementsMatch(t, c.want, doc.Warnings)
		})
	}
}

func Test_run_cashflow_json_gives_a_period_in_the_other_currency_only_where_the_store_found_one(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, cashFlowRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-usd", category: "cat-salary", currency: "USD", day: day(2025, 12, 20), cents: 8000},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 2, 10), cents: 10000},
	), rateOnJan2)

	doc, _ := runCashFlowJSON(t, "--since", "2025-12", "--until", "2026-02")

	periods := make([]string, len(doc.Periods))
	for i, p := range doc.Periods {
		periods[i] = p.Period + " " + p.Currency
	}
	assert.Equal(t, []string{"2025-12 CAD", "2025-12 USD", "2026-01 CAD", "2026-02 CAD"}, periods)
}

func Test_run_cashflow_of_an_unrated_empty_window_gives_only_the_empty_window_note(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, cashFlowRows(
		[]store.Account{usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-usd", category: "cat-salary", currency: "USD", day: day(2026, 3, 10), cents: 8000},
	))

	exitCode, _, stderr := runSpendCapture(context.Background(), []string{"cashflow", "--since", "2020-01-01", "--until", "2020-12-31"})

	require.Equal(t, 0, exitCode, stderr.String())
	const note = "no income or spending from 2020-01-01 to 2020-12-31; the store's transactions run 2026-03-10 to 2026-03-10"
	assert.Equal(t, warningLines([]string{note}), stderr.String())
	doc, echoedStderr := runCashFlowJSON(t, "--since", "2020-01-01", "--until", "2020-12-31")
	assert.Equal(t, stderr.String(), echoedStderr)
	assert.Equal(t, []string{note}, doc.Warnings)
}
