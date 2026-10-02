package fx

import (
	"time"

	"github.com/koblas/quarry/internal/store"
)

// The Valet series: FXUSDCAD is current; IEXE0101 is the discontinued one covering the days before it.
const (
	seriesCurrent = store.SeriesCurrent
	seriesLegacy  = store.SeriesLegacy
)

// askSpan is one run of dates to fetch. legacy says the legacy series may cover the days FXUSDCAD does not.
type askSpan struct {
	span   store.DateSpan
	legacy bool
}

// planSpans returns the runs to ask so that Have plus those runs is one unbroken interval covering Need. A run
// before Have ends the day before Have.First, even when Need ends sooner, and a run after Have starts the day after
// Have.Last, even when Need starts later: a stored gap would be claimed by the floor and converted at a stale rate.
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
		spans = append(spans, askSpan{span: store.DateSpan{First: need.First, Last: dayBefore(have.First)}, legacy: true})
	}
	if need.Last.After(have.Last) {
		spans = append(spans, askSpan{span: store.DateSpan{First: dayAfter(have.Last), Last: need.Last}})
	}
	return spans
}

// empty reports a span with no first day, or one that ends before it starts.
func empty(s store.DateSpan) bool { return s.First.IsZero() || s.Last.Before(s.First) }

func dayBefore(t time.Time) time.Time { return t.AddDate(0, 0, -1) }
func dayAfter(t time.Time) time.Time  { return t.AddDate(0, 0, 1) }
