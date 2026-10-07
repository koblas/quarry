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

// netWorthRowOf is one converted net worth table line with the type column typeW wide; the table ends a line at
// its last non-blank cell.
func netWorthRowOf(typeW int, typ, currency, balance, in string) string {
	return strings.TrimRight(fmt.Sprintf("%-*s  %-8s  %8s  %8s", typeW, typ, currency, balance, in), " ") + "\n"
}

// netWorthNativeRowOf is netWorthRowOf without the In column.
func netWorthNativeRowOf(typeW int, typ, currency, balance string) string {
	return fmt.Sprintf("%-*s  %-8s  %8s\n", typeW, typ, currency, balance)
}

// netWorthLine is one converted net worth table line, each cell as wide as the fixture's widest.
func netWorthLine(typ, currency, balance, in string) string {
	return netWorthRowOf(11, typ, currency, balance, in)
}

// netWorthNativeLine is netWorthLine without the In column.
func netWorthNativeLine(typ, currency, balance string) string {
	return netWorthNativeRowOf(11, typ, currency, balance)
}

// seedNetWorthStore builds the net worth store (CAD and USD accounts, a USD brokerage, a USD rate from March 10) under a temp HOME.
func seedNetWorthStore(t *testing.T) {
	t.Helper()
	home := newHome(t)
	march := func(d int) time.Time { return day(2026, time.March, d) }
	card := store.Account{ID: "acct-card", SourceID: 3, Name: "Card", Type: "credit_card", Currency: "CAD", Active: true}
	closed := closedAccount(chequingAccount("acct-closed", 5))
	closed.Name = "Old Chequing"
	notInReports := chequingAccount("acct-out", 6)
	notInReports.Name, notInReports.NotInReports = "Not In Reports", true
	savings := store.Account{ID: "acct-savings", SourceID: 7, Name: "Savings", Type: "savings", Currency: "CAD", Active: true}
	rows := spendRows(
		[]store.Account{
			chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2), card,
			brokerageAccount("acct-brokerage", 4, "USD"), closed, notInReports, savings,
		},
		spendSplit{id: "cad", account: "acct-cad", currency: "CAD", day: march(2), cents: 100_000},
		spendSplit{id: "usd", account: "acct-usd", currency: "USD", day: march(2), cents: 80_000},
		spendSplit{id: "card", account: "acct-card", currency: "CAD", day: march(2), cents: -25_050},
		spendSplit{id: "deposit", account: "acct-brokerage", currency: "USD", day: march(2), cents: 100_000},
		spendSplit{id: "closed", account: "acct-closed", currency: "CAD", day: march(2), cents: 2_500},
		spendSplit{id: "out", account: "acct-out", currency: "CAD", day: march(2), cents: 500},
		spendSplit{id: "saved", account: "acct-savings", currency: "CAD", day: march(2), cents: 5_000},
		spendSplit{id: "spent", account: "acct-savings", currency: "CAD", day: march(2), cents: -5_000},
	)
	rows.Securities = []store.Security{{ID: "sec-vti", SourceID: 1, Name: "Vanguard Total Stock", Ticker: new("VTI"), Currency: new("USD")}}
	rows.InvestmentTransactions = []store.InvestmentTransaction{{
		ID: "inv-vti", SourceID: 1, AccountID: "acct-brokerage", SecurityID: new("sec-vti"), Date: march(2),
		Action: store.ActionBuy, Shares: new(int64(2_000_000)), Amount: -10_000, Currency: "USD",
	}}
	rows.Prices = []store.Price{{SecurityID: "sec-vti", SourceID: 1, Date: march(1), Price: 10_000_000}}
	rows.Transactions = append(rows.Transactions, store.Transaction{
		ID: "txn-inv-vti", SourceID: 99, AccountID: "acct-brokerage", Date: march(2), Amount: -10_000, Currency: "USD",
		Status: "uncleared", InvestmentTransactionID: new("inv-vti"),
	})
	rows.Splits = append(rows.Splits, store.Split{ID: "split-inv-vti", SourceID: 99, TransactionID: "txn-inv-vti", Amount: -10_000})
	replaceStoreWithRates(t, home, rows, usdRate(march(10), 1_360_000))
}

func Test_run_networth_prints_todays_balances_by_type_and_currency_with_a_total_in_the_reporting_currency(t *testing.T) {
	seedNetWorthStore(t)

	stdout, stderr := mustRunNetWorth(t)

	assert.Empty(t, stderr)
	assert.Equal(t, "Net worth on 2026-03-12, amounts in CAD\n\n"+
		netWorthLine("Type", "Currency", "Balance", "In CAD")+
		netWorthLine("brokerage", "USD", "920.00", "1,251.20")+
		netWorthLine("chequing", "CAD", "1,025.00", "1,025.00")+
		netWorthLine("chequing", "USD", "800.00", "1,088.00")+
		netWorthLine("credit_card", "CAD", "-250.50", "-250.50")+
		netWorthLine("Total", "", "", "3,113.70"),
		stdout)
}

func Test_run_networth_lists_cad_and_usd_separately_with_one_total_each_in_native_mode(t *testing.T) {
	seedNetWorthStore(t)

	stdout, stderr := mustRunNetWorth(t, "--currency", "native")

	assert.Empty(t, stderr)
	assert.Equal(t, "Net worth on 2026-03-12\n\n"+
		netWorthNativeLine("Type", "Currency", "Balance")+
		netWorthNativeLine("brokerage", "USD", "920.00")+
		netWorthNativeLine("chequing", "CAD", "1,025.00")+
		netWorthNativeLine("chequing", "USD", "800.00")+
		netWorthNativeLine("credit_card", "CAD", "-250.50")+
		netWorthNativeLine("Total", "CAD", "774.50")+
		netWorthNativeLine("Total", "USD", "1,720.00"),
		stdout)
}

const (
	netWorthAsOfNotADate = "quarry: --as-of \"2024-13\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n"
	netWorthAsOfConflict = "quarry: --as-of cannot be combined with --since or --until; " +
		"pass --as-of for one day, or --since and --until for month ends\n"
)

// netWorthAsOfLine is one converted table line of seedNetWorthAsOfStore's output, each cell as wide as its widest.
func netWorthAsOfLine(typ, currency, balance, in string) string {
	return netWorthRowOf(9, typ, currency, balance, in)
}

// netWorthAsOfNativeLine is netWorthAsOfLine without the In column.
func netWorthAsOfNativeLine(typ, currency, balance string) string {
	return netWorthNativeRowOf(9, typ, currency, balance)
}

