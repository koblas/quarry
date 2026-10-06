package mcp

import (
	"context"

	"github.com/koblas/quarry/internal/report/document"
)

// monthlySummary is the monthly_summary tool: summary --json's document for one ended month.
func (s *Server) monthlySummary(_ context.Context, _ monthlySummaryInput) (any, error) {
	return document.Summary{}, nil
}
