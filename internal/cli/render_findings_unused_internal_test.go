// White-box: unusedCategoryRows and findingsHeader are unexported layout rules driven directly.
package cli

import (
	"testing"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

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