// netWorthAsOfRefusal is the refusal of a future as-of or since, as net worth words it.
func netWorthAsOfRefusal(flag, value string) string {
	return fmt.Sprintf("quarry: %s %s is after today; net worth is valued up to today only, so pass an earlier %s\n", flag, value, flag)
}

// seedNetWorthAsOfStore builds a store with CAD, USD, closed and brokerage activity on both sides of 2025-12-31,
// and USD rates from 2025-12-01 (1.36) and 2026-01-20 (1.50).
func seedNetWorthAsOfStore(t *testing.T) {
	t.Helper()
	home := newHome(t)
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

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), append([]string{"networth"}, c.args...), holdingsClock())

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}

	t.Run("a future as-of with --json prints nothing to stdout", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())

		exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"networth", "--as-of", "2027", "--json"}, holdingsClock())

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

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), append([]string{"networth"}, c.args...), holdingsClock())

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

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"networth", "--as-of", asOf}, holdingsClock())

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

	stdout, stderr := mustRunNetWorth(t, "--as-of", "2025-12", "--currency", "native")

	assert.Empty(t, stderr)
	assert.Equal(t, "Net worth on 2025-12-31\n\n"+
		netWorthAsOfNativeLine("Type", "Currency", "Balance")+
		netWorthAsOfNativeLine("brokerage", "CAD", "920.00")+
		netWorthAsOfNativeLine("chequing", "CAD", "1,125.00")+
		netWorthAsOfNativeLine("chequing", "USD", "800.00")+
		netWorthAsOfNativeLine("Total", "CAD", "2,045.00")+
		netWorthAsOfNativeLine("Total", "USD", "800.00"),
		stdout)
}

