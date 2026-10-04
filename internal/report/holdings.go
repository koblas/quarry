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

// Converted is h's value in the reporting currency in cents; nil in a native listing and when no
// conversion exists (no price, no currency, another currency, no rate).
func (l Holdings) Converted(h store.Holding) *big.Int {
	if l.Currency == money.CAD {
		return h.ValueCAD
	}
	if l.Currency == money.USD {
		return h.ValueUSD
	}
	return nil
}

// Holdings lists the holdings on req.AsOf with the total of their values in req.Currency.
// It refuses like Status, and reads the store once.
func (s *Server) Holdings(ctx context.Context, req HoldingsRequest) (Holdings, error) {
	read, err := s.store.Holdings(ctx, store.HoldingsParams{AsOf: req.AsOf})
	if err != nil {
		return Holdings{}, s.readRefusal(ctx, "holdings", err)
	}
	listing := Holdings{Rows: read.Holdings, AsOf: req.AsOf, Currency: req.Currency}
	listing.Totals = listing.total()
	return listing, nil
}

// total is the sum of the rows' converted values, one entry in the reporting currency; none when no row has one.
func (l Holdings) total() []HoldingsTotal {
	sum := new(big.Int)
	contributed := false
	for _, row := range l.Rows {
		if value := l.Converted(row); value != nil {
			sum.Add(sum, value)
			contributed = true
		}
	}
	if !contributed {
		return nil
	}
	return []HoldingsTotal{{Currency: l.Currency.String(), Value: sum}}
}
