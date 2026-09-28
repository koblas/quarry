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

// transferLeg adds a one-entry transaction in account whose entry carries
// quickenID and the ZTRANSFER text link, returning the entry's Z_PK.
func transferLeg(b *v9fixture.Builder, account int64, amount string, quickenID int64, link string) int64 {
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	txn := b.Transaction(v9fixture.TransactionRow{Account: account, Amount: amount, PostedDate: &day})
	return b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount, QuickenID: quickenID, Transfer: link})
}

func splitIDFor(pk int64) string { return fmt.Sprintf("split-%d", pk) }

func transferIDFor(pk int64) string { return fmt.Sprintf("xfer-%d", pk) }

func accountIDFor(pk int64) string { return fmt.Sprintf("acct-%d", pk) }

func splitByID(fake *fakeStore, id string) store.Split {
	for _, s := range fake.Rows.Splits {
		if s.ID == id {
			return s
		}
	}
	return store.Split{}
}

func transactionAccount(fake *fakeStore, txnID string) string {
	for _, txn := range fake.Rows.Transactions {
		if txn.ID == txnID {
			return txn.AccountID
		}
	}
	return ""
}

func Test_import_pairs_cross_currency_and_brokerage_transfers(t *testing.T) {
	b := v9fixture.NewBuilder()
	cadPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	usdPK := b.Account(v9fixture.AccountRow{Name: "US Savings", Type: "SAVINGS", Currency: "USD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	cadLeg := transferLeg(b, cadPK, "-100.00", 1001, "2002")
	usdLeg := transferLeg(b, usdPK, "73.50", 2002, "1001")
	contributionLeg := transferLeg(b, cadPK, "-500.00", 3003, "4004")
	depositLeg := transferLeg(b, brokeragePK, "500.00", 4004, "3003")
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
	split := func(pk int64) string { return fmt.Sprintf("split-%d", pk) }
	assert.Equal(t, []store.Transfer{
		{ID: fmt.Sprintf("xfer-%d", cadLeg), FromSplitID: split(cadLeg), ToSplitID: strPtr(split(usdLeg)), CrossCurrency: true},
		{ID: fmt.Sprintf("xfer-%d", contributionLeg), FromSplitID: split(contributionLeg), ToSplitID: strPtr(split(depositLeg))},
	}, fake.Rows.Transfers)
	assert.Equal(t, store.TransferCheck{Paired: 2, CrossCurrency: 1}, result.Validation.Transfers)
	assert.Equal(t, int64(-10000), splitByID(fake, split(cadLeg)).Amount)
	assert.Equal(t, int64(7350), splitByID(fake, split(usdLeg)).Amount)
	deposit := splitByID(fake, split(depositLeg))
	assert.Equal(t, fmt.Sprintf("acct-%d", brokeragePK), transactionAccount(fake, deposit.TransactionID))
}

// Entries 5-8 are fillers so the last pair's legs are source ids 9 and 10.
func Test_import_stores_each_split_in_at_most_one_transfer(t *testing.T) {
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	legA := transferLeg(b, chequingPK, "-10.00", 101, "102")
	legB := transferLeg(b, savingsPK, "10.00", 102, "103")
	legC := transferLeg(b, chequingPK, "-10.00", 103, "102")
	selfLeg := transferLeg(b, chequingPK, "-1.00", 104, "104")
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	fillerTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "4.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: fillerTxn, Amount: "1.00", QuickenID: 900})
	b.Entry(v9fixture.EntryRow{Parent: fillerTxn, Amount: "1.00", QuickenID: 900})
	b.Entry(v9fixture.EntryRow{Parent: fillerTxn, Amount: "1.00", QuickenID: 900})
	b.Entry(v9fixture.EntryRow{Parent: fillerTxn, Amount: "1.00", QuickenID: 900})
	leg9 := transferLeg(b, chequingPK, "-20.00", 109, "110")
	leg10 := transferLeg(b, savingsPK, "20.00", 110, "109")
	require.Equal(t, []int64{9, 10}, []int64{leg9, leg10})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
	assert.Equal(t, []store.Transfer{
		{ID: transferIDFor(legA), FromSplitID: splitIDFor(legA), ToSplitID: strPtr(splitIDFor(legB))},
		{ID: transferIDFor(legC), FromSplitID: splitIDFor(legC)},
		{ID: transferIDFor(selfLeg), FromSplitID: splitIDFor(selfLeg)},
		{ID: "xfer-9", FromSplitID: "split-9", ToSplitID: strPtr("split-10")},
	}, fake.Rows.Transfers)
}

func Test_import_keys_a_pair_by_its_lower_leg_when_only_the_higher_leg_links(t *testing.T) {
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	unlinkedLeg := transferLeg(b, chequingPK, "-10.00", 101, "")
	linkingLeg := transferLeg(b, savingsPK, "10.00", 102, "101")
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
	assert.Equal(t, []store.Transfer{
		{ID: transferIDFor(unlinkedLeg), FromSplitID: splitIDFor(unlinkedLeg), ToSplitID: strPtr(splitIDFor(linkingLeg))},
	}, fake.Rows.Transfers)
}

func Test_import_links_each_paired_leg_to_its_counterparts_account(t *testing.T) {
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	outLeg := transferLeg(b, chequingPK, "-10.00", 101, "102")
	inLeg := transferLeg(b, savingsPK, "10.00", 102, "101")
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
	assert.Equal(t, strPtr(accountIDFor(savingsPK)), splitByID(fake, splitIDFor(outLeg)).TransferAccountID)
	assert.Equal(t, strPtr(accountIDFor(chequingPK)), splitByID(fake, splitIDFor(inLeg)).TransferAccountID)
}

// Two accounts share the name "Savings"; the lower source id is the match.
func Test_import_keeps_a_name_form_leg_as_a_one_sided_transfer(t *testing.T) {
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "USD", Active: true})
	matchedLeg := transferLeg(b, chequingPK, "-10.00", 101, "Savings")
	unmatchedLeg := transferLeg(b, chequingPK, "-5.00", 102, "Old Visa")
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
	assert.Equal(t, store.TransferCheck{OneSided: []store.OneSidedTransfer{
		{ID: transferIDFor(matchedLeg), SourceID: matchedLeg, OtherAccount: strPtr("Savings"), OtherAccountID: strPtr(accountIDFor(savingsPK))},
		{ID: transferIDFor(unmatchedLeg), SourceID: unmatchedLeg, OtherAccount: strPtr("Old Visa")},
	}}, result.Validation.Transfers)
	assert.Equal(t, strPtr(accountIDFor(savingsPK)), splitByID(fake, splitIDFor(matchedLeg)).TransferAccountID)
	assert.Nil(t, splitByID(fake, splitIDFor(unmatchedLeg)).TransferAccountID)
}

