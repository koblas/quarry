package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// runHoldingsOn stores rows and runs holdings with args at holdingsClock.
func runHoldingsOn(t *testing.T, rows store.Rows, args ...string) (int, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), append([]string{"holdings"}, args...), spendEnvAt(&stdout, &stderr, holdingsClock()))

	return exitCode, stdout.String(), stderr.String()
}

func Test_run_holdings_before_the_first_investment_transaction_warns_where_they_start_and_prints_no_total(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rows := holdingsRows()
	rows.InvestmentTransactions[2].Date = holdingsDay(5)
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings", "--as-of", "2026-03-01"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Holdings on 2026-03-01 in all accounts, amounts in CAD; cash not included\n\n"+
		"Account  Security  Shares  Price  Priced on  Currency  Value  In CAD\n", stdout.String())
	assert.Equal(t, "quarry: warning: no holdings on 2026-03-01; the store's investment transactions run 2026-03-02 to 2026-03-05\n",
		stderr.String())
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