func Test_run_networth_json_as_of_sets_the_day_and_leaves_since_and_until_null(t *testing.T) {
	seedNetWorthAsOfStore(t)

	stdout, _ := mustRunNetWorth(t, "--as-of", "2025-12", "--json")

	var got struct {
		AsOf  *string `json:"as_of"`
		Since *string `json:"since"`
		Until *string `json:"until"`
		Dates []struct {
			Date   string           `json:"date"`
			Totals []map[string]any `json:"totals"`
		} `json:"dates"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.NotNil(t, got.AsOf)
	assert.Equal(t, "2025-12-31", *got.AsOf)
	assert.Nil(t, got.Since)
	assert.Nil(t, got.Until)
	require.Len(t, got.Dates, 1)
	assert.Equal(t, "2025-12-31", got.Dates[0].Date)
	assert.Equal(t, []map[string]any{{"currency": "CAD", "value": "3133.00"}}, got.Dates[0].Totals)
}

const (
	beforeSnapshotLine = "no account has a balance on 2026-03-01; the first balance is on 2026-03-02"
	beforeHistoryLine  = "no account has a balance at any month end from 2026-01-31 to 2026-02-28; the first balance is on 2026-03-02"

	beforeSnapshotWarning = "quarry: warning: " + beforeSnapshotLine + "\n"
	beforeHistoryWarning  = "quarry: warning: " + beforeHistoryLine + "\n"
)

// seedUncountedOnlyStore builds a store whose only transaction is in an account left out of Quicken's reports.
func seedUncountedOnlyStore(t *testing.T) {
	t.Helper()
	home := newHome(t)
	notInReports := chequingAccount("acct-out", 1)
	notInReports.NotInReports = true
	replaceStore(t, home, spendRows([]store.Account{notInReports},
		spendSplit{id: "out", account: "acct-out", currency: "CAD", day: day(2026, time.January, 5), cents: 500}))
}

func Test_run_networth_before_the_first_transaction_prints_the_caption_and_the_first_balance_warning(t *testing.T) {
	seedNetWorthStore(t)

	stdout, stderr := mustRunNetWorth(t, "--as-of", "2026-03-01")

	assert.Equal(t, "Net worth on 2026-03-01, amounts in CAD\n\nType  Currency  Balance  In CAD\n", stdout)
	assert.Equal(t, beforeSnapshotWarning, stderr)
}

func Test_run_networth_before_the_first_transaction_lists_each_month_end_without_a_total_and_warns(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStdout string
		wantStderr string
	}{
		{
			name:       "history",
			args:       []string{"--since", "2026-01", "--until", "2026-02"},
			wantStdout: "Net worth at each month end 2026-01-31 to 2026-02-28, amounts in CAD\n\nMonth end   Total\n2026-01-31\n2026-02-28\n",
			wantStderr: beforeHistoryWarning,
		},
		{
			name:       "native snapshot",
			args:       []string{"--as-of", "2026-03-01", "--currency", "native"},
			wantStdout: "Net worth on 2026-03-01\n\nType  Currency  Balance\n",
			wantStderr: beforeSnapshotWarning,
		},
		{
			name:       "native history",
			args:       []string{"--since", "2026-01", "--until", "2026-02", "--currency", "native"},
			wantStdout: "Net worth at each month end 2026-01-31 to 2026-02-28\n\nMonth end   Currency  Total\n2026-01-31\n2026-02-28\n",
			wantStderr: beforeHistoryWarning,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedNetWorthStore(t)

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), append([]string{"networth"}, c.args...), holdingsClock())

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, c.wantStdout, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

func Test_run_networth_json_before_the_first_transaction_keeps_the_empty_dates_and_carries_the_warning(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		want     map[string]any
		wantLine string
	}{
		{
			name: "snapshot",
			args: []string{"--as-of", "2026-03-01"},
			want: map[string]any{
				"as_of": "2026-03-01", "since": nil, "until": nil,
				"dates": []any{map[string]any{"date": "2026-03-01", "balances": []any{}, "totals": []any{}}},
			},
			wantLine: beforeSnapshotLine,
		},
		{
			name: "history",
			args: []string{"--since", "2026-01", "--until", "2026-02"},
			want: map[string]any{
				"as_of": nil, "since": "2026-01-01", "until": "2026-02-28",
				"dates": []any{
					map[string]any{"date": "2026-01-31", "balances": []any{}, "totals": []any{}},
					map[string]any{"date": "2026-02-28", "balances": []any{}, "totals": []any{}},
				},
			},
			wantLine: beforeHistoryLine,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedNetWorthStore(t)

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), append([]string{"networth", "--json"}, c.args...), holdingsClock())

			require.Equal(t, 0, exitCode, stderr.String())
			var got map[string]any
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
			c.want["currency"] = "CAD"
			c.want["warnings"] = []any{c.wantLine}
			assert.Equal(t, c.want, got)
			assert.Equal(t, "quarry: warning: "+c.wantLine+"\n", stderr.String())
		})
	}
}

func Test_run_networth_says_no_account_in_the_reports_has_data_when_only_an_uncounted_account_does(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantWarn   string
		wantStdout string
	}{
		{
			name:       "snapshot",
			args:       []string{"--as-of", "2026-03-01"},
			wantWarn:   "quarry: warning: no account has a balance on 2026-03-01; no account in Quicken's reports has transactions or holdings\n",
			wantStdout: "Net worth on 2026-03-01, amounts in CAD\n\nType  Currency  Balance  In CAD\n",
		},
		{
			name: "history",
			args: []string{"--since", "2026-01", "--until", "2026-02"},
			wantWarn: "quarry: warning: no account has a balance at any month end from 2026-01-31 to 2026-02-28; " +
				"no account in Quicken's reports has transactions or holdings\n",
			wantStdout: "Net worth at each month end 2026-01-31 to 2026-02-28, amounts in CAD\n\nMonth end   Total\n2026-01-31\n2026-02-28\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedUncountedOnlyStore(t)

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), append([]string{"networth"}, c.args...), holdingsClock())

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, c.wantStdout, stdout.String())
			assert.Equal(t, c.wantWarn, stderr.String())
		})
	}
}

func Test_run_networth_on_the_first_balance_day_does_not_say_no_account_has_a_balance(t *testing.T) {
	seedNetWorthStore(t)

	stdout, stderr := mustRunNetWorth(t, "--as-of", "2026-03-02")

	assert.Contains(t, stdout, "chequing")
	assert.NotContains(t, stderr, "no account has a balance")
}

// netWorthHistoryLine is one converted history table line over the chequing and credit_card columns.
func netWorthHistoryLine(monthEnd, chequing, creditCard, total string) string {
	return fmt.Sprintf("%-10s  %8s  %11s  %8s\n", monthEnd, chequing, creditCard, total)
}

// seedNetWorthHistoryStore builds the store under a temp HOME with CAD chequing, a CAD credit card and USD
// chequing, each with a balance on 2026-01-31, 2026-02-28 and 2026-03-12; the USD rate is 1.36 from January 2.
func seedNetWorthHistoryStore(t *testing.T) {
	t.Helper()
	seedNetWorthHistoryStoreWithRates(t, usdRate(day(2026, time.January, 2), 1_360_000))
}

// seedNetWorthHistoryStoreWithRates is seedNetWorthHistoryStore holding rates, none when none are given.
func seedNetWorthHistoryStoreWithRates(t *testing.T, rates ...store.Rate) {
	t.Helper()
	home := newHome(t)
	card := store.Account{ID: "acct-card", SourceID: 3, Name: "Card", Type: "credit_card", Currency: "CAD", Active: true}
	rows := spendRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2), card},
		spendSplit{id: "cad-jan", account: "acct-cad", currency: "CAD", day: day(2026, time.January, 15), cents: 100_000},
		spendSplit{id: "cad-feb", account: "acct-cad", currency: "CAD", day: day(2026, time.February, 10), cents: 50_000},
		spendSplit{id: "cad-mar", account: "acct-cad", currency: "CAD", day: day(2026, time.March, 5), cents: -20_000},
		spendSplit{id: "usd-jan", account: "acct-usd", currency: "USD", day: day(2026, time.January, 10), cents: 80_000},
		spendSplit{id: "usd-mar", account: "acct-usd", currency: "USD", day: day(2026, time.March, 3), cents: 20_000},
		spendSplit{id: "card-jan", account: "acct-card", currency: "CAD", day: day(2026, time.January, 20), cents: -25_000},
		spendSplit{id: "card-feb", account: "acct-card", currency: "CAD", day: day(2026, time.February, 15), cents: -10_000},
		spendSplit{id: "card-mar", account: "acct-card", currency: "CAD", day: day(2026, time.March, 8), cents: -5_000},
	)
	replaceStoreWithRates(t, home, rows, rates...)
}

func Test_run_networth_lists_each_month_end_with_a_column_per_type_ending_with_today(t *testing.T) {
	seedNetWorthHistoryStore(t)

	stdout, stderr := mustRunNetWorth(t, "--since", "2026-01", "--until", "2027")

	assert.Empty(t, stderr)
	assert.Equal(t, "Net worth at each month end 2026-01-31 to 2026-03-12, amounts in CAD\n\n"+
		netWorthHistoryLine("Month end", "chequing", "credit_card", "Total")+
		netWorthHistoryLine("2026-01-31", "2,088.00", "-250.00", "1,838.00")+
		netWorthHistoryLine("2026-02-28", "2,588.00", "-350.00", "2,238.00")+
		netWorthHistoryLine("2026-03-12", "2,660.00", "-400.00", "2,260.00"),
		stdout)
}

// netWorthHistoryNativeLine is one native history table line over the chequing and credit_card columns.
func netWorthHistoryNativeLine(monthEnd, currency, chequing, creditCard, total string) string {
	return fmt.Sprintf("%-10s  %-8s  %8s  %11s  %6s\n", monthEnd, currency, chequing, creditCard, total)
}

// HOME holds no store: exit 2 (not the missing-store 1) shows each check runs first.
func Test_run_networth_rejects_a_period_it_cannot_list(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name:       "a since that is not a date",
			args:       []string{"networth", "--since", "2026-13"},
			wantStderr: "quarry: --since \"2026-13\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n",
		},
		{
			name:       "an until that is not a date",
			args:       []string{"networth", "--since", "2026", "--until", "yesterday"},
			wantStderr: "quarry: --until \"yesterday\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n",
		},
		{
			name:       "a since after until",
			args:       []string{"networth", "--since", "2026-03", "--until", "2026-02"},
			wantStderr: "quarry: --since 2026-03 is after --until 2026-02\n",
		},
		{
			name:       "an until alone before the default since",
			args:       []string{"networth", "--until", "2025-12"},
			wantStderr: "quarry: --until 2025-12 is before the default --since 2026-01-01; pass --since too\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), c.args, holdingsClock())

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

func Test_run_networth_lists_from_january_first_when_only_until_is_given(t *testing.T) {
	seedNetWorthHistoryStore(t)

	stdout, _ := mustRunNetWorth(t, "--until", "2026-02")

	assert.Equal(t, "Net worth at each month end 2026-01-31 to 2026-02-28, amounts in CAD\n\n"+
		netWorthHistoryLine("Month end", "chequing", "credit_card", "Total")+
		netWorthHistoryLine("2026-01-31", "2,088.00", "-250.00", "1,838.00")+
		netWorthHistoryLine("2026-02-28", "2,588.00", "-350.00", "2,238.00"),
		stdout)
}

func Test_run_networth_lists_only_today_when_the_since_is_in_this_month(t *testing.T) {
	seedNetWorthHistoryStore(t)

	stdout, _ := mustRunNetWorth(t, "--since", "2026-03-05")

	assert.Equal(t, "Net worth at each month end 2026-03-12 to 2026-03-12, amounts in CAD\n\n"+
		netWorthHistoryLine("Month end", "chequing", "credit_card", "Total")+
		netWorthHistoryLine("2026-03-12", "2,660.00", "-400.00", "2,260.00"),
		stdout)
}

func Test_run_networth_lists_one_line_per_currency_with_a_total_each_in_native_mode(t *testing.T) {
	seedNetWorthHistoryStore(t)

	stdout, _ := mustRunNetWorth(t, "--since", "2026-01", "--until", "2026-01", "--currency", "native")

	assert.Equal(t, "Net worth at each month end 2026-01-31 to 2026-01-31\n\n"+
		netWorthHistoryNativeLine("Month end", "Currency", "chequing", "credit_card", "Total")+
		netWorthHistoryNativeLine("2026-01-31", "CAD", "1,000.00", "-250.00", "750.00")+
		netWorthHistoryNativeLine("2026-01-31", "USD", "800.00", "", "800.00"),
		stdout)
}

func Test_run_networth_json_history_names_the_period_and_every_month_end(t *testing.T) {
	seedNetWorthHistoryStore(t)

	stdout, _ := mustRunNetWorth(t, "--json", "--since", "2026-01", "--until", "2027")

	var got struct {
		AsOf  *string `json:"as_of"`
		Since string  `json:"since"`
		Until string  `json:"until"`
		Dates []struct {
			Date string `json:"date"`
		} `json:"dates"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Nil(t, got.AsOf)
	assert.Equal(t, "2026-01-01", got.Since)
	assert.Equal(t, "2026-03-12", got.Until)
	require.Len(t, got.Dates, 3)
	assert.Equal(t, []string{"2026-01-31", "2026-02-28", "2026-03-12"},
		[]string{got.Dates[0].Date, got.Dates[1].Date, got.Dates[2].Date})
}

const (
	leftOutNoPriceLine = `quarry: warning: "Brokerage" holds 1 security with no price on or before 2026-03-12, ` +
		`so its balance leaves it out; enter a price in Quicken, then run quarry sync`
	leftOutNoCurrencyLine = `quarry: warning: "Mystery Fund" has no currency in Quicken, so quarry leaves its value out of ` +
		`"Brokerage"'s balance; set its currency in Quicken, then run quarry sync`
	leftOutOtherCurrencyLine = `quarry: warning: "Euro Fund" is priced in EUR, which quarry does not convert, ` +
		`so its value is left out of "Brokerage"'s balance`
)

// seedLeftOutHoldingsStore stores leftOutHoldingsRows under a fresh HOME.
func seedLeftOutHoldingsStore(t *testing.T) string {
	t.Helper()
	return seedLeftOutHoldingsRows(t, leftOutHoldingsRows())
}

// seedLeftOutHoldingsRows stores rows under a fresh HOME, which it returns.
func seedLeftOutHoldingsRows(t *testing.T, rows store.Rows) string {
	t.Helper()
	home := newHome(t)
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
	return home
}

// mustRunNetWorth runs networth with args at holdingsClock and returns its stdout and stderr.
func mustRunNetWorth(t *testing.T, args ...string) (string, string) {
	t.Helper()

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), append([]string{"networth"}, args...), holdingsClock())

	require.Equal(t, 0, exitCode, stderr.String())
	return stdout.String(), stderr.String()
}

