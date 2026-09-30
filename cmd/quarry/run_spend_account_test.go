// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_spend_counts_only_the_accounts_it_is_given(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, spendRows(
		[]store.Account{
			chequingAccount("acct-chq", 1),
			{ID: "acct-visa", SourceID: 2, Name: "Visa Infinite", Type: "credit_card", Currency: "CAD", Closed: true},
			{ID: "acct-sav", SourceID: 3, Name: "Savings", Type: "savings", Currency: "CAD", Active: true},
		},
		spendSplit{id: "s01", account: "acct-chq", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -1000},
		spendSplit{id: "s02", account: "acct-visa", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 11), cents: -500},
		spendSplit{id: "s03", account: "acct-sav", category: "cat-fuel", currency: "CAD", day: day(2026, 3, 12), cents: -300},
	))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(),
		[]string{"spend", "--account", "chequing", "--account", "acct-visa", "--account", "Chequing"},
		spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	const row = "%-14s  %-8s  %5s\n"
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in Chequing, Visa Infinite\n\n"+
		fmt.Sprintf(row, "Category", "Currency", "Spent")+
		fmt.Sprintf(row, "Food:Groceries", "CAD", "15.00")+
		fmt.Sprintf(row, "Total", "CAD", "15.00"),
		stdout.String())
}

func Test_run_spend_warns_that_a_named_account_is_left_out_of_reports(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, spendRows(
		[]store.Account{
			{ID: "acct-old", SourceID: 1, Name: "Old Card", Type: "credit_card", Currency: "CAD", Active: true, NotInReports: true},
		},
		spendSplit{id: "s01", account: "acct-old", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -900},
	))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"spend", "--account", "Old Card"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: account \"Old Card\" is not used in reports in Quicken, so spend leaves it out; "+
		"to include it, turn on reports for it in Quicken's account settings, then run quarry sync\n",
		stderr.String())
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in Old Card\n\nCategory  Currency  Spent\n", stdout.String())
}

func Test_run_spend_warns_that_a_named_linked_tracking_account_is_left_out(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, spendRows(
		[]store.Account{
			{ID: "acct-401k", SourceID: 1, Name: "Netskope 401(k)", Type: "retirement", Currency: "USD", Active: true, NotInReports: true, LinkedTracking: true},
		},
		spendSplit{id: "s01", account: "acct-401k", category: "cat-groceries", currency: "USD", day: day(2026, 3, 10), cents: -900},
	))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"spend", "--account", "Netskope 401(k)"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: account \"Netskope 401(k)\" uses linked account tracking in Quicken, "+
		"so spend leaves it out, as Quicken's reports do\n",
		stderr.String())
	assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in Netskope 401(k)\n\nCategory  Currency  Spent\n", stdout.String())
}

func Test_run_spend_ranges_a_linked_and_a_reported_named_account_over_the_reported_one(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, spendRows(
		[]store.Account{
			{ID: "acct-401k", SourceID: 1, Name: "Netskope 401(k)", Type: "retirement", Currency: "USD", Active: true, LinkedTracking: true},
			chequingAccount("acct-chq", 2),
		},
		spendSplit{id: "s01", account: "acct-401k", category: "cat-groceries", currency: "USD", day: day(2003, 1, 4), cents: -900},
		spendSplit{id: "s02", account: "acct-chq", category: "cat-groceries", currency: "CAD", day: day(2019, 3, 2), cents: -100},
		spendSplit{id: "s03", account: "acct-chq", category: "cat-groceries", currency: "CAD", day: day(2019, 3, 5), cents: -100},
	))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(),
		[]string{"spend", "--account", "Netskope 401(k)", "--account", "Chequing"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: account \"Netskope 401(k)\" uses linked account tracking in Quicken, "+
		"so spend leaves it out, as Quicken's reports do\n"+
		"quarry: warning: no spending from 2026-01-01 to 2026-09-29 in the named accounts; "+
		"their transactions run 2019-03-02 to 2019-03-05\n", stderr.String())
}

func Test_run_spend_refuses_an_account_it_cannot_pick(t *testing.T) {
	cases := []struct {
		name string
		arg  string
		want string
	}{
		{
			name: "no account has the name",
			arg:  "Chequeing",
			want: "quarry: no account named \"Chequeing\"; run quarry accounts --all to list them\n",
		},
		{
			name: "the argument is empty",
			arg:  "",
			want: "quarry: no account named \"\"; run quarry accounts --all to list them\n",
		},
		{
			name: "two accounts share the name, listed by sorted id",
			arg:  "Visa",
			want: "quarry: 2 accounts are named \"Visa\"; pass one of their ids instead: acct-812, acct-977\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			replaceStore(t, home, spendRows([]store.Account{
				chequingAccount("acct-chq", 1),
				{ID: "acct-977", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
				{ID: "acct-812", SourceID: 3, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
			}))
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), []string{"spend", "--account", c.arg}, spendEnv(&stdout, &stderr))

			assert.Equal(t, 1, exitCode)
			assert.Equal(t, c.want, stderr.String())
			assert.Empty(t, stdout.String())
		})
	}
}
