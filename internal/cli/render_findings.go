package cli

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// findingsConfigShown is the config file as findings names it in its copy.
const findingsConfigShown = "~/Library/Application Support/quarry/config.toml"

// findingsHint tells the reader how to ignore a finding.
const findingsHint = "Ignore a finding by adding its id to findings.ignore in " + findingsConfigShown + "; see quarry findings --help"

// findingsNotShown ends the footer clause that counts the findings the default view leaves out.
const findingsNotShown = " not shown (--status all)"

// renderFindings renders the open findings group by group, each group with its header and rows and
// a blank line after, then the footer, and the ignore hint when showHint and a finding is open.
func renderFindings(listing report.FindingsListing, showHint bool) string {
	var b strings.Builder
	for _, group := range listing.Groups {
		b.WriteString(findingsHeader(group) + "\n")
		for _, line := range findingLines(group) {
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(findingsFooter(listing.Counts) + "\n")
	if showHint && listing.Counts.Open > 0 {
		b.WriteString(findingsHint + "\n")
	}
	return b.String()
}

// findingsHeader is a group's header: its heading, how many it lists and the fix in one clause.
func findingsHeader(group report.FindingsGroup) string {
	fix := group.Type.Fix()
	return fmt.Sprintf("%s (%s): %s", fix.Heading, findingsGroupCount(group), fix.GroupClause)
}

// findingsGroupCount is the count in a group header: the findings listed, and for uncategorized
// the payees and splits they cover.
func findingsGroupCount(group report.FindingsGroup) string {
	if group.Type != finding.Uncategorized {
		return humanize.Thousands(len(group.Findings))
	}
	splits := 0
	for _, f := range group.Findings {
		splits += len(f.Items)
	}
	return humanize.Count(len(group.Findings), "payee", "payees") + ", " + humanize.Count(splits, "split", "splits")
}

// findingLines is the rows of a group's findings, in order; a type without a row layout gets one id line per finding.
func findingLines(group report.FindingsGroup) []string {
	switch group.Type { //nolint:exhaustive // the other types' rows arrive with their detectors
	case finding.Duplicate:
		var lines []string
		for _, f := range group.Findings {
			lines = append(lines, "  "+f.ID)
			for _, row := range itemRows(f.Items) {
				lines = append(lines, "    "+row)
			}
		}
		return lines
	case finding.OneSidedTransfer:
		return oneSidedFindingRows(group.Findings)
	case finding.Uncategorized:
		return uncategorizedRows(group.Findings)
	}
	lines := make([]string, len(group.Findings))
	for i, f := range group.Findings {
		lines[i] = "  " + f.ID
	}
	return lines
}

// itemRows renders one row per item: date, account label and payee padded to the widest among them,
// then the amount right-aligned.
func itemRows(items []store.FindingItem) []string {
	labels := make([]string, len(items))
	payees := make([]string, len(items))
	amounts := make([]string, len(items))
	for i, item := range items {
		labels[i] = accountLabel(item.Account, item.Currency, item.Closed, item.Active)
		payees[i] = payeeLabel(item.Payee)
		amounts[i] = formatMoney(item.Amount)
	}
	labelWidth, payeeWidth, amountWidth := widestRunes(labels), widestRunes(payees), widestRunes(amounts)

	rows := make([]string, len(items))
	for i, item := range items {
		rows[i] = item.Date.Format(time.DateOnly) + "  " + padRight(labels[i], labelWidth) + "  " +
			padRight(payees[i], payeeWidth) + "  " + padLeft(amounts[i], amountWidth)
	}
	return rows
}

// oneSidedFindingRows renders one row per one-sided finding item: the leg as a "?" row shows it,
// behind the finding's id padded to the widest among them.
func oneSidedFindingRows(findings []store.Finding) []string {
	var legs []store.OneSidedTransfer
	var ids []string
	for _, f := range findings {
		for _, item := range f.Items {
			legs = append(legs, legOf(item))
			ids = append(ids, f.ID)
		}
	}
	idWidth := widestRunes(ids)

	leads := make([]string, len(ids))
	for i, id := range ids {
		leads[i] = "  " + padRight(id, idWidth) + "  "
	}
	return legRows(legs, leads)
}

// legOf is the one-sided leg an item describes.
func legOf(item store.FindingItem) store.OneSidedTransfer {
	return store.OneSidedTransfer{
		Date: item.Date, Account: item.Account, Currency: item.Currency, Closed: item.Closed, Active: item.Active,
		Payee: item.Payee, Amount: item.Amount, OtherAccount: item.OtherAccount, OtherAccountID: item.OtherAccountID,
	}
}

// uncategorizedRows renders one row per uncategorized finding: its id, payee, how many splits and
// the dates they run from and to, each column padded to the widest among them.
func uncategorizedRows(findings []store.Finding) []string {
	ids := make([]string, len(findings))
	payees := make([]string, len(findings))
	counts := make([]string, len(findings))
	spans := make([]string, len(findings))
	for i, f := range findings {
		payee, first, last := uncategorizedSpan(f.Items)
		ids[i] = f.ID
		payees[i] = payeeLabel(payee)
		counts[i] = humanize.Count(len(f.Items), "split", "splits")
		spans[i] = first.Format(time.DateOnly)
		if !last.Equal(first) {
			spans[i] += " to " + last.Format(time.DateOnly)
		}
	}
	idWidth, payeeWidth, countWidth := widestRunes(ids), widestRunes(payees), widestRunes(counts)

	rows := make([]string, len(findings))
	for i := range findings {
		rows[i] = "  " + padRight(ids[i], idWidth) + "  " + padRight(payees[i], payeeWidth) + "  " +
			padLeft(counts[i], countWidth) + "  " + spans[i]
	}
	return rows
}

// uncategorizedSpan is the payee the items share and the earliest and latest of their dates.
func uncategorizedSpan(items []store.FindingItem) (string, time.Time, time.Time) {
	var payee string
	var first, last time.Time
	for i, item := range items {
		if i == 0 || item.Date.Before(first) {
			first = item.Date
		}
		if i == 0 || item.Date.After(last) {
			last = item.Date
		}
		payee = item.Payee
	}
	return payee, first, last
}

// findingsFooter is the default view's last line: how many findings are open, then how many ignored
// and fixed ones it leaves out.
func findingsFooter(counts finding.Counts) string {
	line := "No open findings"
	if counts.Open > 0 {
		line = humanize.Count(counts.Open, "open finding", "open findings")
	}
	var hidden []string
	if counts.Ignored > 0 {
		hidden = append(hidden, humanize.Thousands(counts.Ignored)+" ignored")
	}
	if counts.Fixed > 0 {
		hidden = append(hidden, humanize.Thousands(counts.Fixed)+" fixed")
	}
	if len(hidden) == 0 {
		return line
	}
	return line + "; " + strings.Join(hidden, " and ") + findingsNotShown
}

// widestRunes is the width in runes of the widest of ss, 0 for an empty slice.
func widestRunes(ss []string) int {
	n := 0
	for _, s := range ss {
		n = max(n, utf8.RuneCountInString(s))
	}
	return n
}
