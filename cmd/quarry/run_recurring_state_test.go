package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// quietCase is a steady 10.00 series of count charges gapDays apart whose last charge came quietDays before today.
type quietCase struct {
	name      string
	every     string
	gapDays   int
	count     int
	quietDays int
	perYear   string
}

// quietSeriesOutput runs quarry recurring over the series c describes and returns stdout, then the series' first and last dates.
func quietSeriesOutput(t *testing.T, c quietCase) (string, string, string) {
	t.Helper()
	home := newHome(t)
	lastCharge := day(2026, time.September, 29).AddDate(0, 0, -c.quietDays)
	var charges []chargeTxn
	for i := c.count - 1; i >= 0; i-- {
		charges = append(charges, groceryCharge("Gym", lastCharge.AddDate(0, 0, -c.gapDays*i), 1000))
	}
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))
	var out, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"recurring", "--since", "2000"}, spendEnv(&out, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	return out.String(), lastCharge.AddDate(0, 0, -c.gapDays*(c.count-1)).Format(time.DateOnly), lastCharge.Format(time.DateOnly)
}

func Test_run_recurring_keeps_a_series_active_on_the_last_day_of_its_cadences_quiet_period(t *testing.T) {
	cases := []quietCase{
		{name: "weekly after 14 days", every: "week", gapDays: 7, count: 4, quietDays: 14, perYear: "520.00"},
		{name: "monthly after 45 days", every: "month", gapDays: 30, count: 3, quietDays: 45, perYear: "120.00"},
		{name: "quarterly after 120 days", every: "quarter", gapDays: 91, count: 3, quietDays: 120, perYear: "40.00"},
		{name: "annual after 400 days", every: "year", gapDays: 365, count: 2, quietDays: 400, perYear: "10.00"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stdout, first, last := quietSeriesOutput(t, c)

			assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
				[]string{"Gym", "CAD", c.every, "10.00", c.perYear, first, last, "active, new", ""},
				[]string{"Total", "CAD", "", "", c.perYear, "", "", "", ""}),
				stdout)
		})
	}
}

func Test_run_recurring_marks_a_series_ended_one_day_past_its_cadences_quiet_period(t *testing.T) {
	cases := []quietCase{
		{name: "weekly after 15 days", every: "week", gapDays: 7, count: 4, quietDays: 15},
		{name: "monthly after 46 days", every: "month", gapDays: 30, count: 3, quietDays: 46},
		{name: "quarterly after 121 days", every: "quarter", gapDays: 91, count: 3, quietDays: 121},
		{name: "annual after 401 days", every: "year", gapDays: 365, count: 2, quietDays: 401},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stdout, first, last := quietSeriesOutput(t, c)

			assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
				[]string{"Gym", "CAD", c.every, "10.00", "", first, last, "ended, new", ""}),
				stdout)
		})
	}
}

func Test_run_recurring_marks_a_series_first_charged_in_the_window_as_new(t *testing.T) {
	home := newHome(t)
	var charges []chargeTxn
	for month := time.March; month <= time.September; month++ {
		charges = append(charges, groceryCharge("Crave", day(2026, month, 2), 1500))
	}
	charges = append(charges, groceryCharge("Rogers", day(2025, time.December, 3), 2500))
	for month := time.January; month <= time.September; month++ {
		charges = append(charges, groceryCharge("Rogers", day(2026, month, 3), 2500))
	}
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, recurringTable("Recurring charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD",
		[]string{"Rogers", "CAD", "month", "25.00", "300.00", "2025-12-03", "2026-09-03", "active", ""},
		[]string{"Crave", "CAD", "month", "15.00", "180.00", "2026-03-02", "2026-09-02", "active, new", ""},
		[]string{"Total", "CAD", "", "", "480.00", "", "", "", ""}),
		stdout.String())
}
