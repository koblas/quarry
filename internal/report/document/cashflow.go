package document

import "github.com/koblas/quarry/internal/report"

// CashFlow is cashflow's --json document and the cash-flow tool's structured result.
type CashFlow struct {
	Since         string              `json:"since"`
	Until         string              `json:"until"`
	By            string              `json:"by"`
	Currency      string              `json:"currency"`
	AccountFilter []AccountFilter     `json:"account_filter"`
	Periods       []CashFlowPeriodRow `json:"periods"`
	Totals        []CashFlowTotal     `json:"totals"`
	Warnings      []string            `json:"warnings"`
}

// CashFlowPeriodRow is one entry of "periods"; SavingsRatePct is null when the period has no income.
type CashFlowPeriodRow struct {
	Period         string   `json:"period"`
	Currency       string   `json:"currency"`
	Income         string   `json:"income"`
	Spent          string   `json:"spent"`
	Net            string   `json:"net"`
	SavingsRatePct *float64 `json:"savings_rate_pct"`
	Partial        bool     `json:"partial"`
}

// CashFlowTotal is one entry of "totals"; SavingsRatePct is null when the currency has no income.
type CashFlowTotal struct {
	Currency       string   `json:"currency"`
	Income         string   `json:"income"`
	Spent          string   `json:"spent"`
	Net            string   `json:"net"`
	SavingsRatePct *float64 `json:"savings_rate_pct"`
}

// NewCashFlow converts c into cashflow's document with warnings;
// periods, totals and account_filter are [] rather than null when c holds none.
func NewCashFlow(c report.CashFlow, warnings []string) CashFlow {
	periods := make([]CashFlowPeriodRow, len(c.Rows))
	for i, r := range c.Rows {
		periods[i] = CashFlowPeriodRow{
			Period: r.Period, Currency: r.Currency,
			Income: Money(r.Income), Spent: Money(r.Spent), Net: Money(r.Net),
			SavingsRatePct: r.SavingsRatePct, Partial: r.Partial,
		}
	}
	totals := make([]CashFlowTotal, len(c.Totals))
	for i, t := range c.Totals {
		totals[i] = CashFlowTotal{
			Currency: t.Currency,
			Income:   Money(t.Income), Spent: Money(t.Spent), Net: Money(t.Net),
			SavingsRatePct: t.SavingsRatePct,
		}
	}
	return CashFlow{
		Since:         c.Window.Since.Format(DateLayout),
		Until:         c.Window.Until.Format(DateLayout),
		By:            c.By.String(),
		Currency:      c.Currency.String(),
		AccountFilter: NewAccountFilters(c.Accounts),
		Periods:       periods,
		Totals:        totals,
		Warnings:      append([]string{}, warnings...),
	}
}
