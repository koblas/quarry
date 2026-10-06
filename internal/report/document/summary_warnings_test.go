package document_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

var easternDaylight = time.FixedZone("EDT", -4*60*60)

// summaryOf is a summary of the month whose first day is start, read in zone, from a snapshot taken at taken.
func summaryOf(start time.Time, zone *time.Location, taken time.Time, coverage report.SnapshotCoverage) report.Summary {
	return report.Summary{
		Month:    report.Month{Start: start, End: start.AddDate(0, 1, -1), Zone: zone},
		Status:   store.Status{Run: store.ImportRun{Snapshot: store.SnapshotRef{TakenAt: taken}}},
		Coverage: coverage,
	}
}

func Test_SnapshotWarning_says_nothing_unless_the_snapshot_is_known_to_miss_part_of_the_month(t *testing.T) {
	cases := []struct {
		name     string
		coverage report.SnapshotCoverage
	}{
		{name: "the snapshot covers the month", coverage: report.SnapshotCovers},
		{name: "a coverage the report does not define", coverage: report.SnapshotCoverage(99)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			summary := summaryOf(civil(2026, time.September, 1), easternDaylight, civil(2026, time.October, 2), c.coverage)

			assert.Empty(t, document.SnapshotWarning(summary, "run quarry summary again"))
		})
	}
}

func Test_SnapshotWarning_names_when_the_snapshot_was_taken_and_the_month_it_missed(t *testing.T) {
	taken := time.Date(2026, time.September, 28, 18, 2, 0, 0, time.UTC)
	summary := summaryOf(civil(2026, time.September, 1), easternDaylight, taken, report.SnapshotPredatesMonthEnd)

	got := document.SnapshotWarning(summary, "run quarry summary again")

	assert.Equal(t, "the store was built from a snapshot taken 2026-09-28 14:02 EDT, before September 2026 ended, "+
		"so transactions from the rest of the month are missing; open your Quicken file, run quarry sync, "+
		"then run quarry summary again", got)
}

func Test_SnapshotWarning_ends_with_the_phrase_the_surface_gives_for_running_the_report_again(t *testing.T) {
	const body = "the store was built from a snapshot taken 2026-08-30 20:00 UTC, before August 2026 ended, " +
		"so transactions from the rest of the month are missing; open your Quicken file, run quarry sync, then "
	cases := []struct {
		name  string
		again string
	}{
		{name: "the command", again: "run quarry summary again"},
		{name: "the command with a month", again: "run quarry summary --month 2026-08 again"},
		{name: "the tool", again: "call monthly_summary again"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			taken := time.Date(2026, time.August, 30, 20, 0, 0, 0, time.UTC)
			summary := summaryOf(civil(2026, time.August, 1), time.UTC, taken, report.SnapshotPredatesMonthEnd)

			assert.Equal(t, body+c.again, document.SnapshotWarning(summary, c.again))
		})
	}
}

func Test_SnapshotWarning_reads_the_snapshot_time_in_the_zone_of_the_month(t *testing.T) {
	taken := time.Date(2026, time.September, 28, 18, 2, 0, 0, time.UTC)
	cases := []struct {
		name string
		zone *time.Location
		want string
	}{
		{name: "eastern daylight time", zone: easternDaylight, want: "taken 2026-09-28 14:02 EDT,"},
		{name: "UTC", zone: time.UTC, want: "taken 2026-09-28 18:02 UTC,"},
		{name: "no zone reads as UTC", zone: nil, want: "taken 2026-09-28 18:02 UTC,"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			summary := summaryOf(civil(2026, time.September, 1), c.zone, taken, report.SnapshotPredatesMonthEnd)

			assert.Contains(t, document.SnapshotWarning(summary, "run quarry summary again"), c.want)
		})
	}
}

func Test_SnapshotWarning_says_it_cannot_tell_when_the_snapshot_records_no_time(t *testing.T) {
	summary := summaryOf(civil(2026, time.September, 1), easternDaylight, time.Time{}, report.SnapshotTimeUnknown)

	got := document.SnapshotWarning(summary, "run quarry summary again")

	assert.Equal(t, "cannot tell whether the store holds all of September 2026: "+
		"its snapshot's manifest does not record when it was taken; "+
		"open your Quicken file and run quarry sync to take a new snapshot", got)
}

func Test_SnapshotWarning_names_any_month_and_year(t *testing.T) {
	cases := []struct {
		name  string
		start time.Time
		want  string
	}{
		{name: "a leap February", start: civil(2024, time.February, 1), want: "all of February 2024:"},
		{name: "the first month the calendar has", start: civil(1, time.January, 1), want: "all of January 0001:"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			summary := summaryOf(c.start, time.UTC, time.Time{}, report.SnapshotTimeUnknown)

			assert.Contains(t, document.SnapshotWarning(summary, "run quarry summary again"), c.want)
		})
	}
}
