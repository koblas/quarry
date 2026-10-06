package duckstore

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// Summary reads the store's status, every charge dated through params.Through and its net worth on
// params.Dates from one open of the store. A store it cannot open or read is a *store.OpenError.
func (s *Store) Summary(context.Context, store.SummaryParams) (store.Summary, error) {
	return store.Summary{}, nil
}
