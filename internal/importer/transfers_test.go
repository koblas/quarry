package importer_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func transferIDFor(pk int64) string { return fmt.Sprintf("xfer-%d", pk) }

func transactionAccount(fake *fakeStore, txnID string) string {
	for _, txn := range fake.Rows.Transactions {
		if txn.ID == txnID {
			return txn.AccountID
		}
	}
	return ""
}

func Test_import_pairs_cross_currency_and_brokerage_transfers(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	cadPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	usdPK := b.Account(v9fixture.AccountRow{Name: "US Savings", Type: "SAVINGS", Currency: "USD", Active: true})
	brokeragePK := newBrokerage(b)
	cadLeg := transferLeg(b, cadPK, "-100.00", 1001, "2002")
	usdLeg := transferLeg(b, usdPK, "73.50", 2002, "1001")
	contributionLeg := transferLeg(b, cadPK, "-500.00", 3003, "4004")
	depositLeg := transferLeg(b, brokeragePK, "500.00", 4004, "3003")

	fake, result := importOK(t, b)

	split := func(pk int64) string { return fmt.Sprintf("split-%d", pk) }
	assert.Equal(t, []store.Transfer{
		{ID: fmt.Sprintf("xfer-%d", cadLeg), FromSplitID: split(cadLeg), ToSplitID: new(split(usdLeg)), CrossCurrency: true},
		{ID: fmt.Sprintf("xfer-%d", contributionLeg), FromSplitID: split(contributionLeg), ToSplitID: new(split(depositLeg))},
	}, fake.Rows.Transfers)
	assert.Equal(t, store.TransferCheck{Paired: 2, CrossCurrency: 1}, result.Validation.Transfers)
	assert.Equal(t, int64(-10000), splitByID(fake, split(cadLeg)).Amount)
	assert.Equal(t, int64(7350), splitByID(fake, split(usdLeg)).Amount)
	deposit := splitByID(fake, split(depositLeg))
	assert.Equal(t, fmt.Sprintf("acct-%d", brokeragePK), transactionAccount(fake, deposit.TransactionID))
}

// Entries 5-8 are fillers so the last pair's legs are source ids 9 and 10.
func Test_import_stores_each_split_in_at_most_one_transfer(t *testing.T) {
	t.Parallel()
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

	fake, _ := importOK(t, b)

	assert.Equal(t, []store.Transfer{
		{ID: transferIDFor(legA), FromSplitID: splitIDFor(legA), ToSplitID: new(splitIDFor(legB))},
		{ID: transferIDFor(legC), FromSplitID: splitIDFor(legC)},
		{ID: transferIDFor(selfLeg), FromSplitID: splitIDFor(selfLeg)},
		{ID: "xfer-9", FromSplitID: "split-9", ToSplitID: new("split-10")},
	}, fake.Rows.Transfers)
}

func Test_import_keys_a_pair_by_its_lower_leg_when_only_the_higher_leg_links(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	unlinkedLeg := transferLeg(b, chequingPK, "-10.00", 101, "")
	linkingLeg := transferLeg(b, savingsPK, "10.00", 102, "101")

	fake, _ := importOK(t, b)

	assert.Equal(t, []store.Transfer{
		{ID: transferIDFor(unlinkedLeg), FromSplitID: splitIDFor(unlinkedLeg), ToSplitID: new(splitIDFor(linkingLeg))},
	}, fake.Rows.Transfers)
}

func Test_import_links_each_paired_leg_to_its_counterparts_account(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	outLeg := transferLeg(b, chequingPK, "-10.00", 101, "102")
	inLeg := transferLeg(b, savingsPK, "10.00", 102, "101")

	fake, _ := importOK(t, b)

	assert.Equal(t, new(accountIDFor(savingsPK)), splitByID(fake, splitIDFor(outLeg)).TransferAccountID)
	assert.Equal(t, new(accountIDFor(chequingPK)), splitByID(fake, splitIDFor(inLeg)).TransferAccountID)
}

