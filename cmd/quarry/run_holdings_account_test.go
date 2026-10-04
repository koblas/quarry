package main

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// holdingsAccountLine is one table line sized to the Brokerage-only listing: Acme Corp (ACME) is its widest security.
func holdingsAccountLine(account, security, shares, price, pricedOn, currency, value, in string) string {
	return fmt.Sprintf("%-9s  %-16s  %6s  %5s  %-10s  %-8s  %9s  %9s\n",
		account, security, shares, price, pricedOn, currency, value, in)
}

func Test_run_holdings_account_filter_lists_the_named_accounts_and_warns_for_chequing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rows := holdingsRows()
	rows.Accounts = append(rows.Accounts, chequingAccount("acct-chq", 4))
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings", "--account", "Brokerage", "--account", "Chequing"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Holdings on 2026-03-12 in Brokerage, Chequing, amounts in CAD; cash not included\n\n"+
		holdingsAccountLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsAccountLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsAccountLine("Total", "", "", "", "", "", "", "37,704.00"),
		stdout.String())
	assert.Equal(t, "quarry: warning: account \"Chequing\" is not a brokerage or retirement account, so it has no holdings\n",
		stderr.String())
}
