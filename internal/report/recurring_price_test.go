package report_test

import (
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// monthlyAmounts is one charge a month (30 days apart) ending 2026-09-15, one per amount.
func monthlyAmounts(t *testing.T, amounts ...int64) []store.Charge {
	t.Helper()
	charges := monthlyEndingOn(t, "2026-09-15", len(amounts))
	for i := range charges {
		charges[i].Amount = amounts[i]
	}
	return charges
}

// steppedAmounts is count amounts that start at 1000 and step up 10% of 1000 at each of the first changes steps.
func steppedAmounts(count, changes int) []int64 {
	amounts := make([]int64, count)
	level := int64(1000)
	for i := range amounts {
		if i >= 1 && i <= changes {
			level += 100
		}
		amounts[i] = level
	}
	return amounts
}

// flatThen is eight charges of base followed by last.
func flatThen(base, last int64) []int64 {
	return append(slices.Repeat([]int64{base}, 8), last)
}

func Test_recurring_treats_exactly_5_percent_as_no_price_change(t *testing.T) {
	cases := []struct {
		name string
		last int64
		want int
	}{
		{name: "exactly 5% up is steady", last: 10500, want: 0},
		{name: "exactly 5% down is steady", last: 9500, want: 0},
		{name: "a cent past 5% up is a change", last: 10501, want: 1},
		{name: "a cent past 5% down is a change", last: 9499, want: 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := recurringOf(t, monthlyAmounts(t, flatThen(10000, c.last)...))

			require.Len(t, result.Series, 1)
			assert.Len(t, result.Series[0].PriceChanges, c.want)
		})
	}
}

func Test_recurring_lists_a_run_with_floor_steps_over_4_changes_and_drops_one_more(t *testing.T) {
	cases := []struct {
		name    string
		amounts []int64
		listed  bool
	}{
		{name: "3 steps allow none: 1 change is dropped", amounts: steppedAmounts(4, 1), listed: false},
		{name: "3 steps allow none: no change is listed", amounts: steppedAmounts(4, 0), listed: true},
		{name: "4 steps allow 1: 1 change is listed", amounts: steppedAmounts(5, 1), listed: true},
		{name: "4 steps allow 1: 2 changes are dropped", amounts: steppedAmounts(5, 2), listed: false},
		{name: "8 steps allow 2: 2 changes are listed", amounts: steppedAmounts(9, 2), listed: true},
		{name: "8 steps allow 2: 3 changes are dropped", amounts: steppedAmounts(9, 3), listed: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := recurringOf(t, monthlyAmounts(t, c.amounts...))

			assert.Equal(t, c.listed, len(result.Series) == 1)
		})
	}
}

func Test_recurring_lists_an_annual_run_of_two_charges_only_within_5_percent(t *testing.T) {
	cases := []struct {
		name   string
		last   int64
		listed bool
	}{
		{name: "within 5% is listed", last: 10500, listed: true},
		{name: "past 5% is dropped", last: 10501, listed: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := everyDaysEndingOn(t, "2026-09-15", 365, 2)
			charges[0].Amount, charges[1].Amount = 10000, c.last

			result := recurringOf(t, charges)

			assert.Equal(t, c.listed, len(result.Series) == 1)
		})
	}
}

func Test_recurring_lists_a_price_change_whichever_way_the_price_moves_first(t *testing.T) {
	cases := []struct {
		name    string
		amounts []int64
	}{
		{name: "up then down", amounts: []int64{1000, 1000, 1000, 1000, 1000, 1000, 1100, 1000, 1000}},
		{name: "down then up", amounts: []int64{1000, 1000, 1000, 1000, 1000, 1000, 900, 1000, 1000}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := recurringOf(t, monthlyAmounts(t, c.amounts...))

			require.Len(t, result.Series, 1)
			assert.Len(t, result.Series[0].PriceChanges, 2)
		})
	}
}

func Test_recurring_rounds_change_pct_half_away_from_zero(t *testing.T) {
	cases := []struct {
		name string
		last int64
		want int64
	}{
		{name: "50.5 tenths up rounds to 51", last: 2101, want: 51},
		{name: "50.5 tenths down rounds to -51", last: 1899, want: -51},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := recurringOf(t, monthlyAmounts(t, flatThen(2000, c.last)...))

			require.Len(t, result.Series, 1)
			assert.Equal(t, c.want, result.Series[0].PriceChanges[0].Tenths)
		})
	}
}

func Test_recurring_measures_the_overall_change_from_the_first_charge_to_the_latest(t *testing.T) {
	amounts := []int64{10000, 10000, 10000, 10000, 10000, 10000, 10000, 11000, 9999}

	result := recurringOf(t, monthlyAmounts(t, amounts...))

	require.Len(t, result.Series, 1)
	assert.Equal(t, int64(10000), result.Series[0].FirstAmount)
	assert.Equal(t, int64(0), result.Series[0].ChangeTenths)
}

func Test_recurring_gives_a_price_change_the_date_of_the_charge_it_lands_on(t *testing.T) {
	charges := monthlyAmounts(t, flatThen(10000, 12000)...)

	result := recurringOf(t, charges)

	require.Len(t, result.Series, 1)
	assert.Equal(t, []report.PriceChange{
		{Date: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), From: 10000, To: 12000, Tenths: 200},
	}, result.Series[0].PriceChanges)
}

func Test_recurring_leaves_a_dropped_series_out_of_the_totals(t *testing.T) {
	steady := monthlyEndingOn(t, "2026-09-15", 9, paidTo("payee-a", "Gym"))
	wobbly := monthlyAmounts(t, steppedAmounts(9, 3)...)
	for i := range wobbly {
		wobbly[i].PayeeID, wobbly[i].Payee = new("payee-b"), new("Rent")
	}

	result := recurringOf(t, steady, wobbly)

	assert.Equal(t, []report.RecurringTotal{{Currency: "CAD", PerYear: 14400}}, result.Totals)
}
