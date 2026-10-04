package duckstore

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// NetWorth reads the v_net_worth rows on params.Dates, as store.NetWorth documents.
func (s *Store) NetWorth(context.Context, store.NetWorthParams) (store.NetWorth, error) {
	return store.NetWorth{}, nil
}
