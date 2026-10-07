package importer_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_import_refuses_a_category_with_an_unmapped_type(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Category(v9fixture.TagRow{Name: "Food:Groceries", Type: new(int64(5))})

	reason, _ := importRefused(t, b)

	assert.Equal(t, `category "Food:Groceries" has type 5, which quarry does not map yet`, reason)
}

func Test_import_refuses_a_category_with_no_name(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	catPK := b.Category(v9fixture.TagRow{Type: new(int64(1))})

	reason, _ := importRefused(t, b)

	assert.Equal(t, `category (source id `+itoa(catPK)+`) has no name`, reason)
}

func Test_import_refuses_a_category_with_no_type(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Category(v9fixture.TagRow{Name: "Groceries"})

	reason, _ := importRefused(t, b)

	assert.Equal(t, `category "Groceries" has no type`, reason)
}

func Test_import_reads_every_payee_and_user_tag(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	payee1 := b.Payee(v9fixture.PayeeRow{Name: "Coffee Shop"})
	payee2 := b.Payee(v9fixture.PayeeRow{Name: "Landlord"})
	tag1 := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})
	tag2 := b.UserTag(v9fixture.TagRow{Name: "Business"})

	fake, _ := importOK(t, b)

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
	acctPK := newChequing(b)
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "5.00", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "5.00", CategoryTag: catPK})

	fake, _ := importOK(t, b)

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

// A category's full_path drops a deleted parent and stops there,
// rather than refusing or including the deleted ancestor's name.
func Test_import_category_full_path_ignores_a_deleted_parent(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	deletedParentPK := b.Category(v9fixture.TagRow{Name: "OldParent", Type: new(int64(1)), Deleted: true})
	b.Category(v9fixture.TagRow{Name: "Child", Type: new(int64(1)), ParentCategory: deletedParentPK})

	fake, _ := importOK(t, b)

	require.Len(t, fake.Rows.Categories, 1)
	assert.Equal(t, "Child", fake.Rows.Categories[0].FullPath)
	assert.Nil(t, fake.Rows.Categories[0].ParentID)
}

// A category referencing a nameless parent must still build a full_path,
// falling back to "(source id N)" for that ancestor.
func Test_import_full_path_falls_back_to_source_id_for_a_nameless_parent(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	parentPK := b.Category(v9fixture.TagRow{Type: new(int64(1))})
	b.Category(v9fixture.TagRow{Name: "Groceries", Type: new(int64(1)), ParentCategory: parentPK})

	reason, _ := importRefused(t, b)

	assert.Equal(t, `category (source id `+itoa(parentPK)+`) has no name`, reason)
}

// A cyclic ZPARENTCATEGORY chain must not loop: the walk is bounded by the
// number of categories read.
func Test_import_bounds_a_cyclic_category_parent_chain(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Category(v9fixture.TagRow{Name: "A", Type: new(int64(1)), ParentCategory: 2})
	b.Category(v9fixture.TagRow{Name: "B", Type: new(int64(1)), ParentCategory: 1})

	fake, _ := importOK(t, b)

	assert.Len(t, fake.Rows.Categories, 2)
}
