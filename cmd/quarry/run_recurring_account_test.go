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

// monthlyCharges is twelve monthly charges of cents by payee on account, the last on 2026-09-12.
func monthlyCharges(account, payee string, cents int64) []chargeTxn {
	var charges []chargeTxn
	for month := time.October; len(charges) < 12; month++ {
		charge := groceryCharge(payee, day(2025, month, 12), cents)
		charge.account = account
		charges = append(charges, charge)
	}
	return charges
}

func Test_run_recurring_lists_only_the_series_charged_in_the_named_account(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	visa := store.Account{ID: "acct-visa", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true}
	charges := append(monthlyCharges("acct-visa", "Netflix.com", 2099), monthlyCharges("acct-cad", "Gym", 4000)...)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1), visa}, charges...))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"recurring", "--since", "2000", "--account", "Visa"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in Visa",
		[]string{"Netflix.com", "CAD", "month", "20.99", "251.88", "2025-10-12", "2026-09-12", "active, new", ""},
		[]string{"Total", "CAD", "", "", "251.88", "", "", "", ""}),
		stdout.String())
}
