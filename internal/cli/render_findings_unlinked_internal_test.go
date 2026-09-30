// White-box: the unlinked-transfer row layout and its category cell are unexported rules
// whose column padding and cell forms are best driven directly over a report.FindingsListing.
package cli

import (
	"testing"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

const unlinkedFix = ": make each pair one transfer between the two accounts in Quicken, or ignore it if no money moved between your accounts\n"

// unlinkedFinding is an unlinked-transfer finding of the two items.
func unlinkedFinding(id string, first, second store.FindingItem) report.ListedFinding {
	return openFinding(store.Finding{ID: id, Type: finding.UnlinkedTransfer, Items: []store.FindingItem{first, second}})
}

// unlinkedListing lists f alone as an open unlinked-transfer group.
func unlinkedListing(f ...report.ListedFinding) report.FindingsListing {
	return report.FindingsListing{
		Groups: []report.FindingsGroup{{Type: finding.UnlinkedTransfer, Findings: f}},
		Counts: finding.Counts{Open: len(f)},
	}
}

func Test_renderFindings_the_specification_example_unlinked_transfer_pair(t *testing.T) {
	first := store.FindingItem{
		Account: "Chequing", Currency: "CAD", Active: true, Payee: "Visa payment", Date: findingDay(2026, 7, 2),
		Amount: -50000, Category: new("Bills"), Splits: 1,
	}
	second := store.FindingItem{
		Account: "Visa", Currency: "CAD", Active: true, Payee: "Payment thank you", Date: findingDay(2026, 7, 3),
		Amount: 50000, Category: new("Income:Other"), Splits: 1,
	}

	got := renderFindings(unlinkedListing(unlinkedFinding("unlinked-transfer:txn-5000+txn-5003", first, second)), openView, true)

	assert.Equal(t, "Unlinked transfers (1)"+unlinkedFix+
		"  unlinked-transfer:txn-5000+txn-5003\n"+
		"    2026-07-02  Chequing (CAD)  Visa payment       -500.00  Bills\n"+
		"    2026-07-03  Visa (CAD)      Payment thank you   500.00  Income:Other\n"+
		"\n"+
		"1 open finding\n"+
		ignoreHint, got)
}

func Test_renderFindings_an_unlinked_transfer_row_ends_with_split_and_uncategorized_cells_without_trailing_spaces(t *testing.T) {
	split := store.FindingItem{Account: "Chequing", Currency: "CAD", Active: true, Payee: "Rent", Date: findingDay(2026, 7, 2), Amount: -120000, Splits: 2}
	bare := store.FindingItem{Account: "Visa", Currency: "CAD", Active: true, Payee: "Rent", Date: findingDay(2026, 7, 3), Amount: 120000, Splits: 1}

	got := renderFindings(unlinkedListing(unlinkedFinding("unlinked-transfer:txn-1+txn-2", split, bare)), openView, false)

	assert.Equal(t, "Unlinked transfers (1)"+unlinkedFix+
		"  unlinked-transfer:txn-1+txn-2\n"+
		"    2026-07-02  Chequing (CAD)  Rent  -1,200.00  (split)\n"+
		"    2026-07-03  Visa (CAD)      Rent   1,200.00  (uncategorized)\n"+
		"\n1 open finding\n", got)
}

func Test_renderFindings_labels_a_closed_and_a_USD_account_and_a_payeeless_item_in_an_unlinked_transfer(t *testing.T) {
	closed := store.FindingItem{Account: "Old Visa", Currency: "CAD", Closed: true, Date: findingDay(2019, 1, 10), Amount: -245, Category: new("Fees"), Splits: 1}
	usd := store.FindingItem{Account: "US Savings", Currency: "USD", Active: true, Payee: "Transfer", Date: findingDay(2019, 1, 11), Amount: 245, Splits: 1}

	got := renderFindings(unlinkedListing(unlinkedFinding("unlinked-transfer:txn-7+txn-9", closed, usd)), openView, false)

	assert.Equal(t, "Unlinked transfers (1)"+unlinkedFix+
		"  unlinked-transfer:txn-7+txn-9\n"+
		"    2019-01-10  Old Visa (CAD, closed)  (no payee)  -2.45  Fees\n"+
		"    2019-01-11  US Savings (USD)        Transfer     2.45  (uncategorized)\n"+
		"\n1 open finding\n", got)
}

func Test_findingLines_ends_the_id_line_of_an_ignored_unlinked_transfer_with_a_marker_under_the_all_view(t *testing.T) {
	item := store.FindingItem{Account: "Visa", Currency: "CAD", Active: true, Payee: "P", Date: findingDay(2026, 7, 2), Amount: 100, Category: new("Bills"), Splits: 1}
	group := report.FindingsGroup{Type: finding.UnlinkedTransfer, Findings: []report.ListedFinding{
		unlinkedFinding("unlinked-transfer:txn-1+txn-2", item, item),
		withStatus(unlinkedFinding("unlinked-transfer:txn-3+txn-4", item, item), finding.StatusIgnored),
	}}

	got := findingLines(group, allView)

	assert.Equal(t, []string{
		"  unlinked-transfer:txn-1+txn-2",
		"    2026-07-02  Visa (CAD)  P  1.00  Bills",
		"    2026-07-02  Visa (CAD)  P  1.00  Bills",
		"  unlinked-transfer:txn-3+txn-4  ignored",
		"    2026-07-02  Visa (CAD)  P  1.00  Bills",
		"    2026-07-02  Visa (CAD)  P  1.00  Bills",
	}, got)
}

func Test_categoryCell_is_split_for_several_splits_uncategorized_for_none_else_the_path(t *testing.T) {
	cases := []struct {
		name string
		item store.FindingItem
		want string
	}{
		{name: "the full path of one categorized split", item: store.FindingItem{Category: new("Income:Other"), Splits: 1}, want: "Income:Other"},
		{name: "several splits, none read as the category", item: store.FindingItem{Splits: 2}, want: "(split)"},
		{name: "one split without a category", item: store.FindingItem{Splits: 1}, want: "(uncategorized)"},
		{name: "a transaction with no split", item: store.FindingItem{}, want: "(uncategorized)"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, categoryCell(c.item))
		})
	}
}
