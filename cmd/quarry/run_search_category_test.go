package main

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// categorySearchStore: one transaction per category, dated January 1..5 in the order listed,
// plus the category Travel that no split uses.
func categorySearchStore() store.Rows {
	categories := []struct {
		id, path string
		hidden   bool
	}{
		{"cat-food", "Food", false},
		{"cat-groceries", "Food:Groceries", false},
		{"cat-organic", "Food:Groceries:Organic", false},
		{"cat-foo", "Foo", false},
		{"cat-archive", "Archive", true},
		{"cat-travel", "Travel", false},
	}
	var txns []searchTxn
	var cats []store.Category
	var referenced []string
	for i, c := range categories {
		cats = append(cats, store.Category{ID: c.id, SourceID: int64(i + 1), Name: c.path, FullPath: c.path, Kind: "expense", Hidden: c.hidden})
		if c.id == "cat-travel" {
			continue
		}
		referenced = append(referenced, c.id)
		txns = append(txns, searchTxn{
			id: c.id, account: "acct-chq", sourceID: int64(i + 1), day: day(2026, time.January, i+1),
			splits: []searchSplit{{category: c.id, sourceID: 1, cents: -1000}},
		})
	}
	rows := searchRows([]store.Account{chequingAccount("acct-chq", 1)}, nil, txns...)
	rows.Categories, rows.ReferencedCategoryIDs = cats, referenced
	return rows
}

func Test_run_search_category_lists_the_category_and_everything_under_it(t *testing.T) {
	cases := []struct {
		name         string
		category     string
		wantIDs      []string
		wantWarnings []string
	}{
		{
			name:     "a top category lists itself and every level under it, not its lookalike",
			category: "Food", wantIDs: []string{"txn-cat-organic", "txn-cat-groceries", "txn-cat-food"}, wantWarnings: []string{},
		},
		{
			name:     "any letter case names the same category",
			category: "food:groceries", wantIDs: []string{"txn-cat-organic", "txn-cat-groceries"}, wantWarnings: []string{},
		},
		{
			name:     "a category that only starts with the same letters is not under it",
			category: "Foo", wantIDs: []string{"txn-cat-foo"}, wantWarnings: []string{},
		},
		{
			name: "a hidden category counts", category: "Archive", wantIDs: []string{"txn-cat-archive"}, wantWarnings: []string{},
		},
		{
			name: "a known category no split uses gives the no-match warning", category: "Travel", wantIDs: []string{},
			wantWarnings: []string{"no transactions match the search; the store's transactions run 2026-01-01 to 2026-01-05"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := searchedJSON(t, categorySearchStore(), "--category", c.category)

			assert.Equal(t, c.wantIDs, transactionIDs(doc))
			assert.Equal(t, len(c.wantIDs), doc.Matched)
			assert.Equal(t, c.wantWarnings, doc.Warnings)
		})
	}
}
