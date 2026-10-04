package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