func unprefixed(line string) string { return strings.TrimPrefix(line, "quarry: warning: ") }

// leftOutHoldingsRows is holdingsRows plus, in the brokerage, 40 shares of Bare Fund (never priced),
// 5 of Mystery Fund (no currency) and 20 of Euro Fund (EUR), the last two priced on day 9.
func leftOutHoldingsRows() store.Rows {
	rows := holdingsRows()
	rows.Securities = append(rows.Securities,
		store.Security{ID: "sec-bare", SourceID: 4, Name: "Bare Fund", Ticker: new("BARE"), Currency: new("CAD")},
		store.Security{ID: "sec-null", SourceID: 5, Name: "Mystery Fund", Ticker: new("MYST")},
		store.Security{ID: "sec-eur", SourceID: 6, Name: "Euro Fund", Ticker: new("EURO"), Currency: new("EUR")})
	rows.InvestmentTransactions = append(rows.InvestmentTransactions,
		holdingsBuy("inv-bare", 4, "acct-cad", "sec-bare", "CAD", 40_000_000),
		holdingsBuy("inv-null", 5, "acct-cad", "sec-null", "CAD", 5_000_000),
		holdingsBuy("inv-eur", 6, "acct-cad", "sec-eur", "CAD", 20_000_000))
	rows.Prices = append(rows.Prices,
		store.Price{SecurityID: "sec-null", SourceID: 5, Date: holdingsDay(9), Price: 10_000_000},
		store.Price{SecurityID: "sec-eur", SourceID: 6, Date: holdingsDay(9), Price: 12_500_000})
	return rows
}

// accounts reads today from the store's own clock, so its as-of date is matched, not pinned.
func Test_run_networth_warns_about_each_holding_it_leaves_out_and_accounts_gives_the_same_lines(t *testing.T) {
	seedLeftOutHoldingsStore(t)
	var netWorthOut, netWorthErr, accountsOut, accountsErr bytes.Buffer

	netWorthExit := runWith(context.Background(), []string{"networth"}, spendEnvAt(&netWorthOut, &netWorthErr, holdingsClock()))
	accountsExit := runWith(context.Background(), []string{"accounts"}, spendEnvAt(&accountsOut, &accountsErr, holdingsClock()))

	require.Equal(t, 0, netWorthExit, netWorthErr.String())
	assert.Equal(t, leftOutNoPriceLine+"\n"+leftOutNoCurrencyLine+"\n"+leftOutOtherCurrencyLine+"\n", netWorthErr.String())
	require.Equal(t, 0, accountsExit, accountsErr.String())
	accountsLines := strings.Split(strings.TrimSuffix(accountsErr.String(), "\n"), "\n")
	require.Len(t, accountsLines, 3)
	assert.Regexp(t, `^quarry: warning: "Brokerage" holds 1 security with no price on or before \d{4}-\d{2}-\d{2}, `+
		`so its balance leaves it out; enter a price in Quicken, then run quarry sync$`, accountsLines[0])
	assert.Equal(t, []string{leftOutNoCurrencyLine, leftOutOtherCurrencyLine}, accountsLines[1:])
}

