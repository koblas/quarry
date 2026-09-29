package report

import (
	"time"

	"github.com/koblas/quarry/internal/store"
)

// monthLabelLayout renders a month as YYYY-MM, the key the store gives a month row.
const monthLabelLayout = "2006-01"

// period is one calendar span the window touches: Label is its display key, and Partial
// is set when the window starts after First or ends before Last.
type period struct {
	First, Last time.Time
	Label       string
	Partial     bool
}

// monthSeries is every calendar month the window touches, oldest first.
func monthSeries(window store.Window) []period {
	var series []period
	year, month, _ := window.Since.Date()
	for first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC); !first.After(window.Until); first = first.AddDate(0, 1, 0) {
		last := first.AddDate(0, 1, -1)
		series = append(series, period{
			First:   first,
			Last:    last,
			Label:   first.Format(monthLabelLayout),
			Partial: window.Since.After(first) || window.Until.Before(last),
		})
	}
	return series
}
