// White-box: renderSpending is an unexported layout rule whose column
// widths and row order are best driven directly.
package cli

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func spendingWindow() store.Window {
	return store.Window{
		Since: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC),
	}
}

func Test_renderSpending(t *testing.T) {
	cases := []struct {
		name string
		by   store.SpendingGroup
		rows []report.SpendingRow
		tots []store.SpendingTotal
		want string
	}{
		{
			name: "widths come from the widest cell, uncategorized and totals included",
			rows: []report.SpendingRow{
				{Key: nil, Currency: "CAD", Spent: 4208},
				{Key: new("Auto:Fuel"), Currency: "CAD", Spent: 120450},
				{Key: new("Food:Groceries"), Currency: "USD", Spent: 31210},
			},
			tots: []store.SpendingTotal{{Currency: "CAD", Spent: 124658}, {Currency: "USD", Spent: 31210}},
			want: "" +
				"Spending 2026-01-01 to 2026-03-09 in all accounts\n" +
				"\n" +
				"Category         Currency     Spent\n" +
				"(uncategorized)  CAD          42.08\n" +
				"Auto:Fuel        CAD       1,204.50\n" +
				"Food:Groceries   USD         312.10\n" +
				"Total            CAD       1,246.58\n" +
				"Total            USD         312.10\n",
		},
		{
			name: "a net refund shows a negative amount",
			rows: []report.SpendingRow{{Key: new("Auto:Fuel"), Currency: "CAD", Spent: -2500}},
			tots: []store.SpendingTotal{{Currency: "CAD", Spent: -2500}},
			want: "" +
				"Spending 2026-01-01 to 2026-03-09 in all accounts\n" +
				"\n" +
				"Category   Currency   Spent\n" +
				"Auto:Fuel  CAD       -25.00\n" +
				"Total      CAD       -25.00\n",
		},
		{
			name: "a payee grouping heads column 1 Payee and labels the group with no payee",
			by:   store.SpendByPayee,
			rows: []report.SpendingRow{
				{Key: new("Costco"), Currency: "CAD", Spent: 30000},
				{Key: nil, Currency: "CAD", Spent: 4208},
			},
			tots: []store.SpendingTotal{{Currency: "CAD", Spent: 34208}},
			want: "" +
				"Spending 2026-01-01 to 2026-03-09 in all accounts\n" +
				"\n" +
				"Payee       Currency   Spent\n" +
				"Costco      CAD       300.00\n" +
				"(no payee)  CAD        42.08\n" +
				"Total       CAD       342.08\n",
		},
		{
			name: "an empty window prints the caption, a blank line and the header only",
			want: "" +
				"Spending 2026-01-01 to 2026-03-09 in all accounts\n" +
				"\n" +
				"Category  Currency  Spent\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spending := report.Spending{Rows: c.rows, Totals: c.tots, Window: spendingWindow(), By: c.by}

			assert.Equal(t, c.want, renderSpending(spending))
		})
	}
}