func Test_run_networth_history_counts_the_month_ends_on_which_an_account_held_an_unpriced_security(t *testing.T) {
	rows := leftOutHoldingsRows()
	rows.InvestmentTransactions[3].Date = time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
	seedLeftOutHoldingsRows(t, rows)

	_, stderr := mustRunNetWorth(t, "--since", "2026-01", "--until", "2027")

	assert.Equal(t, `quarry: warning: "Brokerage" holds a security with no price on 3 of the month ends listed, `+
		`so its balance leaves it out on those days; enter prices in Quicken, then run quarry sync`+"\n"+
		leftOutNoCurrencyLine+"\n"+leftOutOtherCurrencyLine+"\n", stderr)
}

func Test_run_networth_json_lists_the_config_warning_before_the_holdings_warnings(t *testing.T) {
	home := seedLeftOutHoldingsStore(t)
	writeConfig(t, home, "snapshot.keep = 3\n")

	stdout, stderr := mustRunNetWorth(t, "--json")

	var got struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Equal(t, []string{
		configPath(home) + ": unknown key snapshot.keep; quarry ignores it",
		unprefixed(leftOutNoPriceLine), unprefixed(leftOutNoCurrencyLine), unprefixed(leftOutOtherCurrencyLine),
	}, got.Warnings)
	assert.Equal(t, "quarry: warning: "+configShown+": unknown key snapshot.keep; quarry ignores it\n"+
		leftOutNoPriceLine+"\n"+leftOutNoCurrencyLine+"\n"+leftOutOtherCurrencyLine+"\n", stderr)
}

func Test_run_networth_in_native_currency_still_warns_about_a_holding_in_a_currency_quarry_does_not_convert(t *testing.T) {
	seedLeftOutHoldingsStore(t)

	_, stderr := mustRunNetWorth(t, "--currency", "native")

	assert.Equal(t, leftOutNoPriceLine+"\n"+leftOutNoCurrencyLine+"\n"+leftOutOtherCurrencyLine+"\n", stderr)
}

func Test_run_networth_as_of_a_day_before_the_prices_warns_that_every_holding_is_unpriced(t *testing.T) {
	seedLeftOutHoldingsStore(t)

	_, stderr := mustRunNetWorth(t, "--as-of", "2026-03-05")

	assert.Equal(t, `quarry: warning: "Brokerage" holds 4 securities with no price on or before 2026-03-05, `+
		`so its balance leaves them out; enter prices in Quicken, then run quarry sync`+"\n"+
		`quarry: warning: "IRA" holds 1 security with no price on or before 2026-03-05, `+
		`so its balance leaves it out; enter a price in Quicken, then run quarry sync`+"\n"+
		leftOutNoCurrencyLine+"\n", stderr)
}

func Test_run_networth_leaves_a_not_in_reports_accounts_unpriced_holding_out_of_its_warnings(t *testing.T) {
	rows := leftOutHoldingsRows()
	rows.Accounts = append(rows.Accounts, store.Account{
		ID: "acct-hidden", SourceID: 7, Name: "Hidden Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true, NotInReports: true,
	})
	rows.InvestmentTransactions = append(rows.InvestmentTransactions, holdingsBuy("inv-hidden", 7, "acct-hidden", "sec-bare", "CAD", 1_000_000))
	seedLeftOutHoldingsRows(t, rows)

	_, stderr := mustRunNetWorth(t)

	assert.Equal(t, leftOutNoPriceLine+"\n"+leftOutNoCurrencyLine+"\n"+leftOutOtherCurrencyLine+"\n", stderr)
}

func Test_run_networth_warns_about_a_closed_accounts_unpriced_holding(t *testing.T) {
	rows := leftOutHoldingsRows()
	rows.InvestmentTransactions = append(rows.InvestmentTransactions, holdingsBuy("inv-old-bare", 7, "acct-old", "sec-bare", "CAD", 1_000_000))
	seedLeftOutHoldingsRows(t, rows)

	_, stderr := mustRunNetWorth(t)

	assert.Equal(t, leftOutNoPriceLine+"\n"+
		`quarry: warning: "Old RRSP" holds 1 security with no price on or before 2026-03-12, `+
		`so its balance leaves it out; enter a price in Quicken, then run quarry sync`+"\n"+
		leftOutNoCurrencyLine+"\n"+leftOutOtherCurrencyLine+"\n", stderr)
}

func Test_run_networth_before_the_first_rate_prints_every_holding_warning_before_the_rate_line(t *testing.T) {
	rows := leftOutHoldingsRows()
	rows.InvestmentTransactions = append(rows.InvestmentTransactions, holdingsBuy("inv-vti-cad", 8, "acct-cad", "sec-vti", "CAD", 3_000_000))
	seedLeftOutHoldingsRows(t, rows)

	_, stderr := mustRunNetWorth(t, "--as-of", "2026-03-09")

	assert.Equal(t, `quarry: warning: "Brokerage" holds 1 security with no price on or before 2026-03-09, `+
		`so its balance leaves it out; enter a price in Quicken, then run quarry sync`+"\n"+
		leftOutNoCurrencyLine+"\n"+leftOutOtherCurrencyLine+"\n"+
		`quarry: warning: "Brokerage" holds 1 USD security valued on 2026-03-09, before 2026-03-10, `+
		`the first exchange rate in the store, so its CAD balance leaves it out`+"\n"+
		`quarry: warning: USD balances on 2026-03-09, before 2026-03-10, the first exchange rate in the store, `+
		`are not converted to CAD and are left out of the CAD total; pass --currency native to list them`+"\n", stderr)
}

