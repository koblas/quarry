package report_test

import (
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_summary_lists_a_series_new_in_the_month_of_its_listing_charge(t *testing.T) {
	cases := []struct {
		name    string
		gapDays int
		charges int
		listing string
		listed  bool
	}{
		{name: "weekly fourth charge on the first of the month", gapDays: 7, charges: 4, listing: "2026-09-01", listed: true},
		{name: "weekly fourth charge the last day of the month before", gapDays: 7, charges: 4, listing: "2026-08-31", listed: false},
		{name: "weekly fourth charge on the last day of the month", gapDays: 7, charges: 4, listing: "2026-09-30", listed: true},
		{name: "monthly third charge on the first of the month", gapDays: 30, charges: 3, listing: "2026-09-01", listed: true},
		{name: "monthly third charge the last day of the month before", gapDays: 30, charges: 3, listing: "2026-08-31", listed: false},
		{name: "monthly third charge on the last day of the month", gapDays: 30, charges: 3, listing: "2026-09-30", listed: true},
		{name: "quarterly third charge on the first of the month", gapDays: 91, charges: 3, listing: "2026-09-01", listed: true},
		{name: "quarterly third charge the last day of the month before", gapDays: 91, charges: 3, listing: "2026-08-31", listed: false},
		{name: "quarterly third charge on the last day of the month", gapDays: 91, charges: 3, listing: "2026-09-30", listed: true},
		{name: "annual second charge on the first of the month", gapDays: 365, charges: 2, listing: "2026-09-01", listed: true},
		{name: "annual second charge the last day of the month before", gapDays: 365, charges: 2, listing: "2026-08-31", listed: false},
		{name: "annual second charge on the last day of the month", gapDays: 365, charges: 2, listing: "2026-09-30", listed: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			next := dateOf(t, c.listing).AddDate(0, 0, c.gapDays).Format(time.DateOnly)
			gym := append(everyDaysEndingOn(t, c.listing, c.gapDays, c.charges), chargeOn(t, 0, next))

			september := summaryRecurring(t, "2026-09", summaryNow, gym)

			assert.Equal(t, c.listed, !september.Empty())
		})
	}
}

func Test_summary_resumption_arms(t *testing.T) {
	cases := []struct {
		name   string
		prior  []store.Charge
		listed bool
	}{
		{
			name:  "prior monthly series 40 days before",
			prior: monthlyEndingOn(t, "2026-07-27", 3),
		},
		{
			name:  "prior monthly series 45 days before",
			prior: monthlyEndingOn(t, "2026-07-22", 3),
		},
		{
			name:   "prior monthly series 46 days before",
			prior:  monthlyEndingOn(t, "2026-07-21", 3),
			listed: true,
		},
		{
			name:   "prior charges too few to make a series 40 days before",
			prior:  monthlyEndingOn(t, "2026-07-27", 2),
			listed: true,
		},
		{
			name: "prior series with a price change 40 days before",
			prior: []store.Charge{
				chargeOn(t, 0, "2026-05-28"),
				chargeOn(t, 0, "2026-06-27", ofAmount(1500)),
				chargeOn(t, 0, "2026-07-27", ofAmount(1500)),
			},
			listed: true,
		},
		{
			name:  "prior weekly series 14 days before",
			prior: everyDaysEndingOn(t, "2026-08-22", 7, 4),
		},
		{
			name:   "prior weekly series 15 days before",
			prior:  everyDaysEndingOn(t, "2026-08-21", 7, 4),
			listed: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gym := slices.Concat(c.prior, monthlyEndingOn(t, "2026-11-04", 3))

			november := summaryRecurring(t, "2026-11", afterNovember, gym)

			assert.Equal(t, c.listed, !november.Empty())
		})
	}
}

func Test_recurring_still_lists_a_resumed_series(t *testing.T) {
	gym := slices.Concat(monthlyEndingOn(t, "2026-07-27", 3), monthlyEndingOn(t, "2026-11-04", 3))

	recurring := recurringAt(t, afterNovember, store.Window{Since: dateOf(t, "2026-11-01"), Until: dateOf(t, "2026-11-30")}, gym)
	november := summaryRecurring(t, "2026-11", afterNovember, gym)

	assert.Equal(t, []string{"Gym"}, payeesOf(recurring))
	assert.Empty(t, payeesOf(november))
}

func Test_summary_omits_a_series_recognized_then_broken_in_the_month(t *testing.T) {
	gym := chargesOn(t, []string{"2026-07-03", "2026-08-02", "2026-09-01", "2026-09-11", "2026-10-11", "2026-11-10"})

	september := summaryRecurring(t, "2026-09", afterNovember, gym)
	november := summaryRecurring(t, "2026-11", afterNovember, gym)
	recurring := recurringAt(t, afterNovember, store.Window{Since: dateOf(t, "2026-11-01"), Until: dateOf(t, "2026-11-30")}, gym)

	assert.Empty(t, payeesOf(september))
	assert.Empty(t, payeesOf(november))
	assert.Equal(t, []string{"Gym"}, payeesOf(recurring))
}

func Test_summary_lists_a_weekly_series_that_ended_by_the_month_end_as_new(t *testing.T) {
	gym := everyDaysEndingOn(t, "2026-09-03", 7, 4)

	september := summaryRecurring(t, "2026-09", summaryNow, gym)

	assert.Equal(t, []string{"Gym"}, payeesOf(september))
	assert.Equal(t, report.SeriesEnded, september.Series[0].State)
}

func Test_summary_totals_only_the_series_new_in_the_month(t *testing.T) {
	old := monthlyEndingOn(t, "2026-09-03", 12, paidTo("payee-old", "Old"), ofAmount(1000))
	fresh := monthlyEndingOn(t, "2026-09-03", 3, paidTo("payee-new", "Fresh"), ofAmount(2000))

	september := summaryRecurring(t, "2026-09", summaryNow, old, fresh)

	assert.Equal(t, []string{"Fresh"}, payeesOf(september))
	assert.Equal(t, []report.RecurringTotal{{Currency: "CAD", PerYear: 24000}}, september.Totals)
}

func Test_summary_counts_only_the_new_series_that_need_a_rate(t *testing.T) {
	old := monthlyEndingOn(t, "2026-09-03", 12, paidTo("payee-old", "Old"), billedIn("USD"))
	fresh := monthlyEndingOn(t, "2026-09-03", 3, paidTo("payee-new", "Fresh"), billedIn("USD"))

	september := summaryRecurring(t, "2026-09", summaryNow, old, fresh)

	assert.Equal(t, []string{"Fresh"}, payeesOf(september))
	assert.Equal(t, 1, september.Unconverted.Transactions)
}
