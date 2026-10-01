// run is unexported, so its tests live in package main rather than
// importing main from outside.
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

// syncCostco syncs a Quicken file in which Costco is spent in Groceries, Household, Groceries, Household, Groceries
// and Auto:Fuel, and returns Costco's payee primary key.
func syncCostco(t *testing.T, home string) int64 {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	costcoPK := b.Payee(v9fixture.PayeeRow{Name: "Costco"})
	groceriesPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1))})
	householdPK := b.Category(v9fixture.TagRow{Name: "Household", Type: new(int64(1))})
	autoPK := b.Category(v9fixture.TagRow{Name: "Auto", Type: new(int64(1))})
	fuelPK := b.Category(v9fixture.TagRow{Name: "Fuel", Type: new(int64(1)), ParentCategory: autoPK})
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, category := range []int64{groceriesPK, householdPK, groceriesPK, householdPK, groceriesPK, fuelPK} {
		day = day.AddDate(0, 0, 10)
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-10.00", PostedDate: &day, Payee: costcoPK})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-10.00", CategoryTag: category})
	}
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	return costcoPK
}

func Test_run_findings_lists_a_mixed_categories_payee_with_its_category_rows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	costcoPK := syncCostco(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings", "--type", "mixed-categories"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, fmt.Sprintf(`Payees in mixed categories (1): pick one category per payee in Quicken, or ignore a payee whose mix is intended
  mixed-categories:payee-%d  Costco  3 categories, 6 transactions
    Groceries  3 transactions
    Household  2 transactions
    Auto:Fuel   1 transaction

1 open finding
Ignore a finding by adding its id to findings.ignore in %s; see quarry findings --help
`, costcoPK, configShown), stdout.String())
}

func Test_run_findings_json_gives_a_mixed_categories_item_its_payee_category_and_count(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	costcoPK := syncCostco(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings", "--json", "--type", "mixed-categories"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	var doc struct {
		Findings []struct {
			Items []struct {
				Payee         string  `json:"payee"`
				Category      string  `json:"category"`
				Transactions  int     `json:"transactions"`
				PayeeID       string  `json:"payee_id"`
				TransactionID *string `json:"transaction_id"`
				Date          *string `json:"date"`
				Amount        *string `json:"amount"`
			} `json:"items"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	require.Len(t, doc.Findings, 1)
	items := doc.Findings[0].Items
	require.Len(t, items, 3)
	assert.Equal(t, []string{"Groceries", "Household", "Auto:Fuel"}, []string{items[0].Category, items[1].Category, items[2].Category})
	assert.Equal(t, []int{3, 2, 1}, []int{items[0].Transactions, items[1].Transactions, items[2].Transactions})
	assert.Equal(t, []string{"Costco", fmt.Sprintf("payee-%d", costcoPK)}, []string{items[0].Payee, items[0].PayeeID})
	assert.Equal(t, []*string{nil, nil, nil}, []*string{items[0].TransactionID, items[0].Date, items[0].Amount})
}
