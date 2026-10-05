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

func Test_WindowError_carries_its_parts(t *testing.T) {
	cases := []struct {
		name  string
		parse func() error
		want  report.WindowError
	}{
		{
			name:  "since that is not a date",
			parse: func() error { _, err := report.ParseWindow(new("2024-13"), nil, windowNow); return err },
			want:  report.WindowError{Kind: report.WindowNotADate, Bound: "since", Value: "2024-13"},
		},
		{
			name:  "until that is not a date",
			parse: func() error { _, err := report.ParseWindow(nil, new("2024-13"), windowNow); return err },
			want:  report.WindowError{Kind: report.WindowNotADate, Bound: "until", Value: "2024-13"},
		},
		{
			name:  "empty since is not a date",
			parse: func() error { _, err := report.ParseWindow(new(""), nil, windowNow); return err },
			want:  report.WindowError{Kind: report.WindowNotADate, Bound: "since", Value: ""},
		},
		{
			name:  "since after today",
			parse: func() error { _, err := report.ParseWindow(new("2099"), nil, windowNow); return err },
			want:  report.WindowError{Kind: report.WindowSinceAfterToday, Bound: "since", Value: "2099"},
		},
		{
			name:  "charge since after today names the command",
			parse: func() error { _, err := report.ParseChargeWindow("anomalies", new("2099"), nil, windowNow); return err },
			want:  report.WindowError{Kind: report.WindowChargeSinceAfterToday, Bound: "since", Value: "2099", Command: "anomalies"},
		},
		{
			name:  "since after until",
			parse: func() error { _, err := report.ParseWindow(new("2025"), new("2024"), windowNow); return err },
			want:  report.WindowError{Kind: report.WindowSinceAfterUntil, Bound: "since", Value: "2025", Other: "2024"},
		},
		{
			name:  "until before the default since",
			parse: func() error { _, err := report.ParseWindow(nil, new("2025-03"), windowNow); return err },
			want:  report.WindowError{Kind: report.WindowUntilBeforeDefault, Bound: "until", Value: "2025-03", DefaultSince: "2026-01-01"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.parse()

			var refusal report.WindowError
			require.ErrorAs(t, err, &refusal)
			assert.Equal(t, c.want, refusal)
		})
	}
}

func Test_parse_search_window_without_bounds_is_open_on_both_sides(t *testing.T) {
	got, err := report.ParseSearchWindow(nil, nil)

	require.NoError(t, err)
	assert.Equal(t, store.SearchWindow{}, got)
}

func Test_parse_search_window_since_alone_starts_on_the_first_day_of_its_period(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  time.Time
	}{
		{name: "a year starts on January 1", value: "2024", want: day(2024, time.January, 1)},
		{name: "a month starts on its first day", value: "2024-02", want: day(2024, time.February, 1)},
		{name: "a day is itself", value: "2024-02-15", want: day(2024, time.February, 15)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := report.ParseSearchWindow(&c.value, nil)

			require.NoError(t, err)
			assert.Equal(t, store.SearchWindow{Since: &c.want}, got)
		})
	}
}

func Test_parse_search_window_until_alone_ends_on_the_last_day_of_its_period(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  time.Time
	}{
		{name: "a year ends on December 31", value: "2024", want: day(2024, time.December, 31)},
		{name: "a leap-year February ends on the 29th", value: "2024-02", want: day(2024, time.February, 29)},
		{name: "a day is itself", value: "2024-02-15", want: day(2024, time.February, 15)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := report.ParseSearchWindow(nil, &c.value)

			require.NoError(t, err)
			assert.Equal(t, store.SearchWindow{Until: &c.want}, got)
		})
	}
}

func Test_parse_search_window_keeps_year_one_as_a_bound_not_an_open_end(t *testing.T) {
	zero := "0001"

	got, err := report.ParseSearchWindow(&zero, nil)

	require.NoError(t, err)
	require.NotNil(t, got.Since)
	assert.True(t, got.Since.IsZero())
	assert.Nil(t, got.Until)
}

func Test_parse_search_window_refuses_a_bound_that_is_not_a_date(t *testing.T) {
	bad := "2024-13"
	good := "2024-01"
	cases := []struct {
		name         string
		since, until *string
		want         string
	}{
		{name: "since", since: &bad, want: `--since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{name: "until", until: &bad, want: `--until "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{name: "until after a good since", since: &good, until: &bad, want: `--until "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := report.ParseSearchWindow(c.since, c.until)

			var refusal report.WindowError
			require.ErrorAs(t, err, &refusal)
			assert.Equal(t, report.WindowNotADate, refusal.Kind)
			assert.EqualError(t, err, c.want)
		})
	}
}

func Test_today_is_the_calendar_day_in_the_instants_own_zone(t *testing.T) {
	assert.Equal(t, day(2026, time.September, 29), report.Today(windowNow))
}

func Test_parse_search_window_refuses_a_since_after_the_until(t *testing.T) {
	since, until := "2026-04", "2026-03-31"

	_, err := report.ParseSearchWindow(&since, &until)

	var refusal report.WindowError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, report.WindowSinceAfterUntil, refusal.Kind)
	assert.EqualError(t, err, "--since 2026-04 is after --until 2026-03-31")
}

