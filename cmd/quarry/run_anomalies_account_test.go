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

func Test_run_anomalies_account_judges_the_named_accounts_charge_against_history_from_every_account(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	visa := store.Account{ID: "acct-visa", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true}
	charges := make([]chargeTxn, 0, 5)
	for i, cents := range []int64{9000, 9300, 9605} {
		charges = append(charges, groceryCharge("Bell Canada", day(2025, time.March, 3+7*i), cents))
	}
	onVisa := groceryCharge("Bell Canada", day(2026, time.March, 2), 41200)
	onVisa.account = "acct-visa"
	charges = append(charges, onVisa, groceryCharge("Bell Canada", day(2026, time.April, 6), 50000))
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1), visa}, charges...))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"anomalies", "--account", "Visa"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in Visa", "1 charge checked",
		[]string{"2026-03-02", "Visa (CAD)", "Bell Canada", "Food:Groceries", "412.00", "93.00", "4.4x", "payee, 3 earlier"}),
		stdout.String())
}

// historyWithBigCharge is three 2025 charges of Bell Canada on Chequing, then a 412.00 charge in 2026 on account.
func historyWithBigCharge(account string) []chargeTxn {
	charges := make([]chargeTxn, 0, 4)
	for i, cents := range []int64{9000, 9300, 9605} {
		charges = append(charges, groceryCharge("Bell Canada", day(2025, time.March, 3+7*i), cents))
	}
	big := groceryCharge("Bell Canada", day(2026, time.March, 2), 41200)
	big.account = account
	return append(charges, big)
}

func Test_run_anomalies_lists_a_charge_in_a_closed_account(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldCard := store.Account{ID: "acct-old", SourceID: 2, Name: "Old Card", Type: "credit_card", Currency: "CAD", Closed: true}
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1), oldCard}, historyWithBigCharge("acct-old")...))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"anomalies", "--account", "Old Card"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in Old Card", "1 charge checked",
		[]string{"2026-03-02", "Old Card (CAD, closed)", "Bell Canada", "Food:Groceries", "412.00", "93.00", "4.4x", "payee, 3 earlier"}),
		stdout.String())
}

func Test_run_anomalies_json_names_the_account_and_counts_only_its_charges(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	visa := store.Account{ID: "acct-visa", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true}
	charges := append(historyWithBigCharge("acct-visa"), groceryCharge("Bell Canada", day(2026, time.April, 6), 500))
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1), visa}, charges...))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"anomalies", "--json", "--account", "Visa"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	doc := decodeAnomaliesJSON(t, stdout.String())
	assert.Equal(t, []recurringIDName{{ID: "acct-visa", Name: "Visa"}}, doc.AccountFilter)
	assert.Len(t, doc.Anomalies, 1)
	assert.Equal(t, 1, doc.Checked)
	assert.Equal(t, 0, doc.NotJudged)
}
