package cli

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// tableTotalLabel is the first cell of a currency's total row.
const tableTotalLabel = "Total"

// tablePartialStatus is the Status cell of a period the window cuts short.
const tablePartialStatus = "partial"

// tableAlign is which edge of its column a table cell sits against.
type tableAlign int

const (
	alignLeft tableAlign = iota
	alignRight
)

// renderTable renders a report table: caption, a blank line, then rows (header first). Each cell is
// padded to its column's widest cell, on the side aligns names; a row loses its trailing spaces.
func renderTable(caption string, aligns []tableAlign, rows [][]string) string {
	widths := make([]int, len(aligns))
	for _, row := range rows {
		for i := range widths {
			widths[i] = max(widths[i], utf8.RuneCountInString(row[i]))
		}
	}

	var b strings.Builder
	b.WriteString(caption + "\n\n")
	for _, row := range rows {
		cells := make([]string, len(aligns))
		for i, align := range aligns {
			if align == alignLeft {
				cells[i] = padRight(row[i], widths[i])
			} else {
				cells[i] = padLeft(row[i], widths[i])
			}
		}
		b.WriteString(strings.TrimRight(strings.Join(cells, accountsColumnGap), " ") + "\n")
	}
	return b.String()
}

// windowCaption is title, the window's first and last day and the accounts it counts, as a table caption;
// a report in CAD or USD ends with ", amounts in <currency>", and a native one adds nothing.
func windowCaption(title string, window store.Window, accounts []store.Account, currency money.Currency) string {
	caption := title + " " + window.Since.Format(time.DateOnly) + " to " + window.Until.Format(time.DateOnly) +
		" in " + accountsCaption(accounts)
	if currency == money.Native {
		return caption
	}
	return caption + ", amounts in " + currency.String()
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
