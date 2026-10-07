// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncSimilarCategories syncs a Quicken file in which expense "Groceries" has two splits and "Grocery" one, and the
// unused income "Gift" and "Gifts" differ only by a plural; it returns the expense categories' primary keys.
func syncSimilarCategories(t *testing.T, home string) (int64, int64) {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	groceriesPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1))})
	groceryPK := b.Category(v9fixture.TagRow{Name: "Grocery", Type: new(int64(1))})
	b.Category(v9fixture.TagRow{Name: "Gift", Type: new(int64(2))})
	b.Category(v9fixture.TagRow{Name: "Gifts", Type: new(int64(2))})
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, category := range []int64{groceriesPK, groceriesPK, groceryPK} {
		day = day.AddDate(0, 0, 10)
		amount := fmt.Sprintf("-%d.01", i+3)
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: amount, PostedDate: &day})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount, CategoryTag: category})
	}
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	return groceriesPK, groceryPK
}

func Test_run_findings_lists_similar_categories_with_a_row_per_category(t *testing.T) {
	home := newHome(t)
	syncSimilarCategories(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--type", "similar-categories"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, fmt.Sprintf(`Similar categories (2 groups): merge each group into one category in Quicken
  similar-categories:grocery      2 categories
    Groceries  2 splits
    Grocery     1 split
  similar-categories:income:gift  2 categories
    Gift   0 splits
    Gifts  0 splits

2 open findings
Ignore a finding by adding its id to findings.ignore in %s; see quarry findings --help
`, configShown), stdout.String())
}

func Test_run_findings_json_gives_a_similar_categories_item_its_category_and_splits_and_null_transaction_fields(t *testing.T) {
	home := newHome(t)
	groceriesPK, groceryPK := syncSimilarCategories(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"findings", "--json", "--type", "similar-categories"})

	require.Equal(t, 0, exitCode, stderr.String())
	var doc struct {
		Findings []struct {
			ID    string `json:"id"`
			Items []struct {
				Category      string  `json:"category"`
				CategoryID    string  `json:"category_id"`
				Splits        int     `json:"splits"`
				Transactions  *int    `json:"transactions"`
				Payee         *string `json:"payee"`
				TransactionID *string `json:"transaction_id"`
				Date          *string `json:"date"`
				Amount        *string `json:"amount"`
			} `json:"items"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	require.Len(t, doc.Findings, 2)
	assert.Equal(t, []string{"similar-categories:grocery", "similar-categories:income:gift"}, []string{doc.Findings[0].ID, doc.Findings[1].ID})
	items := doc.Findings[0].Items
	require.Len(t, items, 2)
	assert.Equal(t, []string{"Groceries", fmt.Sprintf("cat-%d", groceriesPK), "Grocery", fmt.Sprintf("cat-%d", groceryPK)},
		[]string{items[0].Category, items[0].CategoryID, items[1].Category, items[1].CategoryID})
	assert.Equal(t, []int{2, 1, 0}, []int{items[0].Splits, items[1].Splits, doc.Findings[1].Items[0].Splits})
	assert.Equal(t, []any{(*int)(nil), (*string)(nil), (*string)(nil), (*string)(nil), (*string)(nil)},
		[]any{items[0].Transactions, items[0].Payee, items[0].TransactionID, items[0].Date, items[0].Amount})
}
