package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const holdingsBeforeFirstRateLine = "1 holding valued on 2026-03-09, before 2026-03-10, the first exchange rate in the store, " +
	"is not converted to CAD and is totalled in USD"

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
