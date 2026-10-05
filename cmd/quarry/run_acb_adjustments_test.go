package main

import (
	"bytes"
	"context"
	"encoding/json"
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

const acbAdjustmentWarningsConfig = `colour = "red"

[accounts]
non-registered = ["acct-cad"]

[[acb.adjustment]]
security = "sec-99"
date = 2024-06-30
return-of-capital = 5.00

[[acb.adjustment]]
security = "sec-acme"
date = 2025-12-31
return-of-capital = 2000.00

[[acb.adjustment]]
security = "sec-acme"
date = 2023-01-01
return-of-capital = 10.00
`

func acbAdjustmentLines(path string) []string {
	return []string{
		path + `: acb.adjustment item 1 names "sec-99", which is not a security in quarry's store; quarry skips it`,
		path + `: acb.adjustment item 3 is for "Acme Corp", which no non-registered account holds on 2023-01-01; quarry skips it`,
	}
}

const acmeLargeReturnOfCapitalWarning = `"Acme Corp": return of capital on 2025-12-31 is 720.00 more than its ACB, ` +
	"so its ACB is 0.00 and 720.00 is a capital gain in 2025"

func Test_run_acb_lists_config_then_adjustment_then_removal_then_return_of_capital_warnings_in_both_forms(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, acbAdjustmentWarningsConfig)
	replaceStore(t, home, acbSharesRows())
	var textOut, textErr bytes.Buffer

	require.Equal(t, 0, runWith(context.Background(), []string{"acb"}, spendEnvAt(&textOut, &textErr, holdingsClock())), textErr.String())
	warnings, machineErr := jsonWarnings(t, "acb", "--json")

	wantStderr := stderrWarnings(append(append([]string{configShown + orderUnknownKey}, acbAdjustmentLines(configShown)...),
		acmeRemovalWarning, acmeLargeReturnOfCapitalWarning)...)
	assert.Equal(t, append(append([]string{configPath(home) + orderUnknownKey}, acbAdjustmentLines(configPath(home))...),
		acmeRemovalWarning, acmeLargeReturnOfCapitalWarning), warnings)
	assert.Equal(t, wantStderr, textErr.String())
	assert.Equal(t, wantStderr, machineErr)
}

func Test_run_acb_names_two_adjustments_for_one_security_on_one_day_with_the_first_item(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[accounts]\nnon-registered = [\"acct-cad\"]\n\n"+
		"[[acb.adjustment]]\nsecurity = \"sec-acme\"\ndate = 2024-06-30\nreinvested-distribution = 10.00\n\n"+
		"[[acb.adjustment]]\nsecurity = \"sec-acme\"\ndate = 2024-06-30\nreturn-of-capital = 5.00\n")
	replaceStore(t, home, acbAdjustmentsRows())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"acb"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, stderrWarnings(configShown+`: acb.adjustment items 1 and 2 are both for "sec-acme" on 2024-06-30; quarry applies both`),
		stderr.String())
}

func Test_run_acb_writes_return_of_capital_gain_in_json(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, acbAdjustmentsConfig)
	rows := acbAdjustmentsRows()
	rows.InvestmentTransactions = append(rows.InvestmentTransactions,
		acbTrade("inv-sell", 2, "acct-cad", "sec-acme", store.ActionSell, "CAD", day(2024, time.September, 1), -2_000_000, 30_000))
	replaceStore(t, home, rows)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"acb", "--json"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	var doc acbDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	require.Len(t, doc.Years, 2)
	assert.Equal(t, 2024, doc.Years[0].Year)
	assert.Equal(t, 1, doc.Years[0].SaleCount)
	assert.Equal(t, "60.00", doc.Years[0].Gain)
	assert.Equal(t, "0.00", doc.Years[0].ReturnOfCapitalGain)
	assert.Equal(t, 2025, doc.Years[1].Year)
	assert.Equal(t, 0, doc.Years[1].SaleCount)
	assert.Equal(t, "0.00", doc.Years[1].Gain)
	assert.Equal(t, "1490.00", doc.Years[1].ReturnOfCapitalGain)
}
