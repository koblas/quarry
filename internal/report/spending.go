package report

import (
	"context"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// SpendRequest is what a spend read needs from its caller: the current
// instant, in the zone whose calendar day counts as today.
type SpendRequest struct {
	Now time.Time
}

// Spending is a spend read: the window it covered and what the store found.
type Spending struct {
	store.Spending

	Window store.Window
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

// Spend reads this year's spending by category up to today.
func (s *Server) Spend(ctx context.Context, req SpendRequest) (Spending, error) {
	window := DefaultWindow(req.Now)
	spending, err := s.store.Spending(ctx, store.SpendingParams{Window: window, By: store.SpendByCategory})
	if err != nil {
		return Spending{}, s.readRefusal(ctx, "spend", err)
	}
	return Spending{Spending: spending, Window: window}, nil
}
