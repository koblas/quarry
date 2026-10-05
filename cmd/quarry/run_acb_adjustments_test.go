package main

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const acbAdjustmentsConfig = `[accounts]
non-registered = ["acct-cad"]

[[acb.adjustment]]
security = "sec-acme"
date = 2024-06-30
reinvested-distribution = 200.00

[[acb.adjustment]]
security = "sec-acme"
date = 2024-12-31
return-of-capital = 150.00

[[acb.adjustment]]
security = "sec-acme"
date = 2025-12-31
return-of-capital = 2300.00
`

const acmeReturnOfCapitalWarning = `"Acme Corp": return of capital on 2025-12-31 is 1,250.00 more than its ACB, ` +
	"so its ACB is 0.00 and 1,250.00 is a capital gain in 2025"

// acbAdjustmentsRows is one non-registered CAD brokerage holding 10 shares of Acme Corp bought for 1,000.00 and never sold.
func acbAdjustmentsRows() store.Rows {
	rows := spendRows([]store.Account{
		{ID: "acct-cad", SourceID: 1, Name: "CAD Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
	})
	rows.Securities = []store.Security{{ID: "sec-acme", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")}}
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-acme", store.ActionBuy, "CAD", day(2024, time.February, 1), 10_000_000, -100_000),
	}
	return rows
}

func Test_run_acb_lowers_and_raises_the_acb_by_the_adjustments_and_counts_return_of_capital_above_it_as_a_gain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, acbAdjustmentsConfig)
	replaceStore(t, home, acbAdjustmentsRows())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"acb"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		"Year  Sales  Proceeds  Outlays   ACB  Gain or loss\n"+
		fmt.Sprintf("%-4s  %5s  %8s  %7s  %4s  %12s  %s\n", "2025", "0", "0.00", "0.00", "0.00", "0.00",
			"1,250.00 return of capital above ACB, a capital gain")+
		"\nACB on 2026-03-12, in CAD\n\n"+
		fmt.Sprintf("%-9s  %-6s  %6s  %4s  %13s\n", "Security", "Ticker", "Shares", "ACB", "ACB per share")+
		fmt.Sprintf("%-9s  %-6s  %6s  %4s  %13s\n", "Acme Corp", "ACME", "10", "0.00", "0.0000"),
		stdout.String())
	assert.Equal(t, "quarry: warning: "+acmeReturnOfCapitalWarning+"\n", stderr.String())
}
