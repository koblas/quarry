package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// netWorthNoRatesLine is the net-worth warning for a store with no exchange rates, in a CAD summary.
const netWorthNoRatesLine = "the store has no exchange rates, so USD balances are not converted to CAD and are left out of the CAD total; " +
	"pass --currency native to list them, or run quarry sync to fetch rates"

func Test_run_summary_warns_once_per_kind_when_the_store_has_no_exchange_rates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, summaryNativeRows())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"summary"}, spendEnvAt(&stdout, &stderr, summaryClock))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), anomaliesTable("Unusually large charges 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD", "4 charges checked",
		[]string{"2026-09-14", "US Chequing (USD)", "Hulu", "Food:Groceries", "USD 150.00", "USD 20.00", "7.5x", "payee, 5 earlier"},
		[]string{"2026-09-14", "Chequing (CAD)", "Bell Canada", "Food:Groceries", "412.00", "96.05", "4.3x", "payee, 5 earlier"}))
	assert.Contains(t, stdout.String(), recurringTable("Recurring charges new 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD",
		[]string{"Crave", "CAD", "month", "22.59", "271.08", "2026-07-03", "2026-09-03", "active", ""},
		[]string{"Spotify", "USD", "month", "9.99", "119.88", "2026-07-08", "2026-09-08", "active", ""},
		[]string{"Total", "CAD", "", "", "271.08", "", "", "", ""},
		[]string{"Total", "USD", "", "", "119.88", "", "", "", ""}))
	assert.Contains(t, stdout.String(), netWorthHistoryLine("Change", "no rate", "-22.59", "no rate"))
	assert.Equal(t, septemberTimeUnknownWarning+warningLine(noRatesLine)+warningLine(netWorthNoRatesLine), stderr.String())
}
