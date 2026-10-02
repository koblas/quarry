package cli

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// cashFlowDocument is cashflow's --json stdout shape.
type cashFlowDocument struct {
	Since         string                   `json:"since"`
	Until         string                   `json:"until"`
	By            string                   `json:"by"`
	Currency      string                   `json:"currency"`
	AccountFilter []accountFilterDocument  `json:"account_filter"`
	Periods       []cashFlowPeriodDocument `json:"periods"`
	Totals        []cashFlowTotalDocument  `json:"totals"`
	Warnings      []string                 `json:"warnings"`
}

// cashFlowPeriodDocument is one entry of "periods"; SavingsRatePct is null when the period has no income.
type cashFlowPeriodDocument struct {
	Period         string   `json:"period"`
	Currency       string   `json:"currency"`
	Income         string   `json:"income"`
	Spent          string   `json:"spent"`
	Net            string   `json:"net"`
	SavingsRatePct *float64 `json:"savings_rate_pct"`
	Partial        bool     `json:"partial"`
}

// cashFlowTotalDocument is one entry of "totals"; SavingsRatePct is null when the currency has no income.
type cashFlowTotalDocument struct {
	Currency       string   `json:"currency"`
	Income         string   `json:"income"`
	Spent          string   `json:"spent"`
	Net            string   `json:"net"`
	SavingsRatePct *float64 `json:"savings_rate_pct"`
}

// renderCashFlowJSON renders c as cashflow's --json document with warnings;
// periods, totals and account_filter are [] rather than null when c holds none.
func renderCashFlowJSON(c report.CashFlow, warnings []string) ([]byte, error) {
	periods := make([]cashFlowPeriodDocument, len(c.Rows))
	for i, r := range c.Rows {
		periods[i] = cashFlowPeriodDocument{
			Period: r.Period, Currency: r.Currency,
			Income: document.Money(r.Income), Spent: document.Money(r.Spent), Net: document.Money(r.Net),
			SavingsRatePct: r.SavingsRatePct, Partial: r.Partial,
		}
	}
	totals := make([]cashFlowTotalDocument, len(c.Totals))
	for i, t := range c.Totals {
		totals[i] = cashFlowTotalDocument{
			Currency: t.Currency,
			Income:   document.Money(t.Income), Spent: document.Money(t.Spent), Net: document.Money(t.Net),
			SavingsRatePct: t.SavingsRatePct,
		}
	}
	return marshalDocument(cashFlowDocument{
		Since:         c.Window.Since.Format(document.DateLayout),
		Until:         c.Window.Until.Format(document.DateLayout),
		By:            cashFlowPeriods[c.By].name,
		Currency:      c.Currency.String(),
		AccountFilter: accountFilterDocuments(c.Accounts),
		Periods:       periods,
		Totals:        totals,
		Warnings:      warnings,
	})
}
