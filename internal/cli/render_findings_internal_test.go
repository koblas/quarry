// White-box: renderFindings and findingsFooter are unexported layout rules whose
// column widths, group headers and footer forms are best driven directly.
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

func findingDay(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// duplicatePair is a duplicate finding of two items in one account and payee at amount cents.
func duplicatePair(id string, account store.FindingItem, payees [2]string, dates [2]time.Time, cents int64) store.Finding {
	first, second := account, account
	first.Payee, first.Date, first.Amount = payees[0], dates[0], cents
	second.Payee, second.Date, second.Amount = payees[1], dates[1], cents
	return store.Finding{ID: id, Type: finding.Duplicate, Items: []store.FindingItem{first, second}}
}

// oneSidedFinding is a one-sided-transfer finding of one leg.
func oneSidedFinding(id string, item store.FindingItem) store.Finding {
	return store.Finding{ID: id, Type: finding.OneSidedTransfer, Items: []store.FindingItem{item}}
}

// uncategorizedFinding is an uncategorized finding of splits items from first to last.
func uncategorizedFinding(id, payee string, splits int, first, last time.Time) store.Finding {
	items := make([]store.FindingItem, splits)
	for i := range items {
		items[i] = store.FindingItem{Payee: payee, Date: first}
	}
	items[splits-1].Date = last
	return store.Finding{ID: id, Type: finding.Uncategorized, Items: items}
}

func chequing(currency string) store.FindingItem {
	return store.FindingItem{Account: "Chequing", Currency: currency, Active: true}
}

func Test_renderFindings_the_specification_example_rows_and_footer(t *testing.T) {
	visa := store.FindingItem{Account: "Visa", Currency: "CAD", Closed: true, Active: true}
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{
			{Type: finding.Duplicate, Findings: []store.Finding{
				duplicatePair("duplicate:txn-4410+txn-4412", chequing("CAD"), [2]string{"Hydro One", "HYDRO ONE NETWORKS"},
					[2]time.Time{findingDay(2026, 8, 3), findingDay(2026, 8, 5)}, -14217),
				duplicatePair("duplicate:txn-2001+txn-2003", visa, [2]string{"Tim Hortons", "Tim Hortons"},
					[2]time.Time{findingDay(2019, 1, 10), findingDay(2019, 1, 11)}, -245),
			}},
			{Type: finding.OneSidedTransfer, Findings: []store.Finding{
				oneSidedFinding("one-sided-transfer:xfer-301", store.FindingItem{
					Account: "Visa", Currency: "CAD", Active: true, Payee: "Payment", Date: findingDay(2024, 2, 1),
					Amount: 120000, OtherAccount: new("Savings"),
				}),
			}},
			{Type: finding.Uncategorized, Findings: []store.Finding{
				uncategorizedFinding("uncategorized:payee-88", "AMZN MKTP CA", 12, findingDay(2019, 3, 2), findingDay(2026, 9, 12)),
				uncategorizedFinding("uncategorized:no-payee", "", 4, findingDay(2012, 1, 1), findingDay(2020, 5, 5)),
			}},
		},
		Counts: finding.Counts{Open: 5, Ignored: 4, Fixed: 12},
	}

	got := renderFindings(listing, true)

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
		Groups: []report.FindingsGroup{{Type: finding.Uncategorized, Findings: []store.Finding{
			uncategorizedFinding("uncategorized:payee-1", "Amazon", 1, findingDay(2026, 3, 1), findingDay(2026, 3, 1)),
		}}},
		Counts: finding.Counts{Open: 1},
	}

	got := renderFindings(listing, false)

	assert.Equal(t, "Uncategorized (1 payee, 1 split)"+uncategorizedFix+
		"  uncategorized:payee-1  Amazon  1 split  2026-03-01\n\n1 open finding\n", got)
}

func Test_renderFindings_uncategorized_shows_the_date_once_when_first_and_last_are_equal(t *testing.T) {
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{{Type: finding.Uncategorized, Findings: []store.Finding{
			uncategorizedFinding("uncategorized:payee-2", "Bakery", 3, findingDay(2026, 4, 2), findingDay(2026, 4, 2)),
		}}},
		Counts: finding.Counts{Open: 1},
	}

	got := renderFindings(listing, false)

	assert.Contains(t, got, "  uncategorized:payee-2  Bakery  3 splits  2026-04-02\n")
}

func Test_renderFindings_labels_a_USD_account_and_a_payeeless_item_in_a_duplicate(t *testing.T) {
	usd := store.FindingItem{Account: "US Chequing", Currency: "USD", Active: true}
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{{Type: finding.Duplicate, Findings: []store.Finding{
			duplicatePair("duplicate:txn-7+txn-9", usd, [2]string{"", "Costco"},
				[2]time.Time{findingDay(2026, 5, 1), findingDay(2026, 5, 2)}, -1234567),
		}}},
		Counts: finding.Counts{Open: 1},
	}

	got := renderFindings(listing, false)

	assert.Equal(t, "Possible duplicates (1): delete the extra one in Quicken, or ignore the pair if both are real\n"+
		"  duplicate:txn-7+txn-9\n"+
		"    2026-05-01  US Chequing (USD)  (no payee)  -12,345.67\n"+
		"    2026-05-02  US Chequing (USD)  Costco      -12,345.67\n"+
		"\n1 open finding\n", got)
}

