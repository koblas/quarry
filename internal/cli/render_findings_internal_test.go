// White-box: renderFindings, findingsFooter, findingsHeader and each finding type's row builder
// are unexported layout rules whose column widths, headers and footers are best driven directly.
package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

const (
	uncategorizedFix = ": give each payee's splits a category in Quicken\n"
	ignoreHint       = "Ignore a finding by adding its id to findings.ignore in ~/Library/Application Support/quarry/config.toml; see quarry findings --help\n"
)

// openView is the default view: open findings of every type.
var openView = findingsView{status: finding.StatusOpen}

// openFinding lists f as open.
func openFinding(f store.Finding) report.ListedFinding {
	return report.ListedFinding{Finding: f, Status: finding.StatusOpen}
}

// duplicatePair is a duplicate finding of two items in one account and payee at amount cents.
func duplicatePair(id string, account store.FindingItem, payees [2]string, dates [2]time.Time, cents int64) report.ListedFinding {
	first, second := account, account
	first.Payee, first.Date, first.Amount = payees[0], dates[0], cents
	second.Payee, second.Date, second.Amount = payees[1], dates[1], cents
	return openFinding(store.Finding{ID: id, Type: finding.Duplicate, Items: []store.FindingItem{first, second}})
}

// oneSidedFinding is a one-sided-transfer finding of one leg.
func oneSidedFinding(id string, item store.FindingItem) report.ListedFinding {
	return openFinding(store.Finding{ID: id, Type: finding.OneSidedTransfer, Items: []store.FindingItem{item}})
}

// uncategorizedFinding is an uncategorized finding of splits items from first to last.
func uncategorizedFinding(id, payee string, splits int, first, last time.Time) report.ListedFinding {
	items := make([]store.FindingItem, splits)
	for i := range items {
		items[i] = store.FindingItem{Payee: payee, Date: first}
	}
	items[splits-1].Date = last
	return openFinding(store.Finding{ID: id, Type: finding.Uncategorized, Items: items})
}

func chequing(currency string) store.FindingItem {
	return store.FindingItem{Account: "Chequing", Currency: currency, Active: true}
}

func Test_renderFindings_the_specification_example_rows_and_footer(t *testing.T) {
	visa := store.FindingItem{Account: "Visa", Currency: "CAD", Closed: true, Active: true}
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{
			{Type: finding.Duplicate, Findings: []report.ListedFinding{
				duplicatePair("duplicate:txn-4410+txn-4412", chequing("CAD"), [2]string{"Hydro One", "HYDRO ONE NETWORKS"},
					[2]time.Time{utcDay(2026, 8, 3), utcDay(2026, 8, 5)}, -14217),
				duplicatePair("duplicate:txn-2001+txn-2003", visa, [2]string{"Tim Hortons", "Tim Hortons"},
					[2]time.Time{utcDay(2019, 1, 10), utcDay(2019, 1, 11)}, -245),
			}},
			{Type: finding.OneSidedTransfer, Findings: []report.ListedFinding{
				oneSidedFinding("one-sided-transfer:xfer-301", store.FindingItem{
					Account: "Visa", Currency: "CAD", Active: true, Payee: "Payment", Date: utcDay(2024, 2, 1),
					Amount: 120000, OtherAccount: new("Savings"),
				}),
			}},
			{Type: finding.Uncategorized, Findings: []report.ListedFinding{
				uncategorizedFinding("uncategorized:payee-88", "AMZN MKTP CA", 12, utcDay(2019, 3, 2), utcDay(2026, 9, 12)),
				uncategorizedFinding("uncategorized:no-payee", "", 4, utcDay(2012, 1, 1), utcDay(2020, 5, 5)),
			}},
		},
		Counts: finding.Counts{Open: 5, Ignored: 4, Fixed: 12},
	}

	got := renderFindings(listing, openView, true)

	assert.Equal(t, "Possible duplicates (2): delete the extra one in Quicken, or ignore the pair if both are real\n"+
		"  duplicate:txn-4410+txn-4412\n"+
		"    2026-08-03  Chequing (CAD)  Hydro One           -142.17\n"+
		"    2026-08-05  Chequing (CAD)  HYDRO ONE NETWORKS  -142.17\n"+
		"  duplicate:txn-2001+txn-2003\n"+
		"    2019-01-10  Visa (CAD, closed)  Tim Hortons  -2.45\n"+
		"    2019-01-11  Visa (CAD, closed)  Tim Hortons  -2.45\n"+
		"\n"+
		"One-sided transfers (1): re-enter each as a transfer between the two accounts in Quicken, or ignore it if the other account is not in this file\n"+
		"  one-sided-transfer:xfer-301  2024-02-01  Visa (CAD)  Payment  1,200.00  other account: Savings (not in this file)\n"+
		"\n"+
		"Uncategorized (2 payees, 16 splits): give each payee's splits a category in Quicken\n"+
		"  uncategorized:payee-88  AMZN MKTP CA  12 splits  2019-03-02 to 2026-09-12\n"+
		"  uncategorized:no-payee  (no payee)     4 splits  2012-01-01 to 2020-05-05\n"+
		"\n"+
		"5 open findings; 4 ignored and 12 fixed not shown (--status all)\n"+
		ignoreHint, got)
}

func Test_renderFindings_a_single_payee_and_split_use_the_singular_header(t *testing.T) {
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{{Type: finding.Uncategorized, Findings: []report.ListedFinding{
			uncategorizedFinding("uncategorized:payee-1", "Amazon", 1, utcDay(2026, 3, 1), utcDay(2026, 3, 1)),
		}}},
		Counts: finding.Counts{Open: 1},
	}

	got := renderFindings(listing, openView, false)

	assert.Equal(t, "Uncategorized (1 payee, 1 split)"+uncategorizedFix+
		"  uncategorized:payee-1  Amazon  1 split  2026-03-01\n\n1 open finding\n", got)
}

func Test_renderFindings_uncategorized_shows_the_date_once_when_first_and_last_are_equal(t *testing.T) {
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{{Type: finding.Uncategorized, Findings: []report.ListedFinding{
			uncategorizedFinding("uncategorized:payee-2", "Bakery", 3, utcDay(2026, 4, 2), utcDay(2026, 4, 2)),
		}}},
		Counts: finding.Counts{Open: 1},
	}

	got := renderFindings(listing, openView, false)

	assert.Contains(t, got, "  uncategorized:payee-2  Bakery  3 splits  2026-04-02\n")
}

func Test_renderFindings_labels_a_USD_account_and_a_payeeless_item_in_a_duplicate(t *testing.T) {
	usd := store.FindingItem{Account: "US Chequing", Currency: "USD", Active: true}
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{{Type: finding.Duplicate, Findings: []report.ListedFinding{
			duplicatePair("duplicate:txn-7+txn-9", usd, [2]string{"", "Costco"},
				[2]time.Time{utcDay(2026, 5, 1), utcDay(2026, 5, 2)}, -1234567),
		}}},
		Counts: finding.Counts{Open: 1},
	}

	got := renderFindings(listing, openView, false)

	assert.Equal(t, "Possible duplicates (1): delete the extra one in Quicken, or ignore the pair if both are real\n"+
		"  duplicate:txn-7+txn-9\n"+
		"    2026-05-01  US Chequing (USD)  (no payee)  -12,345.67\n"+
		"    2026-05-02  US Chequing (USD)  Costco      -12,345.67\n"+
		"\n1 open finding\n", got)
}

