// White-box: findingLines, findingsHeader and findingsFooter are unexported layout rules whose
// per-status markers, headers and empty lines are best driven directly.
package cli

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

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
	visaLeg := store.FindingItem{Account: "Visa", Currency: "CAD", Active: true, Payee: "Payment", Date: findingDay(2024, 2, 1), Amount: 100}
	cases := []struct {
		name  string
		group report.FindingsGroup
		want  []string
	}{
		{
			name: "duplicate marks its id line",
			group: report.FindingsGroup{Type: finding.Duplicate, Findings: []report.ListedFinding{withStatus(
				duplicatePair("duplicate:txn-1+txn-2", chequing("CAD"), [2]string{"Rogers", "Rogers"},
					[2]time.Time{findingDay(2026, 8, 20), findingDay(2026, 8, 21)}, -5500), finding.StatusIgnored)}},
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
				withStatus(uncategorizedFinding("uncategorized:payee-1", "Amazon", 1, findingDay(2026, 3, 1), findingDay(2026, 3, 1)), finding.StatusIgnored),
				uncategorizedFinding("uncategorized:payee-2", "Bar", 2, findingDay(2026, 3, 1), findingDay(2026, 3, 5)),
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
		withStatus(uncategorizedFinding("uncategorized:payee-1", "Amazon", 1, findingDay(2026, 3, 1), findingDay(2026, 3, 1)), finding.StatusIgnored),
	}}

	assert.Equal(t, []string{"  uncategorized:payee-1  Amazon  1 split  2026-03-01"}, findingLines(group, ignoredView))
}

func Test_findingLines_shows_a_fixed_finding_as_one_unpadded_line_with_the_local_date_of_its_fixed_at(t *testing.T) {
	useZone(t, time.FixedZone("UTC-5", -5*60*60))
	group := report.FindingsGroup{Type: finding.Uncategorized, Findings: []report.ListedFinding{
		uncategorizedFinding("uncategorized:payee-1", "Amazon", 1, findingDay(2026, 3, 1), findingDay(2026, 3, 1)),
		uncategorizedFinding("uncategorized:payee-2", "Bar", 1, findingDay(2026, 3, 1), findingDay(2026, 3, 1)),
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
			f = uncategorizedFinding(id, "Amazon", splits, findingDay(2026, 3, 1), findingDay(2026, 3, 1))
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

func Test_renderFindings_leaves_out_the_hint_when_only_an_ignored_finding_is_listed(t *testing.T) {
	ignored := report.FindingsListing{
		Groups: []report.FindingsGroup{{Type: finding.MixedCategories, Findings: []report.ListedFinding{
			withStatus(openFinding(store.Finding{ID: "mixed-categories:payee-3", Type: finding.MixedCategories}), finding.StatusIgnored),
		}}},
		Counts: finding.Counts{Open: 2, Ignored: 1},
	}

	got := renderFindings(ignored, ignoredView, true)

	assert.NotContains(t, got, ignoreHint)
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
