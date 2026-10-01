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

func Test_run_anomalies_judges_a_first_time_payee_against_its_category(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	charges := make([]chargeTxn, 0, 11)
	for i, cents := range []int64{19000, 19500, 20000, 20500, 21040, 21040, 21500, 22000, 22500, 23000} {
		charges = append(charges, groceryCharge(fmt.Sprintf("Vendor %d", i), day(2025, time.March, 3+7*i), cents))
	}
	charges = append(charges, groceryCharge("Home Depot", day(2026, time.August, 14), 184210))
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"anomalies"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts", "1 charge checked",
		[]string{"2026-08-14", "Chequing (CAD)", "Home Depot", "Food:Groceries", "1,842.10", "210.40", "8.8x", "category, 10 earlier"}),
		stdout.String())
}

func Test_run_anomalies_counts_an_uncategorized_first_time_charge_as_not_judged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	uncategorized := chargeTxn{
		id: "tool-shed", account: "acct-cad", payee: "Tool Shed", currency: "CAD", day: day(2026, time.June, 9),
		splits: []chargeSplit{{cents: -15000}},
	}
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)},
		uncategorized, groceryCharge("Corner Store", day(2026, time.June, 10), 4500)))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"anomalies"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts",
		"2 charges checked; 1 had too little history to judge"), stdout.String())
}
