package report_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// windowNow's own day is 2026-09-29; its UTC day is already 2026-09-30.

func Test_resolve_as_of_is_today_when_no_value_is_given(t *testing.T) {
	got, err := report.ResolveAsOf(nil, windowNow)

	require.NoError(t, err)
	assert.Equal(t, day(2026, time.September, 29), got)
}

func Test_resolve_as_of_reads_a_value_that_is_given(t *testing.T) {
	given := "2025-06-15"

	got, err := report.ResolveAsOf(&given, windowNow)

	require.NoError(t, err)
	assert.Equal(t, day(2025, time.June, 15), got)
}

func Test_resolve_as_of_refuses_a_given_empty_string_rather_than_defaulting_to_today(t *testing.T) {
	given := ""

	_, err := report.ResolveAsOf(&given, windowNow)

	assert.Equal(t, report.AsOfError{Kind: report.AsOfNotADate, Value: ""}, err)
}

func Test_parse_as_of_resolves_a_period_to_its_last_day(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  time.Time
	}{
		{name: "a year is December 31", value: "2025", want: day(2025, time.December, 31)},
		{name: "a leap-year February is the 29th", value: "2024-02", want: day(2024, time.February, 29)},
		{name: "a common-year February is the 28th", value: "2023-02", want: day(2023, time.February, 28)},
		{name: "a day is itself", value: "2025-06-15", want: day(2025, time.June, 15)},
		{name: "a leap day is a date", value: "2024-02-29", want: day(2024, time.February, 29)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := report.ParseAsOf(c.value, windowNow)

			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func Test_parse_as_of_resolves_the_current_year_and_month_to_today(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{name: "the current year", value: "2026"},
		{name: "the current month", value: "2026-09"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := report.ParseAsOf(c.value, windowNow)

			require.NoError(t, err)
			assert.Equal(t, day(2026, time.September, 29), got)
		})
	}
}

func Test_parse_as_of_refuses_a_day_after_today_and_accepts_today(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		want    time.Time
		refused bool
	}{
		{name: "today", value: "2026-09-29", want: day(2026, time.September, 29)},
		{name: "yesterday", value: "2026-09-28", want: day(2026, time.September, 28)},
		{name: "tomorrow, which is already today in UTC", value: "2026-09-30", refused: true},
		{name: "next month", value: "2026-10", refused: true},
		{name: "next year", value: "2027", refused: true},
		{name: "a far-future day", value: "2099-01-01", refused: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := report.ParseAsOf(c.value, windowNow)

			if !c.refused {
				require.NoError(t, err)
				assert.Equal(t, c.want, got)
				return
			}
			var refusal report.AsOfError
			require.ErrorAs(t, err, &refusal)
			assert.Equal(t, report.AsOfAfterToday, refusal.Kind)
			assert.Equal(t, c.value, refusal.Value)
			assert.EqualError(t, err, fmt.Sprintf("--as-of %s is after today; holdings are valued up to today only, so pass an earlier --as-of", c.value))
		})
	}
}

func Test_parse_as_of_refuses_a_value_that_is_not_a_date_as_typed(t *testing.T) {
	values := []string{
		"2024-13", "2025-00", "", " 2025", "2025 ", "2025-1", "last spring", "2025-06-15T00:00:00Z", "2025-02-29",
	}

	for _, value := range values {
		t.Run(fmt.Sprintf("%q", value), func(t *testing.T) {
			_, err := report.ParseAsOf(value, windowNow)

			var refusal report.AsOfError
			require.ErrorAs(t, err, &refusal)
			assert.Equal(t, report.AsOfNotADate, refusal.Kind)
			assert.Equal(t, value, refusal.Value)
			assert.EqualError(t, err, fmt.Sprintf("--as-of %q is not a date; use YYYY, YYYY-MM or YYYY-MM-DD", value))
		})
	}
}
