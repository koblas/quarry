package report_test

import (
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