func Test_import_keeps_a_numeric_link_with_no_imported_counterpart_as_one_sided(t *testing.T) {
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	missingLeg := transferLeg(b, chequingPK, "-1.00", 101, "999")
	deletedLinkLeg := transferLeg(b, chequingPK, "-2.00", 102, "555")
	investmentLinkLeg := transferLeg(b, chequingPK, "-3.00", 103, "666")
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	deletedTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-2.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: deletedTxn, Amount: "2.00", QuickenID: 555, Deleted: true})
	b.Entry(v9fixture.EntryRow{Parent: deletedTxn, Amount: "-2.00", QuickenID: 556})
	investmentTxn := b.Transaction(v9fixture.TransactionRow{
		Entity: v9fixture.EntInvestmentTransaction, Account: brokeragePK, Amount: "3.00", PostedDate: &day,
	})
	b.Entry(v9fixture.EntryRow{Parent: investmentTxn, Amount: "3.00", QuickenID: 666})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
	assert.Equal(t, store.TransferCheck{OneSided: []store.OneSidedTransfer{
		{ID: transferIDFor(missingLeg), SourceID: missingLeg},
		{ID: transferIDFor(deletedLinkLeg), SourceID: deletedLinkLeg},
		{ID: transferIDFor(investmentLinkLeg), SourceID: investmentLinkLeg},
	}}, result.Validation.Transfers)
	assert.Equal(t, []store.Transfer{
		{ID: transferIDFor(missingLeg), FromSplitID: splitIDFor(missingLeg)},
		{ID: transferIDFor(deletedLinkLeg), FromSplitID: splitIDFor(deletedLinkLeg)},
		{ID: transferIDFor(investmentLinkLeg), FromSplitID: splitIDFor(investmentLinkLeg)},
	}, fake.Rows.Transfers)
}

func Test_import_builds_the_store_with_a_one_sided_transfer(t *testing.T) {
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	transferLeg(b, chequingPK, "-10.00", 101, "Old Visa")
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
	assert.True(t, result.Built)
	assert.Equal(t, 1, fake.replaceCalls)
	assert.Equal(t, 1, result.Counts.Transfers)
}

// The balance check fails, so this is the unbuilt result a V1 block renders.
func Test_import_reports_transfers_when_a_check_fails(t *testing.T) {
	b := v9fixture.NewBuilder()
	chequingPK := chequingWithOneReconciledTxn(b, "100.00")
	day := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: chequingPK, EndDate: &day, EndingBalance: "100.01"})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "USD", Active: true})
	transferLeg(b, chequingPK, "-10.00", 101, "102")
	transferLeg(b, savingsPK, "7.00", 102, "101")
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	require.ErrorIs(t, err, store.ErrValidationFailed)
	assert.Equal(t, 1, result.Counts.Transfers)
	assert.Equal(t, store.TransferCheck{Paired: 1, CrossCurrency: 1}, result.Validation.Transfers)
}
