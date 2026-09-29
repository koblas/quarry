package cli

import (
	"strings"
	"unicode/utf8"

	"github.com/koblas/quarry/internal/report"
)

// spendingTotalLabel is the first cell of a currency's total row.
const spendingTotalLabel = "Total"

// renderSpending renders s as the spend table: a window caption, then a header,
// one row per group and a Total row per currency.
func renderSpending(s report.Spending) string {
	const dateLayout = "2006-01-02"
	grouping := spendGroupings[s.By]
	header := []string{grouping.header, "Currency", "Spent"}
	rows := make([][]string, 0, 1+len(s.Rows)+len(s.Totals))
	rows = append(rows, header)
	for _, r := range s.Rows {
		key := grouping.missing
		if r.Key != nil {
			key = *r.Key
		}
		rows = append(rows, []string{key, r.Currency, formatMoney(r.Spent)})
	}
	for _, t := range s.Totals {
		rows = append(rows, []string{spendingTotalLabel, t.Currency, formatMoney(t.Spent)})
	}

	widths := make([]int, len(header))
	for _, row := range rows {
		for i := range widths {
			widths[i] = max(widths[i], utf8.RuneCountInString(row[i]))
		}
	}

	var b strings.Builder
	b.WriteString("Spending " + s.Window.Since.Format(dateLayout) + " to " + s.Window.Until.Format(dateLayout) + " in all accounts\n\n")
	for _, row := range rows {
		b.WriteString(padRight(row[0], widths[0]) + accountsColumnGap +
			padRight(row[1], widths[1]) + accountsColumnGap +
			padLeft(row[2], widths[2]) + "\n")
	}
	return b.String()
}
