package mcp

import (
	"context"
	"slices"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// netWorthTwin is the quarry command whose output the net_worth tool matches.
const netWorthTwin = "networth"

// asOfConflictLine is the isError text of as_of given beside since or until.
const asOfConflictLine = "as_of cannot be combined with since or until; pass as_of for one day, or since and until for month ends"

// netWorth is the net_worth tool: networth --json's document for one day or, with since or until, at most
// maxRows month ends; a cut drops whole dates and a warning says so.
func (s *Server) netWorth(ctx context.Context, in netWorthInput) (any, error) {
	now := s.now()
	history := in.Since != nil || in.Until != nil
	if in.AsOf != nil && history {
		return nil, withLog(asOfRefusedError(asOfConflictLine), asOfRefusedLog)
	}
	asOf, err := report.ResolveAsOf(in.AsOf, report.NetWorthNoun, now)
	if err != nil {
		return nil, asOfRefusal(err)
	}
	request := report.NetWorthRequest{AsOf: asOf}
	if history {
		window, err := report.ParseMonthEndWindow(in.Since, in.Until, now)
		if err != nil {
			return nil, windowRefusal(err)
		}
		request.Window = &window
	}
	currency, configWarnings, err := s.resolveCurrency(in.Currency, netWorthTwin)
	if err != nil {
		return nil, err
	}
	request.Currency = currency
	srv, err := s.newReport(ctx, commandName)
	if err != nil {
		return nil, err
	}
	listing, err := srv.NetWorth(ctx, request)
	if err != nil {
		return nil, accountRefusal(err)
	}
	doc := document.NewNetWorth(listing, slices.Concat(configWarnings, document.NetWorthWarnings(listing, document.NativeParameter)))
	doc.Dates, doc.Warnings = capList(doc.Dates, doc.Warnings, toolNetWorth, "month ends",
		"pass a later since, or query v_net_worth for the rest")
	return doc, nil
}
