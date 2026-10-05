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

// netWorthHistoryLine is one converted history table line over the chequing and credit_card columns.
func netWorthHistoryLine(monthEnd, chequing, creditCard, total string) string {
	return fmt.Sprintf("%-10s  %8s  %11s  %8s\n", monthEnd, chequing, creditCard, total)
}

// seedNetWorthHistoryStore builds the store under a temp HOME with CAD chequing, a CAD credit card and USD
// chequing, each with a balance on 2026-01-31, 2026-02-28 and 2026-03-12; the USD rate is 1.36 from January 2.
func seedNetWorthHistoryStore(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	card := store.Account{ID: "acct-card", SourceID: 3, Name: "Card", Type: "credit_card", Currency: "CAD", Active: true}
	rows := spendRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2), card},
		spendSplit{id: "cad-jan", account: "acct-cad", currency: "CAD", day: day(2026, time.January, 15), cents: 100_000},
		spendSplit{id: "cad-feb", account: "acct-cad", currency: "CAD", day: day(2026, time.February, 10), cents: 50_000},
		spendSplit{id: "cad-mar", account: "acct-cad", currency: "CAD", day: day(2026, time.March, 5), cents: -20_000},
		spendSplit{id: "usd-jan", account: "acct-usd", currency: "USD", day: day(2026, time.January, 10), cents: 80_000},
		spendSplit{id: "usd-mar", account: "acct-usd", currency: "USD", day: day(2026, time.March, 3), cents: 20_000},
		spendSplit{id: "card-jan", account: "acct-card", currency: "CAD", day: day(2026, time.January, 20), cents: -25_000},
		spendSplit{id: "card-feb", account: "acct-card", currency: "CAD", day: day(2026, time.February, 15), cents: -10_000},
		spendSplit{id: "card-mar", account: "acct-card", currency: "CAD", day: day(2026, time.March, 8), cents: -5_000},
	)
	replaceStoreWithRates(t, home, rows, usdRate(day(2026, time.January, 2), 1_360_000))
}

func Test_run_networth_lists_each_month_end_with_a_column_per_type_ending_with_today(t *testing.T) {
	seedNetWorthHistoryStore(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"networth", "--since", "2026-01", "--until", "2027"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Net worth at each month end 2026-01-31 to 2026-03-12, amounts in CAD\n\n"+
		netWorthHistoryLine("Month end", "chequing", "credit_card", "Total")+
		netWorthHistoryLine("2026-01-31", "2,088.00", "-250.00", "1,838.00")+
		netWorthHistoryLine("2026-02-28", "2,588.00", "-350.00", "2,238.00")+
		netWorthHistoryLine("2026-03-12", "2,660.00", "-400.00", "2,260.00"),
		stdout.String())
}
