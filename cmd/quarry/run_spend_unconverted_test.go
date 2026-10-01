// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
func runSpendUnconverted(t *testing.T) (unconvertedDoc, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	exitCode := runWith(context.Background(), []string{"spend", "--json"}, spendEnv(&stdout, &stderr))
	require.Equal(t, 0, exitCode, stderr.String())
	var doc unconvertedDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	return doc, stderr.String()
}
