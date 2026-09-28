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

// The entity number is overridden so it must be resolved by name.
func Test_import_counts_investment_transactions_not_imported(t *testing.T) {
	const investmentEnt = 9004
	b := v9fixture.NewBuilder().WithEntity("InvestmentTransaction", investmentEnt)
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	deletedAccountPK := b.Account(v9fixture.AccountRow{Name: "Old Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Deleted: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	b.Transaction(v9fixture.TransactionRow{Entity: investmentEnt, Account: brokeragePK, Amount: "-400.00", PostedDate: &day})
	b.Transaction(v9fixture.TransactionRow{Entity: investmentEnt, Account: brokeragePK, Amount: "12.00", PostedDate: &day})
	b.Transaction(v9fixture.TransactionRow{Entity: investmentEnt, Account: brokeragePK, Amount: "5.00", PostedDate: &day, Deleted: true})
	b.Transaction(v9fixture.TransactionRow{Entity: investmentEnt, Account: deletedAccountPK, Amount: "7.00", PostedDate: &day})
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Equal(t, store.NotImported{InvestmentTransactions: 2}, result.NotImported)
}

func Test_import_stores_no_splits_for_investment_transactions(t *testing.T) {
	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	buyTxn := b.Transaction(v9fixture.TransactionRow{
		Entity: v9fixture.EntInvestmentTransaction, Account: brokeragePK, Amount: "-400.00", PostedDate: &day,
	})
	b.Entry(v9fixture.EntryRow{Parent: buyTxn, Amount: "-400.00"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Empty(t, fake.Rows.Transactions)
	assert.Empty(t, fake.Rows.Splits)
}

func Test_import_counts_no_investment_transactions_without_the_entity(t *testing.T) {
	b := v9fixture.NewBuilder().WithoutEntity("InvestmentTransaction")
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	b.Transaction(v9fixture.TransactionRow{
		Entity: v9fixture.EntInvestmentTransaction, Account: brokeragePK, Amount: "-400.00", PostedDate: &day,
	})
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Equal(t, store.NotImported{}, result.NotImported)
}

// Entity 0 is the value an absent entity's map lookup yields; the imported
// transaction carrying it must not be counted as an investment.
func Test_import_counts_no_investment_transactions_when_an_imported_one_shares_the_absent_entitys_zero(t *testing.T) {
	b := v9fixture.NewBuilder().WithoutEntity("InvestmentTransaction").WithEntity("CashFlowTransaction", 0)
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "10.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "10.00"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	require.Len(t, fake.Rows.Transactions, 1)
	assert.Equal(t, store.NotImported{}, result.NotImported)
}

// The balance check fails, so this is the unbuilt result a V1 block renders.
func Test_import_reports_investment_transactions_not_imported_when_a_check_fails(t *testing.T) {
	b := v9fixture.NewBuilder()
	chequingPK := chequingWithOneReconciledTxn(b, "100.00")
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: chequingPK, EndDate: &feb, EndingBalance: "100.01"})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	b.Transaction(v9fixture.TransactionRow{
		Entity: v9fixture.EntInvestmentTransaction, Account: brokeragePK, Amount: "-400.00", PostedDate: &feb,
	})
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.ErrorIs(t, err, store.ErrValidationFailed)
	assert.Equal(t, store.NotImported{InvestmentTransactions: 1}, result.NotImported)
}
