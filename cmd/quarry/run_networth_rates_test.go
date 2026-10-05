package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

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
