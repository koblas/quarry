package fx

import (
	"time"

	"github.com/koblas/quarry/internal/store"
)

// The Valet series: FXUSDCAD is current; IEXE0101 is the discontinued one covering the days before it.
const (
	seriesCurrent = "FXUSDCAD"
	seriesLegacy  = "IEXE0101"
)

// askSpan is one run of dates to fetch. legacy says the legacy series may cover the days FXUSDCAD does not.
type askSpan struct {
	span   store.DateSpan
	legacy bool
}

// planSpans returns the runs of Need that Have does not cover, oldest first.
func planSpans(req store.RatesRequest) []askSpan {
	need, have := req.Need, req.Have
	if empty(need) {
		return nil
	}
	if empty(have) {
		// Nothing stored: the run starts at the beginning of history, where the legacy series applies.
		return []askSpan{{span: need, legacy: true}}
	}
	var spans []askSpan
	if need.First.Before(have.First) {
		// Only a run reaching back before Have may use the legacy series; later days are only ever FXUSDCAD's.
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
