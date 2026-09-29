package report

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// CashFlowRequest is what a cash-flow read needs from its caller: the window to count,
// the period unit, and the accounts to count (each an id or a name; none means every account).
type CashFlowRequest struct {
	Window   store.Window
	By       store.CashFlowPeriod
	Accounts []string
}

// CashFlow is a cash-flow read: what the store found for the window it covered.
type CashFlow struct {
	Window store.Window
}

// CashFlow reads the income and spending inside req.Window.
func (s *Server) CashFlow(context.Context, CashFlowRequest) (CashFlow, error) {
	return CashFlow{}, nil
}
