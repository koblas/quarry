package duckstore_test

import (
	"fmt"
	"testing"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// similarCat is a category with how many splits use it, in acct-1 unless account is set; an empty kind is expense.
type similarCat struct {
	path    string
	kind    string
	hidden  bool
	splits  int
	account string
}

// similarRows is rows whose categories are cat-1.. in argument order, each with its splits.
func similarRows(cats ...similarCat) store.Rows {
	rows := mixedRows()
	rows.Categories = nil
	source := int64(0)
	for i, c := range cats {
		id := fmt.Sprintf("cat-%d", i+1)
		kind := c.kind
		if kind == "" {
			kind = "expense"
		}
		account := c.account
		if account == "" {
			account = "acct-1"
		}
		rows.Categories = append(rows.Categories, store.Category{ID: id, SourceID: int64(i + 1), Name: c.path, FullPath: c.path, Kind: kind, Hidden: c.hidden})
		for range c.splits {
			source++
			txn := fmt.Sprintf("txn-%d", source)
			rows.Transactions = append(rows.Transactions, store.Transaction{
				ID: txn, SourceID: source, AccountID: account, Date: day(2026, 1, int(source)),
				Amount: -1000 * source, Currency: "CAD", Status: "uncleared",
			})
			rows.Splits = append(rows.Splits, store.Split{
				ID: fmt.Sprintf("split-%d", source), SourceID: source, TransactionID: txn, CategoryID: new(id), Amount: -1000 * source,
			})
		}
	}
	return rows
}

// similarStore builds a store from similarRows and returns its read connection.
func similarStore(t *testing.T, cats ...similarCat) *duckdb.DB {
	t.Helper()
	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), similarRows(cats...))
	require.NoError(t, err)
	return openReadOnly(t, replaced.Path)
}

// similarFindings lists each similar-categories finding with its items in stored order, as "id=category,category" joined by "; ".
func similarFindings(t *testing.T, cats ...similarCat) string {
	t.Helper()
	var got string
	require.NoError(t, similarStore(t, cats...).QueryRows(t.Context(),
		`SELECT COALESCE(string_agg(entry, '; ' ORDER BY id), '') FROM (
			SELECT f.id, f.id || '=' || string_agg(i.category_id, ',' ORDER BY i.rowid) AS entry
			FROM findings f JOIN finding_items i ON i.finding_id = f.id
			WHERE f.type = 'similar-categories' GROUP BY f.id)`, nil,
		func(scan func(dest ...any) error) error { return scan(&got) }))
	return got
}

func Test_replace_flags_similar_categories_only_with_two_sharing_a_key(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cats []similarCat
		want string
	}{
		{"one category alone", []similarCat{{path: "Groceries", splits: 2}}, ""},
		{"two categories with different keys", []similarCat{{path: "Groceries", splits: 1}, {path: "Household", splits: 1}}, ""},
		{"two categories sharing a key", []similarCat{{path: "Groceries", splits: 1}, {path: "Grocery", splits: 1}}, "similar-categories:grocery=cat-1,cat-2"},
		{"identical paths share a key", []similarCat{{path: "Auto", splits: 1}, {path: "Auto", splits: 1}}, "similar-categories:auto=cat-1,cat-2"},
		{"a nested path shares its levels", []similarCat{{path: "Auto:Fuel", splits: 1}, {path: "Auto:Fuels", splits: 1}}, "similar-categories:auto/fuel=cat-1,cat-2"},
		{"the same name under another parent is another key", []similarCat{{path: "Auto:Fuel", splits: 1}, {path: "Boat:Fuel", splits: 1}}, ""},
		{"paths with no key are never a group", []similarCat{{path: "&", splits: 1}, {path: "&&", splits: 1}}, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.want, similarFindings(t, c.cats...))
		})
	}
}

func Test_replace_flags_hidden_similar_categories(t *testing.T) {
	t.Parallel()

	got := similarFindings(t, similarCat{path: "Groceries", splits: 1}, similarCat{path: "Grocery", splits: 1, hidden: true})

	assert.Equal(t, "similar-categories:grocery=cat-1,cat-2", got)
}

func Test_replace_flags_similar_categories_no_split_uses(t *testing.T) {
	t.Parallel()

	got := similarFindings(t, similarCat{path: "Groceries"}, similarCat{path: "Grocery"})

	assert.Equal(t, "similar-categories:grocery=cat-1,cat-2", got)
}

func Test_replace_never_flags_a_category_of_another_kind(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cats []similarCat
	}{
		{"two system categories", []similarCat{{path: "Groceries", kind: "system", splits: 1}, {path: "Grocery", kind: "system", splits: 1}}},
		{"a system category beside an expense one", []similarCat{{path: "Groceries", kind: "system", splits: 1}, {path: "Grocery", splits: 1}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			assert.Empty(t, similarFindings(t, c.cats...))
		})
	}
}

func Test_replace_never_groups_an_income_and_an_expense_category(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cats []similarCat
		want string
	}{
		{"one of each kind", []similarCat{{path: "Grocery", kind: "income", splits: 1}, {path: "Grocery", splits: 1}}, ""},
		{
			"a pair of each kind is two findings with distinct ids",
			[]similarCat{
				{path: "Gift", kind: "income", splits: 1},
				{path: "Gifts", kind: "income", splits: 1},
				{path: "Gift", splits: 1},
				{path: "Gifts", splits: 1},
			},
			"similar-categories:gift=cat-3,cat-4; similar-categories:income:gift=cat-1,cat-2",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.want, similarFindings(t, c.cats...))
		})
	}
}

func Test_replace_groups_three_similar_categories_into_one_finding(t *testing.T) {
	t.Parallel()

	got := similarFindings(t, similarCat{path: "Groceries", splits: 1}, similarCat{path: "Grocery", splits: 1}, similarCat{path: "GROCERY", splits: 1})

	assert.Equal(t, "similar-categories:grocery=cat-1,cat-2,cat-3", got)
}

func Test_replace_gives_each_shared_category_key_its_own_finding(t *testing.T) {
	t.Parallel()

	got := similarFindings(t,
		similarCat{path: "Groceries", splits: 1}, similarCat{path: "Fuel", splits: 1},
		similarCat{path: "Grocery", splits: 1}, similarCat{path: "Fuels", splits: 1})

	assert.Equal(t, "similar-categories:fuel=cat-2,cat-4; similar-categories:grocery=cat-1,cat-3", got)
}

func Test_replace_lists_similar_categories_by_splits_then_path_ignoring_case_then_id(t *testing.T) {
	t.Parallel()

	got := similarFindings(t,
		similarCat{path: "Groceries", splits: 1}, similarCat{path: "grocery", splits: 1}, similarCat{path: "GROCERY", splits: 1},
		similarCat{path: "Grocery", splits: 3})

	assert.Equal(t, "similar-categories:grocery=cat-4,cat-1,cat-2,cat-3", got)
}

func Test_replace_counts_a_categorys_splits_in_a_closed_account(t *testing.T) {
	t.Parallel()

	got := similarFindings(t, similarCat{path: "Groceries", splits: 1}, similarCat{path: "Grocery", splits: 2, account: "acct-2"})

	assert.Equal(t, "similar-categories:grocery=cat-2,cat-1", got)
}
