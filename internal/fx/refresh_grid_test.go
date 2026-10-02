package fx_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/fx"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gridDays is how many consecutive days, d(0)..d(gridDays-1), the Need and Have spans are drawn from.
const gridDays = 7

// everyDaySource answers FXUSDCAD with one observation for each day of every span it is asked, and records the spans.
type everyDaySource struct{ asked []store.DateSpan }

func (s *everyDaySource) Observations(_ context.Context, series string, sp store.DateSpan) ([]fx.Observation, error) {
	s.asked = append(s.asked, sp)
	if series != current {
		return nil, nil
	}
	var out []fx.Observation
	for day := sp.First; !day.After(sp.Last); day = day.AddDate(0, 0, 1) {
		out = append(out, fx.Observation{Date: day, Rate: 1_300_000})
	}
	return out, nil
}

// dayIndex is the number of days from d(0) to t.
func dayIndex(t time.Time) int { return int(t.Sub(d(0)) / (24 * time.Hour)) }

// indexesOf lists the day indexes of each span in turn, in the order given; a zero or inverted span has none.
func indexesOf(spans ...store.DateSpan) []int {
	var out []int
	for _, sp := range spans {
		if sp.First.IsZero() {
			continue
		}
		for i := dayIndex(sp.First); i <= dayIndex(sp.Last); i++ {
			out = append(out, i)
		}
	}
	return out
}

// hullIndexes lists every day from the earliest first to the latest last of the non-zero spans.
func hullIndexes(spans ...store.DateSpan) []int {
	var firsts, lasts []int
	for _, sp := range spans {
		if sp.First.IsZero() {
			continue
		}
		firsts = append(firsts, dayIndex(sp.First))
		lasts = append(lasts, dayIndex(sp.Last))
	}
	var out []int
	for i := slices.Min(firsts); i <= slices.Max(lasts); i++ {
		out = append(out, i)
	}
	return out
}

func sorted(a, b []int) []int {
	all := slices.Concat(a, b)
	slices.Sort(all)
	return all
}

// gridNeeds is the zero span, then every pair of grid days, in either order.
func gridNeeds() []store.DateSpan {
	needs := make([]store.DateSpan, 1, 1+gridDays*gridDays) // the zero span first
	for first := range gridDays {
		for last := range gridDays {
			needs = append(needs, span(d(first), d(last)))
		}
	}
	return needs
}

// gridHaves is the zero span, then every pair of grid days that does not end before it starts.
func gridHaves() []store.DateSpan {
	haves := []store.DateSpan{{}}
	for first := range gridDays {
		for last := first; last < gridDays; last++ {
			haves = append(haves, span(d(first), d(last)))
		}
	}
	return haves
}

func spanLabel(sp store.DateSpan) string {
	if sp.First.IsZero() {
		return "none"
	}
	return fmt.Sprintf("d%d..d%d", dayIndex(sp.First), dayIndex(sp.Last))
}

// gridRun is one Refresh over a Need and Have, with the day indexes it asked and the rates it returned.
type gridRun struct {
	need, have store.DateSpan
	asked      []int
	got        store.RatesRefresh
}

// overGrid runs Refresh for each of needs against every Have, and checks each run in its own subtest.
func overGrid(t *testing.T, needs []store.DateSpan, check func(a *assert.Assertions, run gridRun)) {
	t.Helper()
	for _, need := range needs {
		for _, have := range gridHaves() {
			t.Run("need "+spanLabel(need)+" have "+spanLabel(have), func(t *testing.T) {
				src := &everyDaySource{}

				got, err := newRefresher(src).Refresh(t.Context(), store.RatesRequest{Need: need, Have: have})

				require.NoError(t, err)
				check(assert.New(t), gridRun{need: need, have: have, asked: indexesOf(src.asked...), got: got})
			})
		}
	}
}

func nonEmpty(needs []store.DateSpan) []store.DateSpan {
	return slices.DeleteFunc(slices.Clone(needs), func(n store.DateSpan) bool { return n.First.IsZero() || n.Last.Before(n.First) })
}

func Test_refresh_asks_for_nothing_when_need_is_empty_or_ends_before_it_starts(t *testing.T) {
	empty := slices.DeleteFunc(gridNeeds(), func(n store.DateSpan) bool { return !n.First.IsZero() && !n.Last.Before(n.First) })

	overGrid(t, empty, func(a *assert.Assertions, run gridRun) {
		a.Empty(run.asked)
		a.Empty(run.got.Rates)
	})
}

func Test_refresh_asks_for_the_days_that_join_have_and_need_into_one_interval(t *testing.T) {
	overGrid(t, nonEmpty(gridNeeds()), func(a *assert.Assertions, run gridRun) {
		a.IsIncreasing(run.asked, "oldest first, no day twice, none inside have")
		a.Equal(hullIndexes(run.need, run.have), sorted(run.asked, indexesOf(run.have)))
	})
}

func Test_refresh_returns_a_rate_for_each_day_it_asked_for(t *testing.T) {
	overGrid(t, nonEmpty(gridNeeds()), func(a *assert.Assertions, run gridRun) {
		returned := make([]int, 0, len(run.got.Rates))
		for _, r := range run.got.Rates {
			returned = append(returned, dayIndex(r.Date))
		}
		a.Equal(hullIndexes(run.need, run.have), sorted(returned, indexesOf(run.have)))
		a.Equal(len(run.got.Rates), run.got.Added)
	})
}
