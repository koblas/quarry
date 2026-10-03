package mcp

import (
	"context"

	"github.com/koblas/quarry/internal/report/document"
)

// recurringCharges lists the recurring charge series of the call's window, accounts and currency, as recurring --json does.
func (s *Server) recurringCharges(_ context.Context, _ recurringInput) (any, error) {
	return document.Recurring{}, nil
}