func Test_renderFindings_names_the_other_account_of_a_one_sided_leg_in_each_of_its_three_forms(t *testing.T) {
	leg := func(other, otherID *string) store.FindingItem {
		return store.FindingItem{
			Account: "Visa", Currency: "CAD", Active: true, Payee: "Payment", Date: utcDay(2024, 2, 1),
			Amount: 100, OtherAccount: other, OtherAccountID: otherID,
		}
	}
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{{Type: finding.OneSidedTransfer, Findings: []report.ListedFinding{
			oneSidedFinding("one-sided-transfer:xfer-1", leg(new("Savings"), nil)),
			oneSidedFinding("one-sided-transfer:xfer-22", leg(new("Chequing"), new("acct-1"))),
			oneSidedFinding("one-sided-transfer:xfer-3", leg(nil, nil)),
		}}},
		Counts: finding.Counts{Open: 3},
	}

	got := renderFindings(listing, openView, false)

	assert.Equal(t, "One-sided transfers (3): re-enter each as a transfer between the two accounts in Quicken, or ignore it if the other account is not in this file\n"+
		"  one-sided-transfer:xfer-1   2024-02-01  Visa (CAD)  Payment  1.00  other account: Savings (not in this file)\n"+
		"  one-sided-transfer:xfer-22  2024-02-01  Visa (CAD)  Payment  1.00  other account: Chequing\n"+
		"  one-sided-transfer:xfer-3   2024-02-01  Visa (CAD)  Payment  1.00  other account: unknown\n"+
		"\n3 open findings\n", got)
}

func Test_renderFindings_says_so_when_no_finding_is_open(t *testing.T) {
	cases := []struct {
		name   string
		counts finding.Counts
		want   string
	}{
		{name: "nothing at all", counts: finding.Counts{}, want: "No open findings\n"},
		{
			name: "some ignored and some fixed", counts: finding.Counts{Ignored: 4, Fixed: 12},
			want: "No open findings; 4 ignored and 12 fixed not shown (--status all)\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderFindings(report.FindingsListing{Counts: c.counts}, openView, true)

			assert.Equal(t, c.want, got)
		})
	}
}

func openUncategorizedListing() report.FindingsListing {
	return report.FindingsListing{
		Groups: []report.FindingsGroup{{Type: finding.Uncategorized, Findings: []report.ListedFinding{
			uncategorizedFinding("uncategorized:payee-1", "Amazon", 1, utcDay(2026, 3, 1), utcDay(2026, 3, 1)),
		}}},
		Counts: finding.Counts{Open: 1},
	}
}

func Test_renderFindings_shows_the_ignore_hint_when_asked_and_a_finding_is_open(t *testing.T) {
	got := renderFindings(openUncategorizedListing(), openView, true)

	assert.Contains(t, got, "\n1 open finding\n"+ignoreHint)
}

func Test_renderFindings_leaves_out_the_ignore_hint(t *testing.T) {
	cases := []struct {
		name    string
		listing report.FindingsListing
		view    findingsView
		asked   bool
	}{
		{name: "when_not_asked", listing: openUncategorizedListing(), view: openView, asked: false},
		{name: "when_no_finding_is_open", listing: report.FindingsListing{}, view: openView, asked: true},
		{
			name: "when_only_an_ignored_finding_is_listed",
			listing: report.FindingsListing{
				Groups: []report.FindingsGroup{{Type: finding.MixedCategories, Findings: []report.ListedFinding{
					withStatus(openFinding(store.Finding{ID: "mixed-categories:payee-3", Type: finding.MixedCategories}), finding.StatusIgnored),
				}}},
				Counts: finding.Counts{Open: 2, Ignored: 1},
			},
			view:  ignoredView,
			asked: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderFindings(c.listing, c.view, c.asked)

			assert.NotContains(t, got, ignoreHint)
		})
	}
}

func Test_uncategorizedSpan_takes_the_earliest_and_latest_date_whatever_the_item_order(t *testing.T) {
	items := []store.FindingItem{
		{Payee: "Amazon", Date: utcDay(2026, 3, 5)},
		{Payee: "Amazon", Date: utcDay(2026, 3, 1)},
		{Payee: "Amazon", Date: utcDay(2026, 3, 3)},
	}

	_, first, last := uncategorizedSpan(items)

	assert.Equal(t, utcDay(2026, 3, 1), first)
	assert.Equal(t, utcDay(2026, 3, 5), last)
}

func Test_renderFindings_groups_the_thousands_of_a_group_header_count(t *testing.T) {
	findings := make([]report.ListedFinding, 1204)
	for i := range findings {
		findings[i] = openFinding(store.Finding{ID: fmt.Sprintf("mixed-categories:payee-%d", i), Type: finding.MixedCategories})
	}
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{{Type: finding.MixedCategories, Findings: findings}},
		Counts: finding.Counts{Open: 1204},
	}

	got := renderFindings(listing, openView, false)

	assert.True(t, strings.HasPrefix(got, "Payees in mixed categories (1,204): "), got)
}

func Test_renderFindings_pads_columns_by_runes_not_bytes_for_a_non_ASCII_payee(t *testing.T) {
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{
			{Type: finding.Duplicate, Findings: []report.ListedFinding{
				duplicatePair("duplicate:txn-1+txn-2", chequing("CAD"), [2]string{"Café", "Bar"},
					[2]time.Time{utcDay(2026, 5, 1), utcDay(2026, 5, 2)}, -450),
			}},
			{Type: finding.Uncategorized, Findings: []report.ListedFinding{
				uncategorizedFinding("uncategorized:payee-1", "Café", 2, utcDay(2026, 3, 1), utcDay(2026, 3, 1)),
				uncategorizedFinding("uncategorized:payee-2", "Bar", 1, utcDay(2026, 3, 1), utcDay(2026, 3, 1)),
			}},
		},
		Counts: finding.Counts{Open: 3},
	}

	got := renderFindings(listing, openView, false)

	assert.Contains(t, got, "    2026-05-01  Chequing (CAD)  Café  -4.50\n    2026-05-02  Chequing (CAD)  Bar   -4.50\n")
	assert.Contains(t, got, "  uncategorized:payee-1  Café  2 splits  2026-03-01\n  uncategorized:payee-2  Bar    1 split  2026-03-01\n")
}

// mixedRow is one category of a mixed-categories finding and its transaction count.
type mixedRow struct {
	path string
	n    int
}

// mixedFinding is an open mixed-categories finding of payee with one item per row.
func mixedFinding(id, payee string, rows ...mixedRow) report.ListedFinding {
	items := make([]store.FindingItem, len(rows))
	for i, r := range rows {
		items[i] = store.FindingItem{Payee: payee, Category: &r.path, Transactions: r.n}
	}
	return openFinding(store.Finding{ID: id, Type: finding.MixedCategories, Items: items})
}

func mixedGroup(findings ...report.ListedFinding) report.FindingsGroup {
	return report.FindingsGroup{Type: finding.MixedCategories, Findings: findings}
}

func Test_renderFindings_the_specification_example_for_a_mixed_categories_payee(t *testing.T) {
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{mixedGroup(mixedFinding("mixed-categories:payee-12", "Costco",
			mixedRow{"Groceries", 30}, mixedRow{"Household", 12}, mixedRow{"Auto:Fuel", 6}))},
		Counts: finding.Counts{Open: 1},
	}

	got := renderFindings(listing, openView, true)

	assert.Equal(t, "Payees in mixed categories (1): pick one category per payee in Quicken, or ignore a payee whose mix is intended\n"+
		"  mixed-categories:payee-12  Costco  3 categories, 48 transactions\n"+
		"    Groceries  30 transactions\n"+
		"    Household  12 transactions\n"+
		"    Auto:Fuel   6 transactions\n"+
		"\n1 open finding\n"+ignoreHint, got)
}

