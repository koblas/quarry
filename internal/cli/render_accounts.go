package cli

import (
	"strings"
	"unicode/utf8"

	"github.com/koblas/quarry/internal/store"
)

// notImportedBalance is the Balance cell of an account quarry cannot sum.
const notImportedBalance = "not imported"

// accountsColumnGap separates the accounts table's columns.
const accountsColumnGap = "  "

// renderAccounts renders list as the accounts table: a header row, then one row
// per account in list's order, Balance right-aligned.
func renderAccounts(list store.AccountList) string {
	header := []string{"Account", "Type", "Currency", "Balance", "Status"}
	rows := make([][]string, 0, 1+len(list.Accounts))
	rows = append(rows, header)
	for _, a := range list.Accounts {
		rows = append(rows, []string{a.Name, a.Type, a.Currency, accountBalance(a.Balance), accountStatus(a.Closed, a.Active)})
	}

	widths := make([]int, len(header)-1)
	for _, row := range rows {
		for i := range widths {
			widths[i] = max(widths[i], utf8.RuneCountInString(row[i]))
		}
	}

	var b strings.Builder
	for _, row := range rows {
		b.WriteString(padRight(row[0], widths[0]) + accountsColumnGap +
			padRight(row[1], widths[1]) + accountsColumnGap +
			padRight(row[2], widths[2]) + accountsColumnGap +
			padLeft(row[3], widths[3]))
		if status := row[4]; status != "" {
			b.WriteString(accountsColumnGap + status)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// accountBalance renders a balance in cents, or "not imported" when nil.
func accountBalance(cents *int64) string {
	if cents == nil {
		return notImportedBalance
	}
	return formatMoney(*cents)
}

// accountStatus is "closed" for a closed account, "inactive" for an open one
// that is not active, and "" otherwise.
func accountStatus(closed, active bool) string {
	switch {
	case closed:
		return "closed"
	case !active:
		return "inactive"
	default:
		return ""
	}
}

// padRight pads s with spaces to width runes.
func padRight(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-utf8.RuneCountInString(s)))
}

// padLeft pads s with leading spaces to width runes.
func padLeft(s string, width int) string {
	return strings.Repeat(" ", max(0, width-utf8.RuneCountInString(s))) + s
}
