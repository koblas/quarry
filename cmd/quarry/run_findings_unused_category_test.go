package main

import (
	"bytes"
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

// syncUnusedCategories syncs a Quicken file with a used "Food" and the unused expense "Parking" and
// "Vacation", which has the unused subcategories "Hotel" and "Flights"; it returns their primary keys in that order.
func syncUnusedCategories(t *testing.T, home string) (int64, int64, int64) {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	foodPK := b.Category(v9fixture.TagRow{Name: "Food", Type: new(int64(1))})
	parkingPK := b.Category(v9fixture.TagRow{Name: "Parking", Type: new(int64(1))})
	vacationPK := b.Category(v9fixture.TagRow{Name: "Vacation", Type: new(int64(1))})
	hotelPK := b.Category(v9fixture.TagRow{Name: "Hotel", Type: new(int64(1)), ParentCategory: vacationPK})
	b.Category(v9fixture.TagRow{Name: "Flights", Type: new(int64(1)), ParentCategory: vacationPK})
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-3.01", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-3.01", CategoryTag: foodPK})
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	return parkingPK, vacationPK, hotelPK
}

func Test_run_findings_lists_unused_categories_with_their_subcategory_count(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	parkingPK, vacationPK, _ := syncUnusedCategories(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, fmt.Sprintf(`Unused categories (2): no transaction uses them; check that no scheduled transaction or budget does, then delete them in Quicken
  unused-category:cat-%d  Parking
  unused-category:cat-%d  Vacation (and 2 subcategories)

2 open findings
Ignore a finding by adding its id to findings.ignore in %s; see quarry findings --help
`, parkingPK, vacationPK, configShown), stdout.String())
}

func Test_run_findings_json_gives_an_unused_category_item_its_category_and_null_everything_else(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_, vacationPK, hotelPK := syncUnusedCategories(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings", "--json", "--type", "unused-category"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	var doc struct {
		Findings []struct {
			ID    string `json:"id"`
			Items []struct {
				Category      string  `json:"category"`
				CategoryID    string  `json:"category_id"`
				Splits        *int    `json:"splits"`
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
	vacation := doc.Findings[1]
	assert.Equal(t, fmt.Sprintf("unused-category:cat-%d", vacationPK), vacation.ID)
	require.Len(t, vacation.Items, 3)
	assert.Equal(t, []string{"Vacation", fmt.Sprintf("cat-%d", vacationPK), "Vacation:Hotel", fmt.Sprintf("cat-%d", hotelPK)},
		[]string{vacation.Items[0].Category, vacation.Items[0].CategoryID, vacation.Items[2].Category, vacation.Items[2].CategoryID})
	assert.Equal(t, []any{(*int)(nil), (*int)(nil), (*string)(nil), (*string)(nil), (*string)(nil), (*string)(nil)},
		[]any{vacation.Items[0].Splits, vacation.Items[0].Transactions, vacation.Items[0].Payee, vacation.Items[0].TransactionID, vacation.Items[0].Date, vacation.Items[0].Amount})
}