func Test_mixedRows_singularises_a_one_transaction_row_and_right_aligns_counts_to_the_widest(t *testing.T) {
	findings := []report.ListedFinding{mixedFinding("mixed-categories:payee-3", "Bakery", mixedRow{"Food", 12}, mixedRow{"Gifts", 1})}

	got := mixedRows(findings, openView)

	assert.Equal(t, []string{
		"  mixed-categories:payee-3  Bakery  2 categories, 13 transactions",
		"    Food   12 transactions",
		"    Gifts    1 transaction",
	}, got)
}

func Test_mixedRows_pads_ids_and_payees_across_findings_and_paths_and_counts_within_each(t *testing.T) {
	findings := []report.ListedFinding{
		mixedFinding("mixed-categories:payee-3", "Bakery", mixedRow{"Food", 20}, mixedRow{"Gifts", 5}),
		mixedFinding("mixed-categories:payee-12", "Café Nord", mixedRow{"Dining:Out", 2}, mixedRow{"Fuel", 100}),
	}

	got := mixedRows(findings, openView)

	assert.Equal(t, []string{
		"  mixed-categories:payee-3   Bakery     2 categories, 25 transactions",
		"    Food   20 transactions",
		"    Gifts   5 transactions",
		"  mixed-categories:payee-12  Café Nord  2 categories, 102 transactions",
		"    Dining:Out    2 transactions",
		"    Fuel        100 transactions",
	}, got)
}

func Test_mixedRows_groups_the_thousands_of_a_transaction_count(t *testing.T) {
	findings := []report.ListedFinding{mixedFinding("mixed-categories:payee-3", "Costco", mixedRow{"Groceries", 1200}, mixedRow{"Fuel", 34})}

	got := mixedRows(findings, openView)

	assert.Equal(t, []string{
		"  mixed-categories:payee-3  Costco  2 categories, 1,234 transactions",
		"    Groceries  1,200 transactions",
		"    Fuel          34 transactions",
	}, got)
}

func Test_mixedRows_ends_the_id_line_of_an_ignored_finding_with_a_marker_under_the_all_view(t *testing.T) {
	findings := []report.ListedFinding{withStatus(mixedFinding("mixed-categories:payee-3", "Bakery", mixedRow{"Food", 2}, mixedRow{"Gifts", 1}), finding.StatusIgnored)}

	got := mixedRows(findings, allView)

	assert.Equal(t, []string{
		"  mixed-categories:payee-3  Bakery  2 categories, 3 transactions  ignored",
		"    Food   2 transactions",
		"    Gifts   1 transaction",
	}, got)
}

func Test_mixedRows_names_a_payee_less_finding_with_the_no_payee_label(t *testing.T) {
	findings := []report.ListedFinding{mixedFinding("mixed-categories:payee-3", "", mixedRow{"Food", 2})}

	got := mixedRows(findings, openView)

	assert.Equal(t, "  mixed-categories:payee-3  (no payee)  1 category, 2 transactions", got[0])
}

func Test_findingsRows_escape_the_names_they_show(t *testing.T) {
	date := utcDay(2026, time.May, 1)
	t.Run("duplicate item account and payee", func(t *testing.T) {
		item := store.FindingItem{Account: "Chq\tA", Currency: "CAD", Active: true, Payee: "Tim\nHortons", Date: date, Amount: 100}
		rows := itemRows([]store.FindingItem{item})
		assert.Equal(t, []string{`2026-05-01  Chq\tA (CAD)  Tim\nHortons  1.00`}, rows)
	})
	t.Run("unlinked category path", func(t *testing.T) {
		item := store.FindingItem{Account: "A", Currency: "CAD", Active: true, Date: date, Amount: 100, Category: new("Auto\t:Fuel")}
		assert.Equal(t, []string{`2026-05-01  A (CAD)  (no payee)  1.00  Auto\t:Fuel`}, unlinkedRows([]store.FindingItem{item}))
	})
	t.Run("one-sided other account", func(t *testing.T) {
		item := store.FindingItem{
			Account: "Chequing", Currency: "CAD", Active: true, Payee: "Rent", Date: date, Amount: -50000,
			OtherAccount: new("\tAccount Not Synced"),
		}
		rows := oneSidedFindingRows([]report.ListedFinding{oneSidedFinding("one-sided-transfer:x-1", item)}, openView)
		assert.Equal(t, []string{`  one-sided-transfer:x-1  2026-05-01  Chequing (CAD)  Rent  -500.00  other account: \tAccount Not Synced (not in this file)`}, rows)
	})
	t.Run("uncategorized payee", func(t *testing.T) {
		rows := uncategorizedRows([]report.ListedFinding{uncategorizedFinding("uncategorized:payee-1", "Cafe\rBar", 1, date, date)}, openView)
		assert.Equal(t, []string{`  uncategorized:payee-1  Cafe\rBar  1 split  2026-05-01`}, rows)
	})
	t.Run("mixed payee and category paths", func(t *testing.T) {
		rows := mixedRows([]report.ListedFinding{mixedFinding("mixed-categories:payee-1", "Cost\tco", mixedRow{"Auto\t:Fuel", 3}, mixedRow{"Food", 1})}, openView)
		assert.Equal(t, []string{
			`  mixed-categories:payee-1  Cost\tco  2 categories, 4 transactions`,
			`    Auto\t:Fuel  3 transactions`,
			`    Food          1 transaction`,
		}, rows)
	})
	t.Run("payee variants names", func(t *testing.T) {
		rows := payeeVariantRows([]report.ListedFinding{variantFinding("payee-variants:tim", variantRow{"Tim\tHortons", 2}, variantRow{"Tim", 1})}, openView)
		assert.Equal(t, []string{
			`  payee-variants:tim  2 payees, 3 transactions`,
			`    Tim\tHortons  2 transactions`,
			`    Tim            1 transaction`,
		}, rows)
	})
	t.Run("similar categories paths", func(t *testing.T) {
		rows := similarCategoryRows([]report.ListedFinding{similarFinding("similar-categories:grocery", similarRow{"Gro\tceries", 2}, similarRow{"Grocery", 1})}, openView)
		assert.Equal(t, []string{
			`  similar-categories:grocery  2 categories`,
			`    Gro\tceries  2 splits`,
			`    Grocery       1 split`,
		}, rows)
	})
	t.Run("unused category path", func(t *testing.T) {
		rows := unusedCategoryRows([]report.ListedFinding{unusedFinding("unused-category:cat-1", "Va\tcation")}, openView)
		assert.Equal(t, []string{`  unused-category:cat-1  Va\tcation`}, rows)
	})
	t.Run("a clean name is unchanged", func(t *testing.T) {
		rows := unusedCategoryRows([]report.ListedFinding{unusedFinding("unused-category:cat-1", "Vacation")}, openView)
		assert.Equal(t, []string{`  unused-category:cat-1  Vacation`}, rows)
	})
}

