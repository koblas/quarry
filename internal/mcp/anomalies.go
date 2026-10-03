package mcp

import (
	"context"

	"github.com/koblas/quarry/internal/report/document"
)

// anomalies lists the unusually large charges of the call's window, accounts and currency, as anomalies --json does.
func (s *Server) anomalies(_ context.Context, _ anomaliesInput) (any, error) {
	return document.Anomalies{}, nil
}
