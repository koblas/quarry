// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"context"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// noStoreLine is the refusal for a home that holds no store.
func noStoreLine(t *testing.T, home string) string {
	t.Helper()
	return "quarry: no store at " + abbreviated(t, storePathUnder(home), home) + " yet; run quarry sync to build it\n"
}

func Test_run_recurring_refuses_usage_and_account_problems(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		accounts []store.Account
		wantExit int
		wantLine string
	}{
		{
			name: "an argument", args: []string{"recurring", "extra"}, accounts: []store.Account{chequingAccount("acct-cad", 1)},
			wantExit: 2, wantLine: "quarry: recurring takes no arguments\n",
		},
		{
			name: "a since that is not a date", args: []string{"recurring", "--since", "2026-13"}, accounts: []store.Account{chequingAccount("acct-cad", 1)},
			wantExit: 2, wantLine: "quarry: --since \"2026-13\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n",
		},
		{
			name: "an account no account is named", args: []string{"recurring", "--account", "Nope"}, accounts: []store.Account{chequingAccount("acct-cad", 1)},
			wantExit: 1, wantLine: "quarry: no account named \"Nope\"; run quarry accounts --all to list them\n",
		},
		{
			name: "an account name two accounts share", args: []string{"recurring", "--account", "Visa"},
			accounts: []store.Account{
				{ID: "acct-812", SourceID: 1, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
				{ID: "acct-977", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
			},
			wantExit: 1, wantLine: "quarry: 2 accounts are named \"Visa\"; pass one of their ids instead: acct-812, acct-977\n",
		},
		{
			name: "an account no account is named, with --json", args: []string{"recurring", "--json", "--account", "Nope"},
			accounts: []store.Account{chequingAccount("acct-cad", 1)},
			wantExit: 1, wantLine: "quarry: no account named \"Nope\"; run quarry accounts --all to list them\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, chargeRows(c.accounts))

			exitCode, stdout, stderr := runSpendCapture(context.Background(), c.args)

			assert.Equal(t, c.wantExit, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantLine, stderr.String())
		})
	}
}

func Test_run_recurring_refuses_when_there_is_no_store(t *testing.T) {
	home := newHome(t)

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring"})

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, noStoreLine(t, home), stderr.String())
}

// HOME holds no store: exit 2 (not the missing-store 1) shows each check runs first.
func Test_run_recurring_rejects_a_period_it_cannot_use(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name:       "a since after until",
			args:       []string{"recurring", "--since", "2025", "--until", "2024"},
			wantStderr: "quarry: --since 2025 is after --until 2024\n",
		},
		{
			name:       "an until before the default since",
			args:       []string{"recurring", "--until", "2024"},
			wantStderr: "quarry: --until 2024 is before the default --since 2026-01-01; pass --since too\n",
		},
		{
			name:       "a since after today",
			args:       []string{"recurring", "--since", "2099"},
			wantStderr: "quarry: --since 2099 is after today; recurring lists charges up to today only, so pass an earlier --since\n",
		},
		{
			name:       "a since after today, with --json",
			args:       []string{"recurring", "--json", "--since", "2099"},
			wantStderr: "quarry: --since 2099 is after today; recurring lists charges up to today only, so pass an earlier --since\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())

			exitCode, stdout, stderr := runSpendCapture(context.Background(), c.args)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}
