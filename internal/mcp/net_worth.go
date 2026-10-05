package mcp

import (
	"context"

	"github.com/koblas/quarry/internal/report/document"
)

// netWorth is the net_worth tool: net worth on one day or at each month end, as networth --json.
func (s *Server) netWorth(_ context.Context, _ netWorthInput) (any, error) {
	return document.NetWorth{}, nil
}
