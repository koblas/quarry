package fx

import (
	"time"

	"github.com/koblas/quarry/internal/store"
)

// The Valet series: FXUSDCAD is the current daily rate; IEXE0101 is the discontinued legacy series
// that covers the dates before FXUSDCAD begins.
const (
	seriesCurrent = "FXUSDCAD"
	seriesLegacy  = "IEXE0101"
)

// askSpan is one run of dates to fetch. legacy says the legacy series may cover the days FXUSDCAD does not.
type askSpan struct {
	span   store.DateSpan
	legacy bool
}

// planSpans returns the runs of Need that Have does not cover, oldest first. Only the run that ends
// before Have.First (or all of Need when Have is empty) may use the legacy series: history never
// gains older FXUSDCAD days, but later days are only ever FXUSDCAD's.
func planSpans(req store.RatesRequest) []askSpan {
	need, have := req.Need, req.Have
	if empty(need) {
		return nil
	}
	if empty(have) {
		return []askSpan{{span: need, legacy: true}}
	}
	var spans []askSpan
	if need.First.Before(have.First) {
		spans = append(spans, askSpan{span: store.DateSpan{First: need.First, Last: earlier(need.Last, dayBefore(have.First))}, legacy: true})
	}
	if need.Last.After(have.Last) {
		spans = append(spans, askSpan{span: store.DateSpan{First: later(need.First, dayAfter(have.Last)), Last: need.Last}})
	}
	return spans
}

func empty(s store.DateSpan) bool { return s.First.IsZero() }

func dayBefore(t time.Time) time.Time { return t.AddDate(0, 0, -1) }
func dayAfter(t time.Time) time.Time  { return t.AddDate(0, 0, 1) }

func earlier(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
