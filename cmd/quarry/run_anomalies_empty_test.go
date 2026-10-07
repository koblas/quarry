// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_anomalies_says_when_no_charge_falls_in_the_window(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)},
		groceryCharge("Bakery", day(2003, 1, 4), 1000),
		groceryCharge("Bakery", day(2025, 12, 31), 500)))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"anomalies"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD", "0 charges checked"), stdout.String())
	assert.Equal(t, "quarry: warning: no unusually large charges from 2026-01-01 to 2026-09-29; "+
		"the store's transactions run 2003-01-04 to 2025-12-31\n", stderr.String())
}

const (
	anomaliesEmptyWindow = "no unusually large charges from 2026-01-01 to 2026-09-29"
	anomaliesOldCardLine = "account \"Old Card\" is not used in reports in Quicken, so anomalies leaves it out; " +
		"to include it, turn on reports for it in Quicken's account settings, then run quarry sync"
	anomaliesLinkedLine   = "account \"Linked\" uses linked account tracking in Quicken, so anomalies leaves it out, as Quicken's reports do"
	anomaliesAccountsSpan = "their transactions run 2003-01-04 to 2025-12-31"
)

func Test_run_anomalies_prints_each_empty_window_warning_on_stderr_and_in_the_json(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		charges     []chargeTxn
		wantCaption string
		wantWarns   []string
	}{
		{
			name: "a store with no transactions", args: []string{},
			wantCaption: "all accounts",
			wantWarns:   []string{anomaliesEmptyWindow + "; the store has no transactions"},
		},
		{
			name: "the named account has transactions, none in the period", args: []string{"--account", "Chequing"}, charges: bakeryHistory(),
			wantCaption: "Chequing",
			wantWarns:   []string{anomaliesEmptyWindow + " in the named accounts; " + anomaliesAccountsSpan},
		},
		{
			name: "the named account has no transactions", args: []string{"--account", "Visa"}, charges: bakeryHistory(),
			wantCaption: "Visa",
			wantWarns:   []string{anomaliesEmptyWindow + " in the named accounts; they have no transactions"},
		},
		{
			name: "the only named account is not in reports", args: []string{"--account", "Old Card"}, charges: bakeryHistory(),
			wantCaption: "Old Card",
			wantWarns:   []string{anomaliesOldCardLine},
		},
		{
			name: "the only named account uses linked tracking", args: []string{"--account", "Linked"}, charges: bakeryHistory(),
			wantCaption: "Linked",
			wantWarns:   []string{anomaliesLinkedLine},
		},
		{
			name: "an account not in reports named beside one that is", args: []string{"--account", "Old Card", "--account", "Chequing"}, charges: bakeryHistory(),
			wantCaption: "Old Card, Chequing",
			wantWarns:   []string{anomaliesOldCardLine, anomaliesEmptyWindow + " in the named accounts; " + anomaliesAccountsSpan},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			accounts := []store.Account{
				chequingAccount("acct-cad", 1),
				{ID: "acct-visa", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
				{ID: "acct-old", SourceID: 3, Name: "Old Card", Type: "credit_card", Currency: "CAD", Active: true, NotInReports: true},
				{ID: "acct-401k", SourceID: 4, Name: "Linked", Type: "chequing", Currency: "CAD", Active: true, LinkedTracking: true},
			}
			replaceStore(t, home, chargeRows(accounts, c.charges...))
			var textOut, textErr, jsonOut, jsonErr bytes.Buffer

			textExit := runWith(context.Background(), append([]string{"anomalies"}, c.args...), spendEnv(&textOut, &textErr))
			jsonExit := runWith(context.Background(), append([]string{"anomalies", "--json"}, c.args...), spendEnv(&jsonOut, &jsonErr))

			require.Equal(t, 0, textExit, textErr.String())
			require.Equal(t, 0, jsonExit, jsonErr.String())
			wantStderr := warningLines(c.wantWarns)
			assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in "+c.wantCaption+", amounts in CAD", "0 charges checked"), textOut.String())
			assert.Equal(t, wantStderr, textErr.String())
			assert.Equal(t, wantStderr, jsonErr.String())
			var doc struct {
				Warnings []string `json:"warnings"`
			}
			require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &doc), jsonOut.String())
			assert.Equal(t, c.wantWarns, doc.Warnings)
		})
	}
}

func Test_run_anomalies_prints_no_warning_when_charges_were_checked_but_none_is_unusual(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)},
		groceryCharge("Bakery", day(2026, 3, 1), 500)))
	var textOut, textErr, jsonOut, jsonErr bytes.Buffer

	textExit := runWith(context.Background(), []string{"anomalies"}, spendEnv(&textOut, &textErr))
	jsonExit := runWith(context.Background(), []string{"anomalies", "--json"}, spendEnv(&jsonOut, &jsonErr))

	require.Equal(t, 0, textExit, textErr.String())
	require.Equal(t, 0, jsonExit, jsonErr.String())
	assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD", "1 charge checked"), textOut.String())
	assert.Empty(t, textErr.String())
	assert.Empty(t, jsonErr.String())
	var doc struct {
		Warnings []string `json:"warnings"`
		Checked  int      `json:"checked"`
	}
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &doc), jsonOut.String())
	assert.Equal(t, []string{}, doc.Warnings)
	assert.Equal(t, 1, doc.Checked)
}
