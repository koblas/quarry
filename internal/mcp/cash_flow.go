package mcp

import (
	"context"
	"fmt"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
)

// cashFlowTwin is the quarry command whose output the cash_flow tool matches.
const cashFlowTwin = "cashflow"

// cashFlow reports income, spending and net per period of the call's window, accounts and currency, as cashflow --json does.
func (s *Server) cashFlow(ctx context.Context, in cashFlowInput) (any, error) {
	window, err := report.ParseWindow(in.Since, in.Until, s.now())
	if err != nil {
		return nil, windowRefusal(err)
	}
	currency, configWarnings, err := s.resolveCurrency(in.Currency, cashFlowTwin)
	if err != nil {
		return nil, err
	}
	by, ok := store.ParseCashFlowPeriod(in.By)
	if !ok {
		// unreachable: the schema's enum admits only the String of a CashFlowPeriod, and its default supplies month
		return nil, fmt.Errorf("%w: %q", errUnknownBy, in.By)
	}
	srv, err := s.newReport(ctx, commandName)
	if err != nil {
		return nil, err
	}
	flow, err := srv.CashFlow(ctx, report.CashFlowRequest{Window: window, By: by, Accounts: in.Accounts, Currency: currency})
	if err != nil {
		return nil, accountRefusal(err)
	}
	doc := document.NewCashFlow(flow, append(configWarnings, document.CashFlowWarnings(flow, toolCashFlow)...))
	doc.Periods, doc.Warnings = capList(doc.Periods, doc.Warnings, toolCashFlow, "periods",
		"totals count every period; pass a later since, or by year")
	return doc, nil
}
