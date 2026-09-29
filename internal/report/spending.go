package report

import (
	"context"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// SpendRequest is what a spend read needs from its caller: the window to
// count, resolved by ParseWindow or DefaultWindow, what to group by
// (category when unset), and the accounts to count (each an id or a name;
// none means every account).
type SpendRequest struct {
	Window   store.Window
	By       store.SpendingGroup
	Accounts []string
}

// SpendingRow is one row of a spend read: what the store found, or for a
// filled month zero spending. Partial marks a month the window cuts short.
type SpendingRow struct {
	store.SpendingRow

	Partial bool
}

// Spending is a spend read: the window it covered, what it was grouped by,
// the accounts it counted and what the store found.
type Spending struct {
	Rows   []SpendingRow
	Totals []store.SpendingTotal
	// MultiTagSplits counts the splits carrying more than one tag; it is set
	// only when grouping by tag.
	MultiTagSplits int

	Window store.Window
	By     store.SpendingGroup
	// Accounts is the accounts the request named, in the order given and
	// without repeats; empty means every account was counted.
	Accounts []store.Account
}

// DefaultWindow is January 1 of now's year through now's day, both read in
// now's own zone.
func DefaultWindow(now time.Time) store.Window {
	year, month, day := now.Date()
	return store.Window{
		Since: time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(year, month, day, 0, 0, 0, 0, time.UTC),
	}
}

// Spend reads the spending inside req.Window, grouped by req.By.
func (s *Server) Spend(ctx context.Context, req SpendRequest) (Spending, error) {
	spending, err := s.store.Spending(ctx, store.SpendingParams{Window: req.Window, By: req.By})
	if err != nil {
		return Spending{}, s.readRefusal(ctx, "spend", err)
	}
	result := Spending{
		Totals:         spending.Totals,
		MultiTagSplits: spending.MultiTagSplits,
		Window:         req.Window,
		By:             req.By,
	}
	for _, r := range spending.Rows {
		result.Rows = append(result.Rows, SpendingRow{SpendingRow: r})
	}
	if req.By == store.SpendByMonth {
		result.Rows = fillMonths(result, req.Window)
	}
	return result, nil
}

// fillMonths is a month spending's rows with a row for every month of the window in every
// currency of its Totals: the store's row where it has one, else zero.
func fillMonths(spending Spending, window store.Window) []SpendingRow {
	type monthCurrency struct{ label, currency string }
	found := make(map[monthCurrency]SpendingRow, len(spending.Rows))
	for _, r := range spending.Rows {
		found[monthCurrency{*r.Key, r.Currency}] = r
	}
	series := monthSeries(window)
	rows := make([]SpendingRow, 0, len(series)*len(spending.Totals))
	for _, p := range series {
		for _, total := range spending.Totals {
			row, ok := found[monthCurrency{p.Label, total.Currency}]
			if !ok {
				row = SpendingRow{Key: new(p.Label), Currency: total.Currency}
			}
			row.Partial = p.Partial
			rows = append(rows, row)
		}
	}
	return rows
}
