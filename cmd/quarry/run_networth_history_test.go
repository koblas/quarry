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
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--since", "2026-01", "--until", "2027"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Net worth at each month end 2026-01-31 to 2026-03-12, amounts in CAD\n\n"+
		netWorthHistoryLine("Month end", "chequing", "credit_card", "Total")+
		netWorthHistoryLine("2026-01-31", "2,088.00", "-250.00", "1,838.00")+
		netWorthHistoryLine("2026-02-28", "2,588.00", "-350.00", "2,238.00")+
		netWorthHistoryLine("2026-03-12", "2,660.00", "-400.00", "2,260.00"),
		stdout.String())
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
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), c.args, spendEnvAt(&stdout, &stderr, holdingsClock()))

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

func Test_run_networth_lists_from_january_first_when_only_until_is_given(t *testing.T) {
	seedNetWorthHistoryStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--until", "2026-02"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Net worth at each month end 2026-01-31 to 2026-02-28, amounts in CAD\n\n"+
		netWorthHistoryLine("Month end", "chequing", "credit_card", "Total")+
		netWorthHistoryLine("2026-01-31", "2,088.00", "-250.00", "1,838.00")+
		netWorthHistoryLine("2026-02-28", "2,588.00", "-350.00", "2,238.00"),
		stdout.String())
}

func Test_run_networth_lists_only_today_when_the_since_is_in_this_month(t *testing.T) {
	seedNetWorthHistoryStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--since", "2026-03-05"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Net worth at each month end 2026-03-12 to 2026-03-12, amounts in CAD\n\n"+
		netWorthHistoryLine("Month end", "chequing", "credit_card", "Total")+
		netWorthHistoryLine("2026-03-12", "2,660.00", "-400.00", "2,260.00"),
		stdout.String())
}

func Test_run_networth_lists_one_line_per_currency_with_a_total_each_in_native_mode(t *testing.T) {
	seedNetWorthHistoryStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(),
		[]string{"networth", "--since", "2026-01", "--until", "2026-01", "--currency", "native"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Net worth at each month end 2026-01-31 to 2026-01-31\n\n"+
		netWorthHistoryNativeLine("Month end", "Currency", "chequing", "credit_card", "Total")+
		netWorthHistoryNativeLine("2026-01-31", "CAD", "1,000.00", "-250.00", "750.00")+
		netWorthHistoryNativeLine("2026-01-31", "USD", "800.00", "", "800.00"),
		stdout.String())
}

func Test_run_networth_json_history_names_the_period_and_every_month_end(t *testing.T) {
	seedNetWorthHistoryStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--json", "--since", "2026-01", "--until", "2027"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	var got struct {
		AsOf  *string `json:"as_of"`
		Since string  `json:"since"`
		Until string  `json:"until"`
		Dates []struct {
			Date string `json:"date"`
		} `json:"dates"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	assert.Nil(t, got.AsOf)
	assert.Equal(t, "2026-01-01", got.Since)
	assert.Equal(t, "2026-03-12", got.Until)
	require.Len(t, got.Dates, 3)
	assert.Equal(t, []string{"2026-01-31", "2026-02-28", "2026-03-12"},
		[]string{got.Dates[0].Date, got.Dates[1].Date, got.Dates[2].Date})
}
