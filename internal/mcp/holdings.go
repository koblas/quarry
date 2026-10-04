package mcp

import (
	"context"
	"errors"
	"strconv"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// holdingsTwin is the quarry command whose output the holdings tool matches.
const holdingsTwin = "holdings"

// holdings lists the holdings on the call's day in its accounts and currency, as holdings --json does,
// but at most maxRows of them; totals count every holding and a cut adds a warning.
func (s *Server) holdings(ctx context.Context, in holdingsInput) (any, error) {
	asOf, err := report.ResolveAsOf(in.AsOf, s.now())
	if err != nil {
		return nil, asOfRefusal(err)
	}
	currency, configWarnings, err := s.resolveCurrency(in.Currency, holdingsTwin)
	if err != nil {
		return nil, err
	}
	srv, err := s.newReport(ctx, commandName)
	if err != nil {
		return nil, err
	}
	held, err := srv.Holdings(ctx, report.HoldingsRequest{AsOf: asOf, Currency: currency, Accounts: in.Accounts})
	if err != nil {
		return nil, accountRefusal(err)
	}
	doc := document.NewHoldings(held, append(configWarnings, document.HoldingsWarnings(held)...))
	doc.Holdings, doc.Warnings = capList(doc.Holdings, doc.Warnings, toolHoldings, "holdings",
		"totals count every holding; pass fewer accounts, or query v_holdings where date = '"+doc.AsOf+"' for the rest")
	return doc, nil
}

// asOfRefusedError is the isError text of an as_of the model must fix.
type asOfRefusedError string

func (e asOfRefusedError) Error() string { return string(e) }

// asOfRefusal is err, an as_of refusal, worded for the model: the argument is named as_of, and the stderr
// line is the class line, which never carries the caller's value. Any other error comes back as is.
func asOfRefusal(err error) error {
	refusal, ok := errors.AsType[report.AsOfError](err)
	if !ok {
		// unreachable: report.ResolveAsOf (internal/report/asof.go:36-41) returns Today or ParseAsOf's result, whose only errors are AsOfError (:49, :53)
		return err
	}
	return withLog(asOfRefusedError(asOfWording(refusal)), asOfRefusedLog)
}

// asOfWording is the model's text for refusal; each kind's tail names the argument that fixes it.
func asOfWording(refusal report.AsOfError) string {
	switch refusal.Kind {
	case report.AsOfAfterToday:
		return "as_of " + refusal.Value + " is after today; holdings are valued up to today only, so pass an earlier as_of"
	case report.AsOfNotADate:
	}
	return "as_of " + strconv.Quote(refusal.Value) + " is not a date; use YYYY, YYYY-MM or YYYY-MM-DD"
}
