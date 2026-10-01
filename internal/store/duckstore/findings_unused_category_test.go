package duckstore_test

import (
	"fmt"
	"testing"

	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unusedCat is a similarCat with a parent, the 1-based position of another category in the call (0 for none).
type unusedCat struct {
	base   similarCat
	parent int
	// danglingParent is a parent id no category has.
	danglingParent string
}

func cat(path string, parent int, mods ...func(*unusedCat)) unusedCat {
	c := unusedCat{base: similarCat{path: path}, parent: parent}
	for _, m := range mods {
		m(&c)
	}
	return c
}

func withKind(kind string) func(*unusedCat) { return func(c *unusedCat) { c.base.kind = kind } }
func withSplits(n int) func(*unusedCat)     { return func(c *unusedCat) { c.base.splits = n } }
func inAccount(id string) func(*unusedCat) {
	return func(c *unusedCat) { c.base.splits, c.base.account = 1, id }
}
func hiddenCat(c *unusedCat) { c.base.hidden = true }

// unusedFindings lists each unused-category finding as "id=item,item" joined by "; " in id order, items in stored order.
func unusedFindings(t *testing.T, referenced []string, cats ...unusedCat) string {
	t.Helper()
	plain := make([]similarCat, len(cats))
	for i, c := range cats {
		plain[i] = c.base
	}
	rows := similarRows(plain...)
	for i, c := range cats {
		if c.parent != 0 {
			rows.Categories[i].ParentID = new(fmt.Sprintf("cat-%d", c.parent))
		}
		if c.danglingParent != "" {
			rows.Categories[i].ParentID = new(c.danglingParent)
		}
	}
	rows.ReferencedCategoryIDs = referenced
	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)
	require.NoError(t, err)

	var got string
	require.NoError(t, openReadOnly(t, replaced.Path).QueryRows(t.Context(),
		`SELECT COALESCE(string_agg(entry, '; ' ORDER BY id), '') FROM (
			SELECT f.id, f.id || '=' || string_agg(i.category_id, ',' ORDER BY i.rowid) AS entry
			FROM findings f JOIN finding_items i ON i.finding_id = f.id
			WHERE f.type = 'unused-category' GROUP BY f.id)`, nil,
		func(scan func(dest ...any) error) error { return scan(&got) }))
	return got
}

func Test_replace_flags_an_unused_leaf_and_not_a_used_one(t *testing.T) {
	t.Parallel()

	got := unusedFindings(t, nil, cat("Parking", 0), cat("Fuel", 0, withSplits(1)), cat("Tolls", 0))

	assert.Equal(t, "unused-category:cat-1=cat-1; unused-category:cat-3=cat-3", got)
}

func Test_replace_reports_an_unused_parent_once_with_its_subcategories_as_items(t *testing.T) {
	t.Parallel()
	cats := []unusedCat{
		cat("Vacation", 0), cat("Vacation:Hotel", 1), cat("Vacation:Hotel:Deluxe", 2), cat("Vacation:air", 1),
	}

	got := unusedFindings(t, nil, cats...)

	assert.Equal(t, "unused-category:cat-1=cat-1,cat-4,cat-2,cat-3", got)
}

func Test_replace_orders_subcategories_with_the_same_path_by_id(t *testing.T) {
	t.Parallel()
	cats := []unusedCat{cat("Vacation", 0), cat("Vacation:A", 1), cat("Vacation:Q", 1), cat("Vacation:Q", 2)}

	got := unusedFindings(t, nil, cats...)

	assert.Equal(t, "unused-category:cat-1=cat-1,cat-2,cat-3,cat-4", got)
}

func Test_replace_reports_an_unused_child_of_a_used_parent(t *testing.T) {
	t.Parallel()

	got := unusedFindings(t, nil, cat("Auto", 0, withSplits(1)), cat("Auto:Parking", 1))

	assert.Equal(t, "unused-category:cat-2=cat-2", got)
}