// Two accounts share the name "Savings"; the lower source id is the match.
func Test_import_keeps_a_name_form_leg_as_a_one_sided_transfer(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "USD", Active: true})
	matchedLeg := transferLeg(b, chequingPK, "-10.00", 101, "Savings")
	unmatchedLeg := transferLeg(b, chequingPK, "-5.00", 102, "Old Visa")

	fake, result := importOK(t, b)

	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, store.TransferCheck{OneSided: []store.OneSidedTransfer{
		{
			ID: transferIDFor(matchedLeg), SourceID: matchedLeg, Date: day, Account: "Chequing", Currency: "CAD", Active: true,
			Amount: -1000, OtherAccount: new("Savings"), OtherAccountID: new(accountIDFor(savingsPK)),
		},
		{
			ID: transferIDFor(unmatchedLeg), SourceID: unmatchedLeg, Date: day, Account: "Chequing", Currency: "CAD", Active: true,
			Amount: -500, OtherAccount: new("Old Visa"),
		},
	}}, result.Validation.Transfers)
	assert.Equal(t, new(accountIDFor(savingsPK)), splitByID(fake, splitIDFor(matchedLeg)).TransferAccountID)
	assert.Nil(t, splitByID(fake, splitIDFor(unmatchedLeg)).TransferAccountID)
	assert.Equal(t, []store.Transfer{
		{ID: transferIDFor(matchedLeg), FromSplitID: splitIDFor(matchedLeg), OtherAccount: new("Savings")},
		{ID: transferIDFor(unmatchedLeg), FromSplitID: splitIDFor(unmatchedLeg), OtherAccount: new("Old Visa")},
	}, fake.Rows.Transfers)
}

func Test_import_keeps_a_numeric_link_with_no_imported_counterpart_as_one_sided(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	missingLeg := transferLeg(b, chequingPK, "-1.00", 101, "999")
	deletedLinkLeg := transferLeg(b, chequingPK, "-2.00", 102, "555")
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	deletedTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-2.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: deletedTxn, Amount: "2.00", QuickenID: 555, Deleted: true})
	b.Entry(v9fixture.EntryRow{Parent: deletedTxn, Amount: "-2.00", QuickenID: 556})

	fake, result := importOK(t, b)

	chequing := func(id, amount int64) store.OneSidedTransfer {
		return store.OneSidedTransfer{
			ID: transferIDFor(id), SourceID: id, Date: day, Account: "Chequing", Currency: "CAD", Active: true, Amount: amount,
		}
	}
	assert.Equal(t, store.TransferCheck{OneSided: []store.OneSidedTransfer{
		chequing(missingLeg, -100), chequing(deletedLinkLeg, -200),
	}}, result.Validation.Transfers)
	assert.Equal(t, []store.Transfer{
		{ID: transferIDFor(missingLeg), FromSplitID: splitIDFor(missingLeg)},
		{ID: transferIDFor(deletedLinkLeg), FromSplitID: splitIDFor(deletedLinkLeg)},
	}, fake.Rows.Transfers)
}

func Test_import_keeps_a_link_to_a_skipped_split_as_one_sided(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	leg := transferLeg(b, chequingPK, "-12.34", 101, "777")
	b.Entry(v9fixture.EntryRow{Amount: "12.34", QuickenID: 777})

	_, result := importOK(t, b)

	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, store.TransferCheck{OneSided: []store.OneSidedTransfer{{
		ID: transferIDFor(leg), SourceID: leg, Date: day, Account: "Chequing", Currency: "CAD", Active: true, Amount: -1234,
	}}}, result.Validation.Transfers)
}

func Test_import_builds_the_store_with_a_one_sided_transfer(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	transferLeg(b, chequingPK, "-10.00", 101, "Old Visa")

	fake, result := importOK(t, b)

	assert.True(t, result.Built)
	assert.Equal(t, 1, fake.replaceCalls)
	assert.Equal(t, 1, result.Counts.Transfers)
}

