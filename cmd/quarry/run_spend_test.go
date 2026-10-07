// run is unexported, so its tests live in package main rather than
// importing main from outside.
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

func Test_run_spend_shows_this_years_spending_by_category_in_each_currency(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, spendRows(
		[]store.Account{
			chequingAccount("acct-cad", 1),
			usdChequingAccount("acct-usd", 2),
		},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12345},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 9, 29), cents: -1000},
		spendSplit{id: "s03", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 5, 2), cents: -120450},
		spendSplit{id: "s04", account: "acct-cad", currency: "CAD", day: day(2026, 6, 1), cents: -4208},
		spendSplit{id: "s05", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 4, 1), cents: -31210},
		spendSplit{id: "s06", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2025, 12, 31), cents: -99900},
		spendSplit{id: "s07", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 9, 30), cents: -77700},
	))
	var stdout, stderr bytes.Buffer
	env := spendEnvAt(&stdout, &stderr, time.Date(2026, 9, 29, 22, 0, 0, 0, utcMinus5))

	exitCode := runWith(context.Background(), []string{"spend"}, env)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: "+noRatesLine+"\n", stderr.String())
	const row = "%-15s  %-8s  %8s\n"
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
		fmt.Sprintf(row, "Category", "Currency", "Spent")+
		fmt.Sprintf(row, "(uncategorized)", "CAD", "42.08")+
		fmt.Sprintf(row, "Auto:Fuel", "CAD", "1,204.50")+
		fmt.Sprintf(row, "Food:Groceries", "CAD", "133.45")+
		fmt.Sprintf(row, "Food:Groceries", "USD", "312.10")+
		fmt.Sprintf(row, "Total", "CAD", "1,380.03")+
		fmt.Sprintf(row, "Total", "USD", "312.10"),
		stdout.String())
}

func Test_run_spend_leaves_out_accounts_quicken_does_not_use_in_reports(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, spendRows(
		[]store.Account{
			chequingAccount("acct-in", 1),
			{ID: "acct-out", SourceID: 2, Name: "Old Card", Type: "credit_card", Currency: "CAD", Active: true, NotInReports: true},
		},
		spendSplit{id: "s01", account: "acct-in", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -2500},
		spendSplit{id: "s02", account: "acct-out", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 11), cents: -900},
	))
	var stdout, stderr bytes.Buffer
	env := spendEnv(&stdout, &stderr)

	exitCode := runWith(context.Background(), []string{"spend"}, env)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	const row = "%-14s  %-8s  %5s\n"
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
		fmt.Sprintf(row, "Category", "Currency", "Spent")+
		fmt.Sprintf(row, "Food:Groceries", "CAD", "25.00")+
		fmt.Sprintf(row, "Total", "CAD", "25.00"),
		stdout.String())
}
