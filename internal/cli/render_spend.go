package cli

import (
	"strings"
	"unicode/utf8"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// spendingTotalLabel is the first cell of a currency's total row.
const spendingTotalLabel = "Total"

// spendingPartialStatus is the Status cell of a month the window cuts short.
const spendingPartialStatus = "partial"

// renderSpending renders s as the spend table: a window caption, then a header,
// one row per group and a Total row per currency. A month grouping adds a
// trailing Status column, empty (and unpadded) except on a partial month.
func renderSpending(s report.Spending) string {
	const dateLayout = "2006-01-02"
	grouping := spendGroupings[s.By]
	withStatus := s.By == store.SpendByMonth
	header := []string{grouping.header, "Currency", "Spent", ""}
	if withStatus {
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
			status = spendingPartialStatus
		}
		rows = append(rows, []string{key, r.Currency, formatMoney(r.Spent), status})
	}
	for _, t := range s.Totals {
		rows = append(rows, []string{spendingTotalLabel, t.Currency, formatMoney(t.Spent), ""})
	}

	widths := make([]int, 3)
	for _, row := range rows {
		for i := range widths {
			widths[i] = max(widths[i], utf8.RuneCountInString(row[i]))
		}
	}

	var b strings.Builder
	b.WriteString("Spending " + s.Window.Since.Format(dateLayout) + " to " + s.Window.Until.Format(dateLayout) +
		" in " + spendingAccountsCaption(s.Accounts) + "\n\n")
	for _, row := range rows {
		b.WriteString(padRight(row[0], widths[0]) + accountsColumnGap +
			padRight(row[1], widths[1]) + accountsColumnGap +
			padLeft(row[2], widths[2]))
		if row[3] != "" {
			b.WriteString(accountsColumnGap + row[3])
		}
		b.WriteString("\n")
	}
	return b.String()
}

// spendingAccountsCaption is the names of accounts joined by ", ", or "all accounts" when none.
func spendingAccountsCaption(accounts []store.Account) string {
	if len(accounts) == 0 {
		return "all accounts"
	}
	names := make([]string, len(accounts))
	for i, a := range accounts {
		names[i] = a.Name
	}
	return strings.Join(names, ", ")
}
