package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_default_window_runs_from_january_first_to_today_in_the_instants_own_zone(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
		want store.Window
	}{
		{
			name: "mid-year",
			now:  time.Date(2026, 9, 29, 22, 0, 0, 0, time.FixedZone("UTC-5", -5*60*60)),
			want: store.Window{Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)},
		},
		{
			name: "an instant already in the next year in UTC",
			now:  time.Date(2025, 12, 31, 22, 0, 0, 0, time.FixedZone("UTC-5", -5*60*60)),
			want: store.Window{Since: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Until: time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, report.DefaultWindow(c.now))
		})
	}
}

func Test_spend_reads_this_years_spending_by_category(t *testing.T) {
	now := time.Date(2026, 9, 29, 22, 0, 0, 0, time.FixedZone("UTC-5", -5*60*60))
	groceries := "Groceries"
	spending := store.Spending{
		Rows:   []store.SpendingRow{{Key: &groceries, Currency: "CAD", Spent: 1250}},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 1250}},
	}
	var got store.SpendingParams
	srv := report.NewServer(report.WithStore(fakeStore{spending: spending, gotSpending: &got}))

	result, err := srv.Spend(t.Context(), report.SpendRequest{Now: now})

	require.NoError(t, err)
	window := store.Window{Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)}
	assert.Equal(t, store.SpendingParams{Window: window, By: store.SpendByCategory}, got)
	assert.Equal(t, report.Spending{Spending: spending, Window: window}, result)
}

func Test_spend_reads_the_requested_grouping_and_returns_it_with_the_result(t *testing.T) {
	var got store.SpendingParams
	srv := report.NewServer(report.WithStore(fakeStore{gotSpending: &got}))

	result, err := srv.Spend(t.Context(), report.SpendRequest{Now: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC), By: store.SpendByPayee})

	require.NoError(t, err)
	assert.Equal(t, store.SpendByPayee, got.By)
	assert.Equal(t, store.SpendByPayee, result.By)
}

func Test_spend_returns_the_store_fault(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Spend(t.Context(), report.SpendRequest{Now: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)})

	assert.Equal(t, errDiskRead, err)
}
