package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const acbSecurityConfig = `[accounts]
non-registered = ["acct-cad"]

[[acb.adjustment]]
security = "sec-acme"
date = 2024-06-30
return-of-capital = 150.00
`

// acbHistoryRows is one non-registered CAD brokerage that bought Acme twice and sold part of it, and bought Beta once.
func acbHistoryRows() store.Rows {
	rows := spendRows([]store.Account{
		{ID: "acct-cad", SourceID: 1, Name: "CAD Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
	})
	rows.Securities = []store.Security{
		{ID: "sec-acme", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")},
		{ID: "sec-beta", SourceID: 2, Name: "Beta Inc", Ticker: new("BETA"), Currency: new("CAD")},
	}
	buy, sell := store.ActionBuy, store.ActionSell
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-acme-buy-1", 1, "acct-cad", "sec-acme", buy, "CAD", day(2024, time.February, 1), 10_000_000, -100_000),
		acbTrade("inv-acme-buy-2", 2, "acct-cad", "sec-acme", buy, "CAD", day(2024, time.March, 1), 5_000_000, -60_000),
		acbTrade("inv-beta-buy", 3, "acct-cad", "sec-beta", buy, "CAD", day(2024, time.April, 1), 1_000_000, -5_000),
		acbTrade("inv-acme-sell", 4, "acct-cad", "sec-acme", sell, "CAD", day(2025, time.June, 2), -6_000_000, 90_000),
	}
	return rows
}

// acbHistoryLine is one history table line, each cell as wide as acbHistoryRows' widest.
func acbHistoryLine(date, account, action, shares, amount, rate, cad, held, acb, gain string) string {
	return strings.TrimRight(fmt.Sprintf("%-10s  %-13s  %-17s  %6s  %13s  %4s  %9s  %11s  %8s  %12s",
		date, account, action, shares, amount, rate, cad, held, acb, gain), " ") + "\n"
}

func Test_run_acb_security_prints_every_event_of_the_named_security_with_shares_held_acb_and_gain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, acbSecurityConfig)
	replaceStore(t, home, acbHistoryRows())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"acb", "--security", "ACME"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "ACB history of \"Acme Corp\" (ACME), in CAD\n\n"+
		acbHistoryLine("Date", "Account", "Action", "Shares", "Amount", "Rate", "CAD", "Shares held", "ACB", "Gain or loss")+
		acbHistoryLine("2024-02-01", "CAD Brokerage", "buy", "10", "-1,000.00 CAD", "", "-1,000.00", "10", "1,000.00", "")+
		acbHistoryLine("2024-03-01", "CAD Brokerage", "buy", "5", "-600.00 CAD", "", "-600.00", "15", "1,600.00", "")+
		acbHistoryLine("2024-06-30", "", "return of capital", "", "", "", "150.00", "15", "1,450.00", "")+
		acbHistoryLine("2025-06-02", "CAD Brokerage", "sell", "6", "900.00 CAD", "", "900.00", "9", "870.00", "320.00"),
		stdout.String())
}
