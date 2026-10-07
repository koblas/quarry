package report_test

import (
	"cmp"
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The clocks the recurring summary tests read: each falls after the last month the test summarizes.
var (
	afterOctober  = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	afterNovember = time.Date(2026, 12, 2, 12, 0, 0, 0, time.UTC)
	afterMarch    = time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	afterAugust   = time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
)

// summaryRecurring is the recurring section of month (YYYY-MM) summarized at now from a store holding the
// merged charges of groups, oldest first and numbered in that order.
func summaryRecurring(t *testing.T, month string, now time.Time, groups ...[]store.Charge) report.Recurring {
	t.Helper()
	merged := slices.Concat(groups...)
	slices.SortStableFunc(merged, func(a, b store.Charge) int { return a.Date.Compare(b.Date) })
	for i := range merged {
		merged[i].SourceID = int64(i + 1)
	}
	srv := report.NewServer(report.WithStore(fakeStore{charges: store.Charges{Rows: merged}}))
	parsed, err := report.ParseMonth(&month, now)
	require.NoError(t, err)

	got, err := srv.Summary(t.Context(), report.SummaryRequest{Month: parsed, Currency: money.CAD})

	require.NoError(t, err)
	return got.Recurring
}

func Test_summary_lists_a_subscription_as_new_in_the_month_of_its_third_charge(t *testing.T) {
	gym := chargesOn(t, []string{"2026-07-03", "2026-08-02", "2026-09-01", "2026-10-01"})

	august := summaryRecurring(t, "2026-08", afterOctober, gym)
	september := summaryRecurring(t, "2026-09", afterOctober, gym)
	october := summaryRecurring(t, "2026-10", afterOctober, gym)

	assert.Empty(t, payeesOf(august))
	assert.Equal(t, []string{"Gym"}, payeesOf(september))
	assert.Empty(t, payeesOf(october))
}

func Test_summary_lists_a_subscription_with_an_early_price_change_as_new_in_its_first_steady_month(t *testing.T) {
	gym := []store.Charge{
		chargeOn(t, 0, "2026-07-03"),
		chargeOn(t, 0, "2026-08-03", ofAmount(1320)),
		chargeOn(t, 0, "2026-09-03", ofAmount(1320)),
		chargeOn(t, 0, "2026-10-03", ofAmount(1320)),
		chargeOn(t, 0, "2026-11-03", ofAmount(1320)),
	}

	september := summaryRecurring(t, "2026-09", afterNovember, gym)
	november := summaryRecurring(t, "2026-11", afterNovember, gym)

	assert.Empty(t, payeesOf(september))
	assert.Equal(t, []string{"Gym"}, payeesOf(november))
}

func Test_summary_does_not_list_a_late_bill_of_an_old_subscription_as_new(t *testing.T) {
	gym := slices.Concat(monthlyEndingOn(t, "2026-08-05", 24), chargesOn(t, []string{"2026-09-14", "2026-10-14", "2026-11-13"}))
	november := store.Window{Since: dateOf(t, "2026-11-01"), Until: dateOf(t, "2026-11-30")}

	september := summaryRecurring(t, "2026-09", afterNovember, gym)
	summarized := summaryRecurring(t, "2026-11", afterNovember, gym)
	recurring := recurringAt(t, afterNovember, november, gym)

	assert.Empty(t, payeesOf(september))
	assert.Empty(t, payeesOf(summarized))
	assert.Equal(t, []string{"Gym"}, payeesOf(recurring))
}

func Test_summary_marks_a_series_new_only_when_it_was_first_charged_in_the_month(t *testing.T) {
	cases := []struct {
		name    string
		charges []store.Charge
		want    bool
	}{
		{name: "first charged two months before", charges: chargesOn(t, []string{"2026-07-03", "2026-08-02", "2026-09-01"})},
		{name: "first charged late in the month before", charges: everyDaysEndingOn(t, "2026-09-10", 7, 4)},
		{name: "first charged in the month", charges: everyDaysEndingOn(t, "2026-09-24", 7, 4), want: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			september := summaryRecurring(t, "2026-09", summaryNow, c.charges)

			require.Len(t, september.Series, 1)
			assert.Equal(t, c.want, september.Series[0].New)
		})
	}
}

func Test_summary_lists_a_subscription_resumed_after_it_ended_as_new(t *testing.T) {
	gym := slices.Concat(monthlyEndingOn(t, "2025-01-15", 3), monthlyEndingOn(t, "2026-09-03", 3))

	september := summaryRecurring(t, "2026-09", summaryNow, gym)

	assert.Equal(t, []string{"Gym"}, payeesOf(september))
}

func Test_summary_does_not_list_a_subscription_that_kept_charging_through_a_stray_charge_as_new(t *testing.T) {
	gym := slices.Concat(
		chargesOn(t, []string{"2025-10-15", "2025-11-14", "2025-12-14"}, ofAmount(1000)),
		[]store.Charge{chargeOn(t, 0, "2026-01-04")},
		chargesOn(t, []string{"2026-01-13", "2026-02-12", "2026-03-14"}, ofAmount(1500)),
	)
	march := store.Window{Since: dateOf(t, "2026-03-01"), Until: dateOf(t, "2026-03-31")}

	summarized := summaryRecurring(t, "2026-03", afterMarch, gym)
	recurring := recurringAt(t, afterMarch, march, gym)

	assert.Empty(t, payeesOf(summarized))
	assert.Equal(t, []string{"Gym"}, payeesOf(recurring))
}

