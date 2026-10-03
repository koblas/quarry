package mcp

import (
	"context"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// anomalies lists the unusually large charges of the call's window, accounts and currency, as anomalies --json does.
func (s *Server) anomalies(ctx context.Context, in anomaliesInput) (any, error) {
	// Today is read once per call, here: the window and the report must agree on it across midnight.
	now := s.now()
	window, err := report.ParseChargeWindow(toolAnomalies, in.Since, in.Until, now)
	if err != nil {
		return nil, windowRefusal(err)
	}
	// The tool's name is also the command word, so one constant serves the config refusal's twin.
	currency, configWarnings, err := s.resolveCurrency(in.Currency, toolAnomalies)
	if err != nil {
		return nil, err
	}
	srv, err := s.newReport(ctx, commandName)
	if err != nil {
		return nil, err
	}
	result, err := srv.Anomalies(ctx, report.AnomaliesRequest{Window: window, Now: now, Accounts: in.Accounts, Currency: currency})
	if err != nil {
		return nil, accountRefusal(err)
	}
	doc := document.NewAnomalies(result, append(configWarnings, document.AnomaliesWarnings(result, toolAnomalies)...))
	doc.Anomalies, doc.Warnings = capList(doc.Anomalies, doc.Warnings, toolAnomalies, "charges",
		"pass a shorter period or fewer accounts")
	return doc, nil
}
