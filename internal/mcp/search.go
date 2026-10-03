package mcp

import (
	"context"

	"github.com/koblas/quarry/internal/report/document"
)

// searchTransactions is a stub until its behaviour batch lands.
func (s *Server) searchTransactions(context.Context, searchInput) (any, error) {
	return document.Search{}, nil
}
