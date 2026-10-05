package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedWideBrokerageStore stores a CAD brokerage holding 999,999,999,999.999999 shares priced at the same, so its
// balance passes 64 bits of cents.
func seedWideBrokerageStore(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	const widest = 999_999_999_999_999_999
	rows := spendRows([]store.Account{brokerageAccount("acct-cad", 1, "CAD")})
	rows.Securities = []store.Security{{ID: "sec-wide", SourceID: 1, Name: "Wide Fund", Ticker: new("WIDE"), Currency: new("CAD")}}
	rows.InvestmentTransactions = []store.InvestmentTransaction{holdingsBuy("inv-wide", 1, "acct-cad", "sec-wide", "CAD", widest)}
	rows.Prices = []store.Price{{SecurityID: "sec-wide", SourceID: 1, Date: holdingsDay(2), Price: widest}}
	replaceStore(t, home, rows)
}

func Test_run_accounts_lists_a_balance_past_64_bits_in_exact_cents(t *testing.T) {
	seedWideBrokerageStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"accounts"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "999,999,999,999,999,998,000,000.00")
}

func Test_run_holdings_of_a_named_account_resolves_an_account_whose_balance_passes_64_bits(t *testing.T) {
	seedWideBrokerageStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings", "--account", "Brokerage"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "Wide Fund (WIDE)")
}
