package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dateOf(t *testing.T, date string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.DateOnly, date)
	require.NoError(t, err)
	return parsed
}

func Test_recurring_reads_charges_through_today_even_when_the_window_ends_later(t *testing.T) {
	var got store.ChargeParams
	srv := report.NewServer(report.WithStore(fakeStore{gotCharges: &got}))
	window := store.Window{Since: allTime.Since, Until: dateOf(t, "2030-12-31")}

	_, err := srv.Recurring(t.Context(), report.RecurringRequest{Window: window, Now: recurringNow})

	require.NoError(t, err)
	assert.Equal(t, store.ChargeParams{Through: dateOf(t, "2026-09-29")}, got)
}

func Test_recurring_reads_charges_through_the_local_date_when_the_utc_date_is_later(t *testing.T) {
	var got store.ChargeParams
	srv := report.NewServer(report.WithStore(fakeStore{gotCharges: &got}))

	_, err := srv.Recurring(t.Context(), report.RecurringRequest{Window: allTime, Now: windowNow})

	require.NoError(t, err)
	assert.Equal(t, store.ChargeParams{Through: dateOf(t, "2026-09-29")}, got)
}

func Test_recurring_reads_the_charges_once(t *testing.T) {
	reads := 0
	srv := report.NewServer(report.WithStore(fakeStore{chargesReads: &reads}))

	_, err := srv.Recurring(t.Context(), report.RecurringRequest{Window: allTime, Now: recurringNow})

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
}

func Test_recurring_returns_the_window_it_listed(t *testing.T) {
	window := store.Window{Since: dateOf(t, "2026-03-01"), Until: dateOf(t, "2026-03-31")}

	result := recurringIn(t, window)

	assert.Equal(t, window, result.Window)
}

func Test_recurring_lists_an_ended_series_only_when_the_window_touches_its_first_or_last_charge(t *testing.T) {
	cases := []struct {
		name         string
		since, until string
		want         int
	}{
		{name: "last charge on the first day of the window", since: "2026-01-15", until: "2026-12-31", want: 1},
		{name: "last charge the day before the window", since: "2026-01-16", until: "2026-12-31", want: 0},
		{name: "first charge on the last day of the window", since: "2000-01-01", until: "2025-10-17", want: 1},
		{name: "first charge the day after the window", since: "2000-01-01", until: "2025-10-16", want: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ended := monthlyEndingOn(t, "2026-01-15", 4)
			window := store.Window{Since: dateOf(t, c.since), Until: dateOf(t, c.until)}

			result := recurringIn(t, window, ended)

			assert.Len(t, result.Series, c.want)
		})
	}
}

func Test_recurring_lists_an_active_series_whose_last_charge_is_before_the_window(t *testing.T) {
	monthly := monthlyEndingOn(t, "2026-09-01", 3, paidTo("payee-gym", "Gym"))
	weekly := everyDaysEndingOn(t, "2026-09-01", 7, 4, paidTo("payee-paper", "Paper"))
	window := store.Window{Since: dateOf(t, "2026-09-20"), Until: dateOf(t, "2026-09-29")}

	result := recurringIn(t, window, monthly, weekly)

	assert.Equal(t, []string{"Gym"}, payeesOf(result))
}
