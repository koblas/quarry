package importer_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_import_refuses_a_split_with_more_than_2_decimal_places_when_its_transaction_is_valid(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newVisa(b)
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.34", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "12.345"})

	reason, _ := importRefused(t, b)

	assert.Equal(t,
		`a split of a transaction on 2024-03-02 in "Visa Infinite" has an amount of 12.345, which has more than 2 decimal places`,
		reason)
}

func Test_import_refuses_a_split_with_an_amount_too_large_for_quarry(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newVisa(b)
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.34", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "10000000000000.5"})

	reason, _ := importRefused(t, b)

	assert.Equal(t,
		`a split of a transaction on 2024-03-02 in "Visa Infinite" has an amount of 10000000000000.5, which is too large for quarry's amounts`,
		reason)
}

func Test_import_refuses_a_split_with_no_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newVisa(b)
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.34", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK})

	reason, _ := importRefused(t, b)

	assert.Equal(t, `a split of a transaction on 2024-03-02 in "Visa Infinite" has no amount`, reason)
}

func Test_import_skips_a_split_with_no_parent_transaction(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Entry(v9fixture.EntryRow{Amount: "0.00"})

	fake, _ := importOK(t, b)

	assert.Empty(t, fake.Rows.Splits)
}

func Test_import_skips_a_split_with_no_parent_whatever_its_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.34", PostedDate: &posted})
	keptPK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "12.34"})
	b.Entry(v9fixture.EntryRow{Amount: "12.34"})

	fake, _ := importOK(t, b)

	require.Len(t, fake.Rows.Splits, 1)
	assert.Equal(t, "split-"+itoa(keptPK), fake.Rows.Splits[0].ID)
}

func Test_import_refuses_a_split_with_a_text_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newVisa(b)
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.34", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "not-a-number"})

	reason, _ := importRefused(t, b)

	assert.Equal(t, `a split of a transaction on 2024-03-02 in "Visa Infinite" has an amount that is not a number`, reason)
}

func Test_import_refuses_a_split_with_a_blob_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newVisa(b)
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.34", PostedDate: &posted})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "0.00"})
	dataPath := snapshotPath(t, b)
	setColumnBlob(t, dataPath, "ZCASHFLOWTRANSACTIONENTRY", "ZAMOUNT", entryPK, []byte{0x01, 0x02})

	reason, _ := importRefusedFrom(t, dataPath)

	assert.Equal(t, `a split of a transaction on 2024-03-02 in "Visa Infinite" has an amount that is not a number`, reason)
}

// An entry whose parent is a SmartCashFlowTransaction is dropped
// silently, the same as one whose parent was deleted or excluded.
func Test_import_skips_an_entry_whose_parent_is_a_smart_transaction(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	smartPK := b.Transaction(v9fixture.TransactionRow{
		Entity: v9fixture.EntSmartCashFlowTransaction, Account: acctPK, Amount: "1.00", PostedDate: &posted,
	})
	b.Entry(v9fixture.EntryRow{Parent: smartPK, Amount: "1.00"})

	fake, _ := importOK(t, b)

	assert.Empty(t, fake.Rows.Splits)
}

func Test_import_skips_a_split_whose_parent_transaction_does_not_exist(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Entry(v9fixture.EntryRow{Parent: 999, Amount: "12.34"})

	fake, _ := importOK(t, b)

	assert.Empty(t, fake.Rows.Splits)
}

// A split's category reference to a deleted category stores NULL.
func Test_import_nulls_a_splits_category_when_the_category_is_deleted(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	deletedCatPK := b.Category(v9fixture.TagRow{Name: "Old", Type: new(int64(1)), Deleted: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00", CategoryTag: deletedCatPK})

	fake, _ := importOK(t, b)

	require.Len(t, fake.Rows.Splits, 1)
	assert.Nil(t, fake.Rows.Splits[0].CategoryID)
}

// A split's category reference to no category row at all stores NULL,
// the same as a deleted one.
func Test_import_nulls_a_splits_category_when_the_category_does_not_exist(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00", CategoryTag: 999})

	fake, _ := importOK(t, b)

	require.Len(t, fake.Rows.Splits, 1)
	assert.Nil(t, fake.Rows.Splits[0].CategoryID)
}

// A split_tags link to a deleted tag is dropped, not stored with a
// dangling tag_id.
func Test_import_drops_a_split_tag_link_when_the_tag_is_deleted(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	deletedTagPK := b.UserTag(v9fixture.TagRow{Name: "Old", Deleted: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00"})
	b.LinkUserTag(entryPK, deletedTagPK)

	fake, _ := importOK(t, b)

	assert.Empty(t, fake.Rows.SplitTags)
}

// A split_tags link whose split was itself skipped (its parent is a
// Smart transaction) is dropped, even though the tag itself exists.
func Test_import_drops_a_split_tag_link_when_its_split_was_skipped(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	tagPK := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	smartPK := b.Transaction(v9fixture.TransactionRow{
		Entity: v9fixture.EntSmartCashFlowTransaction, Account: acctPK, Amount: "1.00", PostedDate: &posted,
	})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: smartPK, Amount: "1.00"})
	b.LinkUserTag(entryPK, tagPK)

	fake, _ := importOK(t, b)

	assert.Empty(t, fake.Rows.SplitTags)
}

// A split at Z_PK 0 exists, so a NULL split end read as 0 would link to it.
func Test_import_drops_a_split_tag_link_with_no_split(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	tagPK := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00"})
	b.LinkUserTag(entryPK, tagPK)
	b.LinkUserTag(0, tagPK)
	dataPath := snapshotPath(t, b)
	execOn(t, dataPath, "INSERT INTO ZCASHFLOWTRANSACTIONENTRY (Z_PK, ZPARENT, ZAMOUNT, ZQUICKENID) VALUES (0, ?, 0, 0)", txnPK)

	fake, _ := importOKFrom(t, dataPath)

	assert.Equal(t, []store.SplitTag{{SplitID: "split-" + itoa(entryPK), TagID: "tag-" + itoa(tagPK)}}, fake.Rows.SplitTags)
}

// A tag at Z_PK 0 exists, so a NULL tag end read as 0 would link to it.
func Test_import_drops_a_split_tag_link_with_no_tag(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	tagPK := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00"})
	b.LinkUserTag(entryPK, tagPK)
	b.LinkUserTag(entryPK, 0)
	dataPath := snapshotPath(t, b)
	execOn(t, dataPath, "INSERT INTO ZTAG (Z_PK, Z_ENT, ZNAME) VALUES (0, ?, 'Zero')", v9fixture.EntUserTag)

	fake, _ := importOKFrom(t, dataPath)

	assert.Equal(t, []store.SplitTag{{SplitID: "split-" + itoa(entryPK), TagID: "tag-" + itoa(tagPK)}}, fake.Rows.SplitTags)
}

func Test_import_sets_split_memo_when_present(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "1.00", Note: "split memo"})

	fake, _ := importOK(t, b)

	require.Len(t, fake.Rows.Splits, 1)
	require.NotNil(t, fake.Rows.Splits[0].Memo)
	assert.Equal(t, "split memo", *fake.Rows.Splits[0].Memo)
}

func Test_import_skips_a_split_tag_link_whose_split_has_no_parent_transaction(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	tagPK := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})
	entryPK := b.Entry(v9fixture.EntryRow{Amount: "1.00"})
	b.LinkUserTag(entryPK, tagPK)

	fake, _ := importOK(t, b)

	assert.Empty(t, fake.Rows.SplitTags)
}
