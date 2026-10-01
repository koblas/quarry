package cli

import (
	"strconv"
	"strings"

	"github.com/koblas/quarry/internal/report"
)

// findingItemCSVColumns is how many of findings --csv's columns describe an item.
const findingItemCSVColumns = 13

// findingsCSVColumns are the header cells of findings --csv, in the order findingCSVRows fills them.
var findingsCSVColumns = []string{
	"finding_id", "type", "status",
	"date", "account", "currency", "payee", "category", "amount", "other_account", "transactions", "splits",
	"transaction_id", "split_id", "payee_id", "category_id",
	"fix",
}

// renderFindingsCSV renders listing as CSV: the header, then one row per item of each finding in
// listing order; a finding with no items (a fixed one) is one row with empty item cells.
func renderFindingsCSV(listing report.FindingsListing) string {
	header := make([]csvCell, len(findingsCSVColumns))
	for i, name := range findingsCSVColumns {
		header[i] = csvCell{Text: name}
	}
	var b strings.Builder
	b.WriteString(csvRecord(header))
	for _, group := range listing.Groups {
		for _, f := range group.Findings {
			for _, row := range findingCSVRows(newFindingEntryDocument(f)) {
				b.WriteString(csvRecord(row))
			}
		}
	}
	return b.String()
}

// findingCSVRows is the rows of one finding: its id, type, status and fix around each item's cells.
func findingCSVRows(entry findingEntryDocument) [][]csvCell {
	lead := []csvCell{{Text: entry.ID}, {Text: entry.Type}, {Text: entry.Status}}
	fix := csvCell{Text: entry.Fix}
	if len(entry.Items) == 0 {
		empty := make([]csvCell, findingItemCSVColumns)
		for i := range empty {
			empty[i] = csvCell{Null: true}
		}
		return [][]csvCell{findingRow(lead, empty, fix)}
	}
	rows := make([][]csvCell, len(entry.Items))
	for i, item := range entry.Items {
		rows[i] = findingRow(lead, findingItemCSVCells(item), fix)
	}
	return rows
}

// findingRow is lead, then middle, then last as one row.
func findingRow(lead, middle []csvCell, last csvCell) []csvCell {
	return append(append(append(make([]csvCell, 0, len(lead)+len(middle)+1), lead...), middle...), last)
}

// findingItemCSVCells is item's cells in the header's order from date to category_id; fields the
// item does not carry are NULL, as in the --json item.
func findingItemCSVCells(item findingItemDocument) []csvCell {
	return []csvCell{
		csvOptional(item.Date),
		csvOptional(item.Account),
		csvOptional(item.Currency),
		csvOptional(item.Payee), csvOptional(item.Category),
		csvOptional(item.Amount),
		csvOptional(item.OtherAccount), csvOptionalCount(item.Transactions), csvOptionalCount(item.Splits),
		csvOptional(item.TransactionID), csvOptional(item.SplitID), csvOptional(item.PayeeID), csvOptional(item.CategoryID),
	}
}

// csvOptional is s as a cell, NULL when s is nil.
func csvOptional(s *string) csvCell {
	if s == nil {
		return csvCell{Null: true}
	}
	return csvCell{Text: *s}
}

// csvOptionalCount is n as a cell, NULL when n is nil.
func csvOptionalCount(n *int) csvCell {
	if n == nil {
		return csvCell{Null: true}
	}
	return csvCell{Text: strconv.Itoa(*n)}
}
