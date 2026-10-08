// White-box: renderSpending is an unexported layout rule whose column
// widths and row order are best driven directly.
package cli

import (
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

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
		{
			name: "a month grouping adds a Status column that says partial only on a cut-short month",
			by:   store.SpendByMonth,
			rows: []report.SpendingRow{
				{Key: new("2026-01"), Currency: "CAD", Spent: 5000, Partial: true},
				{Key: new("2026-02"), Currency: "CAD", Spent: 0},
				{Key: new("2026-03"), Currency: "CAD", Spent: 120000, Partial: true},
			},
			tots: []store.SpendingTotal{{Currency: "CAD", Spent: 125000}},
			want: "" +
				"Spending 2026-01-01 to 2026-03-09 in all accounts\n" +
				"\n" +
				"Month    Currency     Spent  Status\n" +
				"2026-01  CAD          50.00  partial\n" +
				"2026-02  CAD           0.00\n" +
				"2026-03  CAD       1,200.00  partial\n" +
				"Total    CAD       1,250.00\n",
		},
		{
			name: "an empty month window still prints the Status header",
			by:   store.SpendByMonth,
			want: "" +
				"Spending 2026-01-01 to 2026-03-09 in all accounts\n" +
				"\n" +
				"Month  Currency  Spent  Status\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spending := report.Spending{Rows: c.rows, Totals: c.tots, Window: spendingWindow(), By: c.by}

			assert.Equal(t, c.want, renderSpending(spending))
		})
	}
}

func Test_renderSpending_escapes_a_category_key_and_an_account_name_in_the_caption(t *testing.T) {
	got := renderSpending(report.Spending{
		Window:   spendingWindow(),
		Accounts: []store.Account{{Name: "Chequ\ning"}},
		Rows:     []report.SpendingRow{{Key: new("Auto\t:Fuel"), Currency: "CAD", Spent: 100}, {Key: new("Food"), Currency: "CAD", Spent: 100}},
		Totals:   []store.SpendingTotal{{Currency: "CAD", Spent: 200}},
	})

	assert.Equal(t, ""+
		"Spending 2026-01-01 to 2026-03-09 in Chequ\\ning\n"+
		"\n"+
		"Category     Currency  Spent\n"+
		"Auto\\t:Fuel  CAD        1.00\n"+
		"Food         CAD        1.00\n"+
		"Total        CAD        2.00\n", got)
}
