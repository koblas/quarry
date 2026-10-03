package duckstore_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	catFood      = "cat-food"
	catGroceries = "cat-groceries"
	catOrganic   = "cat-organic"
	catFoo       = "cat-foo"
	catArchive   = "cat-archive"
	catTravel    = "cat-travel"
)

// categoryRows holds a transaction named after each of Food, Food:Groceries, Food:Groceries:Organic, Foo and the
// hidden Archive, ids "txn-<name>"; the category Travel has none.
func categoryRows() store.Rows {
	rows := searchRowsFor()
	hidden := expenseCategory(catArchive, "Archive")
	hidden.Hidden = true
	rows.Categories = append(rows.Categories,
		expenseCategory(catFood, "Food"), expenseCategory(catGroceries, "Food:Groceries"), expenseCategory(catOrganic, "Food:Groceries:Organic"),
		expenseCategory(catFoo, "Foo"), hidden, expenseCategory(catTravel, "Travel"))
	for i, t := range []struct{ name, category string }{
		{"food", catFood}, {"groceries", catGroceries}, {"organic", catOrganic}, {"foo", catFoo}, {"archive", catArchive},
	} {
		addSearch(&rows, categorized(t.name, int64(i+1), t.category))
	}
	return rows
}

// categorized is a one-split transaction in category id.
func categorized(name string, sourceID int64, id string) searchSpec {
	return searchSpec{id: name, sourceID: sourceID, parts: []searchPart{{category: new(id), sourceID: 1, cents: -100}}}
}

func Test_search_category_matches_the_category_and_everything_under_it(t *testing.T) {
	t.Parallel()
	rows := categoryRows()
	cases := []struct {
		name     string
		category string
		want     []string
	}{
		{name: "the category and every category under it", category: "Food", want: []string{"txn-food", "txn-groceries", "txn-organic"}},
		{name: "a lower-case argument", category: "food", want: []string{"txn-food", "txn-groceries", "txn-organic"}},
		{name: "an upper-case argument reaches the children", category: "FOOD", want: []string{"txn-food", "txn-groceries", "txn-organic"}},
		{name: "a child and its own child", category: "FOOD:GROCERIES", want: []string{"txn-groceries", "txn-organic"}},
		{name: "a grandchild alone", category: "food:groceries:organic", want: []string{"txn-organic"}},
		{name: "a sibling prefix is not the category", category: "Foo", want: []string{"txn-foo"}},
		{name: "half of a child's name is no category", category: "Food:Groc"},
		{name: "a hidden category counts", category: "Archive", want: []string{"txn-archive"}},
		{name: "a known category with no transactions", category: "Travel"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := searchOf(t, rows, store.SearchParams{Category: &c.category})

			assert.ElementsMatch(t, c.want, searchedIDs(got))
			assert.Equal(t, len(c.want), got.Matched)
		})
	}
}

func Test_search_category_counts_a_transaction_once_when_several_splits_match(t *testing.T) {
	t.Parallel()
	rows := categoryRows()
	addSearch(&rows, searchSpec{id: "both", sourceID: 9, parts: []searchPart{
		{category: new(catFood), sourceID: 1, cents: -100}, {category: new(catGroceries), sourceID: 2, cents: -200},
	}})
	category := "Food"

	got := searchOf(t, rows, store.SearchParams{Category: &category, Limit: 1})

	assert.Equal(t, []string{"txn-both"}, searchedIDs(got))
	assert.Equal(t, 4, got.Matched)
	assert.Len(t, got.Rows[0].Splits, 2)
}

func Test_search_category_keeps_both_filters_with_named_accounts_and_text(t *testing.T) {
	t.Parallel()
	rows := categoryRows()
	for _, g := range []struct {
		id, payee, account, category string
		day                          int
	}{
		{"hit", "Gym", acctInReports, catGroceries, 20},
		{"other-account", "Gym", acctSecond, catGroceries, 21},
		{"other-text", "Bakery", acctInReports, catGroceries, 22},
		{"other-category", "Gym", acctInReports, catFoo, 23},
		{"other-named", "Gym", acctNotReports, catGroceries, 24},
	} {
		spec := categorized(g.id, 100+int64(g.day), g.category)
		payeeID := "cat-payee-" + g.id
		rows.Payees = append(rows.Payees, store.Payee{ID: payeeID, SourceID: int64(g.day), Name: g.payee})
		spec.payee, spec.account, spec.date = &payeeID, g.account, day(2026, time.April, g.day)
		addSearch(&rows, spec)
	}
	category := "Food"

	got := searchOf(t, rows, store.SearchParams{Category: &category, Text: "gym", AccountIDs: []string{acctInReports, acctNotReports}})

	assert.Equal(t, []string{"txn-other-named", "txn-hit"}, searchedIDs(got))
	assert.False(t, got.UnknownCategory)
	assert.Equal(t, store.TransactionRange{First: day(2026, time.March, 15), Last: day(2026, time.April, 24)}, got.Transactions)
}

func Test_search_category_flags_unknown_only_when_no_path_equals_it(t *testing.T) {
	t.Parallel()
	rows := categoryRows()
	cases := []struct {
		name     string
		category *string
		want     bool
	}{
		{name: "no category given", category: nil},
		{name: "a category with transactions", category: new("Food")},
		{name: "a category in other letter case", category: new("food:GROCERIES")},
		{name: "a known category with no transactions", category: new("Travel")},
		{name: "a misspelling", category: new("Fod"), want: true},
		{name: "the empty path", category: new(""), want: true},
		{name: "a path ending in the separator", category: new("Food:"), want: true},
		{name: "a wildcard character", category: new("%"), want: true},
		{name: "half of a name", category: new("Food:Groc"), want: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := searchOf(t, rows, store.SearchParams{Category: c.category})

			assert.Equal(t, c.want, got.UnknownCategory)
		})
	}
}

func Test_search_category_known_with_no_transactions_in_the_store_is_not_unknown(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	rows.Categories = append(rows.Categories, expenseCategory(catTravel, "Travel"))
	category := "travel"

	got := searchOf(t, rows, store.SearchParams{Category: &category})

	assert.Equal(t, store.Search{}, got)
}

func Test_search_category_runs_two_statements(t *testing.T) {
	t.Parallel()
	category := "Food"
	spy := &spyReadDB{passQueries: 2, queryFault: errQueryFailed}
	st := newBuiltStore(t, spyOpener(spy))

	_, err := st.Search(t.Context(), store.SearchParams{Category: &category})

	require.NoError(t, err)
}
