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
	// Today is read once per call, here: a second read could straddle midnight.
	window, err := report.ParseWindow(in.Since, in.Until, s.now())
	if err != nil {
		return nil, windowRefusal(err)
	}
	currency, configWarnings, err := s.resolveCurrency(in.Currency, cashFlowTwin)
	if err != nil {
		return nil, err
	}
	by, err := parseCashFlowPeriod(in.By)
	if err != nil {
		// unreachable: see parseCashFlowPeriod
		return nil, err
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

// parseCashFlowPeriod is the period whose String is name; a name no period has is an error, never a default.
func parseCashFlowPeriod(name string) (store.CashFlowPeriod, error) {
	for _, period := range store.CashFlowPeriods() {
		if period.String() == name {
			return period, nil
		}
	}
	// unreachable: the schema's enum admits only the String of a CashFlowPeriod, and its default supplies month
	return 0, fmt.Errorf("%w: %q", errUnknownBy, name)
}
