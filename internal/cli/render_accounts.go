package cli

import (
	"strings"
	"unicode/utf8"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// accountsColumnGap separates the accounts table's columns.
const accountsColumnGap = "  "

// noRateCell is the reporting-currency cell of a balance no exchange rate converts.
const noRateCell = "no rate"

// renderAccounts renders l as the accounts table: a header row, then one row per account in l's order,
// Balance and the reporting-currency column (absent in a native listing) right-aligned, trailing spaces trimmed.
func renderAccounts(l report.AccountListing) string {
	converted := l.Currency != money.Native
	header := []string{"Account", "Type", "Currency", "Balance"}
	if converted {
		header = append(header, "In "+l.Currency.String())
	}
	header = append(header, "Status")
	rows := make([][]string, 0, 1+len(l.Accounts))
	rows = append(rows, header)
	for _, a := range l.Accounts {
		row := []string{escapeCell(a.Name), a.Type, a.Currency, formatBigMoney(a.Balance)}
		if converted {
			row = append(row, convertedCell(l, a))
		}
		rows = append(rows, append(row, accountStatus(a.Account)))
	}

	statusAt := len(header) - 1
	widths := make([]int, statusAt)
	for _, row := range rows {
		for i := range widths {
			widths[i] = max(widths[i], utf8.RuneCountInString(row[i]))
		}
	}

	const firstRightAligned = 3
	var b strings.Builder
	for _, row := range rows {
		cells := make([]string, 0, len(row))
		for i, cell := range row[:statusAt] {
			if i < firstRightAligned {
				cell = padRight(cell, widths[i])
			} else {
				cell = padLeft(cell, widths[i])
			}
			cells = append(cells, cell)
		}
		if status := row[statusAt]; status != "" {
			cells = append(cells, status)
		}
		b.WriteString(strings.TrimRight(strings.Join(cells, accountsColumnGap), " ") + "\n")
	}
	return b.String()
}

// convertedCell is a's balance in l's currency, "no rate" when a rate should have converted it, else blank.
func convertedCell(l report.AccountListing, a store.AccountBalance) string {
	if cents := l.ConvertedBalance(a); cents != nil {
		return formatBigMoney(cents)
	}
	if l.NeedsRate(a) {
		return noRateCell
	}
	return ""
}

// accountStatus joins, with ", ", the state (closed or inactive), "not in reports" and
// "linked tracking" that apply; "" when none does.
func accountStatus(a store.Account) string {
	var parts []string
	switch {
	case a.Closed:
		parts = append(parts, "closed")
	case !a.Active:
		parts = append(parts, "inactive")
	}
	if a.NotInReports {
		parts = append(parts, "not in reports")
	}
	if a.LinkedTracking {
		parts = append(parts, "linked tracking")
	}
	return strings.Join(parts, ", ")
}

// padRight pads s with spaces to width runes.
func padRight(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-utf8.RuneCountInString(s)))
}

// padLeft pads s with leading spaces to width runes.
func padLeft(s string, width int) string {
	return strings.Repeat(" ", max(0, width-utf8.RuneCountInString(s))) + s
}

// allClosedNote says how to list the accounts when hidden closed accounts
// are all the store has.
func allClosedNote(hidden int) string {
	if hidden == 1 {
		return "the only account is closed; pass --all to list it"
	}
	return "all " + accountsPhrase(hidden) + " are closed; pass --all to list them"
}
