package report_test

import (
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_recurring_marks_a_series_active_when_a_charge_came_within_its_quiet_period(t *testing.T) {
	result := recurringOf(t, monthlyEndingOn(t, "2026-09-15", 3))

	require.Len(t, result.Series, 1)
	assert.Equal(t, report.SeriesActive, result.Series[0].State)
}

func Test_recurring_marks_a_series_ended_and_gives_it_no_yearly_cost_when_no_charge_has_come_lately(t *testing.T) {
	result := recurringOf(t, monthlyEndingOn(t, "2026-01-15", 3))

	require.Len(t, result.Series, 1)
	assert.Equal(t, report.SeriesEnded, result.Series[0].State)
	assert.Nil(t, result.Series[0].PerYear)
}

func Test_recurring_counts_days_since_the_last_charge_from_the_local_date(t *testing.T) {
	result := recurringAt(t, windowNow, allTime, monthlyEndingOn(t, "2026-08-15", 3))

	require.Len(t, result.Series, 1)
	assert.Equal(t, report.SeriesActive, result.Series[0].State)
}

func Test_recurring_takes_the_amount_and_yearly_cost_from_the_latest_charge(t *testing.T) {
	charges := []store.Charge{
		chargeOn(t, 1, "2026-07-15", ofAmount(1000)),
		chargeOn(t, 2, "2026-08-14", ofAmount(1000)),
		chargeOn(t, 3, "2026-09-13", ofAmount(1040)),
	}

	result := recurringOf(t, charges)

	require.Len(t, result.Series, 1)
	assert.Equal(t, int64(1040), result.Series[0].Amount)
	assert.Equal(t, new(int64(12480)), result.Series[0].PerYear)
}
