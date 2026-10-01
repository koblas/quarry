package importer_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/importer"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const missingCategoryPK = 9999

func importReferencedCategories(t *testing.T, b *v9fixture.Builder) []string {
	t.Helper()
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: b.WriteBundle(t, t.TempDir()).DataPath})

	require.NoError(t, err)
	return fake.Rows.ReferencedCategoryIDs
}

func Test_import_records_the_categories_every_non_imported_reference_uses(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		seed func(b *v9fixture.Builder, category int64)
	}{
		{"an entry under an investment transaction", func(b *v9fixture.Builder, category int64) {
			acct := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
			txn := b.Transaction(v9fixture.TransactionRow{Entity: v9fixture.EntInvestmentTransaction, Account: acct, Amount: "-4.00", PostedDate: &day})
			b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-4.00", CategoryTag: category})
		}},
		{"an entry under a smart transaction", func(b *v9fixture.Builder, category int64) {
			acct := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
			txn := b.Transaction(v9fixture.TransactionRow{Entity: v9fixture.EntSmartCashFlowTransaction, Account: acct, Amount: "-4.00", PostedDate: &day})
			b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-4.00", CategoryTag: category})
		}},
		{"an entry with no parent", func(b *v9fixture.Builder, category int64) {
			b.Entry(v9fixture.EntryRow{Amount: "-4.00", CategoryTag: category})
		}},
		{"an entry whose parent does not exist", func(b *v9fixture.Builder, category int64) {
			b.Entry(v9fixture.EntryRow{Parent: missingCategoryPK, Amount: "-4.00", CategoryTag: category})
		}},
		{"an entry under a deleted transaction", func(b *v9fixture.Builder, category int64) {
			acct := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
			txn := b.Transaction(v9fixture.TransactionRow{Account: acct, Amount: "-4.00", PostedDate: &day, Deleted: true})
			b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-4.00", CategoryTag: category})
		}},
		{"an entry under a transaction of a deleted account", func(b *v9fixture.Builder, category int64) {
			acct := b.Account(v9fixture.AccountRow{Name: "Old", Type: "CHECKING", Currency: "CAD", Deleted: true})
			txn := b.Transaction(v9fixture.TransactionRow{Account: acct, Amount: "-4.00", PostedDate: &day})
			b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-4.00", CategoryTag: category})
		}},
		{"a budget line item", func(b *v9fixture.Builder, category int64) {
			b.BudgetLineItem(v9fixture.BudgetLineItemRow{Category: category})
		}},
		{"a loan split entry", func(b *v9fixture.Builder, category int64) {
			b.LoanSplitEntry(v9fixture.LoanSplitEntryRow{Category: category})
		}},
		{"a loan account's interest category", func(b *v9fixture.Builder, category int64) {
			b.Account(v9fixture.AccountRow{Name: "Mortgage", Type: "CHECKING", Currency: "CAD", Active: true, LoanInterestCategory: category})
		}},
		{"a quickfill rule split entry", func(b *v9fixture.Builder, category int64) {
			b.QuickfillRuleSplitEntry(v9fixture.QuickfillRuleSplitEntryRow{Category: category})
		}},
		{"a product or service", func(b *v9fixture.Builder, category int64) {
			b.ProductService(v9fixture.ProductServiceRow{Category: category})
		}},
		{"a customer credit line item", func(b *v9fixture.Builder, category int64) {
			b.CustomerCreditLineItem(v9fixture.CustomerCreditLineItemRow{Category: category})
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			category := b.Category(v9fixture.TagRow{Name: "Spend", Type: new(int64(1))})
			c.seed(b, category)

			ids := importReferencedCategories(t, b)

			assert.Equal(t, []string{"cat-" + itoa(category)}, ids)
		})
	}
}

