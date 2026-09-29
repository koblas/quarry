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
	return store.Window{}
}

// Spend reads this year's spending by category up to today.
func (s *Server) Spend(ctx context.Context, req SpendRequest) (Spending, error) {
	return Spending{}, nil
}
