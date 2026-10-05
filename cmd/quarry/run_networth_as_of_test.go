package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	netWorthAsOfNotADate = "quarry: --as-of \"2024-13\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n"
	netWorthAsOfConflict = "quarry: --as-of cannot be combined with --since or --until; " +
		"pass --as-of for one day, or --since and --until for month ends\n"
)

// netWorthAsOfLine is one converted table line of seedNetWorthAsOfStore's output, each cell as wide as its widest.
func netWorthAsOfLine(typ, currency, balance, in string) string {
	return fmt.Sprintf("%-9s  %-8s  %8s  %8s\n", typ, currency, balance, in)
}

// netWorthAsOfNativeLine is netWorthAsOfLine without the In column.
func netWorthAsOfNativeLine(typ, currency, balance string) string {
	return fmt.Sprintf("%-9s  %-8s  %8s\n", typ, currency, balance)
}

// netWorthAsOfRefusal is the refusal of a future as-of or since, as net worth words it.
func netWorthAsOfRefusal(flag, value string) string {
	return fmt.Sprintf("quarry: %s %s is after today; net worth is valued up to today only, so pass an earlier %s\n", flag, value, flag)
}

// seedNetWorthAsOfStore builds a store with CAD, USD, closed and brokerage activity on both sides of 2025-12-31,
// and USD rates from 2025-12-01 (1.36) and 2026-01-20 (1.50).
func seedNetWorthAsOfStore(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	closed := closedAccount(chequingAccount("acct-closed", 3))
	closed.Name = "Old Chequing"
	rows := spendRows(
		[]store.Account{
			chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2), closed, brokerageAccount("acct-brokerage", 4, "CAD"),
		},
		spendSplit{id: "cad-before", account: "acct-cad", currency: "CAD", day: day(2025, time.December, 15), cents: 100_000},
		spendSplit{id: "cad-on-day", account: "acct-cad", currency: "CAD", day: day(2025, time.December, 31), cents: 10_000},
		spendSplit{id: "cad-after", account: "acct-cad", currency: "CAD", day: day(2026, time.January, 1), cents: 50_000},
		spendSplit{id: "usd", account: "acct-usd", currency: "USD", day: day(2025, time.December, 20), cents: 80_000},
		spendSplit{id: "closed", account: "acct-closed", currency: "CAD", day: day(2025, time.November, 10), cents: 2_500},
		spendSplit{id: "deposit", account: "acct-brokerage", currency: "CAD", day: day(2025, time.December, 2), cents: 100_000},
	)
	rows.Securities = []store.Security{{ID: "sec-acme", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")}}
	rows.InvestmentTransactions = []store.InvestmentTransaction{{
		ID: "inv-acme", SourceID: 1, AccountID: "acct-brokerage", SecurityID: new("sec-acme"), Date: day(2025, time.December, 2),
		Action: store.ActionBuy, Shares: new(int64(2_000_000)), Amount: -10_000, Currency: "CAD",
	}}
	rows.Prices = []store.Price{
		{SecurityID: "sec-acme", SourceID: 1, Date: day(2025, time.December, 10), Price: 10_000_000},
		{SecurityID: "sec-acme", SourceID: 2, Date: day(2026, time.January, 15), Price: 20_000_000},
	}
	rows.Transactions = append(rows.Transactions, store.Transaction{
		ID: "txn-inv-acme", SourceID: 99, AccountID: "acct-brokerage", Date: day(2025, time.December, 2), Amount: -10_000, Currency: "CAD",
		Status: "uncleared", InvestmentTransactionID: new("inv-acme"),
	})
	rows.Splits = append(rows.Splits, store.Split{ID: "split-inv-acme", SourceID: 99, TransactionID: "txn-inv-acme", Amount: -10_000})
	replaceStoreWithRates(t, home, rows,
		usdRate(day(2025, time.December, 1), 1_360_000), usdRate(day(2026, time.January, 20), 1_500_000))
}

func Test_run_networth_refuses_a_future_as_of_a_future_since_and_as_of_with_since(t *testing.T) {
	refusals := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{"an as-of that is not a date", []string{"--as-of", "2024-13"}, netWorthAsOfNotADate},
		{"an empty as-of", []string{"--as-of", ""}, "quarry: --as-of \"\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n"},
		{"an as-of day after today", []string{"--as-of", "2027-01-01"}, netWorthAsOfRefusal("--as-of", "2027-01-01")},
		{"an as-of month after today, quoted as typed", []string{"--as-of", "2027-01"}, netWorthAsOfRefusal("--as-of", "2027-01")},
		{"an as-of year after today, quoted as typed", []string{"--as-of", "2027"}, netWorthAsOfRefusal("--as-of", "2027")},
		{"a since after today", []string{"--since", "2027-01"}, netWorthAsOfRefusal("--since", "2027-01")},
		{"a since after today with an until after today", []string{"--since", "2027-01", "--until", "2028"}, netWorthAsOfRefusal("--since", "2027-01")},
		{"an as-of with a since", []string{"--as-of", "2026-03", "--since", "2026-01"}, netWorthAsOfConflict},
		{"an as-of with an until", []string{"--as-of", "2026-03", "--until", "2026-02"}, netWorthAsOfConflict},
		{"an as-of with a since and an until", []string{"--as-of", "2026-03", "--since", "2026-01", "--until", "2026-02"}, netWorthAsOfConflict},
		{"an as-of that is not a date with a since reports the conflict", []string{"--as-of", "2024-13", "--since", "2026-01"}, netWorthAsOfConflict},
	}
	for _, c := range refusals {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), append([]string{"networth"}, c.args...),
				spendEnvAt(&stdout, &stderr, holdingsClock()))

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}

	t.Run("a future as-of with --json prints nothing to stdout", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		var stdout, stderr bytes.Buffer

		exitCode := runWith(context.Background(), []string{"networth", "--as-of", "2027", "--json"},
			spendEnvAt(&stdout, &stderr, holdingsClock()))

		assert.Equal(t, 2, exitCode)
		assert.Empty(t, stdout.String())
		assert.Equal(t, netWorthAsOfRefusal("--as-of", "2027"), stderr.String())
	})

	controls := []struct {
		name       string
		args       []string
		wantHeader string
	}{
		{"an as-of year containing today is today", []string{"--as-of", "2026"}, "Net worth on 2026-03-12, amounts in CAD\n"},
		{"a since month containing today lists today", []string{"--since", "2026-03"}, "Net worth at each month end 2026-03-12 to 2026-03-12, amounts in CAD\n"},
	}
	for _, c := range controls {
		t.Run(c.name, func(t *testing.T) {
			seedNetWorthHistoryStore(t)
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), append([]string{"networth"}, c.args...),
				spendEnvAt(&stdout, &stderr, holdingsClock()))

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Empty(t, stderr.String())
			assert.Contains(t, stdout.String(), c.wantHeader)
		})
	}
}

