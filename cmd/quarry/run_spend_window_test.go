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

func Test_run_spend_counts_the_whole_period_it_is_given(t *testing.T) {
	cases := []struct {
		name         string
		since, until string
		caption      string
		total        string
	}{
		{
			name:  "a bare year covers January 1 to December 31",
			since: "2024", until: "2024",
			caption: "Spending 2024-01-01 to 2024-12-31 in all accounts, amounts in CAD",
			total:   "30.00",
		},
		{
			name:  "a bare month covers its last day",
			since: "2024-12", until: "2025-01",
			caption: "Spending 2024-12-01 to 2025-01-31 in all accounts, amounts in CAD",
			total:   "60.00",
		},
		{
			name:  "one day is a period of its own",
			since: "2024-12-31", until: "2024-12-31",
			caption: "Spending 2024-12-31 to 2024-12-31 in all accounts, amounts in CAD",
			total:   "20.00",
		},
		{
			name:  "a period after today counts what is dated in it",
			since: "2099", until: "2099",
			caption: "Spending 2099-01-01 to 2099-12-31 in all accounts, amounts in CAD",
			total:   "80.00",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, spendRows(
				[]store.Account{chequingAccount("acct-cad", 1)},
				spendSplit{id: "s01", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2024, 1, 1), cents: -1000},
				spendSplit{id: "s02", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2024, 12, 31), cents: -2000},
				spendSplit{id: "s03", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2025, 1, 1), cents: -4000},
				spendSplit{id: "s04", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2099, 6, 1), cents: -8000},
			))
			var stdout, stderr bytes.Buffer
			env := spendEnv(&stdout, &stderr)

			exitCode := runWith(context.Background(), []string{"spend", "--since", c.since, "--until", c.until}, env)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Empty(t, stderr.String())
			const row = "%-9s  %-8s  %5s\n"
			assert.Equal(t, c.caption+"\n\n"+
				fmt.Sprintf(row, "Category", "Currency", "Spent")+
				fmt.Sprintf(row, "Auto:Fuel", "CAD", c.total)+
				fmt.Sprintf(row, "Total", "CAD", c.total),
				stdout.String())
		})
	}
}
