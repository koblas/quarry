package mcp

import (
	"context"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// recurringTwin is the quarry command whose output the recurring_charges tool matches.
const recurringTwin = "recurring"

// recurringCharges lists the recurring charge series of the call's window, accounts and currency, as recurring --json does.
func (s *Server) recurringCharges(ctx context.Context, in recurringInput) (any, error) {
	now := s.now()
	window, err := report.ParseChargeWindow(toolRecurring, in.Since, in.Until, now)
	if err != nil {
		return nil, windowRefusal(err)
	}
	currency, configWarnings, err := s.resolveCurrency(in.Currency, recurringTwin)
	if err != nil {
		return nil, err
	}
	srv, err := s.newReport(ctx, commandName)
	if err != nil {
		return nil, err
	}
	rec, err := srv.Recurring(ctx, report.RecurringRequest{Window: window, Now: now, Accounts: in.Accounts, Currency: currency})
	if err != nil {
		return nil, accountRefusal(err)
	}
	doc := document.NewRecurring(rec, append(configWarnings, document.RecurringWarnings(rec, toolRecurring)...))
	doc.Series, doc.Warnings = capList(doc.Series, doc.Warnings, toolRecurring, "series",
		"totals count every series; pass a shorter period or fewer accounts")
	return doc, nil
}
