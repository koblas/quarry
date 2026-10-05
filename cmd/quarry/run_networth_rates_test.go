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

const netWorthBeforeFirstRateLine = "USD balances on 2026-03-05, before 2026-03-10, the first exchange rate in the store, " +
	"are not converted to CAD and are left out of the CAD total; pass --currency native to list them"

// netWorthNoRateLine is netWorthLine trimmed, for a line whose In column may be blank.
func netWorthNoRateLine(typ, currency, balance, in string) string {
	return strings.TrimRight(netWorthLine(typ, currency, balance, in), " \n") + "\n"
}

func Test_run_networth_warns_when_a_usd_balance_has_no_exchange_rate_and_totals_it_apart(t *testing.T) {
	seedNetWorthStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--as-of", "2026-03-05"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: "+netWorthBeforeFirstRateLine+"\n", stderr.String())
	assert.Equal(t, "Net worth on 2026-03-05, amounts in CAD\n\n"+
		netWorthNoRateLine("Type", "Currency", "Balance", "In CAD")+
		netWorthNoRateLine("brokerage", "USD", "920.00", "no rate")+
		netWorthNoRateLine("chequing", "CAD", "1,025.00", "1,025.00")+
		netWorthNoRateLine("chequing", "USD", "800.00", "no rate")+
		netWorthNoRateLine("credit_card", "CAD", "-250.50", "-250.50")+
		netWorthNoRateLine("Total", "", "", "774.50")+
		netWorthNoRateLine("Total", "USD", "1,720.00", ""),
		stdout.String())
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
	home := t.TempDir()
	t.Setenv("HOME", home)
	card := store.Account{ID: "acct-card", SourceID: 3, Name: "Card", Type: "credit_card", Currency: "CAD", Active: true}
	replaceStore(t, home, spendRows([]store.Account{chequingAccount("acct-cad", 1), card},
		spendSplit{id: "cad", account: "acct-cad", currency: "CAD", day: day(2026, time.January, 15), cents: 100_000},
		spendSplit{id: "card", account: "acct-card", currency: "CAD", day: day(2026, time.January, 20), cents: -25_000},
	))
}

func Test_run_networth_history_before_the_first_rate_totals_the_converting_rows_and_counts_the_month_ends(t *testing.T) {
	seedNetWorthHistoryStoreWithRates(t, usdRate(day(2026, time.February, 15), 1_360_000))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--since", "2026-01", "--until", "2026-03"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, netWorthRateNote("USD", "CAD", "on 1 month end before 2026-02-15,"), stderr.String())
	assert.Equal(t, "Net worth at each month end 2026-01-31 to 2026-03-12, amounts in CAD\n\n"+
		netWorthHistoryLine("Month end", "chequing", "credit_card", "Total")+
		netWorthHistoryLine("2026-01-31", "1,000.00", "-250.00", "750.00")+
		netWorthHistoryLine("2026-02-28", "2,588.00", "-350.00", "2,238.00")+
		netWorthHistoryLine("2026-03-12", "2,660.00", "-400.00", "2,260.00"),
		stdout.String())
}

func Test_run_networth_history_says_no_rate_in_the_total_of_a_month_end_whose_rows_all_need_a_rate(t *testing.T) {
	seedNetWorthCADOnlyStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--since", "2026-01", "--until", "2026-02", "--currency", "USD"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, fmt.Sprintf(netWorthNoRatesNote, "CAD", "USD", "USD"), stderr.String())
	assert.Equal(t, "Net worth at each month end 2026-01-31 to 2026-02-28, amounts in USD\n\n"+
		"Month end   chequing  credit_card    Total\n"+
		"2026-01-31   no rate      no rate  no rate\n"+
		"2026-02-28   no rate      no rate  no rate\n",
		stdout.String())
}

func Test_run_networth_in_a_store_with_no_rates_warns_of_it_and_totals_the_usd_balances_apart(t *testing.T) {
	seedNetWorthHistoryStoreWithRates(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--as-of", "2026-01-31"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, fmt.Sprintf(netWorthNoRatesNote, "USD", "CAD", "CAD"), stderr.String())
	assert.Equal(t, "Net worth on 2026-01-31, amounts in CAD\n\n"+
		netWorthNoRateLine("Type", "Currency", "Balance", "In CAD")+
		netWorthNoRateLine("chequing", "CAD", "1,000.00", "1,000.00")+
		netWorthNoRateLine("chequing", "USD", "800.00", "no rate")+
		netWorthNoRateLine("credit_card", "CAD", "-250.00", "-250.00")+
		netWorthNoRateLine("Total", "", "", "750.00")+
		netWorthNoRateLine("Total", "USD", "800.00", ""),
		stdout.String())
}

func Test_run_networth_in_usd_warns_of_cad_balances_before_the_first_rate_and_totals_them_apart(t *testing.T) {
	seedNetWorthHistoryStoreWithRates(t, usdRate(day(2026, time.February, 15), 1_360_000))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--as-of", "2026-01-31", "--currency", "USD"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, netWorthRateNote("CAD", "USD", "on 2026-01-31, before 2026-02-15,"), stderr.String())
	assert.Equal(t, "Net worth on 2026-01-31, amounts in USD\n\n"+
		"Type         Currency   Balance   In USD\n"+
		"chequing     CAD       1,000.00  no rate\n"+
		"chequing     USD         800.00   800.00\n"+
		"credit_card  CAD        -250.00  no rate\n"+
		"Total                             800.00\n"+
		"Total        CAD         750.00\n",
		stdout.String())
}

func Test_run_networth_native_before_the_first_rate_has_no_warning_and_no_no_rate_cell(t *testing.T) {
	seedNetWorthStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--as-of", "2026-03-05", "--currency", "native"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.NotContains(t, stdout.String(), "no rate")
	assert.Contains(t, stdout.String(), netWorthNativeLine("Total", "USD", "1,720.00"))
}

func Test_run_networth_json_history_before_the_first_rate_totals_only_that_month_end_apart(t *testing.T) {
	seedNetWorthHistoryStoreWithRates(t, usdRate(day(2026, time.February, 15), 1_360_000))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--since", "2026-01", "--until", "2026-03", "--json"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	var got struct {
		Dates []struct {
			Balances []map[string]any `json:"balances"`
			Totals   []map[string]any `json:"totals"`
		} `json:"dates"`
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
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
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--as-of", "2026-03-05", "--json"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	var got struct {
		Dates []struct {
			Balances []map[string]any `json:"balances"`
			Totals   []map[string]any `json:"totals"`
		} `json:"dates"`
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	require.Len(t, got.Dates, 1)
	assert.Equal(t, []any{nil, "1025.00", nil, "-250.50", "0.00"}, convertedBalances(got.Dates[0].Balances))
	assert.Equal(t, []map[string]any{
		{"currency": "CAD", "value": "774.50"}, {"currency": "USD", "value": "1720.00"},
	}, got.Dates[0].Totals)
	assert.Equal(t, []string{netWorthBeforeFirstRateLine}, got.Warnings)
}
