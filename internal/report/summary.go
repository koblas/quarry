package report

import (
	"context"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// SummaryRequest is what a summary needs from its caller: the month to summarize and the currency its
// amounts are listed in.
type SummaryRequest struct {
	Month    Month
	Currency money.Currency
}

// Summary is one month read from one open of the store: the store's status, the month's unusually large
// charges, the recurring series new in the month, and net worth at the month end before and at the month's
// end, with the change between them.
type Summary struct {
	Month     Month
	Currency  money.Currency
	Status    store.Status
	Anomalies Anomalies
	Recurring Recurring
	NetWorth  NetWorth
	// Change is nil when the first month end has no balance in any currency.
	Change *NetWorthChange
}

// Summary summarizes req.Month from one read of the store.
func (s *Server) Summary(context.Context, SummaryRequest) (Summary, error) { return Summary{}, nil }
