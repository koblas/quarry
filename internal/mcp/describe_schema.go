package mcp

import (
	"context"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/report/document"
)

// describeSchema describes the store from one read through a report server built for this call:
// at most maxRows accounts and categories each, with a warning for each list it cut.
func (s *Server) describeSchema(ctx context.Context, _ noInput) (any, error) {
	srv, err := s.newReport(ctx, commandName)
	if err != nil {
		return nil, err
	}
	schema, err := srv.DescribeSchema(ctx, maxRows)
	if err != nil {
		return nil, err //nolint:wrapcheck // a RefusalError is the tool's answer, sent verbatim
	}
	warnings := []string{}
	if schema.AccountsTotal > len(schema.Accounts) {
		warnings = append(warnings, listCutWarning(toolDescribe, "accounts", "accounts", schema.AccountsTotal))
	}
	if schema.CategoriesTotal > len(schema.Categories) {
		warnings = append(warnings, listCutWarning(toolDescribe, "categories", "categories", schema.CategoriesTotal))
	}
	return document.NewSchema(schema, warnings), nil
}

// listCutWarning is the document warning that tool listed only the first maxRows of total entries
// of kind, and that table holds the rest.
func listCutWarning(tool, kind, table string, total int) string {
	return tool + " lists the first " + humanize.Thousands(maxRows) + " " + kind + " of " + humanize.Thousands(total) +
		"; query the " + table + " table for the rest"
}
