package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedLowerCaseAccountHoldingsStore is seedHoldingsStore with its first account named in lower case, so
// plain byte order would put IRA and Old RRSP before it.
func seedLowerCaseAccountHoldingsStore(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	rows := holdingsRows()
	rows.Accounts[0].Name = "brokerage"
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
}

func Test_run_holdings_sorts_accounts_ignoring_case_in_the_table(t *testing.T) {
	seedLowerCaseAccountHoldingsStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Holdings on 2026-03-12 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsLine("brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35", "33,536.72")+
		holdingsLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00", "50.00")+
		holdingsLine("Total", "", "", "", "", "", "", "71,290.72"),
		stdout.String())
}

func Test_run_holdings_json_sorts_accounts_ignoring_case(t *testing.T) {
	seedLowerCaseAccountHoldingsStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings", "--json"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	var doc struct {
		Holdings []struct {
			Account string `json:"account"`
		} `json:"holdings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	accounts := make([]string, len(doc.Holdings))
	for i, h := range doc.Holdings {
		accounts[i] = h.Account
	}
	assert.Equal(t, []string{"brokerage", "IRA", "Old RRSP"}, accounts)
}