func Test_summary_lists_a_subscription_as_new_when_its_earlier_series_was_never_listable(t *testing.T) {
	gym := chargesOn(t, []string{"2024-08-15", "2025-08-15", "2025-08-15", "2026-08-15"})
	august := store.Window{Since: dateOf(t, "2026-08-01"), Until: dateOf(t, "2026-08-31")}

	summarized := summaryRecurring(t, "2026-08", afterAugust, gym)
	year := summaryRecurring(t, "2025-08", afterAugust, gym)
	recurring := recurringAt(t, afterAugust, august, gym)

	assert.Equal(t, []string{"Gym"}, payeesOf(summarized))
	assert.Empty(t, payeesOf(year))
	assert.Equal(t, []string{"Gym"}, payeesOf(recurring))
}

func Test_summary_recurring_ignores_charges_after_the_month(t *testing.T) {
	upToSeptember := chargesOn(t, everyDays(t, "2026-07-03", 30, 3))
	withLater := chargesOn(t, everyDays(t, "2026-07-03", 30, 6))
	december := recurringAt(t, afterNovember, allTime, withLater)
	require.Equal(t, 6, december.Series[0].ChargeCount)

	without := summaryRecurring(t, "2026-09", afterNovember, upToSeptember)
	with := summaryRecurring(t, "2026-09", afterNovember, withLater)

	require.Len(t, with.Series, 1)
	assert.Equal(t, 3, with.Series[0].ChargeCount)
	assert.Equal(t, report.SeriesActive, with.Series[0].State)
	assert.Equal(t, without, with)
}

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

func Test_summary_resumption_with_off_schedule_and_same_day_charges(t *testing.T) {
	const earlierEnd = "2026-01-10"
	onDay := func(offset int) string { return dateOf(t, earlierEnd).AddDate(0, 0, offset).Format(time.DateOnly) }
	cases := []struct {
		name    string
		earlier []store.Charge
		strays  []int
		start   int
		listed  bool
		// runCharges is the charges in the later run; 0 means 3.
		runCharges int
	}{
		{name: "one stray", earlier: monthlyEndingOn(t, earlierEnd, 3), strays: []int{21}, start: 30},
		{name: "two strays", earlier: monthlyEndingOn(t, earlierEnd, 3), strays: []int{10, 21}, start: 30},
		{name: "stray, run starts 45 days after the earlier run's last charge", earlier: monthlyEndingOn(t, earlierEnd, 3), strays: []int{21}, start: 45},
		{name: "stray, run starts 46 days after the earlier run's last charge", earlier: monthlyEndingOn(t, earlierEnd, 3), strays: []int{21}, start: 46, listed: true},
		{
			name: "earlier run not steady",
			earlier: []store.Charge{
				chargeOn(t, 0, onDay(-90), ofAmount(1000)),
				chargeOn(t, 0, onDay(-60), ofAmount(1000)),
				chargeOn(t, 0, onDay(-30), ofAmount(1000)),
				chargeOn(t, 0, earlierEnd, ofAmount(1500)),
			},
			strays: []int{3},
			start:  14,
			listed: true,
		},
		{name: "earlier run too short", earlier: monthlyEndingOn(t, earlierEnd, 2), strays: []int{21}, start: 30, listed: true},
		{name: "annual earlier series, run starts 400 days after", earlier: everyDaysEndingOn(t, earlierEnd, 365, 2), strays: []int{100}, start: 400},
		{name: "annual earlier series, run starts 401 days after", earlier: everyDaysEndingOn(t, earlierEnd, 365, 2), strays: []int{100}, start: 401, listed: true},
		{name: "earlier series over a year before the run", earlier: monthlyEndingOn(t, earlierEnd, 3), strays: []int{21}, start: 430, listed: true},
		{name: "no duplicate, run starts 40 days after", earlier: monthlyEndingOn(t, earlierEnd, 3), start: 40},
		{name: "same-day duplicate on the earlier run's last day, run starts 40 days after", earlier: monthlyEndingOn(t, earlierEnd, 3), strays: []int{0}, start: 40, listed: true},
		{name: "duplicate earlier in the earlier run, run starts 40 days after", earlier: monthlyEndingOn(t, earlierEnd, 3), strays: []int{-60}, start: 40},
		{name: "duplicate is the run's first charge", earlier: monthlyEndingOn(t, earlierEnd, 3), strays: []int{0}, start: 30, runCharges: 2, listed: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			strays := make([]store.Charge, len(c.strays))
			for i, offset := range c.strays {
				strays[i] = chargeOn(t, 0, onDay(offset))
			}
			runCharges := cmp.Or(c.runCharges, 3)
			runEnd := dateOf(t, onDay(c.start+30*(runCharges-1)))
			month := runEnd.Format("2006-01")
			now := time.Date(runEnd.Year(), runEnd.Month()+1, 2, 12, 0, 0, 0, time.UTC)
			gym := slices.Concat(c.earlier, strays, monthlyEndingOn(t, runEnd.Format(time.DateOnly), runCharges))

			summarized := summaryRecurring(t, month, now, gym)

			assert.Equal(t, c.listed, !summarized.Empty())
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
