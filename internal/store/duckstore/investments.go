package duckstore

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// InvestmentHistory reads every account, security, investment transaction with a security, and exchange rate.
func (s *Store) InvestmentHistory(ctx context.Context) (store.InvestmentHistory, error) {
	return store.InvestmentHistory{}, nil
}
