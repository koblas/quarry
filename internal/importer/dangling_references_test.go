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

// A transaction in a deleted account, and its splits, are dropped
// silently — never validated, never counted, never in Rows.
func Test_import_skips_a_transaction_and_its_splits_in_a_deleted_account(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	deletedAcctPK := b.Account(v9fixture.AccountRow{Name: "Old", Type: "CHECKING", Currency: "CAD", Deleted: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: deletedAcctPK, Amount: "1.00", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Empty(t, fake.Rows.Transactions)
	assert.Empty(t, fake.Rows.Splits)
}

// A transaction whose account reference points to no row at all
// refuses as having no account, the same text as a NULL account reference.
func Test_import_refuses_a_transaction_whose_account_does_not_exist(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: 999, Amount: "1.00", PostedDate: &posted})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `a transaction (source id `+itoa(txnPK)+`) has no account`, importReason(t, err))
}

// An entry whose parent is a SmartCashFlowTransaction is dropped
// silently, the same as one whose parent was deleted or excluded.
func Test_import_skips_an_entry_whose_parent_is_a_smart_transaction(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	smartPK := b.Transaction(v9fixture.TransactionRow{
		Entity: v9fixture.EntSmartCashFlowTransaction, Account: acctPK, Amount: "1.00", PostedDate: &posted,
	})
	b.Entry(v9fixture.EntryRow{Parent: smartPK, Amount: "1.00"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Empty(t, fake.Rows.Splits)
}

func Test_import_skips_a_split_whose_parent_transaction_does_not_exist(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Entry(v9fixture.EntryRow{Parent: 999, Amount: "12.34"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Empty(t, fake.Rows.Splits)
}

// A split's category reference to a deleted category stores NULL.
func Test_import_nulls_a_splits_category_when_the_category_is_deleted(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	deletedCatPK := b.Category(v9fixture.TagRow{Name: "Old", Type: new(int64(1)), Deleted: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00", CategoryTag: deletedCatPK})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	require.Len(t, fake.Rows.Splits, 1)
	assert.Nil(t, fake.Rows.Splits[0].CategoryID)
}

// A split's category reference to no category row at all stores NULL,
// the same as a deleted one.
func Test_import_nulls_a_splits_category_when_the_category_does_not_exist(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00", CategoryTag: 999})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	require.Len(t, fake.Rows.Splits, 1)
	assert.Nil(t, fake.Rows.Splits[0].CategoryID)
}

// A transaction's payee reference to a deleted payee stores NULL.
func Test_import_nulls_a_transactions_payee_when_the_payee_is_deleted(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	deletedPayeePK := b.Payee(v9fixture.PayeeRow{Name: "Old Shop", Deleted: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted, Payee: deletedPayeePK})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	require.Len(t, fake.Rows.Transactions, 1)
	assert.Nil(t, fake.Rows.Transactions[0].PayeeID)
}

// A transaction's payee reference to no payee row at all stores NULL,
// the same as a deleted one.
func Test_import_nulls_a_transactions_payee_when_the_payee_does_not_exist(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted, Payee: 999})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	require.Len(t, fake.Rows.Transactions, 1)
	assert.Nil(t, fake.Rows.Transactions[0].PayeeID)
}

// A category's full_path drops a deleted parent and stops there,
// rather than refusing or including the deleted ancestor's name.
func Test_import_category_full_path_ignores_a_deleted_parent(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	deletedParentPK := b.Category(v9fixture.TagRow{Name: "OldParent", Type: new(int64(1)), Deleted: true})
	b.Category(v9fixture.TagRow{Name: "Child", Type: new(int64(1)), ParentCategory: deletedParentPK})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	require.Len(t, fake.Rows.Categories, 1)
	assert.Equal(t, "Child", fake.Rows.Categories[0].FullPath)
	assert.Nil(t, fake.Rows.Categories[0].ParentID)
}

// A split_tags link to a deleted tag is dropped, not stored with a
// dangling tag_id.
func Test_import_drops_a_split_tag_link_when_the_tag_is_deleted(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	deletedTagPK := b.UserTag(v9fixture.TagRow{Name: "Old", Deleted: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00"})
	b.LinkUserTag(entryPK, deletedTagPK)
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Empty(t, fake.Rows.SplitTags)
}

// A split_tags link whose split was itself skipped (its parent is a
// Smart transaction) is dropped, even though the tag itself exists.
func Test_import_drops_a_split_tag_link_when_its_split_was_skipped(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	tagPK := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	smartPK := b.Transaction(v9fixture.TransactionRow{
		Entity: v9fixture.EntSmartCashFlowTransaction, Account: acctPK, Amount: "1.00", PostedDate: &posted,
	})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: smartPK, Amount: "1.00"})
	b.LinkUserTag(entryPK, tagPK)
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Empty(t, fake.Rows.SplitTags)
}

// A split at Z_PK 0 exists, so a NULL split end read as 0 would link to it.
func Test_import_drops_a_split_tag_link_with_no_split(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	tagPK := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00"})
	b.LinkUserTag(entryPK, tagPK)
	b.LinkUserTag(0, tagPK)
	bundle := b.WriteBundle(t, t.TempDir())
	execOn(t, bundle.DataPath, "INSERT INTO ZCASHFLOWTRANSACTIONENTRY (Z_PK, ZPARENT, ZAMOUNT, ZQUICKENID) VALUES (0, ?, 0, 0)", txnPK)
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Equal(t, []store.SplitTag{{SplitID: "split-" + itoa(entryPK), TagID: "tag-" + itoa(tagPK)}}, fake.Rows.SplitTags)
}

// A tag at Z_PK 0 exists, so a NULL tag end read as 0 would link to it.
func Test_import_drops_a_split_tag_link_with_no_tag(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	tagPK := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00"})
	b.LinkUserTag(entryPK, tagPK)
	b.LinkUserTag(entryPK, 0)
	bundle := b.WriteBundle(t, t.TempDir())
	execOn(t, bundle.DataPath, "INSERT INTO ZTAG (Z_PK, Z_ENT, ZNAME) VALUES (0, ?, 'Zero')", v9fixture.EntUserTag)
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Equal(t, []store.SplitTag{{SplitID: "split-" + itoa(entryPK), TagID: "tag-" + itoa(tagPK)}}, fake.Rows.SplitTags)
}
