package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func spendByMonth(t *testing.T, spending store.Spending, since, until time.Time) report.Spending {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{spending: spending}))

	result, err := srv.Spend(t.Context(), report.SpendRequest{
		Window: store.Window{Since: since, Until: until},
		By:     store.SpendByMonth,
	})

	require.NoError(t, err)
	return result
}

// cadOnly is a spending with a CAD total, so a month series has a currency to fill.
func cadOnly() store.Spending {
	return store.Spending{Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 1}}}
}

func partialFlags(result report.Spending) []bool {
	flags := make([]bool, len(result.Rows))
	for i, r := range result.Rows {
		flags[i] = r.Partial
	}
	return flags
}

func monthLabels(result report.Spending) []string {
	labels := make([]string, len(result.Rows))
	for i, r := range result.Rows {
		labels[i] = *r.Key
	}
	return labels
}

func Test_spend_by_month_marks_the_first_month_partial_only_when_the_window_starts_after_its_first_day(t *testing.T) {
	cases := []struct {
		name  string
		since time.Time
		want  []bool
	}{
		{name: "since on the first day", since: day(2026, time.January, 1), want: []bool{false, false}},
		{name: "since on the second day", since: day(2026, time.January, 2), want: []bool{true, false}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := spendByMonth(t, cadOnly(), c.since, day(2026, time.February, 28))

			assert.Equal(t, c.want, partialFlags(result))
		})
	}
}

func Test_spend_by_month_marks_the_last_month_partial_only_when_the_window_ends_before_its_last_day(t *testing.T) {
	cases := []struct {
		name  string
		until time.Time
		want  []bool
	}{
		{name: "until on the last day", until: day(2026, time.February, 28), want: []bool{false, false}},
		{name: "until the day before the last", until: day(2026, time.February, 27), want: []bool{false, true}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := spendByMonth(t, cadOnly(), day(2026, time.January, 1), c.until)

			assert.Equal(t, c.want, partialFlags(result))
		})
	}
}

func Test_spend_by_month_ends_february_on_its_last_day_in_leap_and_common_years(t *testing.T) {
	cases := []struct {
		name  string
		since time.Time
		until time.Time
		want  bool
	}{
		{name: "leap year, until the 29th", since: day(2024, time.February, 1), until: day(2024, time.February, 29), want: false},
		{name: "leap year, until the 28th", since: day(2024, time.February, 1), until: day(2024, time.February, 28), want: true},
		{name: "common year, until the 28th", since: day(2025, time.February, 1), until: day(2025, time.February, 28), want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := spendByMonth(t, cadOnly(), c.since, c.until)

			assert.Equal(t, []bool{c.want}, partialFlags(result))
		})
	}
}

func Test_spend_by_month_runs_the_series_across_a_year_end(t *testing.T) {
	result := spendByMonth(t, cadOnly(), day(2025, time.November, 10), day(2026, time.February, 10))

	assert.Equal(t, []string{"2025-11", "2025-12", "2026-01", "2026-02"}, monthLabels(result))
}

func Test_spend_by_month_fills_a_month_with_no_spending_as_zero(t *testing.T) {
	spending := store.Spending{
		Rows: []store.SpendingRow{
			{Key: new("2026-01"), Currency: "CAD", Spent: 1250},
			{Key: new("2026-03"), Currency: "CAD", Spent: 400},
		},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 1650}},
	}

	result := spendByMonth(t, spending, day(2026, time.January, 1), day(2026, time.March, 31))

	assert.Equal(t, []report.SpendingRow{
		{Key: new("2026-01"), Currency: "CAD", Spent: 1250},
		{Key: new("2026-02"), Currency: "CAD", Spent: 0},
		{Key: new("2026-03"), Currency: "CAD", Spent: 400},
	}, result.Rows)
}

func Test_spend_by_month_fills_every_month_for_each_currency_in_the_totals(t *testing.T) {
	spending := store.Spending{
		Rows:   []store.SpendingRow{{Key: new("2026-01"), Currency: "CAD", Spent: 900}},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 900}, {Currency: "USD", Spent: 0}},
	}

	result := spendByMonth(t, spending, day(2026, time.January, 1), day(2026, time.February, 28))

	assert.Equal(t, []report.SpendingRow{
		{Key: new("2026-01"), Currency: "CAD", Spent: 900},
		{Key: new("2026-01"), Currency: "USD", Spent: 0},
		{Key: new("2026-02"), Currency: "CAD", Spent: 0},
		{Key: new("2026-02"), Currency: "USD", Spent: 0},
	}, result.Rows)
}

func Test_spend_by_month_lists_no_rows_when_the_window_holds_no_spending(t *testing.T) {
	result := spendByMonth(t, store.Spending{}, day(2026, time.January, 1), day(2026, time.March, 31))

	assert.Empty(t, result.Rows)
}

func Test_spend_by_month_marks_a_one_month_window_cut_at_both_ends_partial(t *testing.T) {
	result := spendByMonth(t, cadOnly(), day(2026, time.March, 5), day(2026, time.March, 20))

	assert.Equal(t, []bool{true}, partialFlags(result))
}

func Test_spend_by_month_marks_only_the_first_and_last_month_partial(t *testing.T) {
	result := spendByMonth(t, cadOnly(), day(2026, time.January, 15), day(2026, time.March, 10))

	assert.Equal(t, []bool{true, false, true}, partialFlags(result))
}
