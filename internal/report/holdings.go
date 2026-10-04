package report

import (
	"context"
	"math/big"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// HoldingsRequest is what Holdings reads: the holdings on AsOf, shown in Currency.
type HoldingsRequest struct {
	AsOf     time.Time
	Currency money.Currency
}

// HoldingsTotal is the sum of the converted values of the rows that have one, in Currency.
type HoldingsTotal struct {
	Currency string
	Value    *big.Int
}

// Holdings is the holdings on AsOf, in the store's order, with their totals.
type Holdings struct {
	Rows     []store.Holding
	Totals   []HoldingsTotal
	AsOf     time.Time
	Currency money.Currency
}

// Holdings lists the holdings on req.AsOf.
func (s *Server) Holdings(context.Context, HoldingsRequest) (Holdings, error) {
	return Holdings{}, nil
}
