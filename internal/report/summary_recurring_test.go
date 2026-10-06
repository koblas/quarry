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
	gym := append(monthlyEndingOn(t, "2026-08-05", 24), chargeOn(t, 0, "2026-09-14"))

	september := summaryRecurring(t, "2026-09", summaryNow, gym)

	assert.Empty(t, payeesOf(september))
}

func Test_summary_lists_a_subscription_resumed_after_it_ended_as_new(t *testing.T) {
	gym := slices.Concat(monthlyEndingOn(t, "2025-01-15", 3), monthlyEndingOn(t, "2026-09-03", 3))

	september := summaryRecurring(t, "2026-09", summaryNow, gym)

	assert.Equal(t, []string{"Gym"}, payeesOf(september))
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