func Test_renderFindings_names_the_other_account_of_a_one_sided_leg_in_each_of_its_three_forms(t *testing.T) {
	leg := func(other, otherID *string) store.FindingItem {
		return store.FindingItem{
			Account: "Visa", Currency: "CAD", Active: true, Payee: "Payment", Date: findingDay(2024, 2, 1),
			Amount: 100, OtherAccount: other, OtherAccountID: otherID,
		}
	}
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{{Type: finding.OneSidedTransfer, Findings: []store.Finding{
			oneSidedFinding("one-sided-transfer:xfer-1", leg(new("Savings"), nil)),
			oneSidedFinding("one-sided-transfer:xfer-22", leg(new("Chequing"), new("acct-1"))),
			oneSidedFinding("one-sided-transfer:xfer-3", leg(nil, nil)),
		}}},
		Counts: finding.Counts{Open: 3},
	}

	got := renderFindings(listing, false)

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
			got := renderFindings(report.FindingsListing{Counts: c.counts}, true)

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_findingsFooter(t *testing.T) {
	cases := []struct {
		name   string
		counts finding.Counts
		want   string
	}{
		{name: "one open", counts: finding.Counts{Open: 1}, want: "1 open finding"},
		{name: "two open", counts: finding.Counts{Open: 2}, want: "2 open findings"},
		{name: "a thousands-grouped count", counts: finding.Counts{Open: 1204}, want: "1,204 open findings"},
		{name: "ignored only", counts: finding.Counts{Open: 2, Ignored: 4}, want: "2 open findings; 4 ignored not shown (--status all)"},
		{name: "fixed only", counts: finding.Counts{Open: 2, Fixed: 12}, want: "2 open findings; 12 fixed not shown (--status all)"},
		{
			name: "ignored and fixed", counts: finding.Counts{Open: 1, Ignored: 4, Fixed: 12},
			want: "1 open finding; 4 ignored and 12 fixed not shown (--status all)",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, findingsFooter(c.counts))
		})
	}
}

func Test_renderFindings_shows_the_ignore_hint_only_when_asked_and_a_finding_is_open(t *testing.T) {
	open := report.FindingsListing{
		Groups: []report.FindingsGroup{{Type: finding.Uncategorized, Findings: []store.Finding{
			uncategorizedFinding("uncategorized:payee-1", "Amazon", 1, findingDay(2026, 3, 1), findingDay(2026, 3, 1)),
		}}},
		Counts: finding.Counts{Open: 1},
	}

	assert.Contains(t, renderFindings(open, true), "\n1 open finding\n"+ignoreHint)
	assert.NotContains(t, renderFindings(open, false), "Ignore a finding")
	assert.NotContains(t, renderFindings(report.FindingsListing{}, true), "Ignore a finding")
}

func Test_renderFindings_lists_one_id_per_finding_of_a_type_with_no_row_layout(t *testing.T) {
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{
			{Type: finding.MixedCategories, Findings: []store.Finding{
				{ID: "mixed-categories:payee-3", Type: finding.MixedCategories},
				{ID: "mixed-categories:payee-7", Type: finding.MixedCategories},
			}},
			{Type: finding.Uncategorized, Findings: []store.Finding{
				uncategorizedFinding("uncategorized:payee-1", "Amazon", 1, findingDay(2026, 3, 1), findingDay(2026, 3, 1)),
			}},
		},
		Counts: finding.Counts{Open: 3},
	}

	got := renderFindings(listing, false)

	assert.Equal(t, "Payees in mixed categories (2): pick one category per payee in Quicken, or ignore a payee whose mix is intended\n"+
		"  mixed-categories:payee-3\n"+
		"  mixed-categories:payee-7\n"+
		"\n"+
		"Uncategorized (1 payee, 1 split)"+uncategorizedFix+
		"  uncategorized:payee-1  Amazon  1 split  2026-03-01\n"+
		"\n3 open findings\n", got)
}

func Test_renderFindings_groups_the_thousands_of_a_group_header_count(t *testing.T) {
	findings := make([]store.Finding, 1204)
	for i := range findings {
		findings[i] = store.Finding{ID: fmt.Sprintf("mixed-categories:payee-%d", i), Type: finding.MixedCategories}
	}
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{{Type: finding.MixedCategories, Findings: findings}},
		Counts: finding.Counts{Open: 1204},
	}

	got := renderFindings(listing, false)

	assert.True(t, strings.HasPrefix(got, "Payees in mixed categories (1,204): "), got)
}

func Test_renderFindings_pads_columns_by_runes_not_bytes_for_a_non_ASCII_payee(t *testing.T) {
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{
			{Type: finding.Duplicate, Findings: []store.Finding{
				duplicatePair("duplicate:txn-1+txn-2", chequing("CAD"), [2]string{"Café", "Bar"},
					[2]time.Time{findingDay(2026, 5, 1), findingDay(2026, 5, 2)}, -450),
			}},
			{Type: finding.Uncategorized, Findings: []store.Finding{
				uncategorizedFinding("uncategorized:payee-1", "Café", 2, findingDay(2026, 3, 1), findingDay(2026, 3, 1)),
				uncategorizedFinding("uncategorized:payee-2", "Bar", 1, findingDay(2026, 3, 1), findingDay(2026, 3, 1)),
			}},
		},
		Counts: finding.Counts{Open: 3},
	}

	got := renderFindings(listing, false)

	assert.Contains(t, got, "    2026-05-01  Chequing (CAD)  Café  -4.50\n    2026-05-02  Chequing (CAD)  Bar   -4.50\n")
	assert.Contains(t, got, "  uncategorized:payee-1  Café  2 splits  2026-03-01\n  uncategorized:payee-2  Bar    1 split  2026-03-01\n")
}
