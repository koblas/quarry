package cli

import "github.com/koblas/quarry/internal/report"

// cashFlowDocument is cashflow's --json stdout shape.
type cashFlowDocument struct {
	Since         string                   `json:"since"`
	Until         string                   `json:"until"`
	By            string                   `json:"by"`
	AccountFilter []spendAccountDocument   `json:"account_filter"`
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
			Income: jsonMoney(r.Income), Spent: jsonMoney(r.Spent), Net: jsonMoney(r.Net),
			SavingsRatePct: r.SavingsRatePct, Partial: r.Partial,
		}
	}
	totals := make([]cashFlowTotalDocument, len(c.Totals))
	for i, t := range c.Totals {
		totals[i] = cashFlowTotalDocument{
			Currency: t.Currency,
			Income:   jsonMoney(t.Income), Spent: jsonMoney(t.Spent), Net: jsonMoney(t.Net),
			SavingsRatePct: t.SavingsRatePct,
		}
	}
	accountFilter := make([]spendAccountDocument, len(c.Accounts))
	for i, a := range c.Accounts {
		accountFilter[i] = spendAccountDocument{ID: a.ID, Name: a.Name}
	}
	return marshalDocument(cashFlowDocument{
		Since:         c.Window.Since.Format(jsonDateLayout),
		Until:         c.Window.Until.Format(jsonDateLayout),
		By:            cashFlowPeriods[c.By].name,
		AccountFilter: accountFilter,
		Periods:       periods,
		Totals:        totals,
		Warnings:      warnings,
	})
}