func Test_replace_counts_a_parent_used_when_a_subcategory_is(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		referenced []string
		grandchild unusedCat
	}{
		{"a split on a grandchild", nil, cat("Auto:Fuel:Premium", 2, withSplits(1))},
		{"a referenced grandchild", []string{"cat-3"}, cat("Auto:Fuel:Premium", 2)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := unusedFindings(t, c.referenced, cat("Auto", 0), cat("Auto:Fuel", 1), c.grandchild)

			assert.Empty(t, got)
		})
	}
}

func Test_replace_never_reports_a_hidden_category_or_one_under_it(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cats []unusedCat
	}{
		{"a hidden leaf", []unusedCat{cat("Old", 0, hiddenCat)}},
		{"an unused child of a used hidden parent", []unusedCat{cat("Old", 0, hiddenCat, withSplits(1)), cat("Old:Child", 1)}},
		{"a visible unused parent with a hidden child", []unusedCat{cat("Auto", 0), cat("Auto:Old", 1, hiddenCat)}},
		{"a hidden grandchild under an unused visible chain", []unusedCat{cat("Auto", 0), cat("Auto:Fuel", 1), cat("Auto:Fuel:Old", 2, hiddenCat)}},
		{"an unused leaf under a used middle and a used hidden grandparent", []unusedCat{
			cat("Old", 0, hiddenCat, withSplits(1)), cat("Old:Fuel", 1, withSplits(1)), cat("Old:Fuel:Premium", 2),
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, unusedFindings(t, nil, c.cats...))
		})
	}
}

func Test_replace_still_reports_an_unused_child_of_a_used_parent_that_has_a_hidden_sibling(t *testing.T) {
	t.Parallel()

	got := unusedFindings(t, nil, cat("Auto", 0, withSplits(1)), cat("Auto:Fuel", 1), cat("Auto:Old", 1, hiddenCat))

	assert.Equal(t, "unused-category:cat-2=cat-2", got)
}

func Test_replace_never_reports_a_system_category(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cats []unusedCat
	}{
		{"an unused system category", []unusedCat{cat("Transfer", 0, withKind("system"))}},
		{"an expense child of an unused system parent", []unusedCat{cat("Transfer", 0, withKind("system")), cat("Transfer:Fee", 1)}},
		{"an expense parent of a used system child", []unusedCat{cat("Auto", 0), cat("Auto:Adjust", 1, withKind("system"), withSplits(1))}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, unusedFindings(t, nil, c.cats...))
		})
	}
}

func Test_replace_reports_an_unused_income_category(t *testing.T) {
	t.Parallel()

	got := unusedFindings(t, nil, cat("Gift", 0, withKind("income")))

	assert.Equal(t, "unused-category:cat-1=cat-1", got)
}

func Test_replace_counts_a_split_in_a_closed_account_as_use(t *testing.T) {
	t.Parallel()

	got := unusedFindings(t, nil, cat("Parking", 0, inAccount("acct-2")))

	assert.Empty(t, got)
}

func Test_replace_keeps_a_referenced_category_off_the_findings(t *testing.T) {
	t.Parallel()

	got := unusedFindings(t, []string{"cat-1", "cat-99"}, cat("Charity", 0), cat("Parking", 0))

	assert.Equal(t, "unused-category:cat-2=cat-2", got)
}

func Test_replace_reports_a_category_whose_parent_is_not_in_the_store_as_a_top_node(t *testing.T) {
	t.Parallel()
	orphan := cat("Orphan", 0)
	orphan.danglingParent = "cat-99"

	got := unusedFindings(t, nil, orphan)

	assert.Equal(t, "unused-category:cat-1=cat-1", got)
}

func Test_replace_survives_a_parent_cycle_between_categories(t *testing.T) {
	t.Parallel()
	cats := []unusedCat{cat("A", 2), cat("B", 1, withSplits(1)), cat("Parking", 0)}

	got := unusedFindings(t, nil, cats...)

	assert.Equal(t, "unused-category:cat-3=cat-3", got)
}
