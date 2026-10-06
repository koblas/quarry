package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func coverageOf(t *testing.T, month report.Month, taken time.Time) report.SnapshotCoverage {
	t.Helper()
	status := store.Status{Run: store.ImportRun{Snapshot: store.SnapshotRef{TakenAt: taken}}}
	srv := report.NewServer(report.WithStore(fakeStore{status: status}))

	got, err := srv.Summary(t.Context(), report.SummaryRequest{Month: month, Currency: money.CAD})

	require.NoError(t, err)
	return got.Coverage
}

func Test_a_snapshot_covers_a_month_only_when_taken_at_or_after_local_midnight_ending_it(t *testing.T) {
	toronto, err := time.LoadLocation("America/Toronto")
	require.NoError(t, err)
	cases := []struct {
		name  string
		zone  *time.Location
		month string
		taken time.Time
		want  report.SnapshotCoverage
	}{
		{name: "UTC: exactly midnight", zone: time.UTC, month: "2026-09", taken: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), want: report.SnapshotCovers},
		{name: "UTC: one nanosecond before midnight", zone: time.UTC, month: "2026-09", taken: time.Date(2026, 9, 30, 23, 59, 59, 999999999, time.UTC), want: report.SnapshotPredatesMonthEnd},
		{name: "fixed EDT: exactly local midnight", zone: edt, month: "2026-09", taken: time.Date(2026, 10, 1, 4, 0, 0, 0, time.UTC), want: report.SnapshotCovers},
		{name: "fixed EDT: one nanosecond before local midnight", zone: edt, month: "2026-09", taken: time.Date(2026, 10, 1, 3, 59, 59, 999999999, time.UTC), want: report.SnapshotPredatesMonthEnd},
		{name: "fixed EDT: past UTC midnight but before local midnight", zone: edt, month: "2026-09", taken: time.Date(2026, 10, 1, 0, 30, 0, 0, time.UTC), want: report.SnapshotPredatesMonthEnd},
		{name: "Toronto: October ends at 04:00Z, still on daylight time", zone: toronto, month: "2026-10", taken: time.Date(2026, 11, 1, 4, 0, 0, 0, time.UTC), want: report.SnapshotCovers},
		{name: "Toronto: one nanosecond before October's end", zone: toronto, month: "2026-10", taken: time.Date(2026, 11, 1, 3, 59, 59, 999999999, time.UTC), want: report.SnapshotPredatesMonthEnd},
		{name: "Toronto: March ends at 04:00Z after the clocks went forward", zone: toronto, month: "2026-03", taken: time.Date(2026, 4, 1, 4, 30, 0, 0, time.UTC), want: report.SnapshotCovers},
		{name: "Toronto: December ends at 05:00Z on standard time", zone: toronto, month: "2026-12", taken: time.Date(2027, 1, 1, 4, 30, 0, 0, time.UTC), want: report.SnapshotPredatesMonthEnd},
		{name: "Toronto: exactly the year's end", zone: toronto, month: "2026-12", taken: time.Date(2027, 1, 1, 5, 0, 0, 0, time.UTC), want: report.SnapshotCovers},
		{name: "a leap February ends after the 29th", zone: time.UTC, month: "2024-02", taken: time.Date(2024, 2, 29, 23, 59, 0, 0, time.UTC), want: report.SnapshotPredatesMonthEnd},
		{name: "a leap February's end", zone: time.UTC, month: "2024-02", taken: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), want: report.SnapshotCovers},
		{name: "taken long after the month", zone: time.UTC, month: "2026-09", taken: time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC), want: report.SnapshotCovers},
		{name: "taken before the month began", zone: time.UTC, month: "2026-09", taken: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), want: report.SnapshotPredatesMonthEnd},
		{name: "no time recorded", zone: time.UTC, month: "2026-09", taken: time.Time{}, want: report.SnapshotTimeUnknown},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			month, err := report.ParseMonth(&c.month, time.Date(2028, 1, 1, 12, 0, 0, 0, c.zone))
			require.NoError(t, err)

			assert.Equal(t, c.want, coverageOf(t, month, c.taken))
		})
	}
}

func Test_a_snapshot_taken_after_midnight_on_the_first_covers_the_default_month(t *testing.T) {
	cases := []struct {
		name  string
		taken time.Time
		want  report.SnapshotCoverage
	}{
		{name: "one minute into the first", taken: time.Date(2026, 10, 1, 0, 1, 0, 0, edt), want: report.SnapshotCovers},
		{name: "the minute before midnight", taken: time.Date(2026, 9, 30, 23, 59, 0, 0, edt), want: report.SnapshotPredatesMonthEnd},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			month, err := report.ParseMonth(nil, time.Date(2026, 10, 1, 0, 5, 0, 0, edt))
			require.NoError(t, err)

			assert.Equal(t, c.want, coverageOf(t, month, c.taken))
		})
	}
}

func Test_a_month_without_a_zone_ends_at_UTC_midnight(t *testing.T) {
	cases := []struct {
		name  string
		taken time.Time
		want  report.SnapshotCoverage
	}{
		{name: "UTC midnight", taken: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), want: report.SnapshotCovers},
		{name: "a minute before", taken: time.Date(2026, 9, 30, 23, 59, 0, 0, time.UTC), want: report.SnapshotPredatesMonthEnd},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			month := report.Month{Start: day(2026, time.September, 1), End: day(2026, time.September, 30)}

			assert.Equal(t, c.want, coverageOf(t, month, c.taken))
		})
	}
}