func Test_renderFindings_a_hidden_finding_status_leaves_names_escaped(t *testing.T) {
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{variantsGroup(openFinding(store.Finding{
			ID: "payee-variants:a", Type: finding.PayeeVariants,
			Items: []store.FindingItem{{Payee: "A\nB", Transactions: 1}, {Payee: "a b", Transactions: 1}},
		}))},
		Counts: finding.Counts{Open: 1},
	}

	got := renderFindings(listing, openView, false)

	assert.Contains(t, got, "    A\\nB  1 transaction\n    a b   1 transaction\n")
}

// sharesFinding is an open shares-without-cost finding for the add the arguments describe, shares in millionths.
func sharesFinding(txn string, date time.Time, account, security string, shares int64) report.ListedFinding {
	return openFinding(store.Finding{
		ID:   finding.ID(finding.SharesWithoutCost, txn),
		Type: finding.SharesWithoutCost,
		Items: []store.FindingItem{{
			Date: date, Account: account, Security: security, Shares: shares,
		}},
	})
}

func Test_sharesWithoutCostRows_pads_the_ids_accounts_and_securities_to_the_widest(t *testing.T) {
	findings := []report.ListedFinding{
		sharesFinding("itxn-9", utcDay(2016, time.March, 1), "Margin", "XEQT", 100_000_000),
		sharesFinding("itxn-123", utcDay(2015, time.December, 31), "Questrade Margin", "Vanguard FTSE", 2_500_000),
	}

	got := sharesWithoutCostRows(findings, openView)

	assert.Equal(t, []string{
		"  shares-without-cost:itxn-9    2016-03-01  Margin            XEQT           100 shares",
		"  shares-without-cost:itxn-123  2015-12-31  Questrade Margin  Vanguard FTSE  2.5 shares",
	}, got)
}

func Test_sharesWithoutCostRows_escapes_the_account_and_security_before_padding_them(t *testing.T) {
	findings := []report.ListedFinding{
		sharesFinding("itxn-1", utcDay(2016, time.March, 1), "A\tB", "X\nY", 3_000_000),
		sharesFinding("itxn-2", utcDay(2016, time.March, 1), "Cash", "ZZZZZ", 3_000_000),
	}

	got := sharesWithoutCostRows(findings, openView)

	assert.Equal(t, []string{
		`  shares-without-cost:itxn-1  2016-03-01  A\tB  X\nY   3 shares`,
		"  shares-without-cost:itxn-2  2016-03-01  Cash  ZZZZZ  3 shares",
	}, got)
}

func Test_sharesWithoutCostRows_ends_an_ignored_finding_with_a_marker_under_the_all_view_only(t *testing.T) {
	findings := []report.ListedFinding{
		withStatus(sharesFinding("itxn-1", utcDay(2016, time.March, 1), "Margin", "XEQT", 3_000_000), finding.StatusIgnored),
		sharesFinding("itxn-2", utcDay(2016, time.March, 1), "Margin", "XEQT", 3_000_000),
	}

	assert.Equal(t, []string{
		"  shares-without-cost:itxn-1  2016-03-01  Margin  XEQT  3 shares  ignored",
		"  shares-without-cost:itxn-2  2016-03-01  Margin  XEQT  3 shares",
	}, sharesWithoutCostRows(findings, allView))
	assert.Equal(t, "  shares-without-cost:itxn-1  2016-03-01  Margin  XEQT  3 shares", sharesWithoutCostRows(findings, ignoredView)[0])
}

func Test_sharesCount_says_share_only_for_exactly_one(t *testing.T) {
	cases := []struct {
		name      string
		millionth int64
		want      string
	}{
		{name: "exactly one", millionth: 1_000_000, want: "1 share"},
		{name: "half", millionth: 500_000, want: "0.5 shares"},
		{name: "one and a bit", millionth: 1_000_001, want: "1.000001 shares"},
		{name: "just under one", millionth: 999_999, want: "0.999999 shares"},
		{name: "many, grouped", millionth: 1_200_000_000, want: "1,200 shares"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, sharesCount(c.millionth))
		})
	}
}

func Test_renderFindings_lists_shares_added_with_no_cost_under_the_ruled_heading(t *testing.T) {
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{{
			Type:     finding.SharesWithoutCost,
			Findings: []report.ListedFinding{sharesFinding("itxn-123", utcDay(2016, time.March, 1), "Questrade Margin", "XEQT", 100_000_000)},
		}},
		Counts: finding.Counts{Open: 1},
	}

	got := renderFindings(listing, findingsView{status: finding.StatusOpen}, false)

	assert.Equal(t, "Shares added with no cost (1): enter what each one cost on its Add Shares transaction in Quicken, then run quarry sync; until then quarry acb counts those shares at no cost\n"+
		"  shares-without-cost:itxn-123  2016-03-01  Questrade Margin  XEQT  100 shares\n\n"+
		"1 open finding\n", got)
}

const similarFix = "merge each group into one category in Quicken"

// similarRow is one category of a similar-categories finding and its split count.
type similarRow struct {
	path   string
	splits int
}

// similarFinding is an open similar-categories finding with one item per row.
func similarFinding(id string, rows ...similarRow) report.ListedFinding {
	items := make([]store.FindingItem, len(rows))
	for i, r := range rows {
		items[i] = store.FindingItem{Category: new(r.path), Splits: r.splits}
	}
	return openFinding(store.Finding{ID: id, Type: finding.SimilarCategories, Items: items})
}

func similarGroup(findings ...report.ListedFinding) report.FindingsGroup {
	return report.FindingsGroup{Type: finding.SimilarCategories, Findings: findings}
}

func Test_renderFindings_the_specification_example_for_similar_categories(t *testing.T) {
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{similarGroup(similarFinding("similar-categories:grocery",
			similarRow{"Groceries", 812}, similarRow{"Grocery", 14}))},
		Counts: finding.Counts{Open: 1},
	}

	got := renderFindings(listing, openView, true)

	assert.Equal(t, "Similar categories (1 group): "+similarFix+"\n"+
		"  similar-categories:grocery  2 categories\n"+
		"    Groceries  812 splits\n"+
		"    Grocery     14 splits\n"+
		"\n1 open finding\n"+ignoreHint, got)
}

func Test_findingsHeader_counts_similar_categories_in_groups(t *testing.T) {
	cases := []struct {
		name  string
		group report.FindingsGroup
		want  string
	}{
		{"one group", similarGroup(similarFinding("similar-categories:a")), "Similar categories (1 group): " + similarFix},
		{"two groups", similarGroup(similarFinding("similar-categories:a"), similarFinding("similar-categories:b")), "Similar categories (2 groups): " + similarFix},
		{"no open one listed", similarGroup(withStatus(similarFinding("similar-categories:a"), finding.StatusIgnored)), "Similar categories (1 group)"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, findingsHeader(c.group))
		})
	}
}

func Test_similarCategoryRows_pads_ids_across_findings_and_paths_and_counts_within_each(t *testing.T) {
	findings := []report.ListedFinding{
		similarFinding("similar-categories:fuel", similarRow{"Auto:Fuel", 20}, similarRow{"Auto:Fuels", 5}),
		similarFinding("similar-categories:café-nord", similarRow{"Café Nord", 2}, similarRow{"Cafe Nord", 100}),
	}

	got := similarCategoryRows(findings, openView)

	assert.Equal(t, []string{
		"  similar-categories:fuel       2 categories",
		"    Auto:Fuel   20 splits",
		"    Auto:Fuels   5 splits",
		"  similar-categories:café-nord  2 categories",
		"    Café Nord    2 splits",
		"    Cafe Nord  100 splits",
	}, got)
}

