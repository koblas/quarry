package report

import (
	"context"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// SpendRequest is what a spend read needs from its caller: the window to
// count, resolved by ParseWindow or DefaultWindow, and what to group by
// (category when unset).
type SpendRequest struct {
	Window store.Window
	By     store.SpendingGroup
}

// Spending is a spend read: the window it covered, what it was grouped by
// and what the store found.
type Spending struct {
	store.Spending

	Window store.Window
	By     store.SpendingGroup
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
	return Spending{Spending: spending, Window: req.Window, By: req.By}, nil
}
