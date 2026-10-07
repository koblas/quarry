package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	holdingsBeforeFirstRateLine = "1 holding valued on 2026-03-09, before 2026-03-10, the first exchange rate in the store, " +
		"is not converted to CAD and is totalled in USD"
	holdingsBeforeFirstRateUSDLine = "2 holdings valued on 2026-03-09, before 2026-03-10, the first exchange rate in the store, " +
		"are not converted to USD and are totalled in CAD"
	holdingsNoRatesWarningLine = "the store has no exchange rates, so values are listed in each security's own currency; " +
		"run quarry sync to fetch them"
)

// holdingsNoRateLine is one table line of seedHoldingsStore's holdings; the In column may be blank, so the line is trimmed.
func holdingsNoRateLine(account, security, shares, price, pricedOn, currency, value, in string) string {
	return strings.TrimRight(holdingsLine(account, security, shares, price, pricedOn, currency, value, in), " \n") + "\n"
}

func Test_run_holdings_before_the_first_rate_shows_no_rate_and_totals_usd_separately(t *testing.T) {
	seedHoldingsStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings", "--as-of", "2026-03-09"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: "+holdingsBeforeFirstRateLine+"\n", stderr.String())
	assert.Equal(t, "Holdings on 2026-03-09 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsNoRateLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsNoRateLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsNoRateLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35", "no rate")+
		holdingsNoRateLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00", "50.00")+
		holdingsNoRateLine("Total", "", "", "", "", "", "", "37,754.00")+
		holdingsNoRateLine("Total", "", "", "", "", "USD", "24,659.35", ""),
		stdout.String())
}

func Test_run_holdings_json_before_the_first_rate_lists_the_converted_total_then_the_usd_one_and_the_stderr_warning(t *testing.T) {
	seedHoldingsStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings", "--json", "--as-of", "2026-03-09"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	var doc holdingsJSONDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
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
	assert.Equal(t, "quarry: warning: "+doc.Warnings[0]+"\n", stderr.String())
}

func Test_run_holdings_in_usd_before_the_first_rate_shows_no_rate_for_the_cad_rows_and_totals_cad_separately(t *testing.T) {
	seedHoldingsStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings", "--currency", "USD", "--as-of", "2026-03-09"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: "+holdingsBeforeFirstRateUSDLine+"\n", stderr.String())
	assert.Equal(t, "Holdings on 2026-03-09 in all accounts, amounts in USD; cash not included\n\n"+
		holdingsNoRateLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In USD")+
		holdingsNoRateLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "no rate")+
		holdingsNoRateLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35", "24,659.35")+
		holdingsNoRateLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00", "no rate")+
		holdingsNoRateLine("Total", "", "", "", "", "", "", "24,659.35")+
		holdingsNoRateLine("Total", "", "", "", "", "CAD", "37,754.00", ""),
		stdout.String())
}

func Test_run_holdings_native_before_the_first_rate_has_no_no_rate_cell_and_no_warning(t *testing.T) {
	seedHoldingsStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings", "--currency", "native", "--as-of", "2026-03-09"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Holdings on 2026-03-09 in all accounts; cash not included\n\n"+
		holdingsNativeLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value")+
		holdingsNativeLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00")+
		holdingsNativeLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35")+
		holdingsNativeLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00")+
		holdingsNativeLine("Total", "", "", "", "", "CAD", "37,754.00")+
		holdingsNativeLine("Total", "", "", "", "", "USD", "24,659.35"),
		stdout.String())
}

func Test_run_holdings_in_a_store_with_no_rates_says_so_and_shows_no_rate_for_the_usd_row(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, holdingsRows())

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"holdings"}, holdingsClock())

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: "+holdingsNoRatesWarningLine+"\n", stderr.String())
	assert.Equal(t, "Holdings on 2026-03-12 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsNoRateLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsNoRateLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsNoRateLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35", "no rate")+
		holdingsNoRateLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00", "50.00")+
		holdingsNoRateLine("Total", "", "", "", "", "", "", "37,754.00")+
		holdingsNoRateLine("Total", "", "", "", "", "USD", "24,659.35", ""),
		stdout.String())
}

func Test_run_holdings_on_a_day_inside_a_rate_gap_converts_at_the_earlier_rate_and_is_silent(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, holdingsRows(), usdRate(holdingsDay(10), 1_360_000), usdRate(holdingsDay(12), 1_400_000))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings", "--as-of", "2026-03-11"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Holdings on 2026-03-11 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35", "33,536.72")+
		holdingsLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00", "50.00")+
		holdingsLine("Total", "", "", "", "", "", "", "71,290.72"),
		stdout.String())
}

func Test_run_holdings_of_cad_holdings_only_before_the_first_rate_in_cad_needs_no_rate(t *testing.T) {
	home := newHome(t)
	rows := holdingsRows()
	rows.InvestmentTransactions, rows.Prices = rows.InvestmentTransactions[:1], rows.Prices[:1]
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings", "--as-of", "2026-03-09"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.NotContains(t, stdout.String(), "no rate")
	assert.Regexp(t, `(?m)^Total +37,704\.00$`, stdout.String())
}
