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

func Test_run_spend_by_payee_groups_spending_by_payee_and_currency_biggest_first(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, spendRows(
		[]store.Account{
			{ID: "acct-cad", SourceID: 1, Name: "Chequing", Type: "chequing", Currency: "CAD", Active: true},
			{ID: "acct-usd", SourceID: 2, Name: "US Chequing", Type: "chequing", Currency: "USD", Active: true},
		},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", payee: "payee-costco", currency: "CAD", day: day(2026, 3, 10), cents: -30000},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-groceries", payee: "payee-bakery", currency: "CAD", day: day(2026, 3, 11), cents: -1000},
		spendSplit{id: "s03", account: "acct-cad", category: "cat-fuel", payee: "payee-bakery", currency: "CAD", day: day(2026, 5, 2), cents: -250},
		spendSplit{id: "s04", account: "acct-cad", currency: "CAD", day: day(2026, 6, 1), cents: -4208},
		spendSplit{id: "s05", account: "acct-usd", category: "cat-groceries", payee: "payee-costco", currency: "USD", day: day(2026, 4, 1), cents: -31210},
	))
	var stdout, stderr bytes.Buffer
	env := defaultEnv(&stdout, &stderr)
	env.Now = func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) }

	exitCode := runWith(context.Background(), []string{"spend", "--by", "payee"}, env)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	const row = "%-10s  %-8s  %6s\n"
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts\n\n"+
		fmt.Sprintf(row, "Payee", "Currency", "Spent")+
		fmt.Sprintf(row, "Costco", "CAD", "300.00")+
		fmt.Sprintf(row, "(no payee)", "CAD", "42.08")+
		fmt.Sprintf(row, "Bakery", "CAD", "12.50")+
		fmt.Sprintf(row, "Costco", "USD", "312.10")+
		fmt.Sprintf(row, "Total", "CAD", "354.58")+
		fmt.Sprintf(row, "Total", "USD", "312.10"),
		stdout.String())
}
