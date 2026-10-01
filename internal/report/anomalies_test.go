package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// thisYear is the window the anomalies tests list: January 1 through recurringNow's day.
var thisYear = store.Window{
	Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	Until: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC),
}

func Test_anomalies_thresholds_hold_at_their_boundaries(t *testing.T) {
	cases := []struct {
		name   string
		amount int64
		want   []int64
	}{
		{name: "exactly twice the median is not listed", amount: 12000, want: []int64{}},
		{name: "a cent over twice the median is listed", amount: 12001, want: []int64{12001}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			charges := append(earlierCharges(t, 3, 6000), chargeOn(t, 9, "2026-09-01", ofAmount(c.amount)))

			got := anomaliesIn(t, thisYear, charges)

			assert.Equal(t, c.want, amountsOf(got.Listed))
		})
	}
}
