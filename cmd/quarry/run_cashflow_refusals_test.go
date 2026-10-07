// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_run_cashflow_refuses_and_reports_empty_periods_like_spend(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		accounts   []store.Account
		splits     []spendSplit
		wantExit   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "a since that is not a date",
			args:       []string{"cashflow", "--since", "2024-13"},
			wantExit:   2,
			wantStderr: "quarry: --since \"2024-13\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n",
		},
		{
			name:       "a period that is neither month nor year",
			args:       []string{"cashflow", "--by", "week"},
			wantExit:   2,
			wantStderr: "quarry: --by must be month or year\n",
		},
		{
			name: "an account name two accounts share",
			args: []string{"cashflow", "--account", "Visa"},
			accounts: []store.Account{
				{ID: "acct-812", SourceID: 1, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
				{ID: "acct-977", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
			},
			wantExit:   1,
			wantStderr: "quarry: 2 accounts are named \"Visa\"; pass one of their ids instead: acct-812, acct-977\n",
		},
		{
			name: "a store with transactions, none in the period",
			args: []string{"cashflow", "--since", "2026-01", "--until", "2026-02"},
			accounts: []store.Account{
				chequingAccount("acct-chq", 1),
			},
			splits: []spendSplit{
				{id: "s01", account: "acct-chq", category: "cat-groceries", currency: "CAD", day: day(2003, 1, 4), cents: -1000},
				{id: "s02", account: "acct-chq", category: "cat-salary", currency: "CAD", day: day(2025, 12, 31), cents: 5000},
			},
			wantExit: 0,
			wantStdout: "Cash flow 2026-01-01 to 2026-02-28 in all accounts, amounts in CAD\n\n" +
				"Month  Currency  Income  Spent  Net  Savings rate  Status\n",
			wantStderr: "quarry: warning: no income or spending from 2026-01-01 to 2026-02-28; " +
				"the store's transactions run 2003-01-04 to 2025-12-31\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, cashFlowRows(c.accounts, c.splits...))

			exitCode, stdout, stderr := runSpendCapture(context.Background(), c.args)

			assert.Equal(t, c.wantExit, exitCode)
			assert.Equal(t, c.wantStdout, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

// HOME holds no store: exit 2 (not the missing-store 1) shows each check runs first.
func Test_run_cashflow_rejects_a_period_it_cannot_use(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name:       "a since after until",
			args:       []string{"cashflow", "--since", "2025", "--until", "2024"},
			wantStderr: "quarry: --since 2025 is after --until 2024\n",
		},
		{
			name:       "an until before the default since",
			args:       []string{"cashflow", "--until", "2024"},
			wantStderr: "quarry: --until 2024 is before the default --since 2026-01-01; pass --since too\n",
		},
		{
			name:       "a since after today",
			args:       []string{"cashflow", "--since", "2099"},
			wantStderr: "quarry: --since 2099 is after today; pass --until to include future-dated transactions\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			var stdout, stderr bytes.Buffer
			env := spendEnv(&stdout, &stderr)

			exitCode := runWith(context.Background(), c.args, env)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}
