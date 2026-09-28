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

func Test_import_maps_cleared_and_reconciled_transaction_status(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	cleared := int64(1)
	reconciled := int64(2)
	clearedTxnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted, Status: &cleared})
	b.Entry(v9fixture.EntryRow{Parent: clearedTxnPK, Amount: "1.00"})
	reconciledTxnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "2.00", PostedDate: &posted, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: reconciledTxnPK, Amount: "2.00"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	statuses := make([]string, len(fake.Rows.Transactions))
	for i, txn := range fake.Rows.Transactions {
		statuses[i] = txn.Status
	}
	assert.ElementsMatch(t, []string{"cleared", "reconciled"}, statuses)
}

func Test_import_sets_transaction_memo_and_cheque_number_when_present(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted, Note: "Groceries", CheckNumber: "101"})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	require.Len(t, fake.Rows.Transactions, 1)
	require.NotNil(t, fake.Rows.Transactions[0].Memo)
	require.NotNil(t, fake.Rows.Transactions[0].ChequeNumber)
	assert.Equal(t, "Groceries", *fake.Rows.Transactions[0].Memo)
	assert.Equal(t, "101", *fake.Rows.Transactions[0].ChequeNumber)
}

func Test_import_sets_split_memo_when_present(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00", Note: "split memo"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	require.Len(t, fake.Rows.Splits, 1)
	require.NotNil(t, fake.Rows.Splits[0].Memo)
	assert.Equal(t, "split memo", *fake.Rows.Splits[0].Memo)
}

// A transaction whose account was itself excluded is not double-reported:
// only the account's own reason is reported.
func Test_import_skips_a_transaction_whose_account_was_itself_excluded(t *testing.T) {
	b := v9fixture.NewBuilder()
	badAcctPK := b.Account(v9fixture.AccountRow{Name: "Euro Savings", Type: "CHECKING", Currency: "EUR", Active: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	b.Transaction(v9fixture.TransactionRow{Account: badAcctPK, Amount: "1.00", PostedDate: &posted})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t,
		`account "Euro Savings" uses currency EUR; quarry supports CAD and USD accounts`,
		importReason(t, err))
}

// A Z_15USERTAGS link to an entry that was itself skipped (no transaction)
// must not crash mapSplitTags: the entry's own offender is reported.
func Test_import_skips_a_split_tag_link_whose_split_was_itself_skipped(t *testing.T) {
	b := v9fixture.NewBuilder()
	tagPK := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})
	entryPK := b.Entry(v9fixture.EntryRow{Amount: "1.00"})
	b.LinkUserTag(entryPK, tagPK)
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `a split (source id `+itoa(entryPK)+`) has no transaction`, importReason(t, err))
}

// A category referencing a nameless parent must still build a full_path,
// falling back to "(source id N)" for that ancestor.
func Test_import_full_path_falls_back_to_source_id_for_a_nameless_parent(t *testing.T) {
	b := v9fixture.NewBuilder()
	parentPK := b.Category(v9fixture.TagRow{Type: v9fixture.Int64Ptr(1)})
	b.Category(v9fixture.TagRow{Name: "Groceries", Type: v9fixture.Int64Ptr(1), ParentCategory: parentPK})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `category (source id `+itoa(parentPK)+`) has no name`, importReason(t, err))
}

// A cyclic ZPARENTCATEGORY chain must not loop: the walk is bounded by the
// number of categories read.
func Test_import_bounds_a_cyclic_category_parent_chain(t *testing.T) {
	b := v9fixture.NewBuilder()
	b.Category(v9fixture.TagRow{Name: "A", Type: v9fixture.Int64Ptr(1), ParentCategory: 2})
	b.Category(v9fixture.TagRow{Name: "B", Type: v9fixture.Int64Ptr(1), ParentCategory: 1})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Len(t, fake.Rows.Categories, 2)
}

// A higher-priority offender added after a lower-priority one (accounts
// are read before categories) must still be the one reported.
func Test_import_upgrades_the_reported_class_when_a_higher_priority_offender_is_added_later(t *testing.T) {
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Type: "CHECKING", Currency: "CAD", Active: true})
	b.Category(v9fixture.TagRow{Name: "Food:Groceries", Type: v9fixture.Int64Ptr(5)})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t,
		`category "Food:Groceries" has type 5, which quarry does not map yet`,
		importReason(t, err))
}

func Test_UnmappableError_Error_returns_the_reason(t *testing.T) {
	err := &importer.UnmappableError{Reason: "boom"}

	assert.Equal(t, "boom", err.Error())
}
