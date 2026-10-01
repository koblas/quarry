package cli

import (
	"testing"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

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

func Test_similarCategoryRows_labels_an_income_group_by_its_prefixed_id(t *testing.T) {
	findings := []report.ListedFinding{similarFinding("similar-categories:income:gift", similarRow{"Gift", 3}, similarRow{"Gifts", 1})}

	got := similarCategoryRows(findings, openView)

	assert.Equal(t, "  similar-categories:income:gift  2 categories", got[0])
}
