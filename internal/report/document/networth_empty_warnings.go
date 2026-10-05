package document

import (
	"fmt"
	"slices"

	"github.com/koblas/quarry/internal/report"
)

// emptyNetWorthWarnings is one line when no account has a balance on any day n lists, naming the first balance
// when it comes after them and otherwise that the store has none; none when a day has a row or nothing is listed.
func emptyNetWorthWarnings(n report.NetWorth) []string {
	if len(n.Dates) == 0 || slices.ContainsFunc(n.Dates, func(date report.NetWorthDate) bool { return len(date.Rows) > 0 }) {
		return nil
	}
	first, last := n.Dates[0].Date, n.Dates[len(n.Dates)-1].Date
	reason := "no account in Quicken's reports has transactions or holdings"
	if !n.FirstBalance.IsZero() {
		if !n.FirstBalance.After(last) {
			return nil
		}
		reason = "the first balance is on " + n.FirstBalance.Format(DateLayout)
	}
	if n.Window != nil {
		return []string{fmt.Sprintf("no account has a balance at any month end from %s to %s; %s",
			first.Format(DateLayout), last.Format(DateLayout), reason)}
	}
	return []string{fmt.Sprintf("no account has a balance on %s; %s", first.Format(DateLayout), reason)}
}
