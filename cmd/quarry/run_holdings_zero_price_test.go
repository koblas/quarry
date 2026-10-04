package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_holdings_lists_a_holding_priced_at_zero_with_a_value_of_zero_and_no_warning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rows := holdingsRows()
	rows.Securities = append(rows.Securities,
		store.Security{ID: "sec-zero", SourceID: 4, Name: "Zero Fund", Ticker: new("ZERO"), Currency: new("CAD")})
	rows.InvestmentTransactions = append(rows.InvestmentTransactions,
		holdingsBuy("inv-zero", 4, "acct-cad", "sec-zero", "CAD", 40_000_000))
	rows.Prices = append(rows.Prices, store.Price{SecurityID: "sec-zero", SourceID: 4, Date: holdingsDay(9), Price: 0})
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Holdings on 2026-03-12 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsLine("Brokerage", "Zero Fund (ZERO)", "40", "0.00", "2026-03-09", "CAD", "0.00", "0.00")+
		holdingsLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35", "33,536.72")+
		holdingsLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00", "50.00")+
		holdingsLine("Total", "", "", "", "", "", "", "71,290.72"),
		stdout.String())
}
