// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// eveningOfTheLocalDay is 22:00 on 2026-09-29 in UTC-5, already 2026-09-30 in UTC.
func eveningOfTheLocalDay() time.Time { return time.Date(2026, 9, 29, 22, 0, 0, 0, utcMinus5) }

func Test_run_recurring_leaves_out_a_charge_dated_after_the_local_day_even_when_until_reaches_past_it(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	charges := make([]chargeTxn, 0, 5)
	for _, date := range []time.Time{day(2026, 6, 29), day(2026, 7, 29), day(2026, 8, 29), day(2026, 9, 29), day(2026, 9, 30)} {
		charges = append(charges, groceryCharge("Netflix.com", date, 999))
	}
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"recurring", "--until", "2027"}, spendEnvAt(&stdout, &stderr, eveningOfTheLocalDay()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, recurringTable("Recurring charges 2026-01-01 to 2027-12-31 in all accounts",
		[]string{"Netflix.com", "CAD", "month", "9.99", "119.88", "2026-06-29", "2026-09-29", "active, new", ""},
		[]string{"Total", "CAD", "", "", "119.88", "", "", "", ""}),
		stdout.String())
}

func Test_run_anomalies_leaves_out_a_charge_dated_after_the_local_day_even_when_until_reaches_past_it(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	charges := make([]chargeTxn, 0, 4)
	for i, cents := range []int64{9000, 9300, 9605} {
		charges = append(charges, groceryCharge("Bell Canada", day(2026, time.March, 3+7*i), cents))
	}
	charges = append(charges, groceryCharge("Bell Canada", day(2026, 9, 30), 41200))
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"anomalies", "--until", "2027"}, spendEnvAt(&stdout, &stderr, eveningOfTheLocalDay()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2027-12-31 in all accounts", "3 charges checked"), stdout.String())
}

func Test_run_anomalies_lists_a_charge_of_two_categories_as_split_against_its_payees_usual(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	charges := make([]chargeTxn, 0, 6)
	for i, cents := range []int64{9000, 9300, 9605, 9900, 10200} {
		charges = append(charges, groceryCharge("Bell Canada", day(2025, time.March, 3+7*i), cents))
	}
	split := groceryCharge("Bell Canada", day(2026, time.March, 2), 0)
	split.splits = []chargeSplit{{category: "cat-groceries", cents: -30000}, {category: "cat-fuel", cents: -11200}}
	charges = append(charges, split)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"anomalies"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts", "1 charge checked",
		[]string{"2026-03-02", "Chequing (CAD)", "Bell Canada", "(split)", "412.00", "96.05", "4.3x", "payee, 5 earlier"}),
		stdout.String())
}

func Test_run_anomalies_counts_a_first_large_charge_without_a_payee_as_too_little_history_to_judge(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)},
		groceryCharge("", day(2026, time.March, 2), 25000)))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"anomalies"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts",
		"1 charge checked; 1 had too little history to judge"), stdout.String())
}
