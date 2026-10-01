package report_test

import (
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// thisYear is the window the anomalies tests list: January 1 through recurringNow's day.
var thisYear = store.Window{
	Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	Until: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC),
}

func Test_anomalies_thresholds_hold_at_their_boundaries(t *testing.T) {
	cases := []struct {
		name   string
		median int64
		amount int64
		want   []int64
	}{
		{name: "under the minimum is never listed", median: 4000, amount: 9999, want: []int64{}},
		{name: "exactly twice the median is not listed", median: 6000, amount: 12000, want: []int64{}},
		{name: "a cent over twice the median is listed", median: 6000, amount: 12001, want: []int64{12001}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			earlier := chargesOn(t, everyDays(t, "2025-06-01", 40, 3), ofAmount(c.median))
			charges := slices.Concat(earlier, []store.Charge{chargeOn(t, 4, "2026-09-01", ofAmount(c.amount))})
			for i := range charges {
				charges[i].SourceID = int64(i + 1)
			}
			srv := report.NewServer(report.WithStore(fakeStore{charges: store.Charges{Rows: charges}}))

			got, err := srv.Anomalies(t.Context(), report.AnomaliesRequest{Window: thisYear, Now: recurringNow})

			require.NoError(t, err)
			listed := []int64{}
			for _, a := range got.Listed {
				listed = append(listed, a.Amount)
			}
			assert.Equal(t, c.want, listed)
		})
	}
}