func Test_similarCategoryRows_singularises_one_split_and_groups_the_thousands_of_a_row(t *testing.T) {
	findings := []report.ListedFinding{similarFinding("similar-categories:gift", similarRow{"Gift", 1200}, similarRow{"Gifts", 1}, similarRow{"gifts", 0})}

	got := similarCategoryRows(findings, openView)

	assert.Equal(t, []string{
		"  similar-categories:gift  3 categories",
		"    Gift   1,200 splits",
		"    Gifts       1 split",
		"    gifts      0 splits",
	}, got)
}

func Test_similarCategoryRows_prints_a_splits_count_of_two_not_the_split_marker(t *testing.T) {
	findings := []report.ListedFinding{similarFinding("similar-categories:gift", similarRow{"Gift", 2}, similarRow{"Gifts", 2})}

	got := similarCategoryRows(findings, openView)

	assert.Equal(t, []string{
		"  similar-categories:gift  2 categories",
		"    Gift   2 splits",
		"    Gifts  2 splits",
	}, got)
}

func Test_similarCategoryRows_ends_the_id_line_of_an_ignored_finding_with_a_marker_under_the_all_view(t *testing.T) {
	findings := []report.ListedFinding{withStatus(similarFinding("similar-categories:gift", similarRow{"Gift", 2}, similarRow{"Gifts", 1}), finding.StatusIgnored)}

	got := similarCategoryRows(findings, allView)

	assert.Equal(t, []string{
		"  similar-categories:gift  2 categories  ignored",
		"    Gift   2 splits",
		"    Gifts   1 split",
	}, got)
}

var (
	allView     = findingsView{status: report.FindingsAll}
	ignoredView = findingsView{status: finding.StatusIgnored}
	fixedView   = findingsView{status: finding.StatusFixed}
)

// withStatus is f listed as status.
func withStatus(f report.ListedFinding, status finding.Status) report.ListedFinding {
	f.Status = status
	return f
}

// fixedFinding is a fixed finding of typ: it has a fixed_at and no items.
func fixedFinding(id string, typ finding.Type, at time.Time) report.ListedFinding {
	return report.ListedFinding{ID: id, Type: typ, FixedAt: &at, Status: finding.StatusFixed}
}

func Test_findingsFooter_by_view(t *testing.T) {
	cases := []struct {
		name   string
		view   findingsView
		counts finding.Counts
		want   string
	}{
		{name: "open one open", view: openView, counts: finding.Counts{Open: 1}, want: "1 open finding"},
		{name: "open two open", view: openView, counts: finding.Counts{Open: 2}, want: "2 open findings"},
		{name: "open a thousands-grouped count", view: openView, counts: finding.Counts{Open: 1204}, want: "1,204 open findings"},
		{name: "open ignored only", view: openView, counts: finding.Counts{Open: 2, Ignored: 4}, want: "2 open findings; 4 ignored not shown (--status all)"},
		{name: "open fixed only", view: openView, counts: finding.Counts{Open: 2, Fixed: 12}, want: "2 open findings; 12 fixed not shown (--status all)"},
		{name: "open ignored and fixed", view: openView, counts: finding.Counts{Open: 1, Ignored: 4, Fixed: 12}, want: "1 open finding; 4 ignored and 12 fixed not shown (--status all)"},
		{name: "all lists each status", view: allView, counts: finding.Counts{Open: 5, Ignored: 4, Fixed: 12}, want: "21 findings: 5 open, 4 ignored, 12 fixed"},
		{name: "all omits a zero clause", view: allView, counts: finding.Counts{Open: 2, Fixed: 1}, want: "3 findings: 2 open, 1 fixed"},
		{name: "all with one finding is singular", view: allView, counts: finding.Counts{Fixed: 1}, want: "1 finding: 1 fixed"},
		{name: "all groups thousands", view: allView, counts: finding.Counts{Open: 1204}, want: "1,204 findings: 1,204 open"},
		{name: "all with nothing", view: allView, want: "No findings"},
		{name: "all with nothing of a type", view: findingsView{status: report.FindingsAll, typ: finding.Duplicate}, want: "No findings of type duplicate"},
		{name: "ignored", view: ignoredView, counts: finding.Counts{Open: 3, Ignored: 4}, want: "4 ignored findings"},
		{name: "one ignored", view: ignoredView, counts: finding.Counts{Ignored: 1}, want: "1 ignored finding"},
		{name: "ignored with nothing", view: ignoredView, counts: finding.Counts{Open: 3, Fixed: 2}, want: "No ignored findings"},
		{name: "ignored with nothing of a type", view: findingsView{status: finding.StatusIgnored, typ: finding.Duplicate}, want: "No ignored findings of type duplicate"},
		{name: "fixed", view: fixedView, counts: finding.Counts{Open: 3, Fixed: 12}, want: "12 fixed findings"},
		{name: "one fixed", view: fixedView, counts: finding.Counts{Fixed: 1}, want: "1 fixed finding"},
		{name: "fixed with nothing", view: fixedView, want: "No fixed findings"},
		{name: "fixed with nothing of a type", view: findingsView{status: finding.StatusFixed, typ: finding.Duplicate}, want: "No fixed findings of type duplicate"},
		{name: "open with nothing of a type", view: findingsView{status: finding.StatusOpen, typ: finding.Duplicate}, want: "No open findings of type duplicate"},
		{
			name: "open with nothing of a type but others hidden", view: findingsView{status: finding.StatusOpen, typ: finding.Duplicate},
			counts: finding.Counts{Ignored: 1, Fixed: 2}, want: "No open findings of type duplicate; 1 ignored and 2 fixed not shown (--status all)",
		},
		{name: "open of a type names no type", view: findingsView{status: finding.StatusOpen, typ: finding.Duplicate}, counts: finding.Counts{Open: 2}, want: "2 open findings"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, findingsFooter(c.counts, c.view))
		})
	}
}

func Test_findingLines_ends_an_ignored_finding_with_a_marker_under_the_all_view(t *testing.T) {
	visaLeg := store.FindingItem{Account: "Visa", Currency: "CAD", Active: true, Payee: "Payment", Date: utcDay(2024, 2, 1), Amount: 100}
	cases := []struct {
		name  string
		group report.FindingsGroup
		want  []string
	}{
		{
			name: "duplicate marks its id line",
			group: report.FindingsGroup{Type: finding.Duplicate, Findings: []report.ListedFinding{withStatus(
				duplicatePair("duplicate:txn-1+txn-2", chequing("CAD"), [2]string{"Rogers", "Rogers"},
					[2]time.Time{utcDay(2026, 8, 20), utcDay(2026, 8, 21)}, -5500), finding.StatusIgnored)}},
			want: []string{
				"  duplicate:txn-1+txn-2  ignored",
				"    2026-08-20  Chequing (CAD)  Rogers  -55.00",
				"    2026-08-21  Chequing (CAD)  Rogers  -55.00",
			},
		},
		{
			name: "one-sided marks the end of its row",
			group: report.FindingsGroup{Type: finding.OneSidedTransfer, Findings: []report.ListedFinding{
				withStatus(oneSidedFinding("one-sided-transfer:xfer-3", visaLeg), finding.StatusIgnored),
				oneSidedFinding("one-sided-transfer:xfer-44", visaLeg),
			}},
			want: []string{
				"  one-sided-transfer:xfer-3   2024-02-01  Visa (CAD)  Payment  1.00  other account: unknown  ignored",
				"  one-sided-transfer:xfer-44  2024-02-01  Visa (CAD)  Payment  1.00  other account: unknown",
			},
		},
		{
			name: "uncategorized marks the end of its row",
			group: report.FindingsGroup{Type: finding.Uncategorized, Findings: []report.ListedFinding{
				withStatus(uncategorizedFinding("uncategorized:payee-1", "Amazon", 1, utcDay(2026, 3, 1), utcDay(2026, 3, 1)), finding.StatusIgnored),
				uncategorizedFinding("uncategorized:payee-2", "Bar", 2, utcDay(2026, 3, 1), utcDay(2026, 3, 5)),
			}},
			want: []string{
				"  uncategorized:payee-1  Amazon   1 split  2026-03-01  ignored",
				"  uncategorized:payee-2  Bar     2 splits  2026-03-01 to 2026-03-05",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, findingLines(c.group, allView))
		})
	}
}