func Test_run_networth_json_keeps_a_zero_balance_row_and_counts_a_closed_account_but_not_one_left_out_of_reports(t *testing.T) {
	seedNetWorthStore(t)

	stdout, stderr := mustRunNetWorth(t, "--json")

	assert.Empty(t, stderr)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Equal(t, map[string]any{
		"as_of": "2026-03-12", "since": nil, "until": nil, "currency": "CAD",
		"dates": []any{map[string]any{
			"date": "2026-03-12",
			"balances": []any{
				map[string]any{"type": "brokerage", "currency": "USD", "balance": "920.00", "converted_balance": "1251.20"},
				map[string]any{"type": "chequing", "currency": "CAD", "balance": "1025.00", "converted_balance": "1025.00"},
				map[string]any{"type": "chequing", "currency": "USD", "balance": "800.00", "converted_balance": "1088.00"},
				map[string]any{"type": "credit_card", "currency": "CAD", "balance": "-250.50", "converted_balance": "-250.50"},
				map[string]any{"type": "savings", "currency": "CAD", "balance": "0.00", "converted_balance": "0.00"},
			},
			"totals": []any{map[string]any{"currency": "CAD", "value": "3113.70"}},
		}},
		"warnings": []any{},
	}, got)
}

func Test_run_networth_json_lists_each_currency_total_and_no_converted_balance_in_native_mode(t *testing.T) {
	seedNetWorthStore(t)

	stdout, _ := mustRunNetWorth(t, "--json", "--currency", "native")

	var got struct {
		Currency string `json:"currency"`
		Dates    []struct {
			Balances []map[string]any `json:"balances"`
			Totals   []map[string]any `json:"totals"`
		} `json:"dates"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got.Dates, 1)
	assert.Equal(t, "native", got.Currency)
	assert.Equal(t, []map[string]any{
		{"currency": "CAD", "value": "774.50"}, {"currency": "USD", "value": "1720.00"},
	}, got.Dates[0].Totals)
	assert.Len(t, got.Dates[0].Balances, 5)
	assert.Equal(t, []any{nil, nil, nil, nil, nil}, convertedBalances(got.Dates[0].Balances))
}

func convertedBalances(balances []map[string]any) []any {
	converted := make([]any, len(balances))
	for i, b := range balances {
		converted[i] = b["converted_balance"]
	}
	return converted
}

const (
	noRateHoldingBeforeFirstRate = `quarry: warning: "Brokerage" holds 1 USD security valued on %s, before %s, ` +
		`the first exchange rate in the store, so its CAD balance leaves it out` + "\n"
	noRateHoldingNoRates = `quarry: warning: "Brokerage" holds a USD security and the store has no exchange rates, ` +
		`so its CAD balance leaves it out; run quarry sync to fetch rates` + "\n"
)

// noRateHoldingRows is holdingsRows plus 10 shares of Dollar Fund, priced in USD, in the CAD brokerage from
// 2026-01-15, priced on 2026-01-20.
func noRateHoldingRows() store.Rows {
	rows := holdingsRows()
	rows.Securities = append(rows.Securities,
		store.Security{ID: "sec-usd", SourceID: 4, Name: "Dollar Fund", Ticker: new("DOLR"), Currency: new("USD")})
	buy := holdingsBuy("inv-usd", 4, "acct-cad", "sec-usd", "CAD", 10_000_000)
	buy.Date = day(2026, time.January, 15)
	rows.InvestmentTransactions = append(rows.InvestmentTransactions, buy)
	rows.Prices = append(rows.Prices, store.Price{SecurityID: "sec-usd", SourceID: 4, Date: day(2026, time.January, 20), Price: 10_000_000})
	return rows
}

// seedNoRateHoldingStore stores noRateHoldingRows under a fresh HOME with rates.
func seedNoRateHoldingStore(t *testing.T, rates ...store.Rate) {
	t.Helper()
	home := newHome(t)
	replaceStoreWithRates(t, home, noRateHoldingRows(), rates...)
}

func Test_run_networth_before_the_first_rate_warns_that_a_usd_holding_in_a_cad_account_is_left_out(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "in the reporting currency", args: []string{"--as-of", "2026-02-28"}},
		{name: "in native currency", args: []string{"--currency", "native", "--as-of", "2026-02-28"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seedNoRateHoldingStore(t, usdRate(holdingsDay(10), 1_360_000))

			_, stderr := mustRunNetWorth(t, c.args...)

			assert.Equal(t, fmt.Sprintf(noRateHoldingBeforeFirstRate, "2026-02-28", "2026-03-10"), stderr)
		})
	}
}

func Test_run_networth_history_counts_the_month_ends_a_usd_holding_in_a_cad_account_lacked_a_rate(t *testing.T) {
	seedNoRateHoldingStore(t, usdRate(holdingsDay(10), 1_360_000))

	_, stderr := mustRunNetWorth(t, "--since", "2026-01", "--until", "2026-03")

	assert.Equal(t, `quarry: warning: "Brokerage" holds a USD security on 2 of the month ends listed, before 2026-03-10, `+
		`the first exchange rate in the store, so its CAD balance leaves it out on those days`+"\n", stderr)
}

func Test_run_networth_in_a_store_with_no_rates_warns_of_the_holding_before_the_rate_line(t *testing.T) {
	seedNoRateHoldingStore(t)

	_, stderr := mustRunNetWorth(t)

	assert.Equal(t, noRateHoldingNoRates+fmt.Sprintf(netWorthNoRatesNote, "USD", "CAD", "CAD"), stderr)
}

// accounts reads today from the store's own clock, so its as-of date is matched, not pinned.
func Test_run_accounts_warns_of_a_usd_holding_in_a_cad_account_before_the_first_rate_before_the_rate_line(t *testing.T) {
	seedNoRateHoldingStore(t, usdRate(day(2099, time.January, 1), 1_360_000))

	exitCode, _, stderr := runSpendCaptureAt(context.Background(), []string{"accounts"}, holdingsClock())

	require.Equal(t, 0, exitCode, stderr.String())
	lines := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n")
	require.Len(t, lines, 2)
	assert.Regexp(t, `^quarry: warning: "Brokerage" holds 1 USD security valued on \d{4}-\d{2}-\d{2}, before 2099-01-01, `+
		`the first exchange rate in the store, so its CAD balance leaves it out$`, lines[0])
	assert.Equal(t, "quarry: warning: the first exchange rate in the store, 2099-01-01, is dated after today, "+
		"so USD balances show no rate in the In CAD column; check the Mac's date and time", lines[1])
}

func Test_run_accounts_in_a_store_with_no_rates_warns_of_the_holding_before_the_rate_line(t *testing.T) {
	seedNoRateHoldingStore(t)

	exitCode, _, stderr := runSpendCaptureAt(context.Background(), []string{"accounts"}, holdingsClock())

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, noRateHoldingNoRates+"quarry: warning: the store has no exchange rates, so USD balances show no rate in the "+
		"In CAD column; run quarry sync to fetch them\n", stderr.String())
}

const netWorthBeforeFirstRateLine = "USD balances on 2026-03-05, before 2026-03-10, the first exchange rate in the store, " +
	"are not converted to CAD and are left out of the CAD total; pass --currency native to list them"

func Test_run_networth_warns_when_a_usd_balance_has_no_exchange_rate_and_totals_it_apart(t *testing.T) {
	seedNetWorthStore(t)

	stdout, stderr := mustRunNetWorth(t, "--as-of", "2026-03-05")

	assert.Equal(t, "quarry: warning: "+netWorthBeforeFirstRateLine+"\n", stderr)
	assert.Equal(t, "Net worth on 2026-03-05, amounts in CAD\n\n"+
		netWorthLine("Type", "Currency", "Balance", "In CAD")+
		netWorthLine("brokerage", "USD", "920.00", "no rate")+
		netWorthLine("chequing", "CAD", "1,025.00", "1,025.00")+
		netWorthLine("chequing", "USD", "800.00", "no rate")+
		netWorthLine("credit_card", "CAD", "-250.50", "-250.50")+
		netWorthLine("Total", "", "", "774.50")+
		netWorthLine("Total", "USD", "1,720.00", ""),
		stdout)
}

// netWorthRateNote is the stderr warning that other balances, from when on, are not converted to reporting.
func netWorthRateNote(other, reporting, when string) string {
	return fmt.Sprintf("quarry: warning: %s balances %s the first exchange rate in the store, are not converted to %s and are "+
		"left out of the %s total; pass --currency native to list them\n", other, when, reporting, reporting)
}

const netWorthNoRatesNote = "quarry: warning: the store has no exchange rates, so %s balances are not converted to %s and are " +
	"left out of the %s total; pass --currency native to list them, or run quarry sync to fetch rates\n"

// seedNetWorthCADOnlyStore builds a store of a CAD chequing and a CAD card with a balance on 2026-01-15 and
// 2026-01-20, and no exchange rates.
func seedNetWorthCADOnlyStore(t *testing.T) {
	t.Helper()
	home := newHome(t)
	card := store.Account{ID: "acct-card", SourceID: 3, Name: "Card", Type: "credit_card", Currency: "CAD", Active: true}
	replaceStore(t, home, spendRows([]store.Account{chequingAccount("acct-cad", 1), card},
		spendSplit{id: "cad", account: "acct-cad", currency: "CAD", day: day(2026, time.January, 15), cents: 100_000},
		spendSplit{id: "card", account: "acct-card", currency: "CAD", day: day(2026, time.January, 20), cents: -25_000},
	))
}

func Test_run_networth_history_before_the_first_rate_totals_the_converting_rows_and_counts_the_month_ends(t *testing.T) {
	seedNetWorthHistoryStoreWithRates(t, usdRate(day(2026, time.February, 15), 1_360_000))

	stdout, stderr := mustRunNetWorth(t, "--since", "2026-01", "--until", "2026-03")

	assert.Equal(t, netWorthRateNote("USD", "CAD", "on 1 month end before 2026-02-15,"), stderr)
	assert.Equal(t, "Net worth at each month end 2026-01-31 to 2026-03-12, amounts in CAD\n\n"+
		netWorthHistoryLine("Month end", "chequing", "credit_card", "Total")+
		netWorthHistoryLine("2026-01-31", "1,000.00", "-250.00", "750.00")+
		netWorthHistoryLine("2026-02-28", "2,588.00", "-350.00", "2,238.00")+
		netWorthHistoryLine("2026-03-12", "2,660.00", "-400.00", "2,260.00"),
		stdout)
}

func Test_run_networth_history_says_no_rate_in_the_total_of_a_month_end_whose_rows_all_need_a_rate(t *testing.T) {
	seedNetWorthCADOnlyStore(t)

	stdout, stderr := mustRunNetWorth(t, "--since", "2026-01", "--until", "2026-02", "--currency", "USD")

	assert.Equal(t, fmt.Sprintf(netWorthNoRatesNote, "CAD", "USD", "USD"), stderr)
	assert.Equal(t, "Net worth at each month end 2026-01-31 to 2026-02-28, amounts in USD\n\n"+
		"Month end   chequing  credit_card    Total\n"+
		"2026-01-31   no rate      no rate  no rate\n"+
		"2026-02-28   no rate      no rate  no rate\n",
		stdout)
}

func Test_run_networth_in_a_store_with_no_rates_warns_of_it_and_totals_the_usd_balances_apart(t *testing.T) {
	seedNetWorthHistoryStoreWithRates(t)

	stdout, stderr := mustRunNetWorth(t, "--as-of", "2026-01-31")

	assert.Equal(t, fmt.Sprintf(netWorthNoRatesNote, "USD", "CAD", "CAD"), stderr)
	assert.Equal(t, "Net worth on 2026-01-31, amounts in CAD\n\n"+
		netWorthLine("Type", "Currency", "Balance", "In CAD")+
		netWorthLine("chequing", "CAD", "1,000.00", "1,000.00")+
		netWorthLine("chequing", "USD", "800.00", "no rate")+
		netWorthLine("credit_card", "CAD", "-250.00", "-250.00")+
		netWorthLine("Total", "", "", "750.00")+
		netWorthLine("Total", "USD", "800.00", ""),
		stdout)
}

func Test_run_networth_in_usd_warns_of_cad_balances_before_the_first_rate_and_totals_them_apart(t *testing.T) {
	seedNetWorthHistoryStoreWithRates(t, usdRate(day(2026, time.February, 15), 1_360_000))

	stdout, stderr := mustRunNetWorth(t, "--as-of", "2026-01-31", "--currency", "USD")

	assert.Equal(t, netWorthRateNote("CAD", "USD", "on 2026-01-31, before 2026-02-15,"), stderr)
	assert.Equal(t, "Net worth on 2026-01-31, amounts in USD\n\n"+
		"Type         Currency   Balance   In USD\n"+
		"chequing     CAD       1,000.00  no rate\n"+
		"chequing     USD         800.00   800.00\n"+
		"credit_card  CAD        -250.00  no rate\n"+
		"Total                             800.00\n"+
		"Total        CAD         750.00\n",
		stdout)
}

func Test_run_networth_native_before_the_first_rate_has_no_warning_and_no_no_rate_cell(t *testing.T) {
	seedNetWorthStore(t)

	stdout, stderr := mustRunNetWorth(t, "--as-of", "2026-03-05", "--currency", "native")

	assert.Empty(t, stderr)
	assert.NotContains(t, stdout, "no rate")
	assert.Contains(t, stdout, netWorthNativeLine("Total", "USD", "1,720.00"))
}

func Test_run_networth_json_history_before_the_first_rate_totals_only_that_month_end_apart(t *testing.T) {
	seedNetWorthHistoryStoreWithRates(t, usdRate(day(2026, time.February, 15), 1_360_000))

	stdout, _ := mustRunNetWorth(t, "--since", "2026-01", "--until", "2026-03", "--json")

	var got struct {
		Dates []struct {
			Balances []map[string]any `json:"balances"`
			Totals   []map[string]any `json:"totals"`
		} `json:"dates"`
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got.Dates, 3)
	assert.Equal(t, []any{"1000.00", nil, "-250.00"}, convertedBalances(got.Dates[0].Balances))
	assert.Equal(t, []map[string]any{
		{"currency": "CAD", "value": "750.00"}, {"currency": "USD", "value": "800.00"},
	}, got.Dates[0].Totals)
	assert.Equal(t, []map[string]any{{"currency": "CAD", "value": "2238.00"}}, got.Dates[1].Totals)
	assert.Equal(t, []string{"USD balances on 1 month end before 2026-02-15, the first exchange rate in the store, " +
		"are not converted to CAD and are left out of the CAD total; pass --currency native to list them"}, got.Warnings)
}

func Test_run_networth_json_before_the_first_rate_nulls_the_converted_balance_and_carries_the_total_apart(t *testing.T) {
	seedNetWorthStore(t)

	stdout, _ := mustRunNetWorth(t, "--as-of", "2026-03-05", "--json")

	var got struct {
		Dates []struct {
			Balances []map[string]any `json:"balances"`
			Totals   []map[string]any `json:"totals"`
		} `json:"dates"`
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got.Dates, 1)
	assert.Equal(t, []any{nil, "1025.00", nil, "-250.50", "0.00"}, convertedBalances(got.Dates[0].Balances))
	assert.Equal(t, []map[string]any{
		{"currency": "CAD", "value": "774.50"}, {"currency": "USD", "value": "1720.00"},
	}, got.Dates[0].Totals)
	assert.Equal(t, []string{netWorthBeforeFirstRateLine}, got.Warnings)
}

// Needs run(): the help text is assembled with the flags, not by calling a command directly.
// The expected blocks are hand copies at the help's wrap width, so a re-wrap fails here.
func Test_run_networth_help_carries_its_copy_and_the_currency_flag(t *testing.T) {
	t.Setenv("HOME", "")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"networth", "--help"})

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), `Show net worth on one day (--as-of, default today), or at the end of each
month from --since to --until: account balances added up by account type
and currency. A balance is the sum of the account's transactions dated
that day or earlier; a brokerage or retirement account adds the value of
its holdings that day, each at the latest price Quicken recorded on or
before it (quarry holdings lists them). Credit card, loan and other
liability balances are negative, so they reduce the total. Closed
accounts count with their balance on the day. Accounts Quicken leaves out
of reports ("not in reports" or "linked tracking" in quarry accounts) are
left out, as Quicken's reports do.

Amounts are in CAD unless --currency or reporting.currency in
~/Library/Application Support/quarry/config.toml names another currency.
Each account's balance converts at the Bank of Canada rate for the day it
is valued on, or the latest earlier rate, and is rounded to the cent
before it is added. With --currency native, CAD and USD are listed
separately, never added together.

Month ends after today are not listed; a history that reaches this month
ends with today.
`)
	assert.Contains(t, stdout.String(), `Examples:
  quarry networth
  quarry networth --as-of 2025-12-31
  quarry networth --since 2020 --currency native --json
`)
	assert.Contains(t, stdout.String(), "      --as-of date      value net worth on date "+
		"(YYYY, YYYY-MM or YYYY-MM-DD; a year or month means its last day; default today)\n")
	assert.Contains(t, stdout.String(), "      --since date      list net worth at each month end on or after date "+
		"(YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year when --until is given)\n")
	assert.Contains(t, stdout.String(), "      --until date      list net worth at each month end on or before date "+
		"(YYYY, YYYY-MM or YYYY-MM-DD; default today; a later date means today)\n")
	assert.Contains(t, stdout.String(), "      --currency code   show amounts in currency code: CAD, USD, or native for each account's own "+
		"(default reporting.currency in the config file, else CAD)\n")
}

func Test_run_sql_sums_net_worth_by_type_and_currency_over_the_accounts_quickens_reports_count(t *testing.T) {
	home := newHome(t)
	day := func(d int) time.Time { return time.Date(2026, time.March, d, 0, 0, 0, 0, time.UTC) }
	closed := chequingAccount("acct-closed", 2)
	closed.Name, closed.Closed, closed.Active = "Old Chequing", true, false
	notInReports := chequingAccount("acct-out", 4)
	notInReports.Name, notInReports.NotInReports = "Not In Reports", true
	linked := chequingAccount("acct-linked", 5)
	linked.Name, linked.LinkedTracking = "Linked", true
	rows := spendRows(
		[]store.Account{chequingAccount("acct-cad", 1), closed, usdChequingAccount("acct-usd", 3), notInReports, linked},
		spendSplit{id: "cad", account: "acct-cad", currency: "CAD", day: day(2), cents: 100_000},
		spendSplit{id: "closed", account: "acct-closed", currency: "CAD", day: day(2), cents: 25_000},
		spendSplit{id: "usd", account: "acct-usd", currency: "USD", day: day(2), cents: 80_000},
		spendSplit{id: "out", account: "acct-out", currency: "CAD", day: day(2), cents: 5_000},
		spendSplit{id: "linked", account: "acct-linked", currency: "CAD", day: day(2), cents: 7_000},
	)
	replaceStoreWithRates(t, home, rows, usdRate(day(5), 1_250_000))
	const query = `SELECT date, type, currency, accounts, balance, balance_cad, balance_usd
		FROM v_net_worth WHERE date = '2026-03-10' ORDER BY currency`

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sql", "--csv", query})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"date,type,currency,accounts,balance,balance_cad,balance_usd\n"+
		"2026-03-10,chequing,CAD,2,1250.00,1250.00,1000.00\n"+
		"2026-03-10,chequing,USD,1,800.00,1000.00,800.00\n",
		stdout.String())
}
