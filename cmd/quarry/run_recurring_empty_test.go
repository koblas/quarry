// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_recurring_says_when_no_series_runs_in_the_period(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)},
		groceryCharge("Bakery", day(2003, 1, 4), 1000),
		groceryCharge("Bakery", day(2025, 12, 31), 500)))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"recurring"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, recurringTable("Recurring charges 2026-01-01 to 2026-09-29 in all accounts"), stdout.String())
	assert.Equal(t, "quarry: warning: no recurring charges from 2026-01-01 to 2026-09-29; "+
		"the store's transactions run 2003-01-04 to 2025-12-31\n", stderr.String())
}

const (
	recurringEmptyWindow = "no recurring charges from 2026-01-01 to 2026-09-29"
	recurringHeaderOnly  = "Payee  Currency  Every  Amount  Per year  First  Last  Status  Price changes\n"
	recurringOldCardLine = "account \"Old Card\" is not used in reports in Quicken, so recurring leaves it out; " +
		"to include it, turn on reports for it in Quicken's account settings, then run quarry sync"
	recurringLinkedLine   = "account \"Linked\" uses linked account tracking in Quicken, so recurring leaves it out, as Quicken's reports do"
	recurringAccountsSpan = "their transactions run 2003-01-04 to 2025-12-31"
)

// bakeryHistory is two charges from one payee, 2003-01-04 and 2025-12-31: none runs in the default period.
func bakeryHistory() []chargeTxn {
	return []chargeTxn{
		groceryCharge("Bakery", day(2003, 1, 4), 1000),
		groceryCharge("Bakery", day(2025, 12, 31), 500),
	}
}

// warningLines is the stderr text of warnings: each on its own prefixed line.
func warningLines(warnings []string) string {
	var text strings.Builder
	for _, w := range warnings {
		text.WriteString("quarry: warning: " + w + "\n")
	}
	return text.String()
}

func Test_run_recurring_prints_each_empty_period_warning_on_stderr_and_in_the_json(t *testing.T) {
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
			wantWarns:   []string{recurringEmptyWindow + "; the store has no transactions"},
		},
		{
			name: "the named account has transactions, none recurring in the period", args: []string{"--account", "Chequing"}, charges: bakeryHistory(),
			wantCaption: "Chequing",
			wantWarns:   []string{recurringEmptyWindow + " in the named accounts; " + recurringAccountsSpan},
		},
		{
			name: "the named account has no transactions", args: []string{"--account", "Visa"}, charges: bakeryHistory(),
			wantCaption: "Visa",
			wantWarns:   []string{recurringEmptyWindow + " in the named accounts; they have no transactions"},
		},
		{
			name: "the only named account is not in reports", args: []string{"--account", "Old Card"}, charges: bakeryHistory(),
			wantCaption: "Old Card",
			wantWarns:   []string{recurringOldCardLine},
		},
		{
			name: "the only named account uses linked tracking", args: []string{"--account", "Linked"}, charges: bakeryHistory(),
			wantCaption: "Linked",
			wantWarns:   []string{recurringLinkedLine},
		},
		{
			name: "an account not in reports named beside one that is", args: []string{"--account", "Old Card", "--account", "Chequing"}, charges: bakeryHistory(),
			wantCaption: "Old Card, Chequing",
			wantWarns:   []string{recurringOldCardLine, recurringEmptyWindow + " in the named accounts; " + recurringAccountsSpan},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			accounts := []store.Account{
				chequingAccount("acct-cad", 1),
				{ID: "acct-visa", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
				{ID: "acct-old", SourceID: 3, Name: "Old Card", Type: "credit_card", Currency: "CAD", Active: true, NotInReports: true},
				{ID: "acct-401k", SourceID: 4, Name: "Linked", Type: "chequing", Currency: "CAD", Active: true, LinkedTracking: true},
			}
			replaceStore(t, home, chargeRows(accounts, c.charges...))
			var textOut, textErr, jsonOut, jsonErr bytes.Buffer

			textExit := runWith(context.Background(), append([]string{"recurring"}, c.args...), spendEnv(&textOut, &textErr))
			jsonExit := runWith(context.Background(), append([]string{"recurring", "--json"}, c.args...), spendEnv(&jsonOut, &jsonErr))

			require.Equal(t, 0, textExit, textErr.String())
			require.Equal(t, 0, jsonExit, jsonErr.String())
			wantStderr := warningLines(c.wantWarns)
			assert.Equal(t, "Recurring charges 2026-01-01 to 2026-09-29 in "+c.wantCaption+"\n\n"+recurringHeaderOnly, textOut.String())
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