func Test_findingLines_leaves_an_ignored_finding_unmarked_when_the_view_lists_only_ignored_ones(t *testing.T) {
	group := report.FindingsGroup{Type: finding.Uncategorized, Findings: []report.ListedFinding{
		withStatus(uncategorizedFinding("uncategorized:payee-1", "Amazon", 1, utcDay(2026, 3, 1), utcDay(2026, 3, 1)), finding.StatusIgnored),
	}}

	assert.Equal(t, []string{"  uncategorized:payee-1  Amazon  1 split  2026-03-01"}, findingLines(group, ignoredView))
}

func Test_findingLines_shows_a_fixed_finding_as_one_unpadded_line_with_the_local_date_of_its_fixed_at(t *testing.T) {
	useZone(t, time.FixedZone("UTC-5", -5*60*60))
	group := report.FindingsGroup{Type: finding.Uncategorized, Findings: []report.ListedFinding{
		uncategorizedFinding("uncategorized:payee-1", "Amazon", 1, utcDay(2026, 3, 1), utcDay(2026, 3, 1)),
		uncategorizedFinding("uncategorized:payee-2", "Bar", 1, utcDay(2026, 3, 1), utcDay(2026, 3, 1)),
		fixedFinding("uncategorized:payee-123456", finding.Uncategorized, time.Date(2026, 10, 2, 3, 0, 0, 0, time.UTC)),
	}}

	got := findingLines(group, allView)

	assert.Equal(t, []string{
		"  uncategorized:payee-1  Amazon  1 split  2026-03-01",
		"  uncategorized:payee-2  Bar     1 split  2026-03-01",
		"  uncategorized:payee-123456  fixed 2026-10-01",
	}, got)
}

func Test_findingLines_shows_the_local_date_when_it_is_later_than_the_utc_date(t *testing.T) {
	useZone(t, time.FixedZone("UTC+9", 9*60*60))
	group := report.FindingsGroup{Type: finding.Duplicate, Findings: []report.ListedFinding{
		fixedFinding("duplicate:txn-1+txn-2", finding.Duplicate, time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)),
	}}

	assert.Equal(t, []string{"  duplicate:txn-1+txn-2  fixed 2026-10-02"}, findingLines(group, fixedView))
}

func Test_findingsHeader_keeps_the_fix_clause_only_for_a_group_that_lists_an_open_finding(t *testing.T) {
	duplicate := func(status finding.Status) report.ListedFinding {
		return withStatus(openFinding(store.Finding{ID: "duplicate:txn-1+txn-2", Type: finding.Duplicate}), status)
	}
	cases := []struct {
		name  string
		group report.FindingsGroup
		want  string
	}{
		{
			name:  "open and ignored",
			group: report.FindingsGroup{Type: finding.Duplicate, Findings: []report.ListedFinding{duplicate(finding.StatusOpen), duplicate(finding.StatusIgnored)}},
			want:  "Possible duplicates (2): delete the extra one in Quicken, or ignore the pair if both are real",
		},
		{
			name:  "ignored only",
			group: report.FindingsGroup{Type: finding.Duplicate, Findings: []report.ListedFinding{duplicate(finding.StatusIgnored), duplicate(finding.StatusIgnored)}},
			want:  "Possible duplicates (2)",
		},
		{
			name:  "fixed only",
			group: report.FindingsGroup{Type: finding.Duplicate, Findings: []report.ListedFinding{duplicate(finding.StatusFixed)}},
			want:  "Possible duplicates (1)",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, findingsHeader(c.group))
		})
	}
}

func Test_findingsHeader_counts_the_splits_of_an_uncategorized_group_only_when_it_lists_some(t *testing.T) {
	payee := func(id string, splits int, status finding.Status) report.ListedFinding {
		f := fixedFinding(id, finding.Uncategorized, fixedTime())
		if splits > 0 {
			f = uncategorizedFinding(id, "Amazon", splits, utcDay(2026, 3, 1), utcDay(2026, 3, 1))
		}
		return withStatus(f, status)
	}
	cases := []struct {
		name  string
		group report.FindingsGroup
		want  string
	}{
		{
			name:  "fixed only lists no splits",
			group: report.FindingsGroup{Type: finding.Uncategorized, Findings: []report.ListedFinding{payee("uncategorized:payee-1", 0, finding.StatusFixed)}},
			want:  "Uncategorized (1 payee)",
		},
		{
			name: "open beside fixed counts the open splits",
			group: report.FindingsGroup{Type: finding.Uncategorized, Findings: []report.ListedFinding{
				payee("uncategorized:payee-1", 3, finding.StatusOpen), payee("uncategorized:payee-2", 0, finding.StatusFixed),
			}},
			want: "Uncategorized (2 payees, 3 splits): give each payee's splits a category in Quicken",
		},
		{
			name:  "ignored only counts its splits and has no fix",
			group: report.FindingsGroup{Type: finding.Uncategorized, Findings: []report.ListedFinding{payee("uncategorized:payee-1", 1, finding.StatusIgnored)}},
			want:  "Uncategorized (1 payee, 1 split)",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, findingsHeader(c.group))
		})
	}
}

func fixedTime() time.Time { return time.Date(2026, 10, 2, 3, 0, 0, 0, time.UTC) }

func Test_renderFindings_lists_a_fixed_group_with_its_count_and_no_fix_under_the_fixed_view(t *testing.T) {
	useZone(t, time.UTC)
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{{Type: finding.Duplicate, Findings: []report.ListedFinding{
			fixedFinding("duplicate:txn-1+txn-2", finding.Duplicate, fixedTime()),
		}}},
		Counts: finding.Counts{Open: 2, Fixed: 1},
	}

	got := renderFindings(listing, fixedView, true)

	assert.Equal(t, "Possible duplicates (1)\n  duplicate:txn-1+txn-2  fixed 2026-10-02\n\n1 fixed finding\n", got)
}

func Test_renderFindings_prints_the_hint_when_an_open_finding_is_listed_beside_an_ignored_one(t *testing.T) {
	withOpen := report.FindingsListing{
		Groups: []report.FindingsGroup{{Type: finding.MixedCategories, Findings: []report.ListedFinding{
			openFinding(store.Finding{ID: "mixed-categories:payee-7", Type: finding.MixedCategories}),
			withStatus(openFinding(store.Finding{ID: "mixed-categories:payee-3", Type: finding.MixedCategories}), finding.StatusIgnored),
		}}},
		Counts: finding.Counts{Open: 1, Ignored: 1},
	}

	got := renderFindings(withOpen, allView, true)

	assert.Contains(t, got, ignoreHint)
}

