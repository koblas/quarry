package report_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// windowNow is 2026-09-29 in a zone where that evening is already 2026-09-30 in UTC.
var windowNow = time.Date(2026, 9, 29, 22, 0, 0, 0, time.FixedZone("UTC-5", -5*60*60))

func day(year int, month time.Month, dayOfMonth int) time.Time {
	return time.Date(year, month, dayOfMonth, 0, 0, 0, 0, time.UTC)
}

func Test_parse_window_resolves_a_bare_year_or_month_to_its_last_day(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  store.Window
	}{
		{name: "a year covers January 1 to December 31", value: "2024", want: store.Window{Since: day(2024, time.January, 1), Until: day(2024, time.December, 31)}},
		{name: "a leap-year February ends on the 29th", value: "2024-02", want: store.Window{Since: day(2024, time.February, 1), Until: day(2024, time.February, 29)}},
		{name: "a common-year February ends on the 28th", value: "2023-02", want: store.Window{Since: day(2023, time.February, 1), Until: day(2023, time.February, 28)}},
		{name: "December ends on the 31st", value: "2024-12", want: store.Window{Since: day(2024, time.December, 1), Until: day(2024, time.December, 31)}},
		{name: "a day is both bounds", value: "2024-06-15", want: store.Window{Since: day(2024, time.June, 15), Until: day(2024, time.June, 15)}},
		{name: "a leap day is a date", value: "2024-02-29", want: store.Window{Since: day(2024, time.February, 29), Until: day(2024, time.February, 29)}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := report.ParseWindow(&c.value, &c.value, windowNow)

			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func Test_parse_window_refuses_a_value_that_is_not_a_date(t *testing.T) {
	values := []string{
		"2024-13", "2024-00", "2024-02-30", "2023-02-29", "2024-1", "2024-01-1", "24",
		"20240101", "2024/01", "2024-01-01x", " 2024", "2024 ", "", "yesterday",
	}

	flags := []struct {
		name string
		pair func(value *string) (since, until *string)
	}{
		{name: "--since", pair: func(value *string) (*string, *string) { return value, nil }},
		{name: "--until", pair: func(value *string) (*string, *string) { return nil, value }},
	}

	for _, flag := range flags {
		for _, value := range values {
			t.Run(fmt.Sprintf("%s %q", flag.name, value), func(t *testing.T) {
				since, until := flag.pair(&value)

				_, err := report.ParseWindow(since, until, windowNow)

				var refusal report.WindowError
				require.ErrorAs(t, err, &refusal)
				assert.EqualError(t, err, fmt.Sprintf("%s %q is not a date; use YYYY, YYYY-MM or YYYY-MM-DD", flag.name, value))
			})
		}
	}
}

func Test_parse_window_without_flags_is_the_default_window(t *testing.T) {
	got, err := report.ParseWindow(nil, nil, windowNow)

	require.NoError(t, err)
	assert.Equal(t, report.DefaultWindow(windowNow), got)
}

func Test_parse_window_keeps_the_default_for_the_flag_not_given(t *testing.T) {
	sinceOnly, err := report.ParseWindow(new("2026-03"), nil, windowNow)
	require.NoError(t, err)
	untilOnly, err := report.ParseWindow(nil, new("2026-03"), windowNow)
	require.NoError(t, err)

	assert.Equal(t, store.Window{Since: day(2026, time.March, 1), Until: day(2026, time.September, 29)}, sinceOnly)
	assert.Equal(t, store.Window{Since: day(2026, time.January, 1), Until: day(2026, time.March, 31)}, untilOnly)
}

func Test_parse_window_accepts_equal_days_and_refuses_since_after_until(t *testing.T) {
	t.Run("the same day is a one-day window", func(t *testing.T) {
		got, err := report.ParseWindow(new("2024-12-31"), new("2024-12-31"), windowNow)

		require.NoError(t, err)
		assert.Equal(t, store.Window{Since: day(2024, time.December, 31), Until: day(2024, time.December, 31)}, got)
	})

	t.Run("a since inside the until's month is not after it", func(t *testing.T) {
		got, err := report.ParseWindow(new("2024-12-15"), new("2024-12"), windowNow)

		require.NoError(t, err)
		assert.Equal(t, store.Window{Since: day(2024, time.December, 15), Until: day(2024, time.December, 31)}, got)
	})

	t.Run("a later since than until is refused with both values as given", func(t *testing.T) {
		_, err := report.ParseWindow(new("2025"), new("2024"), windowNow)

		var refusal report.WindowError
		require.ErrorAs(t, err, &refusal)
		assert.EqualError(t, err, "--since 2025 is after --until 2024")
	})
}

func Test_parse_window_refuses_a_since_after_today_only_when_until_is_absent(t *testing.T) {
	t.Run("today is not after today", func(t *testing.T) {
		got, err := report.ParseWindow(new("2026-09-29"), nil, windowNow)

		require.NoError(t, err)
		assert.Equal(t, store.Window{Since: day(2026, time.September, 29), Until: day(2026, time.September, 29)}, got)
	})

	t.Run("tomorrow is refused even where it is already today in UTC", func(t *testing.T) {
		_, err := report.ParseWindow(new("2026-09-30"), nil, windowNow)

		var refusal report.WindowError
		require.ErrorAs(t, err, &refusal)
		assert.EqualError(t, err, "--since 2026-09-30 is after today; pass --until to include future-dated transactions")
	})

	t.Run("a future year is refused with the value as given", func(t *testing.T) {
		_, err := report.ParseWindow(new("2027"), nil, windowNow)

		assert.EqualError(t, err, "--since 2027 is after today; pass --until to include future-dated transactions")
	})

	t.Run("an until lets a future since through", func(t *testing.T) {
		got, err := report.ParseWindow(new("2099"), new("2100"), windowNow)

		require.NoError(t, err)
		assert.Equal(t, store.Window{Since: day(2099, time.January, 1), Until: day(2100, time.December, 31)}, got)
	})
}

func Test_parse_charge_window_names_the_command_in_the_refusal_of_a_since_after_today(t *testing.T) {
	cases := []struct {
		command string
		since   string
		want    string
	}{
		{command: "recurring", since: "2030", want: "--since 2030 is after today; recurring lists charges up to today only, so pass an earlier --since"},
		{command: "anomalies", since: "2026-09-30", want: "--since 2026-09-30 is after today; anomalies lists charges up to today only, so pass an earlier --since"},
	}

	for _, c := range cases {
		t.Run(c.command, func(t *testing.T) {
			_, err := report.ParseChargeWindow(c.command, &c.since, nil, windowNow)

			var refusal report.WindowError
			require.ErrorAs(t, err, &refusal)
			assert.EqualError(t, err, c.want)
		})
	}
}

func Test_parse_charge_window_lets_a_future_since_through_when_until_is_given(t *testing.T) {
	got, err := report.ParseChargeWindow("recurring", new("2030"), new("2031"), windowNow)

	require.NoError(t, err)
	assert.Equal(t, store.Window{Since: day(2030, time.January, 1), Until: day(2031, time.December, 31)}, got)
}

func Test_parse_charge_window_refuses_the_other_periods_as_parse_window_does(t *testing.T) {
	_, err := report.ParseChargeWindow("recurring", new("2025"), new("2024"), windowNow)

	assert.EqualError(t, err, "--since 2025 is after --until 2024")
}

func Test_parse_window_refuses_an_until_before_the_default_since(t *testing.T) {
	t.Run("the last day of last year is refused with the default since resolved", func(t *testing.T) {
		_, err := report.ParseWindow(nil, new("2025-12-31"), windowNow)

		var refusal report.WindowError
		require.ErrorAs(t, err, &refusal)
		assert.EqualError(t, err, "--until 2025-12-31 is before the default --since 2026-01-01; pass --since too")
	})

	t.Run("an until on the default since is a one-day window", func(t *testing.T) {
		got, err := report.ParseWindow(nil, new("2026-01-01"), windowNow)

		require.NoError(t, err)
		assert.Equal(t, store.Window{Since: day(2026, time.January, 1), Until: day(2026, time.January, 1)}, got)
	})

	t.Run("an until after today is accepted", func(t *testing.T) {
		got, err := report.ParseWindow(nil, new("2027"), windowNow)

		require.NoError(t, err)
		assert.Equal(t, store.Window{Since: day(2026, time.January, 1), Until: day(2027, time.December, 31)}, got)
	})

	t.Run("a bare earlier year is refused as given", func(t *testing.T) {
		_, err := report.ParseWindow(nil, new("2024"), windowNow)

		assert.EqualError(t, err, "--until 2024 is before the default --since 2026-01-01; pass --since too")
	})
}

func Test_parse_window_reports_the_since_first_and_keeps_each_refusal_to_its_own_flags(t *testing.T) {
	t.Run("a bad until is a bad date even beside a future since", func(t *testing.T) {
		_, err := report.ParseWindow(new("2099"), new("2024-13"), windowNow)

		assert.EqualError(t, err, `--until "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`)
	})

	t.Run("a future since with an earlier until is since-after-until, not after today", func(t *testing.T) {
		_, err := report.ParseWindow(new("2099"), new("2024"), windowNow)

		assert.EqualError(t, err, "--since 2099 is after --until 2024")
	})

	t.Run("two bad dates report the since", func(t *testing.T) {
		_, err := report.ParseWindow(new("2024-13"), new("2024-14"), windowNow)

		assert.EqualError(t, err, `--since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`)
	})
}
