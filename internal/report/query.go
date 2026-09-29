package report

import (
	"context"
	"math"

	"github.com/koblas/quarry/internal/store"
)

// QueryResult is a query's result cut to the limit, and whether rows were cut.
type QueryResult struct {
	store.QueryResult

	Truncated bool
}

// Query runs query against the store and keeps at most limit rows, setting
// Truncated when the query returned more; a limit of 0 or less keeps every
// row. A store that cannot be opened is refused with a RefusalError; other
// store errors, an interrupted query's included, are returned unchanged.
func (s *Server) Query(ctx context.Context, query string, limit int) (QueryResult, error) {
	maxRows := 0
	if limit > 0 && limit < math.MaxInt {
		// One row past the limit is enough to tell whether there were more.
		maxRows = limit + 1
	}
	result, err := s.store.Query(ctx, query, maxRows)
	if err != nil {
		return QueryResult{}, storeRefusal(err, s.home)
	}
	if limit <= 0 || len(result.Rows) <= limit {
		return QueryResult{QueryResult: result}, nil
	}
	result.Rows = result.Rows[:limit]
	return QueryResult{QueryResult: result, Truncated: true}, nil
}
