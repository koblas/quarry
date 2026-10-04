package report

import (
	"context"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// NetWorthRequest is what NetWorth reads: the net worth on AsOf, shown in Currency.
type NetWorthRequest struct {
	AsOf     time.Time
	Currency money.Currency
}

// NetWorth is the net worth on AsOf, shown in Currency.
type NetWorth struct {
	Rows     []store.NetWorthRow
	AsOf     time.Time
	Currency money.Currency
}

// NetWorth lists the net worth on req.AsOf in req.Currency.
func (s *Server) NetWorth(context.Context, NetWorthRequest) (NetWorth, error) {
	return NetWorth{}, nil
}
