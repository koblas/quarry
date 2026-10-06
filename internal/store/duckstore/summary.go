package duckstore

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// Summary reads the store's status, every charge dated through params.Through and its net worth on
// params.Dates from one open of the store. A store it cannot open or read is a *store.OpenError.
func (s *Store) Summary(ctx context.Context, params store.SummaryParams) (store.Summary, error) {
	db, err := s.openRead(ctx)
	if err != nil {
		return store.Summary{}, err
	}
	defer func() { _ = db.Close() }()

	status, err := readStatus(ctx, db, s.Path())
	if err != nil {
		return store.Summary{}, err
	}
	charges, err := readCharges(ctx, db, s.Path(), store.ChargeParams{Through: params.Through})
	if err != nil {
		return store.Summary{}, err
	}
	netWorth, err := readNetWorth(ctx, db, s.Path(), store.NetWorthParams{Dates: params.Dates})
	if err != nil {
		return store.Summary{}, err
	}
	return store.Summary{Status: status, Charges: charges, NetWorth: netWorth}, nil
}
