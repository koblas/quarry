package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// narrowCategoryStore: five transactions that each differ from "hit" in one variable (account, text, amount or category).
func narrowCategoryStore() store.Rows {
	visa := store.Account{ID: "acct-visa", SourceID: 2, Name: "Visa", Type: "credit", Currency: "CAD", Active: true}
	groceries, other := "cat-groceries", "cat-foo"
	rows := searchRows([]store.Account{chequingAccount("acct-chq", 1), visa}, nil,
		searchTxn{id: "hit", account: "acct-chq", payee: "Gym", sourceID: 1, day: day(2026, time.March, 10), splits: []searchSplit{{category: groceries, sourceID: 1, cents: -6000}}},
		searchTxn{id: "other-account", account: "acct-visa", payee: "Gym", sourceID: 2, day: day(2026, time.March, 11), splits: []searchSplit{{category: groceries, sourceID: 1, cents: -6000}}},
		searchTxn{id: "other-text", account: "acct-chq", payee: "Bakery", sourceID: 3, day: day(2026, time.March, 12), splits: []searchSplit{{category: groceries, sourceID: 1, cents: -6000}}},
		searchTxn{id: "other-amount", account: "acct-chq", payee: "Gym", sourceID: 4, day: day(2026, time.March, 13), splits: []searchSplit{{category: groceries, sourceID: 1, cents: -1000}}},
		searchTxn{id: "other-category", account: "acct-chq", payee: "Gym", sourceID: 5, day: day(2026, time.March, 14), splits: []searchSplit{{category: other, sourceID: 1, cents: -6000}}},
	)
	rows.Categories = []store.Category{
		{ID: "cat-food", SourceID: 3, Name: "Food", FullPath: "Food", Kind: "expense"},
		{ID: "cat-groceries", SourceID: 1, Name: "Food:Groceries", FullPath: "Food:Groceries", Kind: "expense"},
		{ID: "cat-foo", SourceID: 2, Name: "Foo", FullPath: "Foo", Kind: "expense"},
	}
	rows.ReferencedCategoryIDs = []string{groceries, other}
	return rows
}

func Test_run_search_category_narrows_with_account_text_and_amount(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "category alone", args: []string{"--category", "food"},
			want: []string{"txn-other-amount", "txn-other-text", "txn-other-account", "txn-hit"},
		},
		{name: "and an account", args: []string{"--category", "food", "--account", "Chequing"}, want: []string{"txn-other-amount", "txn-other-text", "txn-hit"}},
		{name: "and text", args: []string{"--category", "food", "gym"}, want: []string{"txn-other-amount", "txn-other-account", "txn-hit"}},
		{name: "and a minimum", args: []string{"--category", "food", "--min", "50"}, want: []string{"txn-other-text", "txn-other-account", "txn-hit"}},
		{
			name: "and all three", args: []string{"--category", "food", "--account", "Chequing", "--min", "50", "gym"},
			want: []string{"txn-hit"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := searchedJSON(t, narrowCategoryStore(), c.args...)

			assert.Equal(t, c.want, transactionIDs(doc))
			assert.Equal(t, len(c.want), doc.Matched)
		})
	}
}

func Test_run_search_json_echoes_the_category_as_given_and_null_when_absent(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want *string
	}{
		{name: "not given", args: nil},
		{name: "as typed, in its own letter case", args: []string{"--category", "food:GROCERIES"}, want: new("food:GROCERIES")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := searchedJSON(t, narrowCategoryStore(), c.args...)

			assert.Equal(t, c.want, doc.Category)
		})
	}
}

func Test_run_search_text_names_the_category_in_the_caption(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, narrowCategoryStore())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"search", "--category", "food:GROCERIES", "--min", "50"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "Transactions in all accounts, all dates, category \"food:GROCERIES\", amount at least 50.00\n")
}

func Test_run_search_category_as_text(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantExit   int
		wantStdout string
		wantStderr string
	}{
		{
			name: "a known category no split uses prints the empty result and the no-match warning", args: []string{"--category", "Travel"},
			wantStdout: "0 matching transactions\n",
			wantStderr: "quarry: warning: no transactions match the search; the store's transactions run 2026-01-01 to 2026-01-05\n",
		},
		{
			name: "a misspelling with a named account is refused on the category", args: []string{"--account", "Chequing", "--category", "Fod"}, wantExit: 1,
			wantStderr: "quarry: no category named \"Fod\"; list them with quarry sql \"SELECT full_path FROM categories ORDER BY full_path\"\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			replaceStore(t, home, categorySearchStore())
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), append([]string{"search"}, c.args...), spendEnv(&stdout, &stderr))

			require.Equal(t, c.wantExit, exitCode, stderr.String())
			assert.Contains(t, stdout.String(), c.wantStdout)
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}