// unclassifiedFinding is an open unclassified-account finding for the account the arguments describe.
func unclassifiedFinding(id, name, accountType, currency string, closed bool) report.ListedFinding {
	return openFinding(store.Finding{
		ID:   finding.ID(finding.UnclassifiedAccount, id),
		Type: finding.UnclassifiedAccount,
		Items: []store.FindingItem{{
			AccountID: id, Account: name, AccountType: accountType, Currency: currency, Closed: closed,
		}},
	})
}

func Test_unclassifiedRows_pads_the_ids_and_names_to_the_widest(t *testing.T) {
	findings := []report.ListedFinding{
		unclassifiedFinding("acct-9", "Old RRSP", "retirement", "CAD", false),
		unclassifiedFinding("acct-12", "Questrade TFSA", "brokerage", "USD", false),
	}

	got := unclassifiedRows(findings, openView)

	assert.Equal(t, []string{
		"  unclassified-account:acct-9   Old RRSP        retirement, CAD",
		"  unclassified-account:acct-12  Questrade TFSA  brokerage, USD",
	}, got)
}

func Test_unclassifiedRows_ends_a_closed_account_with_closed(t *testing.T) {
	findings := []report.ListedFinding{
		unclassifiedFinding("acct-31", "Old RRSP", "retirement", "CAD", true),
		unclassifiedFinding("acct-12", "Questrade TFSA", "brokerage", "CAD", false),
	}

	got := unclassifiedRows(findings, openView)

	assert.Equal(t, []string{
		"  unclassified-account:acct-31  Old RRSP        retirement, CAD, closed",
		"  unclassified-account:acct-12  Questrade TFSA  brokerage, CAD",
	}, got)
}

func Test_unclassifiedRows_escapes_the_account_name_before_padding_it(t *testing.T) {
	findings := []report.ListedFinding{
		unclassifiedFinding("acct-1", "A\tB", "brokerage", "CAD", false),
		unclassifiedFinding("acct-2", "Cash", "brokerage", "CAD", false),
	}

	got := unclassifiedRows(findings, openView)

	assert.Equal(t, []string{
		`  unclassified-account:acct-1  A\tB  brokerage, CAD`,
		"  unclassified-account:acct-2  Cash  brokerage, CAD",
	}, got)
}

func Test_unclassifiedRows_ends_an_ignored_finding_with_a_marker_under_the_all_view_only(t *testing.T) {
	findings := []report.ListedFinding{
		withStatus(unclassifiedFinding("acct-12", "Questrade TFSA", "brokerage", "CAD", true), finding.StatusIgnored),
		unclassifiedFinding("acct-31", "Old RRSP", "retirement", "CAD", false),
	}

	assert.Equal(t, []string{
		"  unclassified-account:acct-12  Questrade TFSA  brokerage, CAD, closed  ignored",
		"  unclassified-account:acct-31  Old RRSP        retirement, CAD",
	}, unclassifiedRows(findings, allView))
	assert.Equal(t, "  unclassified-account:acct-12  Questrade TFSA  brokerage, CAD, closed", unclassifiedRows(findings, ignoredView)[0])
}

func Test_renderFindings_lists_unclassified_accounts_under_the_ruled_heading(t *testing.T) {
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{{Type: finding.UnclassifiedAccount, Findings: []report.ListedFinding{
			unclassifiedFinding("acct-31", "Old RRSP", "retirement", "CAD", true),
			unclassifiedFinding("acct-12", "Questrade TFSA", "brokerage", "CAD", false),
		}}},
		Counts: finding.Counts{Open: 2},
	}

	got := renderFindings(listing, openView, true)

	assert.Equal(t, "Unclassified investment accounts (2): "+
		"list each account's id (acct-…) in accounts.registered or accounts.non-registered in ~/Library/Application Support/quarry/config.toml; see quarry findings --help\n"+
		"  unclassified-account:acct-31  Old RRSP        retirement, CAD, closed\n"+
		"  unclassified-account:acct-12  Questrade TFSA  brokerage, CAD\n"+
		"\n2 open findings\n"+ignoreHint, got)
}

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
		Account: "Chequing", Currency: "CAD", Active: true, Payee: "Visa payment", Date: utcDay(2026, 7, 2),
		Amount: -50000, Category: new("Bills"), Splits: 1,
	}
	second := store.FindingItem{
		Account: "Visa", Currency: "CAD", Active: true, Payee: "Payment thank you", Date: utcDay(2026, 7, 3),
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
	split := store.FindingItem{Account: "Chequing", Currency: "CAD", Active: true, Payee: "Rent", Date: utcDay(2026, 7, 2), Amount: -120000, Splits: 2}
	bare := store.FindingItem{Account: "Visa", Currency: "CAD", Active: true, Payee: "Rent", Date: utcDay(2026, 7, 3), Amount: 120000, Splits: 1}

	got := renderFindings(unlinkedListing(unlinkedFinding("unlinked-transfer:txn-1+txn-2", split, bare)), openView, false)

	assert.Equal(t, "Unlinked transfers (1)"+unlinkedFix+
		"  unlinked-transfer:txn-1+txn-2\n"+
		"    2026-07-02  Chequing (CAD)  Rent  -1,200.00  (split)\n"+
		"    2026-07-03  Visa (CAD)      Rent   1,200.00  (uncategorized)\n"+
		"\n1 open finding\n", got)
}

func Test_renderFindings_labels_a_closed_and_a_USD_account_and_a_payeeless_item_in_an_unlinked_transfer(t *testing.T) {
	closed := store.FindingItem{Account: "Old Visa", Currency: "CAD", Closed: true, Date: utcDay(2019, 1, 10), Amount: -245, Category: new("Fees"), Splits: 1}
	usd := store.FindingItem{Account: "US Savings", Currency: "USD", Active: true, Payee: "Transfer", Date: utcDay(2019, 1, 11), Amount: 245, Splits: 1}

	got := renderFindings(unlinkedListing(unlinkedFinding("unlinked-transfer:txn-7+txn-9", closed, usd)), openView, false)

	assert.Equal(t, "Unlinked transfers (1)"+unlinkedFix+
		"  unlinked-transfer:txn-7+txn-9\n"+
		"    2019-01-10  Old Visa (CAD, closed)  (no payee)  -2.45  Fees\n"+
		"    2019-01-11  US Savings (USD)        Transfer     2.45  (uncategorized)\n"+
		"\n1 open finding\n", got)
}

func Test_findingLines_ends_the_id_line_of_an_ignored_unlinked_transfer_with_a_marker_under_the_all_view(t *testing.T) {
	item := store.FindingItem{Account: "Visa", Currency: "CAD", Active: true, Payee: "P", Date: utcDay(2026, 7, 2), Amount: 100, Category: new("Bills"), Splits: 1}
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

const unusedFix = "no transaction uses them; check that no scheduled transaction or budget does, then delete them in Quicken"

// unusedFinding is an open unused-category finding: the category at path first, then one item per subcategory path.
func unusedFinding(id, path string, subcategories ...string) report.ListedFinding {
	items := make([]store.FindingItem, 1, 1+len(subcategories))
	items[0] = store.FindingItem{Category: new(path)}
	for _, sub := range subcategories {
		items = append(items, store.FindingItem{Category: new(sub)})
	}
	return openFinding(store.Finding{ID: id, Type: finding.UnusedCategory, Items: items})
}

func unusedGroup(findings ...report.ListedFinding) report.FindingsGroup {
	return report.FindingsGroup{Type: finding.UnusedCategory, Findings: findings}
}

func Test_renderFindings_the_specification_example_for_unused_categories(t *testing.T) {
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{unusedGroup(
			unusedFinding("unused-category:cat-17", "Auto:Parking"),
			unusedFinding("unused-category:cat-40", "Vacation", "Vacation:Hotel", "Vacation:Hotel:Suite", "Vacation:Flights"))},
		Counts: finding.Counts{Open: 2},
	}

	got := renderFindings(listing, openView, true)

	assert.Equal(t, "Unused categories (2): "+unusedFix+"\n"+
		"  unused-category:cat-17  Auto:Parking\n"+
		"  unused-category:cat-40  Vacation (and 3 subcategories)\n"+
		"\n2 open findings\n"+ignoreHint, got)
}