func Test_parse_search_window_accepts_a_since_on_the_same_day_as_the_until(t *testing.T) {
	both := "2026-03-31"

	got, err := report.ParseSearchWindow(&both, &both)

	require.NoError(t, err)
	assert.Equal(t, store.SearchWindow{Since: new(day(2026, time.March, 31)), Until: new(day(2026, time.March, 31))}, got)
}

func Test_parse_month_end_window_refuses_a_value_that_is_not_a_date(t *testing.T) {
	cases := []struct {
		name         string
		since, until *string
		want         string
	}{
		{name: "since", since: new("2024-13"), want: `--since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{name: "until", until: new("yesterday"), want: `--until "yesterday" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := report.ParseMonthEndWindow(c.since, c.until, windowNow)

			var refusal report.WindowError
			require.ErrorAs(t, err, &refusal)
			assert.EqualError(t, err, c.want)
		})
	}
}

func Test_parse_month_end_window_refuses_a_since_after_the_until_as_given(t *testing.T) {
	t.Run("a since a year after the until", func(t *testing.T) {
		_, err := report.ParseMonthEndWindow(new("2025"), new("2024"), windowNow)

		var refusal report.WindowError
		require.ErrorAs(t, err, &refusal)
		assert.EqualError(t, err, "--since 2025 is after --until 2024")
	})

	t.Run("a since on the same day as the until is accepted", func(t *testing.T) {
		both := "2026-03-05"

		got, err := report.ParseMonthEndWindow(&both, &both, windowNow)

		require.NoError(t, err)
		assert.Equal(t, store.Window{Since: day(2026, time.March, 5), Until: day(2026, time.March, 5)}, got)
	})
}

func Test_parse_month_end_window_refuses_an_until_alone_before_the_default_since(t *testing.T) {
	t.Run("the last day of last year", func(t *testing.T) {
		_, err := report.ParseMonthEndWindow(nil, new("2025-12-31"), windowNow)

		var refusal report.WindowError
		require.ErrorAs(t, err, &refusal)
		assert.EqualError(t, err, "--until 2025-12-31 is before the default --since 2026-01-01; pass --since too")
	})

	t.Run("the first day of this year is accepted", func(t *testing.T) {
		got, err := report.ParseMonthEndWindow(nil, new("2026-01-01"), windowNow)

		require.NoError(t, err)
		assert.Equal(t, store.Window{Since: day(2026, time.January, 1), Until: day(2026, time.January, 1)}, got)
	})
}

func Test_parse_month_end_window_without_flags_is_the_default_window(t *testing.T) {
	got, err := report.ParseMonthEndWindow(nil, nil, windowNow)

	require.NoError(t, err)
	assert.Equal(t, report.DefaultWindow(windowNow), got)
}

func Test_parse_month_end_window_clamps_the_until_to_today(t *testing.T) {
	cases := []struct {
		name  string
		until string
		want  time.Time
	}{
		{name: "today is kept", until: "2026-09-29", want: day(2026, time.September, 29)},
		{name: "yesterday is kept", until: "2026-09-28", want: day(2026, time.September, 28)},
		{name: "tomorrow is today", until: "2026-09-30", want: day(2026, time.September, 29)},
		{name: "a future year is today", until: "2027", want: day(2026, time.September, 29)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := report.ParseMonthEndWindow(new("2026-01"), &c.until, windowNow)

			require.NoError(t, err)
			assert.Equal(t, store.Window{Since: day(2026, time.January, 1), Until: c.want}, got)
		})
	}
}

func Test_parse_month_end_window_refuses_a_since_after_today(t *testing.T) {
	cases := []struct {
		name  string
		since string
		until *string
	}{
		{name: "alone", since: "2027-01"},
		{name: "with a later until", since: "2027-01", until: new("2028")},
		{name: "tomorrow, which is already today in UTC", since: "2026-09-30"},
		{name: "a year after this one", since: "2027"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := report.ParseMonthEndWindow(&c.since, c.until, windowNow)

			assert.Equal(t, report.WindowError{Kind: report.WindowNetWorthSinceAfterToday, Bound: "since", Value: c.since}, err)
			assert.EqualError(t, err, "--since "+c.since+" is after today; net worth is valued up to today only, so pass an earlier --since")
		})
	}
}

func Test_parse_month_end_window_accepts_a_since_that_is_today_or_in_the_month_containing_today(t *testing.T) {
	cases := []struct {
		name  string
		since string
		want  time.Time
	}{
		{name: "today", since: "2026-09-29", want: day(2026, time.September, 29)},
		{name: "the month containing today", since: "2026-09", want: day(2026, time.September, 1)},
		{name: "the year containing today", since: "2026", want: day(2026, time.January, 1)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := report.ParseMonthEndWindow(&c.since, nil, windowNow)

			require.NoError(t, err)
			assert.Equal(t, store.Window{Since: c.want, Until: day(2026, time.September, 29)}, got)
		})
	}
}

func Test_parse_month_end_window_reports_a_since_after_the_until_before_a_since_after_today(t *testing.T) {
	_, err := report.ParseMonthEndWindow(new("2027"), new("2024"), windowNow)

	assert.Equal(t, report.WindowError{Kind: report.WindowSinceAfterUntil, Bound: "since", Value: "2027", Other: "2024"}, err)
}
