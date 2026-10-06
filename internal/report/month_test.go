package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// edt is a zone whose midnight falls on the previous day in UTC.
var edt = time.FixedZone("EDT", -4*60*60)

func Test_the_default_month_is_the_calendar_month_before_the_local_day(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
		want string
	}{
		{name: "five minutes into the next month", now: time.Date(2026, 10, 1, 0, 5, 0, 0, edt), want: "2026-09"},
		{name: "the last minute of the month", now: time.Date(2026, 10, 31, 23, 59, 0, 0, edt), want: "2026-09"},
		{name: "local September 30 that is already October 1 in UTC", now: time.Date(2026, 9, 30, 23, 59, 0, 0, edt), want: "2026-08"},
		{name: "a new year reaches back into December", now: time.Date(2027, 1, 3, 12, 0, 0, 0, time.UTC), want: "2026-12"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			month, err := report.ParseMonth(nil, c.now)

			require.NoError(t, err)
			assert.Equal(t, c.want, month.String())
		})
	}
}

func Test_a_month_covers_its_first_to_its_last_day(t *testing.T) {
	cases := []struct {
		name         string
		value        string
		since, until string
		heading      string
	}{
		{name: "a thirty-day month", value: "2026-09", since: "2026-09-01", until: "2026-09-30", heading: "September 2026"},
		{name: "a leap February", value: "2024-02", since: "2024-02-01", until: "2024-02-29", heading: "February 2024"},
		{name: "the first month the calendar has", value: "0001-01", since: "0001-01-01", until: "0001-01-31", heading: "January 0001"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			month, err := report.ParseMonth(&c.value, summaryNow)

			require.NoError(t, err)
			assert.Equal(t, c.since, month.Start.Format(time.DateOnly))
			assert.Equal(t, c.until, month.End.Format(time.DateOnly))
			assert.Equal(t, month.Start, month.Window().Since)
			assert.Equal(t, month.End, month.Window().Until)
			assert.Equal(t, c.value, month.String())
			assert.Equal(t, c.heading, month.Name())
		})
	}
}

func Test_a_value_that_is_not_a_month_is_refused_with_the_default_month_as_the_example(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{name: "one-digit month", value: "2026-9"},
		{name: "a bare year", value: "2026"},
		{name: "a day", value: "2026-09-15"},
		{name: "empty", value: ""},
		{name: "month thirteen", value: "2026-13"},
		{name: "year zero", value: "0000-01"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := report.ParseMonth(&c.value, summaryNow)

			assert.Equal(t, report.MonthError{Kind: report.MonthNotAMonth, Value: c.value, Example: "2026-09"}, err)
			assert.EqualError(t, err, `--month "`+c.value+`" is not a month; use YYYY-MM, such as 2026-09`)
		})
	}
}

func Test_a_month_that_has_not_ended_is_refused_with_the_default_month_as_the_example(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{name: "the current month", value: "2026-10"},
		{name: "a later month", value: "2027-01"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := report.ParseMonth(&c.value, summaryNow)

			assert.Equal(t, report.MonthError{Kind: report.MonthNotEnded, Value: c.value, Example: "2026-09"}, err)
			assert.EqualError(t, err, "--month "+c.value+" has not ended; summary covers whole months, so pass 2026-09 or earlier")
		})
	}
}

func Test_a_month_ends_at_local_midnight(t *testing.T) {
	september := "2026-09"
	lastSecond := time.Date(2026, 9, 30, 23, 59, 59, 0, edt)
	midnight := time.Date(2026, 10, 1, 0, 0, 0, 0, edt)

	_, refused := report.ParseMonth(&september, lastSecond)
	month, accepted := report.ParseMonth(&september, midnight)

	assert.Equal(t, report.MonthError{Kind: report.MonthNotEnded, Value: "2026-09", Example: "2026-08"}, refused)
	require.NoError(t, accepted)
	assert.Equal(t, "2026-09", month.String())
}