func Test_import_records_no_category_for_a_reference_that_is_gone(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		seed func(b *v9fixture.Builder, category int64)
	}{
		{"a deleted entry under an investment transaction", func(b *v9fixture.Builder, category int64) {
			acct := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
			txn := b.Transaction(v9fixture.TransactionRow{Entity: v9fixture.EntInvestmentTransaction, Account: acct, Amount: "-4.00", PostedDate: &day})
			b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-4.00", CategoryTag: category, Deleted: true})
		}},
		{"a deleted budget line item", func(b *v9fixture.Builder, category int64) {
			b.BudgetLineItem(v9fixture.BudgetLineItemRow{Category: category, Deleted: true})
		}},
		{"a deleted loan split entry", func(b *v9fixture.Builder, category int64) {
			b.LoanSplitEntry(v9fixture.LoanSplitEntryRow{Category: category, Deleted: true})
		}},
		{"a deleted loan account", func(b *v9fixture.Builder, category int64) {
			b.Account(v9fixture.AccountRow{Name: "Mortgage", Type: "CHECKING", Currency: "CAD", Deleted: true, LoanInterestCategory: category})
		}},
		{"a deleted quickfill rule split entry", func(b *v9fixture.Builder, category int64) {
			b.QuickfillRuleSplitEntry(v9fixture.QuickfillRuleSplitEntryRow{Category: category, Deleted: true})
		}},
		{"a deleted product or service", func(b *v9fixture.Builder, category int64) {
			b.ProductService(v9fixture.ProductServiceRow{Category: category, Deleted: true})
		}},
		{"a deleted customer credit line item", func(b *v9fixture.Builder, category int64) {
			b.CustomerCreditLineItem(v9fixture.CustomerCreditLineItemRow{Category: category, Deleted: true})
		}},
		{"a budget line item naming a deleted category", func(b *v9fixture.Builder, _ int64) {
			gone := b.Category(v9fixture.TagRow{Name: "Gone", Type: new(int64(1)), Deleted: true})
			b.BudgetLineItem(v9fixture.BudgetLineItemRow{Category: gone})
		}},
		{"a quickfill rule split entry naming a missing category", func(b *v9fixture.Builder, _ int64) {
			b.QuickfillRuleSplitEntry(v9fixture.QuickfillRuleSplitEntryRow{Category: missingCategoryPK})
		}},
		{"a budget line item with no category", func(b *v9fixture.Builder, _ int64) {
			b.BudgetLineItem(v9fixture.BudgetLineItemRow{})
		}},
		{"a split under an imported transaction", func(b *v9fixture.Builder, category int64) {
			acct := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
			txn := b.Transaction(v9fixture.TransactionRow{Account: acct, Amount: "-4.00", PostedDate: &day})
			b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-4.00", CategoryTag: category})
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			category := b.Category(v9fixture.TagRow{Name: "Spend", Type: new(int64(1))})
			c.seed(b, category)

			ids := importReferencedCategories(t, b)

			assert.Nil(t, ids)
		})
	}
}

func Test_import_lists_each_referenced_category_once_in_source_id_order(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	categories := make([]int64, 0, 10)
	for _, name := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"} {
		categories = append(categories, b.Category(v9fixture.TagRow{Name: name, Type: new(int64(1))}))
	}
	tenth, ninth := categories[9], categories[8]
	b.BudgetLineItem(v9fixture.BudgetLineItemRow{Category: tenth})
	b.BudgetLineItem(v9fixture.BudgetLineItemRow{Category: tenth})
	b.QuickfillRuleSplitEntry(v9fixture.QuickfillRuleSplitEntryRow{Category: ninth})
	b.Entry(v9fixture.EntryRow{Amount: "-4.00", CategoryTag: tenth})

	ids := importReferencedCategories(t, b)

	assert.Equal(t, []string{"cat-" + itoa(ninth), "cat-" + itoa(tenth)}, ids)
	assert.Greater(t, tenth, int64(9), "ids span two digit widths, so a text sort would put the tenth first")
}
