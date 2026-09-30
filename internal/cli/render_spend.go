package cli

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// renderSpending renders s as the spend table: caption, header, one row per group and a Total
// row per currency; a month grouping adds a trailing, unpadded Status column.
func renderSpending(s report.Spending) string {
	grouping := spendGroupings[s.By]
	header := []string{grouping.header, "Currency", "Spent", ""}
	if s.By == store.SpendByMonth {
		header[3] = "Status"
	}
	rows := make([][]string, 0, 1+len(s.Rows)+len(s.Totals))
	rows = append(rows, header)
	for _, r := range s.Rows {
		key := grouping.missing
		if r.Key != nil {
			key = *r.Key
		}
		status := ""
		if r.Partial {
			status = tablePartialStatus
		}
		rows = append(rows, []string{key, r.Currency, formatMoney(r.Spent), status})
	}
	for _, t := range s.Totals {
		rows = append(rows, []string{tableTotalLabel, t.Currency, formatMoney(t.Spent), ""})
	}
	return renderTable(windowCaption("Spending", s.Window, s.Accounts), rows)
}
