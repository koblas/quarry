package cli

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/koblas/quarry/internal/store"
)

// tableTotalLabel is the first cell of a currency's total row.
const tableTotalLabel = "Total"

// tablePartialStatus is the Status cell of a period the window cuts short.
const tablePartialStatus = "partial"

// tableFixedColumns is how many leading columns of a table are left-aligned; the rest are right-aligned.
const tableFixedColumns = 2

// renderTable renders a report table: caption, a blank line, then rows (header first). Every cell
// but the last of a row is padded to its column; the first tableFixedColumns are left-aligned, the
// rest right-aligned. The last cell is the Status, unpadded and written only when non-empty.
func renderTable(caption string, rows [][]string) string {
	padded := len(rows[0]) - 1
	widths := make([]int, padded)
	for _, row := range rows {
		for i := range widths {
			widths[i] = max(widths[i], utf8.RuneCountInString(row[i]))
		}
	}

	var b strings.Builder
	b.WriteString(caption + "\n\n")
	for _, row := range rows {
		for i := range padded {
			if i > 0 {
				b.WriteString(accountsColumnGap)
			}
			if i < tableFixedColumns {
				b.WriteString(padRight(row[i], widths[i]))
			} else {
				b.WriteString(padLeft(row[i], widths[i]))
			}
		}
		if status := row[padded]; status != "" {
			b.WriteString(accountsColumnGap + status)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// windowCaption is title, the window's first and last day and the accounts it counts, as a table caption.
func windowCaption(title string, window store.Window, accounts []store.Account) string {
	return title + " " + window.Since.Format(time.DateOnly) + " to " + window.Until.Format(time.DateOnly) +
		" in " + accountsCaption(accounts)
}

// accountsCaption is the names of accounts joined by ", ", or "all accounts" when none.
func accountsCaption(accounts []store.Account) string {
	if len(accounts) == 0 {
		return "all accounts"
	}
	names := make([]string, len(accounts))
	for i, a := range accounts {
		names[i] = escapeCell(a.Name)
	}
	return strings.Join(names, ", ")
}

// cellEscaper writes the characters that would break a text line as backslash forms.
var cellEscaper = strings.NewReplacer("\n", `\n`, "\t", `\t`, "\r", `\r`)

// escapeCell is s as one line of a text table: the only escaper any text renderer uses for a name.
func escapeCell(s string) string { return cellEscaper.Replace(s) }
