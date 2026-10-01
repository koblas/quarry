package cli

import (
	"cmp"
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

// findingsView is which findings the command lists: one status or report.FindingsAll, and one
// finding type or every type when typ is empty.
type findingsView struct {
	status finding.Status
	typ    finding.Type
}

// renderFindings renders the listed findings group by group, each group with its header and rows and
// a blank line after, then the footer, and the ignore hint when showHint and an open finding is listed.
func renderFindings(listing report.FindingsListing, view findingsView, showHint bool) string {
	var b strings.Builder
	for _, group := range listing.Groups {
		b.WriteString(findingsHeader(group) + "\n")
		for _, line := range findingLines(group, view) {
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(findingsFooter(listing.Counts, view) + "\n")
	if showHint && hasOpen(listing.Groups...) {
		b.WriteString(findingsHint + "\n")
	}
	return b.String()
}

// hasOpen reports whether any finding listed in groups is open.
func hasOpen(groups ...report.FindingsGroup) bool {
	for _, group := range groups {
		for _, f := range group.Findings {
			if f.Status == finding.StatusOpen {
				return true
			}
		}
	}
	return false
}

// findingsHeader is a group's header: its heading and how many it lists, then the fix in one clause
// when the group lists an open finding.
func findingsHeader(group report.FindingsGroup) string {
	fix := group.Type.Fix()
	header := fmt.Sprintf("%s (%s)", fix.Heading, findingsGroupCount(group))
	if !hasOpen(group) {
		return header
	}
	return header + ": " + fix.GroupClause
}

// findingsGroupCount is the count in a group header: the findings listed, as groups for payee-variants and
// similar-categories, and for uncategorized the payees and the splits they list, the splits left out when none is listed.
func findingsGroupCount(group report.FindingsGroup) string {
	if group.Type == finding.PayeeVariants || group.Type == finding.SimilarCategories {
		return humanize.Count(len(group.Findings), "group", "groups")
	}
	if group.Type != finding.Uncategorized {
		return humanize.Thousands(len(group.Findings))
	}
	splits := 0
	for _, f := range group.Findings {
		splits += len(f.Items)
	}
	payees := humanize.Count(len(group.Findings), "payee", "payees")
	if splits == 0 {
		return payees
	}
	return payees + ", " + humanize.Count(splits, "split", "splits")
}

// findingLines is the rows of a group's findings: the open and ignored ones laid out together, then
// one line per fixed one.
func findingLines(group report.FindingsGroup, view findingsView) []string {
	var live, fixed []report.ListedFinding
	for _, f := range group.Findings {
		if f.Status == finding.StatusFixed {
			fixed = append(fixed, f)
		} else {
			live = append(live, f)
		}
	}
	lines := liveFindingLines(group.Type, live, view)
	for _, f := range fixed {
		lines = append(lines, "  "+f.ID+"  fixed "+f.FixedAt.In(time.Local).Format(time.DateOnly)) //nolint:gosmopolitan // fixed dates are the user's local dates
	}
	return lines
}

// ignoredMarker ends the line of an ignored finding when the view lists findings of every status.
func ignoredMarker(f report.ListedFinding, view findingsView) string {
	if f.Status == finding.StatusIgnored && view.status == report.FindingsAll {
		return "  ignored"
	}
	return ""
}

// liveFindingLines is the rows of findings that are open or ignored, in order.
func liveFindingLines(typ finding.Type, findings []report.ListedFinding, view findingsView) []string {
	switch typ {
	case finding.Duplicate:
		return pairRows(findings, view, itemRows)
	case finding.UnlinkedTransfer:
		return pairRows(findings, view, unlinkedRows)
	case finding.OneSidedTransfer:
		return oneSidedFindingRows(findings, view)
	case finding.Uncategorized:
		return uncategorizedRows(findings, view)
	case finding.MixedCategories:
		return mixedRows(findings, view)
	case finding.PayeeVariants:
		return payeeVariantRows(findings, view)
	case finding.SimilarCategories:
		return similarCategoryRows(findings, view)
	case finding.UnusedCategory:
		return unusedCategoryRows(findings, view)
	}
	return nil // unreachable: the exhaustive linter fails a switch missing a finding.Types() entry, and report.Findings groups only those
}

// pairRows is each finding's id line, with the ignored marker, then one four-space row per item as rowsOf lays them out.
func pairRows(findings []report.ListedFinding, view findingsView, rowsOf func([]store.FindingItem) []string) []string {
	var lines []string
	for _, f := range findings {
		lines = append(lines, "  "+f.ID+ignoredMarker(f, view))
		for _, row := range rowsOf(f.Items) {
			lines = append(lines, "    "+row)
		}
	}
	return lines
}

// unlinkedRows is itemRows with each row ending in its transaction's category cell.
func unlinkedRows(items []store.FindingItem) []string {
	rows := itemRows(items)
	for i, item := range items {
		rows[i] += "  " + categoryCell(item)
	}
	return rows
}

// categoryCell is an item's category as text shows it: "(split)" for several splits, "(uncategorized)" for
// none, else the full path.
func categoryCell(item store.FindingItem) string {
	switch {
	case item.Splits > 1:
		return "(split)"
	case item.Category == nil:
		return "(uncategorized)"
	}
	return escapeCell(*item.Category)
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
// behind the finding's id padded to the widest among them; an ignored finding's first row ends with its marker.
func oneSidedFindingRows(findings []report.ListedFinding, view findingsView) []string {
	var legs []store.OneSidedTransfer
	var ids, markers []string
	for _, f := range findings {
		for i, item := range f.Items {
			marker := ""
			if i == 0 {
				marker = ignoredMarker(f, view)
			}
			legs = append(legs, legOf(item))
			ids = append(ids, f.ID)
			markers = append(markers, marker)
		}
	}
	idWidth := widestRunes(ids)

	leads := make([]string, len(ids))
	for i, id := range ids {
		leads[i] = "  " + padRight(id, idWidth) + "  "
	}
	rows := legRows(legs, leads)
	for i := range rows {
		rows[i] += markers[i]
	}
	return rows
}

// legOf is the one-sided leg an item describes.
func legOf(item store.FindingItem) store.OneSidedTransfer {
	return store.OneSidedTransfer{
		Date: item.Date, Account: item.Account, Currency: item.Currency, Closed: item.Closed, Active: item.Active,
		Payee: item.Payee, Amount: item.Amount, OtherAccount: item.OtherAccount, OtherAccountID: item.OtherAccountID,
	}
}

// uncategorizedRows renders one row per uncategorized finding: its id, payee, how many splits and
// the dates they run from and to, each column padded to the widest among them, then the ignored marker.
func uncategorizedRows(findings []report.ListedFinding, view findingsView) []string {
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
			padLeft(counts[i], countWidth) + "  " + spans[i] + ignoredMarker(findings[i], view)
	}
	return rows
}

// mixedRows renders each mixed-categories finding as an id line (id, payee, counts, ignored marker) over one
// four-space row per category: its path padded to the widest in the finding, then its right-aligned count.
func mixedRows(findings []report.ListedFinding, view findingsView) []string {
	ids := make([]string, len(findings))
	payees := make([]string, len(findings))
	for i, f := range findings {
		ids[i] = f.ID
		payees[i] = payeeLabel(payeeOf(f.Items))
	}
	idWidth, payeeWidth := widestRunes(ids), widestRunes(payees)

	lines := make([]string, 0, len(findings))
	for i, f := range findings {
		paths := make([]string, len(f.Items))
		for j, item := range f.Items {
			paths[j] = categoryCell(item)
		}
		lines = append(lines, "  "+padRight(ids[i], idWidth)+"  "+padRight(payees[i], payeeWidth)+"  "+
			humanize.Count(len(f.Items), "category", "categories")+", "+transactionsText(f.Items)+ignoredMarker(f, view))
		lines = append(lines, transactionRows(paths, f.Items)...)
	}
	return lines
}

// payeeVariantRows renders each payee-variants finding as an id line over one row per payee with its transaction count.
func payeeVariantRows(findings []report.ListedFinding, view findingsView) []string {
	ids := make([]string, len(findings))
	for i, f := range findings {
		ids[i] = f.ID
	}
	idWidth := widestRunes(ids)

	lines := make([]string, 0, len(findings))
	for _, f := range findings {
		names := make([]string, len(f.Items))
		for j, item := range f.Items {
			names[j] = escapeCell(item.Payee)
		}
		lines = append(lines, "  "+padRight(f.ID, idWidth)+"  "+humanize.Count(len(f.Items), "payee", "payees")+", "+
			transactionsText(f.Items)+ignoredMarker(f, view))
		lines = append(lines, transactionRows(names, f.Items)...)
	}
	return lines
}

// similarCategoryRows renders each similar-categories finding as an id line over one row per category with its split count.
func similarCategoryRows(findings []report.ListedFinding, view findingsView) []string {
	ids := make([]string, len(findings))
	for i, f := range findings {
		ids[i] = f.ID
	}
	idWidth := widestRunes(ids)

	lines := make([]string, 0, len(findings))
	for _, f := range findings {
		paths := make([]string, len(f.Items))
		counts := make([]string, len(f.Items))
		for j, item := range f.Items {
			// unreachable: items come from categories rows in one Replace and full_path is NOT NULL (duckstore/schema.go:30), so the join never yields nil
			paths[j] = escapeCell(*cmp.Or(item.Category, new(string)))
			counts[j] = humanize.Count(item.Splits, "split", "splits")
		}
		lines = append(lines, "  "+padRight(f.ID, idWidth)+"  "+humanize.Count(len(f.Items), "category", "categories")+ignoredMarker(f, view))
		lines = append(lines, countRows(paths, counts)...)
	}
	return lines
}

// unusedCategoryRows renders one row per unused-category finding: its id padded to the widest, the category's path,
// " (and N subcategories)" when it has any, then the ignored marker.
func unusedCategoryRows(findings []report.ListedFinding, view findingsView) []string {
	ids := make([]string, len(findings))
	for i, f := range findings {
		ids[i] = f.ID
	}
	idWidth := widestRunes(ids)

	rows := make([]string, len(findings))
	for i, f := range findings {
		rows[i] = "  " + padRight(f.ID, idWidth) + "  " + topCategory(f.Items) + subcategoriesClause(len(f.Items)-1) + ignoredMarker(f, view)
	}
	return rows
}

// topCategory is the path of the first of items, the unused category itself.
func topCategory(items []store.FindingItem) string {
	if len(items) == 0 || items[0].Category == nil {
		// unreachable: only a fixed finding lacks items (mergeFindings writes them for detected ones, each >= 1) and findingLines routes it away; full_path is NOT NULL (schema.go:30)
		return ""
	}
	return escapeCell(*items[0].Category)
}

// subcategoriesClause is " (and N subcategories)", or nothing when n is below 1.
func subcategoriesClause(n int) string {
	if n < 1 {
		return ""
	}
	return " (and " + humanize.Count(n, "subcategory", "subcategories") + ")"
}

// transactionsText is the sum of items' Transactions as "N transactions".
func transactionsText(items []store.FindingItem) string {
	total := 0
	for _, item := range items {
		total += item.Transactions
	}
	return humanize.Count(total, "transaction", "transactions")
}

// transactionRows is one four-space row per item: its label padded to the widest, then its right-aligned transaction count.
func transactionRows(labels []string, items []store.FindingItem) []string {
	counts := make([]string, len(items))
	for i, item := range items {
		counts[i] = humanize.Count(item.Transactions, "transaction", "transactions")
	}
	return countRows(labels, counts)
}

// countRows is one four-space row per label, padded to the widest, then its right-aligned count text.
func countRows(labels, counts []string) []string {
	labelWidth, countWidth := widestRunes(labels), widestRunes(counts)

	rows := make([]string, len(labels))
	for i := range labels {
		rows[i] = "    " + padRight(labels[i], labelWidth) + "  " + padLeft(counts[i], countWidth)
	}
	return rows
}

// payeeOf is the payee name of the first of items; the items of one finding share their payee.
func payeeOf(items []store.FindingItem) string {
	if len(items) == 0 {
		return ""
	}
	return items[0].Payee
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

// findingsFooter is the view's last line: its count of findings, or the empty line when there are none.
// Counts are over the view's type.
func findingsFooter(counts finding.Counts, view findingsView) string {
	switch view.status {
	case finding.StatusOpen:
		return openFooter(counts, view.typ)
	case finding.StatusIgnored:
		return statusFooter(counts.Ignored, "ignored", view.typ)
	case finding.StatusFixed:
		return statusFooter(counts.Fixed, "fixed", view.typ)
	}
	return allFooter(counts, view.typ)
}

// openFooter is the default view's footer: how many findings are open, then how many ignored and
// fixed ones it leaves out.
func openFooter(counts finding.Counts, typ finding.Type) string {
	line := "No open findings" + ofType(typ)
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

// statusFooter is the footer of a view listing only findings of status: "3 ignored findings".
func statusFooter(n int, status string, typ finding.Type) string {
	if n == 0 {
		return "No " + status + " findings" + ofType(typ)
	}
	return humanize.Count(n, status+" finding", status+" findings")
}

// allFooter is the footer of the view listing every status: the total, then the count of each
// status that has any.
func allFooter(counts finding.Counts, typ finding.Type) string {
	total := counts.Open + counts.Ignored + counts.Fixed
	if total == 0 {
		return "No findings" + ofType(typ)
	}
	var clauses []string
	for _, c := range []struct {
		n     int
		label string
	}{{counts.Open, "open"}, {counts.Ignored, "ignored"}, {counts.Fixed, "fixed"}} {
		if c.n > 0 {
			clauses = append(clauses, humanize.Thousands(c.n)+" "+c.label)
		}
	}
	return humanize.Count(total, "finding", "findings") + ": " + strings.Join(clauses, ", ")
}

// ofType is the phrase that names typ at the end of an empty line, nothing when no type is chosen.
func ofType(typ finding.Type) string {
	if typ == "" {
		return ""
	}
	return " of type " + string(typ)
}

// widestRunes is the width in runes of the widest of ss, 0 for an empty slice.
func widestRunes(ss []string) int {
	n := 0
	for _, s := range ss {
		n = max(n, utf8.RuneCountInString(s))
	}
	return n
}
