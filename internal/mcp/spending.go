package mcp

import (
	"context"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// spending answers the spending tool.
func (s *Server) spending(context.Context, spendingInput) (any, error) {
	return document.NewSpending(report.Spending{}, nil), nil
}
