package mcp

import (
	"context"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// searchTransactions lists the newest transactions matching the call, as search --json does. A cut is told by one
// warning worded for the limit asked. It reads neither the clock nor the config file.
func (s *Server) searchTransactions(ctx context.Context, in searchInput) (any, error) {
	// Text, amounts, window: the order the command line refuses them in.
	if err := report.CheckSearchText(in.Text); err != nil {
		return nil, err //nolint:wrapcheck // the client gets the command line's own refusal text
	}
	amounts, err := report.ParseSearchAmounts(in.Min, in.Max)
	if err != nil {
		return nil, err //nolint:wrapcheck // the client gets the command line's own refusal text
	}
	window, err := report.ParseSearchWindow(in.Since, in.Until)
	if err != nil {
		return nil, windowRefusal(err)
	}
	srv, err := s.newReport(ctx, commandName)
	if err != nil {
		return nil, err
	}
	found, err := srv.Search(ctx, report.SearchRequest{
		Window: window, Accounts: in.Accounts, Text: in.Text, Category: in.Category, Amounts: amounts, Limit: in.Limit,
	})
	if err != nil {
		return nil, accountRefusal(err)
	}
	warnings := document.SearchWarnings(found)
	if found.Truncated() {
		warnings = append(warnings, searchCutLine(found, in.Limit))
	}
	return document.NewSearch(found, warnings), nil
}

// searchCutLine is the warning that found lists only its newest transactions; limit is the limit asked.
// At the most the tool allows, only narrowing helps.
func searchCutLine(found report.Search, limit int) string {
	const narrow = "narrow the search with text, since, until, accounts, category, min or max"
	line := toolSearch + " lists the newest " + humanize.Thousands(len(found.Rows)) + " of " + humanize.Thousands(found.Matched) + " matching transactions; "
	if limit < maxRows {
		return line + "pass a higher limit, up to " + humanize.Thousands(maxRows) + ", or " + narrow
	}
	return line + narrow
}
