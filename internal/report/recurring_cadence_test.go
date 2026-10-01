package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cadenceCase is a Gym charged count times, gapDays apart.
type cadenceCase struct {
	name    string
	gapDays int
	count   int
}

func Test_recurring_recognises_each_cadence_at_the_bounds_of_its_gap_range_and_charge_minimum(t *testing.T) {
	cases := []struct {
		cadenceCase

		want report.Cadence
	}{
		{cadenceCase{"weekly at 6 days", 6, 4}, report.CadenceWeekly},
		{cadenceCase{"weekly at 8 days", 8, 4}, report.CadenceWeekly},
		{cadenceCase{"monthly at 26 days", 26, 3}, report.CadenceMonthly},
		{cadenceCase{"monthly at 35 days", 35, 3}, report.CadenceMonthly},
		{cadenceCase{"quarterly at 84 days", 84, 3}, report.CadenceQuarterly},
		{cadenceCase{"quarterly at 98 days", 98, 3}, report.CadenceQuarterly},
		{cadenceCase{"annual at 350 days", 350, 2}, report.CadenceAnnual},
		{cadenceCase{"annual at 380 days", 380, 2}, report.CadenceAnnual},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := recurringOf(t, chargesOn(t, everyDays(t, "2024-01-01", c.gapDays, c.count)))

			require.Len(t, result.Series, 1)
			assert.Equal(t, c.want, result.Series[0].Cadence)
			assert.Equal(t, c.count, result.Series[0].ChargeCount)
		})
	}
}

func Test_recurring_lists_no_series_outside_a_cadences_gap_range_or_below_its_charge_minimum(t *testing.T) {
	cases := []cadenceCase{
		{"weekly one day short", 5, 4},
		{"weekly one day past, in no cadence", 9, 4},
		{"weekly with 3 charges", 7, 3},
		{"monthly one day short", 25, 3},
		{"monthly one day past", 36, 3},
		{"monthly with 2 charges", 30, 2},
		{"quarterly one day short", 83, 3},
		{"quarterly one day past", 99, 3},
		{"quarterly with 2 charges", 91, 2},
		{"annual one day short", 349, 2},
		{"annual one day past", 381, 2},
		{"annual with 1 charge", 365, 1},
		{"biweekly", 14, 6},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := recurringOf(t, chargesOn(t, everyDays(t, "2024-01-01", c.gapDays, c.count)))

			assert.Empty(t, result.Series)
		})
	}
}

func Test_recurring_multiplies_the_latest_amount_by_the_charges_in_a_year(t *testing.T) {
	cases := []struct {
		name    string
		gapDays int
		count   int
		perYear int64
	}{
		{name: "weekly 52 times", gapDays: 7, count: 4, perYear: 52000},
		{name: "monthly 12 times", gapDays: 30, count: 3, perYear: 12000},
		{name: "quarterly 4 times", gapDays: 91, count: 3, perYear: 4000},
		{name: "annual once", gapDays: 365, count: 2, perYear: 1000},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			first := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -c.gapDays*(c.count-1))
			result := recurringOf(t, chargesOn(t, everyDays(t, first.Format(time.DateOnly), c.gapDays, c.count), ofAmount(1000)))

			require.Len(t, result.Series, 1)
			assert.Equal(t, new(c.perYear), result.Series[0].PerYear)
		})
	}
}

func Test_recurring_takes_the_cadence_from_the_last_gap_when_weekly_charges_turn_monthly(t *testing.T) {
	weekly := everyDays(t, "2025-10-01", 7, 4)
	monthly := everyDays(t, "2026-03-01", 30, 3)

	result := recurringOf(t, chargesOn(t, append(weekly, monthly...)))

	require.Len(t, result.Series, 1)
	assert.Equal(t, report.CadenceMonthly, result.Series[0].Cadence)
	assert.Equal(t, 3, result.Series[0].ChargeCount)
}

func Test_recurring_takes_the_cadence_from_the_last_gap_when_monthly_charges_turn_weekly(t *testing.T) {
	monthly := everyDays(t, "2025-10-01", 30, 3)
	weekly := everyDays(t, "2026-03-01", 7, 4)

	result := recurringOf(t, chargesOn(t, append(monthly, weekly...)))

	require.Len(t, result.Series, 1)
	assert.Equal(t, report.CadenceWeekly, result.Series[0].Cadence)
	assert.Equal(t, 4, result.Series[0].ChargeCount)
}

func Test_recurring_starts_the_run_again_after_a_charge_made_a_few_days_after_the_last(t *testing.T) {
	dates := []string{"2026-01-01", "2026-01-31", "2026-03-02", "2026-03-07", "2026-04-06", "2026-05-06", "2026-06-05"}

	result := recurringOf(t, chargesOn(t, dates))

	require.Len(t, result.Series, 1)
	assert.Equal(t, 4, result.Series[0].ChargeCount)
	assert.Equal(t, "2026-03-07", result.Series[0].First.Format(time.DateOnly))
}
