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
		rows = append(rows, []string{a.Name, a.Type, a.Currency, accountBalance(a.Balance), accountStatus(a.Closed, a.Active, a.NotInReports, a.LinkedTracking)})
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

// accountStatus joins, with ", ", the state (closed or inactive), "not in reports" and
// "linked tracking" that apply; "" when none does.
func accountStatus(closed, active, notInReports, linkedTracking bool) string {
	var parts []string
	switch {
	case closed:
		parts = append(parts, "closed")
	case !active:
		parts = append(parts, "inactive")
	}
	if notInReports {
		parts = append(parts, "not in reports")
	}
	if linkedTracking {
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