func Test_run_networth_values_every_counted_account_on_the_as_of_day(t *testing.T) {
	for _, asOf := range []string{"2025-12", "2025-12-31"} {
		t.Run("as of "+asOf, func(t *testing.T) {
			seedNetWorthAsOfStore(t)
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), []string{"networth", "--as-of", asOf},
				spendEnvAt(&stdout, &stderr, holdingsClock()))

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Empty(t, stderr.String())
			assert.Equal(t, "Net worth on 2025-12-31, amounts in CAD\n\n"+
				netWorthAsOfLine("Type", "Currency", "Balance", "In CAD")+
				netWorthAsOfLine("brokerage", "CAD", "920.00", "920.00")+
				netWorthAsOfLine("chequing", "CAD", "1,125.00", "1,125.00")+
				netWorthAsOfLine("chequing", "USD", "800.00", "1,088.00")+
				netWorthAsOfLine("Total", "", "", "3,133.00"),
				stdout.String())
		})
	}
}

func Test_run_networth_lists_each_currency_on_the_as_of_day_in_native_mode(t *testing.T) {
	seedNetWorthAsOfStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--as-of", "2025-12", "--currency", "native"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Net worth on 2025-12-31\n\n"+
		netWorthAsOfNativeLine("Type", "Currency", "Balance")+
		netWorthAsOfNativeLine("brokerage", "CAD", "920.00")+
		netWorthAsOfNativeLine("chequing", "CAD", "1,125.00")+
		netWorthAsOfNativeLine("chequing", "USD", "800.00")+
		netWorthAsOfNativeLine("Total", "CAD", "2,045.00")+
		netWorthAsOfNativeLine("Total", "USD", "800.00"),
		stdout.String())
}

func Test_run_networth_json_as_of_sets_the_day_and_leaves_since_and_until_null(t *testing.T) {
	seedNetWorthAsOfStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--as-of", "2025-12", "--json"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	var got struct {
		AsOf  *string `json:"as_of"`
		Since *string `json:"since"`
		Until *string `json:"until"`
		Dates []struct {
			Date   string           `json:"date"`
			Totals []map[string]any `json:"totals"`
		} `json:"dates"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	require.NotNil(t, got.AsOf)
	assert.Equal(t, "2025-12-31", *got.AsOf)
	assert.Nil(t, got.Since)
	assert.Nil(t, got.Until)
	require.Len(t, got.Dates, 1)
	assert.Equal(t, "2025-12-31", got.Dates[0].Date)
	assert.Equal(t, []map[string]any{{"currency": "CAD", "value": "3133.00"}}, got.Dates[0].Totals)
}
