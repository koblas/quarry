package duckstore

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// Spending reads spending as params says.
func (s *Store) Spending(ctx context.Context, params store.SpendingParams) (store.Spending, error) {
	return store.Spending{}, nil
}
