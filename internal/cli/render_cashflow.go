package cli

import (
	"math"
	"strconv"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/report"
)

// cashFlowNoRate is the Savings rate cell of a row with no income to divide by.
const cashFlowNoRate = "n/a"

// renderCashFlow renders c as the cashflow table: a window caption, a header, one row per period and
// currency, then a Total row per currency. The trailing Status column is unpadded.
func renderCashFlow(c report.CashFlow) string {
	rows := make([][]string, 0, 1+len(c.Rows)+len(c.Totals))
	rows = append(rows, []string{cashFlowPeriods[c.By].header, "Currency", "Income", "Spent", "Net", "Savings rate", "Status"})
	for _, r := range c.Rows {
		status := ""
		if r.Partial {
			status = tablePartialStatus
		}
		rows = append(rows, cashFlowCells(r.Period, r.Currency, r.Income, r.Spent, r.Net, r.SavingsRatePct, status))
	}
	for _, t := range c.Totals {
		rows = append(rows, cashFlowCells(tableTotalLabel, t.Currency, t.Income, t.Spent, t.Net, t.SavingsRatePct, ""))
	}
	return renderTable(windowCaption("Cash flow", c.Window, c.Accounts), rows)
}

// cashFlowCells is one table row: its amounts in cents formatted as money, rate as a percentage.
func cashFlowCells(label, currency string, income, spent, net int64, rate *float64, status string) []string {
	return []string{label, currency, formatMoney(income), formatMoney(spent), formatMoney(net), formatRate(rate), status}
}

// formatRate is a savings rate as one decimal and a percent sign with the integer part grouped
// in thousands ("-9,990.0%"), or "n/a" for a nil rate.
func formatRate(rate *float64) string {
	if rate == nil {
		return cashFlowNoRate
	}
	sign := ""
	tenths := int(math.Round(*rate * 10))
	if tenths < 0 {
		sign = "-"
		tenths = -tenths
	}
	return sign + humanize.Thousands(tenths/10) + "." + strconv.Itoa(tenths%10) + "%"
}