func Test_findingsHeader_names_the_unused_categories_listed_and_keeps_the_fix_only_with_an_open_one(t *testing.T) {
	cases := []struct {
		name  string
		group report.FindingsGroup
		want  string
	}{
		{"open", unusedGroup(unusedFinding("unused-category:cat-1", "A")), "Unused categories (1): " + unusedFix},
		{"ignored only", unusedGroup(withStatus(unusedFinding("unused-category:cat-1", "A"), finding.StatusIgnored)), "Unused categories (1)"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, findingsHeader(c.group))
		})
	}
}

func Test_unusedCategoryRows_counts_subcategories_and_leaves_the_clause_out_at_none(t *testing.T) {
	cases := []struct {
		name string
		subs []string
		want string
	}{
		{"none", nil, "  unused-category:cat-5  Vacation"},
		{"one is singular", []string{"Vacation:Hotel"}, "  unused-category:cat-5  Vacation (and 1 subcategory)"},
		{"two", []string{"Vacation:Hotel", "Vacation:Flights"}, "  unused-category:cat-5  Vacation (and 2 subcategories)"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := unusedCategoryRows([]report.ListedFinding{unusedFinding("unused-category:cat-5", "Vacation", c.subs...)}, openView)

			assert.Equal(t, []string{c.want}, got)
		})
	}
}

func Test_unusedCategoryRows_pads_the_ids_to_the_widest(t *testing.T) {
	findings := []report.ListedFinding{
		unusedFinding("unused-category:cat-9", "Auto:Parking"),
		unusedFinding("unused-category:cat-40", "Café"),
	}

	got := unusedCategoryRows(findings, openView)

	assert.Equal(t, []string{
		"  unused-category:cat-9   Auto:Parking",
		"  unused-category:cat-40  Café",
	}, got)
}

func Test_unusedCategoryRows_ends_an_ignored_finding_with_a_marker_under_the_all_view_only(t *testing.T) {
	findings := []report.ListedFinding{
		withStatus(unusedFinding("unused-category:cat-17", "Vacation", "Vacation:Hotel"), finding.StatusIgnored),
		unusedFinding("unused-category:cat-40", "Zoo"),
	}

	assert.Equal(t, []string{
		"  unused-category:cat-17  Vacation (and 1 subcategory)  ignored",
		"  unused-category:cat-40  Zoo",
	}, unusedCategoryRows(findings, allView))
	assert.Equal(t, "  unused-category:cat-17  Vacation (and 1 subcategory)", unusedCategoryRows(findings, ignoredView)[0])
}

const variantsFix = "rename each group to one payee in Quicken and add a renaming rule"

// variantRow is one payee of a payee-variants finding and its transaction count.
type variantRow struct {
	name string
	n    int
}

// variantFinding is an open payee-variants finding with one item per row.
func variantFinding(id string, rows ...variantRow) report.ListedFinding {
	items := make([]store.FindingItem, len(rows))
	for i, r := range rows {
		items[i] = store.FindingItem{Payee: r.name, Transactions: r.n}
	}
	return openFinding(store.Finding{ID: id, Type: finding.PayeeVariants, Items: items})
}

func variantsGroup(findings ...report.ListedFinding) report.FindingsGroup {
	return report.FindingsGroup{Type: finding.PayeeVariants, Findings: findings}
}

func Test_renderFindings_the_specification_example_for_payee_variants(t *testing.T) {
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{variantsGroup(variantFinding("payee-variants:tim-hortons",
			variantRow{"TIM HORTONS #1234", 212}, variantRow{"Tim Hortons", 40}))},
		Counts: finding.Counts{Open: 1},
	}

	got := renderFindings(listing, openView, true)

	assert.Equal(t, "Payee variants (1 group): "+variantsFix+"\n"+
		"  payee-variants:tim-hortons  2 payees, 252 transactions\n"+
		"    TIM HORTONS #1234  212 transactions\n"+
		"    Tim Hortons         40 transactions\n"+
		"\n1 open finding\n"+ignoreHint, got)
}

func Test_findingsHeader_counts_payee_variants_in_groups(t *testing.T) {
	cases := []struct {
		name  string
		group report.FindingsGroup
		want  string
	}{
		{"one group", variantsGroup(variantFinding("payee-variants:a")), "Payee variants (1 group): " + variantsFix},
		{"two groups", variantsGroup(variantFinding("payee-variants:a"), variantFinding("payee-variants:b")), "Payee variants (2 groups): " + variantsFix},
		{"no open one listed", variantsGroup(withStatus(variantFinding("payee-variants:a"), finding.StatusIgnored)), "Payee variants (1 group)"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, findingsHeader(c.group))
		})
	}
}

func Test_payeeVariantRows_pads_ids_across_findings_and_names_and_counts_within_each(t *testing.T) {
	findings := []report.ListedFinding{
		variantFinding("payee-variants:shell", variantRow{"Shell #1", 20}, variantRow{"SHELL", 5}),
		variantFinding("payee-variants:café-nord", variantRow{"Café Nord", 2}, variantRow{"Cafe Nord", 100}),
	}

	got := payeeVariantRows(findings, openView)

	assert.Equal(t, []string{
		"  payee-variants:shell      2 payees, 25 transactions",
		"    Shell #1  20 transactions",
		"    SHELL      5 transactions",
		"  payee-variants:café-nord  2 payees, 102 transactions",
		"    Café Nord    2 transactions",
		"    Cafe Nord  100 transactions",
	}, got)
}

func Test_payeeVariantRows_singularises_a_one_transaction_row_and_groups_the_thousands(t *testing.T) {
	findings := []report.ListedFinding{variantFinding("payee-variants:costco", variantRow{"Costco", 1200}, variantRow{"COSTCO", 1})}

	got := payeeVariantRows(findings, openView)

	assert.Equal(t, []string{
		"  payee-variants:costco  2 payees, 1,201 transactions",
		"    Costco  1,200 transactions",
		"    COSTCO       1 transaction",
	}, got)
}

func Test_payeeVariantRows_ends_the_id_line_of_an_ignored_finding_with_a_marker_under_the_all_view(t *testing.T) {
	findings := []report.ListedFinding{withStatus(variantFinding("payee-variants:shell", variantRow{"Shell", 2}, variantRow{"SHELL", 1}), finding.StatusIgnored)}

	got := payeeVariantRows(findings, allView)

	assert.Equal(t, []string{
		"  payee-variants:shell  2 payees, 3 transactions  ignored",
		"    Shell  2 transactions",
		"    SHELL   1 transaction",
	}, got)
}