// The balance check fails, so this is the unbuilt result a failed-validation block renders.
func Test_import_reports_transfers_when_a_check_fails(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	chequingPK := chequingWithOneReconciledTxn(b, "100.00")
	day := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: chequingPK, EndDate: &day, EndingBalance: "100.01"})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "USD", Active: true})
	transferLeg(b, chequingPK, "-10.00", 101, "102")
	transferLeg(b, savingsPK, "7.00", 102, "101")

	result, _ := importFailingValidation(t, b)

	assert.Equal(t, 1, result.Counts.Transfers)
	assert.Equal(t, store.TransferCheck{Paired: 1, CrossCurrency: 1}, result.Validation.Transfers)
}

func Test_import_describes_a_one_sided_leg_by_its_transaction_account_and_own_split(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	closedPK := b.Account(v9fixture.AccountRow{Name: "Old Chequing", Type: "CHECKING", Currency: "USD", Closed: true})
	landlordPK := b.Payee(v9fixture.PayeeRow{Name: "Landlord"})
	day := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	txn := b.Transaction(v9fixture.TransactionRow{Account: closedPK, Amount: "-30.00", PostedDate: &day, Payee: landlordPK})
	b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-10.00"})
	leg := b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "-20.00", QuickenID: 101, Transfer: "Old Visa"})

	_, result := importOK(t, b)

	assert.Equal(t, []store.OneSidedTransfer{{
		ID: transferIDFor(leg), SourceID: leg, Date: day, Account: "Old Chequing", Currency: "USD", Closed: true,
		Payee: "Landlord", Amount: -2000, OtherAccount: new("Old Visa"),
	}}, result.Validation.Transfers.OneSided)
}

// Entries 1-8 put the last transaction's two legs at split source ids 9 and
// 10; Zeta's source id is lower than Alpha's, the reverse of name order.
func Test_import_orders_one_sided_transfers_by_date_account_and_transaction(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	zetaPK := b.Account(v9fixture.AccountRow{Name: "Zeta", Type: "CHECKING", Currency: "CAD", Active: true})
	alphaPK := b.Account(v9fixture.AccountRow{Name: "Alpha", Type: "CHECKING", Currency: "CAD", Active: true})
	early := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	lateTxn := b.Transaction(v9fixture.TransactionRow{Account: alphaPK, Amount: "-1.00", PostedDate: &late})
	lateLeg := b.Entry(v9fixture.EntryRow{Parent: lateTxn, Amount: "-1.00", QuickenID: 101, Transfer: "Old Visa"})
	zetaTxn := b.Transaction(v9fixture.TransactionRow{Account: zetaPK, Amount: "-2.00", PostedDate: &early})
	zetaLeg := b.Entry(v9fixture.EntryRow{Parent: zetaTxn, Amount: "-2.00", QuickenID: 102, Transfer: "Old Visa"})
	fillerTxn := b.Transaction(v9fixture.TransactionRow{Account: alphaPK, Amount: "6.00", PostedDate: &early})
	for range 6 {
		b.Entry(v9fixture.EntryRow{Parent: fillerTxn, Amount: "1.00", QuickenID: 900})
	}
	alphaTxn := b.Transaction(v9fixture.TransactionRow{Account: alphaPK, Amount: "-7.00", PostedDate: &early})
	leg9 := b.Entry(v9fixture.EntryRow{Parent: alphaTxn, Amount: "-3.00", QuickenID: 109, Transfer: "Old Visa"})
	leg10 := b.Entry(v9fixture.EntryRow{Parent: alphaTxn, Amount: "-4.00", QuickenID: 110, Transfer: "Old Visa"})
	require.Equal(t, []int64{9, 10}, []int64{leg9, leg10})

	_, result := importOK(t, b)

	got := make([]string, 0, len(result.Validation.Transfers.OneSided))
	for _, leg := range result.Validation.Transfers.OneSided {
		got = append(got, leg.ID)
	}
	assert.Equal(t, []string{transferIDFor(leg9), transferIDFor(leg10), transferIDFor(zetaLeg), transferIDFor(lateLeg)}, got)
}
