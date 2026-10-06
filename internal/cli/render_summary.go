package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// summaryRecurringTitle is the first words of the recurring section's caption.
const summaryRecurringTitle = "Recurring charges new"

// changeLabel is the first cell of the net worth Change line.
const changeLabel = "Change"

// renderSummary renders s as the summary text: heading, the Snapshot, Dates and Findings rows, then the
// anomalies, recurring and net worth sections, one blank line between parts and one newline at the end.
func renderSummary(s report.Summary, findings document.FindingsTally, now time.Time) string {
	var b strings.Builder
	b.WriteString(summaryHeading(s) + "\n\n")
	fmt.Fprintf(&b, "%-10s%s\n", "Snapshot", snapshotLine(s.Status.Run.Snapshot, now))
	fmt.Fprintf(&b, "%-10s%s\n", "Dates", datesLine(s.Status.FirstDate, s.Status.LastDate))
	fmt.Fprintf(&b, "%-10s%s\n\n", "Findings", summaryFindingsPhrase(findings))
	b.WriteString(summaryAnomaliesSection(s.Anomalies) + "\n")
	b.WriteString(summaryRecurringSection(s.Recurring) + "\n")
	b.WriteString(renderNetWorthWithChange(s.NetWorth))
	return b.String()
}

// summaryAnomaliesSection is renderAnomalies, except that a month listing no charge says so in place of the
// table's header row; the footer still counts the charges checked.
func summaryAnomaliesSection(a report.Anomalies) string {
	if len(a.Listed) > 0 {
		return renderAnomalies(a)
	}
	caption := windowCaption(anomaliesTitle, a.Window, a.Accounts, a.Currency)
	return renderEmptySection(caption, "No unusually large charges.") + "\n" + anomaliesFooter(a.Checked, a.NotJudged) + "\n"
}

// summaryRecurringSection is renderRecurringTitled, except that a month with no new series says so in place of
// the table's header row.
func summaryRecurringSection(r report.Recurring) string {
	if !r.Empty() {
		return renderRecurringTitled(summaryRecurringTitle, r)
	}
	return renderEmptySection(windowCaption(summaryRecurringTitle, r.Window, r.Accounts, r.Currency), "No new recurring charges.")
}

// renderEmptySection is a caption, a blank line and the one line that stands where the section's table would.
func renderEmptySection(caption, line string) string {
	return renderTable(caption, []tableAlign{alignLeft}, [][]string{{line}})
}

// renderNetWorthWithChange is the month-end history table with the change between its first and last month
// end appended; nothing is appended when the first month end has no balance.
func renderNetWorthWithChange(n report.NetWorth) string {
	aligns, rows := netWorthHistoryRows(n)
	if change := n.Change(); change != nil {
		rows = append(rows, changeRows(n, change)...)
	}
	return renderTable(netWorthHistoryCaption(n), aligns, rows)
}

// changeRows is the Change line of a converted history, or one per currency of a native one, in the history
// table's columns; a native line leaves blank the type its currency does not hold.
func changeRows(n report.NetWorth, change *report.NetWorthChange) [][]string {
	types := n.Types()
	if n.Currency != money.Native {
		row := []string{changeLabel}
		for _, entry := range change.Types {
			row = append(row, signedMoney(entry.Value))
		}
		return [][]string{append(row, signedMoney(change.Totals[0].Value))}
	}
	rows := make([][]string, len(change.Totals))
	for i, total := range change.Totals {
		row := []string{changeLabel, total.Currency}
		for _, accountType := range types {
			row = append(row, nativeChangeCell(change, accountType, total.Currency))
		}
		rows[i] = append(row, signedMoney(total.Value))
	}
	return rows
}

// nativeChangeCell is accountType's change in currency, blank when neither month end holds that type in it.
func nativeChangeCell(change *report.NetWorthChange, accountType, currency string) string {
	for _, entry := range change.Types {
		if entry.Type == accountType && entry.Currency == currency {
			return signedMoney(entry.Value)
		}
	}
	return ""
}

// summaryHeading is the month's name and first and last day, then the currency of the amounts unless native.
func summaryHeading(s report.Summary) string {
	heading := "Summary of " + s.Month.Name() +
		" (" + s.Month.Start.Format(time.DateOnly) + " to " + s.Month.End.Format(time.DateOnly) + ")"
	if s.Currency != money.Native {
		heading += ", amounts in " + s.Currency.String()
	}
	return heading
}

// summaryFindingsPhrase is the Findings row text: the open count, the ignored one when findings.ignore was
// read, what the last sync found new and fixed, and, while any finding is open, where to list them.
func summaryFindingsPhrase(f document.FindingsTally) string {
	c := f.Counts
	phrase := "none open"
	if c.Open > 0 {
		phrase = humanize.Thousands(c.Open) + " open"
	}
	if f.IgnoreKnown && c.Ignored > 0 {
		phrase += ", " + humanize.Thousands(c.Ignored) + " ignored"
	}
	phrase += lastSyncClause(c.New, c.NewlyFixed)
	if c.Open > 0 {
		phrase += "; run quarry findings to list them"
	}
	return phrase
}

// lastSyncClause is "; the last sync found <a> new and <b> fixed", with only the nonzero count when one is
// zero and nothing when both are.
func lastSyncClause(added, fixed int) string {
	switch {
	case added > 0 && fixed > 0:
		return "; the last sync found " + humanize.Thousands(added) + " new and " + humanize.Thousands(fixed) + " fixed"
	case added > 0:
		return "; the last sync found " + humanize.Thousands(added) + " new"
	case fixed > 0:
		return "; the last sync found " + humanize.Thousands(fixed) + " fixed"
	default:
		return ""
	}
}
