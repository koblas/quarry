package cli

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// searchAligns is the alignment of the search table's columns: Amount right, the rest left.
var searchAligns = []tableAlign{alignLeft, alignLeft, alignLeft, alignLeft, alignLeft, alignRight, alignLeft}

// searchTransferLabel and searchUncategorizedLabel are the Category cell of a transfer leg and of a split with no category.
const (
	searchTransferLabel      = "(transfer)"
	searchUncategorizedLabel = "(uncategorized)"
)

// renderSearch renders s as the transactions table, a blank line and the footer, which counts every match.
func renderSearch(s report.Search) string {
	rows := make([][]string, 0, 1+len(s.Rows))
	rows = append(rows, []string{"Date", "Account", "Payee", "Category", "Memo", "Amount", "Flags"})
	for _, r := range s.Rows {
		payee := ""
		if r.Payee != nil {
			payee = *r.Payee
		}
		rows = append(rows, []string{
			r.Date.Format(time.DateOnly),
			accountLabel(r.Account.Name, r.Account.Currency, r.Account.Closed, r.Account.Active),
			payeeLabel(payee),
			searchCategoryCell(r.Splits),
			searchMemoCell(r),
			formatMoney(r.Amount),
			searchFlagsCell(r),
		})
	}
	return renderTable(searchCaption(s), searchAligns, rows) +
		"\n" + humanize.Count(s.Matched, "matching transaction", "matching transactions") + "\n"
}

// searchCaption is "Transactions[ matching "<text>"] in <accounts>, <dates>".
func searchCaption(s report.Search) string {
	caption := "Transactions"
	if s.Text != nil {
		caption += fmt.Sprintf(" matching %q", *s.Text)
	}
	return caption + " in " + accountsCaption(s.Accounts) + ", " + searchDates(s.Window)
}

// searchDates is the caption's dates: "all dates", "from D", "through D" or "D to D".
func searchDates(w store.SearchWindow) string {
	switch {
	case w.Since != nil && w.Until != nil:
		return w.Since.Format(time.DateOnly) + " to " + w.Until.Format(time.DateOnly)
	case w.Since != nil:
		return "from " + w.Since.Format(time.DateOnly)
	case w.Until != nil:
		return "through " + w.Until.Format(time.DateOnly)
	}
	return "all dates"
}

// searchCategoryCell is each split's label, distinct and in split order, joined by ", "; no splits is uncategorized.
func searchCategoryCell(splits []store.SearchSplit) string {
	if len(splits) == 0 {
		return searchUncategorizedLabel
	}
	var labels []string
	for _, sp := range splits {
		label := searchUncategorizedLabel
		switch {
		case sp.Transfer:
			label = searchTransferLabel
		case sp.Category != nil:
			label = escapeCell(*sp.Category)
		}
		if !slices.Contains(labels, label) {
			labels = append(labels, label)
		}
	}
	return strings.Join(labels, ", ")
}

// searchMemoCell is the transaction memo, then each distinct split memo that differs from it, joined by " / ".
func searchMemoCell(r store.SearchRow) string {
	var memos []string
	add := func(m *string) {
		if m != nil && *m != "" && !slices.Contains(memos, *m) {
			memos = append(memos, *m)
		}
	}
	add(r.Memo)
	for _, sp := range r.Splits {
		add(sp.Memo)
	}
	for i, m := range memos {
		memos[i] = escapeCell(m)
	}
	return strings.Join(memos, " / ")
}

// searchFlagsCell is "transfer", "excluded", both joined by ", ", or empty.
func searchFlagsCell(r store.SearchRow) string {
	var flags []string
	if r.Transfer {
		flags = append(flags, "transfer")
	}
	if r.Excluded {
		flags = append(flags, "excluded")
	}
	return strings.Join(flags, ", ")
}
