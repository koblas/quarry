package importer_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/importer"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string { return &s }

// Covers every table in one pass: an account with an institution, a
// two-way split transaction, a category, a payee and a tag on one split.
func Test_import_builds_every_table_from_a_v9_snapshot(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	bankPK := b.Institution(v9fixture.InstitutionRow{Name: "Big Bank"})
	acctPK := b.Account(v9fixture.AccountRow{
		Name: "Chequing", Type: "CHECKING", Currency: "CAD", Institution: bankPK, Active: true,
	})
	payeePK := b.Payee(v9fixture.PayeeRow{Name: "Coffee Shop"})
	catPK := b.Category(v9fixture.TagRow{Name: "Groceries", Type: v9fixture.Int64Ptr(1)})
	tagPK := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})

	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{
		Account: acctPK, Amount: "12.34", PostedDate: &posted, Payee: payeePK,
	})
	entry1PK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "7.00", CategoryTag: catPK})
	entry2PK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "5.34"})
	b.LinkUserTag(entry1PK, tagPK)

	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{Path: "/store/quarry.duckdb"}
	srv := importer.NewServer(importer.WithStore(fake))

	result, err := srv.Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Equal(t, fake.Path, result.Path)
	assert.Equal(t, store.Counts{
		Accounts: 1, Categories: 1, Payees: 1, Tags: 1, Transactions: 1, Splits: 2, SplitTags: 1,
	}, result.Counts)

	acctID := fmt.Sprintf("acct-%d", acctPK)
	catID := fmt.Sprintf("cat-%d", catPK)
	payeeID := fmt.Sprintf("payee-%d", payeePK)
	tagID := fmt.Sprintf("tag-%d", tagPK)
	txnID := fmt.Sprintf("txn-%d", txnPK)
	split1ID := fmt.Sprintf("split-%d", entry1PK)
	split2ID := fmt.Sprintf("split-%d", entry2PK)

	assert.Equal(t, []store.Account{{
		ID: acctID, SourceID: acctPK, Name: "Chequing", Type: "chequing", Currency: "CAD",
		Institution: strPtr("Big Bank"), Closed: false, Active: true,
	}}, fake.Rows.Accounts)

	assert.Equal(t, []store.Category{{
		ID: catID, SourceID: catPK, Name: "Groceries", FullPath: "Groceries", Kind: "expense", Hidden: false,
	}}, fake.Rows.Categories)

	assert.Equal(t, []store.Payee{{ID: payeeID, SourceID: payeePK, Name: "Coffee Shop"}}, fake.Rows.Payees)

	assert.Equal(t, []store.Tag{{ID: tagID, SourceID: tagPK, Name: "Reimbursable"}}, fake.Rows.Tags)

	assert.Equal(t, []store.Transaction{{
		ID: txnID, SourceID: txnPK, AccountID: acctID, Date: posted,
		PayeeID: strPtr(payeeID), Amount: 1234, Currency: "CAD", Status: "uncleared",
	}}, fake.Rows.Transactions)

	assert.ElementsMatch(t, []store.Split{
		{ID: split1ID, SourceID: entry1PK, TransactionID: txnID, CategoryID: strPtr(catID), Amount: 700},
		{ID: split2ID, SourceID: entry2PK, TransactionID: txnID, Amount: 534},
	}, fake.Rows.Splits)

	assert.Equal(t, []store.SplitTag{{SplitID: split1ID, TagID: tagID}}, fake.Rows.SplitTags)
}

// Ids must derive only from each row's own Z_PK, never from anything a
// second build could compute differently for the same row.
func Test_import_twice_from_the_same_snapshot_keeps_every_id(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00"})
	bundle := b.WriteBundle(t, t.TempDir())

	fake1 := &fakeStore{}
	_, err := importer.NewServer(importer.WithStore(fake1)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})
	require.NoError(t, err)

	fake2 := &fakeStore{}
	_, err = importer.NewServer(importer.WithStore(fake2)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})
	require.NoError(t, err)

	wantAcctID := fmt.Sprintf("acct-%d", acctPK)
	wantTxnID := fmt.Sprintf("txn-%d", txnPK)
	require.Len(t, fake1.Rows.Accounts, 1)
	require.Len(t, fake2.Rows.Accounts, 1)
	assert.Equal(t, wantAcctID, fake1.Rows.Accounts[0].ID)
	assert.Equal(t, wantAcctID, fake2.Rows.Accounts[0].ID)
	require.Len(t, fake1.Rows.Transactions, 1)
	require.Len(t, fake2.Rows.Transactions, 1)
	assert.Equal(t, wantTxnID, fake1.Rows.Transactions[0].ID)
	assert.Equal(t, wantTxnID, fake2.Rows.Transactions[0].ID)
}

// Each category keeps its own parent_id, full_path, kind and hidden flag,
// across income, expense and system kinds and a nested parent/child pair.
func Test_import_keeps_each_categorys_parent_path_kind_and_hidden(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	incomePK := b.Category(v9fixture.TagRow{Name: "Salary", Type: v9fixture.Int64Ptr(2)})
	systemPK := b.Category(v9fixture.TagRow{Name: "Transfer", Type: v9fixture.Int64Ptr(0)})
	parentPK := b.Category(v9fixture.TagRow{Name: "Food", Type: v9fixture.Int64Ptr(1)})
	childPK := b.Category(v9fixture.TagRow{
		Name: "Groceries", Type: v9fixture.Int64Ptr(1), ParentCategory: parentPK, Hidden: true,
	})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	parentID := fmt.Sprintf("cat-%d", parentPK)
	assert.ElementsMatch(t, []store.Category{
		{ID: fmt.Sprintf("cat-%d", incomePK), SourceID: incomePK, Name: "Salary", FullPath: "Salary", Kind: "income"},
		{ID: fmt.Sprintf("cat-%d", systemPK), SourceID: systemPK, Name: "Transfer", FullPath: "Transfer", Kind: "system"},
		{ID: parentID, SourceID: parentPK, Name: "Food", FullPath: "Food", Kind: "expense"},
		{
			ID: fmt.Sprintf("cat-%d", childPK), SourceID: childPK, ParentID: strPtr(parentID),
			Name: "Groceries", FullPath: "Food:Groceries", Kind: "expense", Hidden: true,
		},
	}, fake.Rows.Categories)
}
