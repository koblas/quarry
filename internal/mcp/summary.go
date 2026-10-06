package mcp

import (
	"context"
	"errors"
	"slices"
	"strconv"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// summaryTwin is the quarry command whose output the monthly_summary tool matches.
const summaryTwin = "summary"

// summaryAgain is the phrase the snapshot warning ends with: the call has no flags to echo, with or without a month.
const summaryAgain = "call " + toolMonthly + " again"

// monthlySummary is the monthly_summary tool: summary --json's document for one ended month, from one read of
// the store, with anomalies.charges and recurring.series cut to maxRows and a warning for each cut.
func (s *Server) monthlySummary(ctx context.Context, in monthlySummaryInput) (any, error) {
	now := s.now()
	month, err := report.ParseMonth(in.Month, now)
	if err != nil {
		return nil, monthRefusal(err)
	}
	choices, err := s.summaryChoices(in.Currency)
	if err != nil {
		return nil, err
	}
	srv, err := s.newReport(ctx, commandName)
	if err != nil {
		return nil, err
	}
	summary, err := srv.Summary(ctx, report.SummaryRequest{Month: month, Currency: choices.currency})
	if err != nil {
		return nil, err //nolint:wrapcheck // a RefusalError is the tool's answer, sent verbatim
	}
	findings := document.FindingsTally{
		Counts:      report.CountFindings(summary.Status, choices.ignore, choices.classification),
		IgnoreKnown: choices.ignoreKnown,
	}
	warnings := slices.Concat(choices.warnings, document.SummaryWarnings(summary, summaryAgain, document.NativeParameter))
	doc := document.NewSummary(summary, findings, warnings)
	doc.Anomalies.Charges, doc.Warnings = capList(doc.Anomalies.Charges, doc.Warnings, toolMonthly, "charges",
		"pass an earlier or later month")
	doc.Recurring.Series, doc.Warnings = capList(doc.Recurring.Series, doc.Warnings, toolMonthly, "series",
		"pass an earlier or later month")
	return doc, nil
}

// summaryChoice is what the config decides for a monthly_summary call. warnings are the config's own, or the
// one saying why ignoreKnown is false; both name the config by its absolute path.
type summaryChoice struct {
	currency       money.Currency
	ignore         []string
	classification report.Classification
	ignoreKnown    bool
	warnings       []string
}

// summaryChoices loads the config once. An unreadable config is refused with a stderr line naming summary, or
// with a currency in the call the findings are counted without the choices and a warning says so.
func (s *Server) summaryChoices(name string) (summaryChoice, error) {
	cfg, err := s.newConfig(commandName)
	if err != nil {
		if name == "" {
			return summaryChoice{}, withLog(err, configRefusalLog(summaryTwin))
		}
		return summaryChoice{
			currency: parseCurrency(name),
			warnings: []string{document.CannotTellChoices(config.ProblemAbsolute(err))},
		}, nil
	}
	currency := cfg.Currency
	if name != "" {
		currency = parseCurrency(name)
	}
	return summaryChoice{
		currency:       currency,
		ignore:         cfg.Ignore,
		classification: classificationOf(cfg),
		ignoreKnown:    true,
		warnings:       slices.Clone(cfg.WarningsAbsolute),
	}, nil
}

// parseCurrency is the currency name spells; the schema's enum admits only spellings money.ParseCurrency reads.
func parseCurrency(name string) money.Currency {
	currency, _ := money.ParseCurrency(name)
	return currency
}

// monthRefusedError is the isError text of a month the model must fix.
type monthRefusedError string

func (e monthRefusedError) Error() string { return string(e) }

// monthRefusal is err, a month refusal, worded for the model; its stderr line is the class line, which never
// carries the caller's value. Any other error comes back as is.
func monthRefusal(err error) error {
	refusal, ok := errors.AsType[report.MonthError](err)
	if !ok {
		// unreachable: report.ParseMonth (internal/report/month.go) returns only MonthError, at its not-a-month and not-ended returns
		return err
	}
	return withLog(monthRefusedError(monthWording(refusal)), monthRefusedLog)
}

// monthWording is the model's text for refusal; each kind's tail names the argument that fixes it.
func monthWording(refusal report.MonthError) string {
	switch refusal.Kind {
	case report.MonthNotEnded:
		return "month " + refusal.Value + " has not ended; " + toolMonthly + " covers whole months, so pass " + refusal.Example + " or earlier"
	case report.MonthNotAMonth:
	}
	return "month " + strconv.Quote(refusal.Value) + " is not a month; use YYYY-MM, such as " + refusal.Example
}
