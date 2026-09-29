package duckstore

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// CashFlow reads income and spending in params.Window, per period and currency.
func (s *Store) CashFlow(context.Context, store.CashFlowParams) (store.CashFlow, error) {
	return store.CashFlow{}, nil
}
