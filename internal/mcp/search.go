package mcp

import (
	"context"
	"errors"
	"strconv"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// searchTransactions lists the newest transactions matching the call, as search --json does. A cut is told by one
// warning worded for the limit asked. It reads neither the clock nor the config file.
func (s *Server) searchTransactions(ctx context.Context, in searchInput) (any, error) {
	// Text, amounts, window: the order the command line refuses them in.
	if err := report.CheckSearchText(in.Text); err != nil {
		return nil, textRefusal(err)
	}
	amounts, err := report.ParseSearchAmounts(in.Min, in.Max)
	if err != nil {
		return nil, amountRefusal(err)
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
		return nil, categoryRefusal(accountRefusal(err))
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

// textRefusedError is the isError text of blank search text.
type textRefusedError string

func (e textRefusedError) Error() string { return string(e) }

// amountRefusedError is the isError text of a refused min or max.
type amountRefusedError string

func (e amountRefusedError) Error() string { return string(e) }

// textRefusal is err, a refusal of the search text, worded for the model with the stderr class line. Any other error comes back as is.
func textRefusal(err error) error {
	if !errors.Is(err, report.ErrBlankSearchText) {
		return err
	}
	return withLog(textRefusedError("text is blank; leave it out to search by date, account, category or amount alone"), textRefusedLog)
}

// amountRefusal is err, a refusal of min or max, worded from its parts in the tool's argument names, with the
// stderr class line, which never carries the caller's values. Any other error comes back as is.
func amountRefusal(err error) error {
	refusal, ok := errors.AsType[report.AmountError](err)
	if !ok {
		return err
	}
	return withLog(amountRefusedError(amountWording(refusal)), amountRefusedLog)
}

// amountWording is the model's text for refusal.
func amountWording(refusal report.AmountError) string {
	var line string
	switch refusal.Kind {
	case report.AmountNotAnAmount:
		line = refusal.Bound + " " + strconv.Quote(refusal.Value) + ` is not an amount; use digits with up to 2 decimals and no sign, such as "25" or "19.99"`
	case report.AmountMinAboveMax:
		line = refusal.Bound + " " + refusal.Value + " is more than max " + refusal.Other
	}
	return line
}
