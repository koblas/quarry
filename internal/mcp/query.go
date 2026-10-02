package mcp

import (
	"context"
	"errors"
	"strings"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/report/document"
)

// errBlankSQL refuses a query holding nothing but whitespace.
var errBlankSQL = errors.New("query needs SQL in the sql parameter")

// query runs in.SQL read-only and returns the sql document: at most the
// effective limit's rows, with a warning when it cut some off. The report
// server is built for this call, so the store is read as it is now.
func (s *Server) query(ctx context.Context, in queryInput) (any, error) {
	if strings.TrimSpace(in.SQL) == "" {
		return nil, errBlankSQL
	}
	srv, err := s.newReport(ctx, commandName)
	if err != nil {
		return nil, err
	}
	limit := effectiveLimit(in.Limit)
	result, err := srv.Query(ctx, in.SQL, limit)
	if err != nil {
		return nil, queryRefusal(err)
	}
	warnings := []string{}
	if result.Truncated {
		warnings = append(warnings, truncationWarning(limit))
	}
	return document.NewSQL(result, limit, warnings), nil
}

// effectiveLimit is limit when it is within 1..maxRows, else maxRows: the cap holds even for a
// call the schema did not check.
func effectiveLimit(limit int) int {
	if limit < 1 || limit > maxRows {
		return maxRows
	}
	return limit
}

// truncationWarning is the document warning that the rows were cut at limit.
func truncationWarning(limit int) string {
	return "returned the first " + humanize.Count(limit, "row", "rows") + "; the query has more; aggregate or filter in SQL to see the rest"
}
