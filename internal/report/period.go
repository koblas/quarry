package report

import (
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// monthLabelLayout renders a month as YYYY-MM, the key the store gives a month row.
const monthLabelLayout = "2006-01"

// period is one calendar span the window touches: Label is its display key, and Partial
// is set when the window starts after the span's first day or ends before its last.
type period struct {
	Label   string
	Partial bool
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
			Label:   first.Format(layout),
			Partial: window.Since.After(first) || window.Until.Before(last),
		})
	}
	return series
}

// periodKey identifies a report row: the period's label and the row's currency.
type periodKey struct{ label, currency string }

// currencyList is the currency of each of totals, in order.
func currencyList[T any](totals []T, currency func(T) string) []string {
	currencies := make([]string, len(totals))
	for i, t := range totals {
		currencies[i] = currency(t)
	}
	return currencies
}

// fillSeries is a row for each period of series: the stored row keyed by keyOf, else blank(key),
// wrapped with the period's Partial; which currencies a period lists is fillOrder's. No currencies
// (the empty window the empty-window note reports) gives no rows in any mode.
func fillSeries[S, R any](series []period, currencies []string, target string, stored []S, keyOf func(S) periodKey, blank func(periodKey) S, wrap func(S, bool) R) []R {
	if len(currencies) == 0 {
		return []R{}
	}
	found := make(map[periodKey]S, len(stored))
	for _, r := range stored {
		found[keyOf(r)] = r
	}
	// Empty target (native) blank-fills every currency in every period; else only target, and another currency appears where stored.
	order := fillOrder(currencies, target)
	rows := make([]R, 0, len(series)*len(order))
	for _, p := range series {
		for _, currency := range order {
			key := periodKey{label: p.Label, currency: currency}
			row, ok := found[key]
			if !ok {
				if target != "" && currency != target {
					continue
				}
				row = blank(key)
			}
			rows = append(rows, wrap(row, p.Partial))
		}
	}
	return rows
}

// fillOrder is the currencies a period lists, in order: currencies as given for a native report
// (empty target), else target first and the others after it.
func fillOrder(currencies []string, target string) []string {
	if target == "" {
		return currencies
	}
	order := []string{target}
	for _, c := range currencies {
		if c != target {
			order = append(order, c)
		}
	}
	return order
}

// fillTarget is the currency fillSeries must always list: c's code, or empty for native, which
// converts nothing and so has no currency of its own to fill.
func fillTarget(c money.Currency) string {
	if c == money.Native {
		return ""
	}
	return c.String()
}
