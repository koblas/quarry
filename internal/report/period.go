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

// yearLabelLayout renders a year as YYYY, the key the store gives a year row.
const yearLabelLayout = "2006"

// monthSeries is every calendar month the window touches, oldest first.
func monthSeries(window store.Window) []period {
	return calendarSeries(window, 1, monthLabelLayout)
}

// yearSeries is every calendar year the window touches, oldest first.
func yearSeries(window store.Window) []period {
	return calendarSeries(window, 12, yearLabelLayout)
}

// calendarSeries is the periods of monthsPer months (1 or 12, aligned to January) the window
// touches, each labelled by layout.
func calendarSeries(window store.Window, monthsPer int, layout string) []period {
	var series []period
	year, month, _ := window.Since.Date()
	if monthsPer == 12 {
		month = time.January
	}
	for first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC); !first.After(window.Until); first = first.AddDate(0, monthsPer, 0) {
		last := first.AddDate(0, monthsPer, -1)
		series = append(series, period{
			First:   first,
			Last:    last,
			Label:   first.Format(layout),
			Partial: window.Since.After(first) || window.Until.Before(last),
		})
	}
	return series
}
