package duckstore_test

import (
	"context"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/koblas/quarry/internal/fx"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/require"
)

// dailySource answers FXUSDCAD with one observation for every day of every span it is asked.
type dailySource struct{}

func (dailySource) Observations(_ context.Context, series string, sp store.DateSpan) ([]fx.Observation, error) {
	var out []fx.Observation
	for date := sp.First; series == store.SeriesCurrent && !date.After(sp.Last); date = date.AddDate(0, 0, 1) {
		out = append(out, fx.Observation{Date: date, Rate: 1_300_000})
	}
	return out, nil
}

// syncOn is one sync: its earliest transaction and the clock, both in days after 2000-01-01, the bubble's start.
type syncOn struct{ transaction, clock int }

const fxRatesSpanText = `SELECT CAST(min(date) AS VARCHAR) || ' ' || CAST(max(date) AS VARCHAR) || ' ' || CAST(count(*) AS VARCHAR) FROM fx_rates`

// Moves the process zone and the clock: no t.Parallel. The zone is pinned so each bubble's clock reads as its UTC date.
func Test_a_second_sync_leaves_the_stored_rates_one_unbroken_interval(t *testing.T) {
	saved := time.Local                      //nolint:gosmopolitan // the test pins the process-local zone; Cleanup restores it
	time.Local = time.UTC                    //nolint:gosmopolitan // see above
	t.Cleanup(func() { time.Local = saved }) //nolint:gosmopolitan // restores the zone
	cases := []struct {
		name          string
		first, second syncOn
	}{
		{name: "the second sync reaches back before the stored rates", first: syncOn{transaction: 10, clock: 20}, second: syncOn{transaction: 0, clock: 20}},
		{name: "the second sync reaches forward past the stored rates", first: syncOn{transaction: 10, clock: 20}, second: syncOn{transaction: 10, clock: 25}},
		{name: "the second sync reaches both ways", first: syncOn{transaction: 10, clock: 20}, second: syncOn{transaction: 0, clock: 25}},
		{name: "the second sync needs only days already stored", first: syncOn{transaction: 0, clock: 20}, second: syncOn{transaction: 5, clock: 12}},
		{name: "the second sync's earliest transaction is after the stored rates", first: syncOn{transaction: 0, clock: 5}, second: syncOn{transaction: 10, clock: 20}},
		{name: "the clock moves back so the second sync needs only days before the stored rates", first: syncOn{transaction: 10, clock: 20}, second: syncOn{transaction: -12, clock: 0}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := duckstore.New(t.TempDir(), duckstore.WithRates(fx.NewServer(fx.WithSource(dailySource{}))))
			sync := func(s syncOn) store.Replaced {
				var replaced store.Replaced
				synctest.Test(t, func(t *testing.T) {
					time.Sleep(time.Duration(s.clock) * 24 * time.Hour)
					rows := minimalRows()
					rows.Transactions[0].Date = day(2000, 1, 1).AddDate(0, 0, s.transaction)

					var err error
					replaced, err = st.Replace(t.Context(), rows)

					require.NoError(t, err)
				})
				return replaced
			}
			sync(c.first)

			replaced := sync(c.second)

			first, last := min(c.first.transaction, c.second.transaction), max(c.first.clock, c.second.clock)
			wantFirst, wantLast := day(2000, 1, 1).AddDate(0, 0, first), day(2000, 1, 1).AddDate(0, 0, last)
			assertScalar(t, openReadOnly(t, replaced.Path), fxRatesSpanText,
				fmt.Sprintf("%s %s %d", wantFirst.Format(time.DateOnly), wantLast.Format(time.DateOnly), last-first+1))
		})
	}
}
