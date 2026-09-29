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

func Test_import_refuses_a_category_with_an_unmapped_type(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Category(v9fixture.TagRow{Name: "Food:Groceries", Type: new(int64(5))})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `category "Food:Groceries" has type 5, which quarry does not map yet`, importReason(t, err))
}

func Test_import_refuses_a_category_with_no_name(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	catPK := b.Category(v9fixture.TagRow{Type: new(int64(1))})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `category (source id `+itoa(catPK)+`) has no name`, importReason(t, err))
}

func Test_import_refuses_a_category_with_no_type(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Category(v9fixture.TagRow{Name: "Groceries"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `category "Groceries" has no type`, importReason(t, err))
}

func Test_import_reads_every_payee_and_user_tag(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	payee1 := b.Payee(v9fixture.PayeeRow{Name: "Coffee Shop"})
	payee2 := b.Payee(v9fixture.PayeeRow{Name: "Landlord"})
	tag1 := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})
	tag2 := b.UserTag(v9fixture.TagRow{Name: "Business"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"payee-" + itoa(payee1), "payee-" + itoa(payee2),
	}, payeeIDs(fake))
	assert.ElementsMatch(t, []string{
		"tag-" + itoa(tag1), "tag-" + itoa(tag2),
	}, tagIDs(fake))
}

// categorizedSplit adds one balanced transaction whose only split points at
// catPK to b, imports the snapshot, and returns the store it wrote to.
func categorizedSplit(t *testing.T, b *v9fixture.Builder, catPK int64) *fakeStore {
	t.Helper()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "5.00", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "5.00", CategoryTag: catPK})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	require.Len(t, fake.Rows.Splits, 1)
	return fake
}

func Test_import_stores_a_split_on_uncategorized_with_no_category(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	catPK := b.Category(v9fixture.TagRow{Name: "Uncategorized", Type: new(int64(0))})

	fake := categorizedSplit(t, b, catPK)

	assert.Nil(t, fake.Rows.Splits[0].CategoryID)
	require.Len(t, fake.Rows.Categories, 1)
	assert.Equal(t, "Uncategorized", fake.Rows.Categories[0].FullPath)
}

func Test_import_keeps_an_expense_category_named_uncategorized(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	catPK := b.Category(v9fixture.TagRow{Name: "Uncategorized", Type: new(int64(1))})

	fake := categorizedSplit(t, b, catPK)

	require.NotNil(t, fake.Rows.Splits[0].CategoryID)
	assert.Equal(t, "cat-"+itoa(catPK), *fake.Rows.Splits[0].CategoryID)
}

func Test_import_keeps_a_system_subcategory_named_uncategorized(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	parentPK := b.Category(v9fixture.TagRow{Name: "Parent", Type: new(int64(0))})
	catPK := b.Category(v9fixture.TagRow{Name: "Uncategorized", Type: new(int64(0)), ParentCategory: parentPK})

	fake := categorizedSplit(t, b, catPK)

	require.NotNil(t, fake.Rows.Splits[0].CategoryID)
	assert.Equal(t, "cat-"+itoa(catPK), *fake.Rows.Splits[0].CategoryID)
}
