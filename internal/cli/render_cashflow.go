package cli

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/report"
)

// cashFlowColumns is the number of columns before the trailing Status column.
const cashFlowColumns = 6

// cashFlowNoRate is the Savings rate cell of a row with no income to divide by.
const cashFlowNoRate = "n/a"

// renderCashFlow renders c as the cashflow table: a window caption, a header, one row per period and
// currency, then a Total row per currency. The trailing Status column is unpadded.
func renderCashFlow(c report.CashFlow) string {
	const dateLayout = "2006-01-02"
	rows := make([][]string, 0, 1+len(c.Rows)+len(c.Totals))
	rows = append(rows, []string{cashFlowPeriods[c.By].header, "Currency", "Income", "Spent", "Net", "Savings rate", "Status"})
	for _, r := range c.Rows {
		status := ""
		if r.Partial {
			status = spendingPartialStatus
		}
		rows = append(rows, cashFlowCells(r.Period, r.Currency, r.Income, r.Spent, r.Net, r.SavingsRatePct, status))
	}
	for _, t := range c.Totals {
		rows = append(rows, cashFlowCells(spendingTotalLabel, t.Currency, t.Income, t.Spent, t.Net, t.SavingsRatePct, ""))
	}

	widths := make([]int, cashFlowColumns)
	for _, row := range rows {
		for i := range widths {
			widths[i] = max(widths[i], utf8.RuneCountInString(row[i]))
		}
	}

	var b strings.Builder
	b.WriteString("Cash flow " + c.Window.Since.Format(dateLayout) + " to " + c.Window.Until.Format(dateLayout) +
		" in " + spendingAccountsCaption(c.Accounts) + "\n\n")
	for _, row := range rows {
		b.WriteString(padRight(row[0], widths[0]) + accountsColumnGap + padRight(row[1], widths[1]))
		for i := 2; i < cashFlowColumns; i++ {
			b.WriteString(accountsColumnGap + padLeft(row[i], widths[i]))
		}
		if status := row[cashFlowColumns]; status != "" {
			b.WriteString(accountsColumnGap + status)
		}
		b.WriteString("\n")
	}
	return b.String()
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
