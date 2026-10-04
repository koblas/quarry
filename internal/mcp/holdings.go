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

// holdings lists the holdings on the call's day in its accounts and currency, as holdings --json does.
func (s *Server) holdings(ctx context.Context, in holdingsInput) (any, error) {
	at := s.now()
	asOf := report.Today(at)
	if in.AsOf != nil {
		parsed, err := report.ParseAsOf(*in.AsOf, at)
		if err != nil {
			return nil, asOfRefusal(err)
		}
		asOf = parsed
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
	return document.NewHoldings(held, append(configWarnings, document.HoldingsWarnings(held)...)), nil
}

// asOfRefusedError is the isError text of an as_of the model must fix.
type asOfRefusedError string

func (e asOfRefusedError) Error() string { return string(e) }

// asOfRefusal is err, an as_of refusal, worded for the model: the argument is named as_of, and the stderr
// line is the class line, which never carries the caller's value. Any other error comes back as is.
func asOfRefusal(err error) error {
	refusal, ok := errors.AsType[report.AsOfError](err)
	if !ok {
		// unreachable: report.ParseAsOf returns only an AsOfError (internal/report/asof.go:37-50)
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
