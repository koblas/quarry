// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// totalsColumn is the field at index column of each "Total <currency> ..." line of out, by currency.
func totalsColumn(out string, column int) map[string]string {
	totals := map[string]string{}
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) > column && fields[0] == "Total" {
			totals[fields[1]] = fields[column]
		}
	}
	return totals
}

func Test_run_cashflow_total_spent_equals_spend_total_per_currency(t *testing.T) {
	cases := []struct {
		name      string
		accounts  []string
		wantSpent map[string]string
	}{
		{
			name:      "every account in reports",
			wantSpent: map[string]string{"CAD": "155.00", "USD": "55.00"},
		},
		{
			name:      "one account in reports and one not in reports",
			accounts:  []string{"Chequing", "Old Card"},
			wantSpent: map[string]string{"CAD": "155.00"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			rows := cashFlowRows(
				[]store.Account{
					{ID: "acct-cad", SourceID: 1, Name: "Chequing", Type: "chequing", Currency: "CAD", Active: true},
					{ID: "acct-sav", SourceID: 2, Name: "Savings", Type: "savings", Currency: "CAD", Active: true},
					{ID: "acct-out", SourceID: 3, Name: "Old Card", Type: "credit_card", Currency: "CAD", Active: true, NotInReports: true},
					{ID: "acct-usd", SourceID: 4, Name: "US Chequing", Type: "chequing", Currency: "USD", Active: true},
				},
				spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -10000},
				spendSplit{id: "s02", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 12), cents: 3000},
				spendSplit{id: "s03", account: "acct-cad", currency: "CAD", day: day(2026, 4, 1), cents: -2500},
				spendSplit{id: "s04", account: "acct-cad", currency: "CAD", day: day(2026, 4, 2), cents: 4000},
				spendSplit{id: "s05", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 5, 2), cents: -6000},
				spendSplit{id: "s06", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 1, 31), cents: 100000},
				spendSplit{id: "s07", account: "acct-cad", currency: "CAD", day: day(2026, 6, 1), cents: -20000},
				spendSplit{id: "s08", account: "acct-sav", currency: "CAD", day: day(2026, 6, 1), cents: 20000},
				spendSplit{id: "s09", account: "acct-out", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 11), cents: -90000},
				spendSplit{id: "s10", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 2, 1), cents: -5000},
				spendSplit{id: "s11", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 2, 3), cents: 500},
				spendSplit{id: "s12", account: "acct-usd", currency: "USD", day: day(2026, 2, 5), cents: -1000},
			)
			rows.Transfers = []store.Transfer{{ID: "xfer-1", FromSplitID: "split-s07", ToSplitID: new("split-s08")}}
			replaceStore(t, home, rows)
			period := []string{"--since", "2026-01", "--until", "2026-09"}
			for _, a := range c.accounts {
				period = append(period, "--account", a)
			}
			var spendOut, cashFlowOut, stderr bytes.Buffer

			spendExit := runWith(context.Background(), append([]string{"spend"}, period...), spendEnv(&spendOut, &stderr))
			cashFlowExit := runWith(context.Background(), append([]string{"cashflow"}, period...), spendEnv(&cashFlowOut, &stderr))

			require.Equal(t, 0, spendExit, stderr.String())
			require.Equal(t, 0, cashFlowExit, stderr.String())
			assert.Equal(t, c.wantSpent, totalsColumn(spendOut.String(), 2))
			assert.Equal(t, c.wantSpent, totalsColumn(cashFlowOut.String(), 3))
		})
	}
}
