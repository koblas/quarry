// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_spend_says_when_the_period_holds_nothing(t *testing.T) {
	const emptyTable = "Spending 2026-01-01 to 2026-02-28 in all accounts, amounts in CAD\n\nCategory  Currency  Spent\n"
	cases := []struct {
		name       string
		splits     []spendSplit
		wantStderr string
	}{
		{
			name: "the store has transactions, none in the period",
			splits: []spendSplit{
				{id: "s01", account: "acct-chq", category: "cat-groceries", currency: "CAD", day: day(2003, 1, 4), cents: -1000},
				{id: "s02", account: "acct-chq", category: "cat-groceries", currency: "CAD", day: day(2025, 12, 31), cents: -500},
			},
			wantStderr: "quarry: warning: no spending from 2026-01-01 to 2026-02-28; " +
				"the store's transactions run 2003-01-04 to 2025-12-31\n",
		},
		{
			name:       "the store has no transactions",
			wantStderr: "quarry: warning: no spending from 2026-01-01 to 2026-02-28; the store has no transactions\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, spendRows([]store.Account{
				chequingAccount("acct-chq", 1),
			}, c.splits...))
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(),
				[]string{"spend", "--since", "2026-01", "--until", "2026-02"}, spendEnv(&stdout, &stderr))

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, emptyTable, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}
