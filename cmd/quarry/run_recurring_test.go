// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recurringHeader is the header cells of the recurring table.
var recurringHeader = []string{"Payee", "Currency", "Every", "Amount", "Per year", "First", "Last", "Status", "Price changes"}

// recurringRightAligned are the table's columns of money, which pad on the left.
var recurringRightAligned = map[int]bool{3: true, 4: true}

// recurringTable is the recurring table under caption: every cell but the last padded to its
// column's widest cell, two-space gaps, money columns right-aligned, trailing spaces trimmed.
func recurringTable(caption string, rows ...[]string) string {
	rows = append([][]string{recurringHeader}, rows...)
	widths := make([]int, len(recurringHeader)-1)
	for _, row := range rows {
		for i := range widths {
			widths[i] = max(widths[i], len(row[i]))
		}
	}
	var b strings.Builder
	b.WriteString(caption + "\n\n")
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, cell := range row {
			switch {
			case i == len(widths):
				cells[i] = cell
			case recurringRightAligned[i]:
				cells[i] = fmt.Sprintf("%*s", widths[i], cell)
			default:
				cells[i] = fmt.Sprintf("%-*s", widths[i], cell)
			}
		}
		b.WriteString(strings.TrimRight(strings.Join(cells, "  "), " ") + "\n")
	}
	return b.String()
}

// groceryCharge is a one-split expense of cents (a positive number is money out) in cat-groceries.
func groceryCharge(payee string, day time.Time, cents int64) chargeTxn {
	return chargeTxn{
		id: payee + day.Format(time.DateOnly), account: "acct-cad", payee: payee, currency: "CAD", day: day,
		splits: []chargeSplit{{category: "cat-groceries", cents: -cents}},
	}
}

func Test_run_recurring_lists_a_monthly_subscription_with_its_yearly_cost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var charges []chargeTxn
	for month := time.October; len(charges) < 12; month++ {
		charges = append(charges, groceryCharge("Netflix.com", day(2025, month, 12), 2099))
	}
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"recurring"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, recurringTable("Recurring charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD",
		[]string{"Netflix.com", "CAD", "month", "20.99", "251.88", "2025-10-12", "2026-09-12", "active", ""},
		[]string{"Total", "CAD", "", "", "251.88", "", "", "", ""}),
		stdout.String())
}

func Test_run_recurring_detects_weekly_quarterly_and_yearly_series(t *testing.T) {
	cases := []struct {
		name    string
		gapDays int
		count   int
		every   string
		cents   int64
		amount  string
		perYear string
		first   string
	}{
		{name: "weekly", gapDays: 7, count: 4, every: "week", cents: 1000, amount: "10.00", perYear: "520.00", first: "2026-08-30"},
		{name: "quarterly", gapDays: 91, count: 3, every: "quarter", cents: 3000, amount: "30.00", perYear: "120.00", first: "2026-03-22"},
		{name: "yearly", gapDays: 365, count: 2, every: "year", cents: 9900, amount: "99.00", perYear: "99.00", first: "2025-09-20"},
	}
	lastCharge := day(2026, time.September, 20)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			var charges []chargeTxn
			for i := c.count - 1; i >= 0; i-- {
				charges = append(charges, groceryCharge("Gym", lastCharge.AddDate(0, 0, -c.gapDays*i), c.cents))
			}
			replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), []string{"recurring", "--since", "2000"}, spendEnv(&stdout, &stderr))

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Empty(t, stderr.String())
			assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
				[]string{"Gym", "CAD", c.every, c.amount, c.perYear, c.first, "2026-09-20", "active, new", ""},
				[]string{"Total", "CAD", "", "", c.perYear, "", "", "", ""}),
				stdout.String())
		})
	}
}

func Test_run_recurring_counts_a_split_charge_once_and_leaves_a_refund_out(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	charges := []chargeTxn{
		groceryCharge("Gym", day(2026, time.April, 10), 2099),
		groceryCharge("Gym", day(2026, time.May, 10), 2099),
		groceryCharge("Gym", day(2026, time.June, 10), 2099),
		groceryCharge("Gym", day(2026, time.July, 10), 2099),
		{
			id: "refund", account: "acct-cad", payee: "Gym", currency: "CAD", day: day(2026, time.July, 25),
			splits: []chargeSplit{{category: "cat-groceries", cents: 2099}},
		},
		groceryCharge("Gym", day(2026, time.August, 10), 2099),
		{
			id: "split", account: "acct-cad", payee: "Gym", currency: "CAD", day: day(2026, time.September, 10),
			splits: []chargeSplit{{category: "cat-groceries", cents: -1000}, {category: "cat-fuel", cents: -1099}},
		},
	}
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"recurring", "--since", "2000"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
		[]string{"Gym", "CAD", "month", "20.99", "251.88", "2026-04-10", "2026-09-10", "active, new", ""},
		[]string{"Total", "CAD", "", "", "251.88", "", "", "", ""}),
		stdout.String())
}
