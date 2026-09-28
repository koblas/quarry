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

// A statement balance one cent off the reconciled sum must refuse the
// import and never call Replace: the store must not be replaced when a
// check fails.
func Test_import_does_not_replace_the_store_when_a_check_fails(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.01"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), bundle.DataPath)

	require.ErrorIs(t, err, store.ErrValidationFailed)
	assert.Equal(t, 0, fake.replaceCalls)
}

// Control for the guard above: an exact match still replaces the store.
func Test_import_replaces_the_store_when_the_balance_check_passes(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.00"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
	assert.Equal(t, 1, fake.replaceCalls)
}

// A reconciled sum one cent short of its statement is reported with the
// exact quarry/quicken/difference cents involved.
func Test_import_reports_an_account_whose_reconciled_sum_differs_from_its_statement(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.01"})
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	require.ErrorIs(t, err, store.ErrValidationFailed)
	require.Len(t, result.Validation.Balances.Mismatched, 1)
	mismatch := result.Validation.Balances.Mismatched[0]
	assert.Equal(t, int64(10000), mismatch.Quarry)
	assert.Equal(t, int64(10001), mismatch.Quicken)
	assert.Equal(t, int64(-1), mismatch.Difference)
}

// A non-reconciled transaction must not count toward the reconciled sum:
// with it excluded the account still matches its statement.
func Test_import_excludes_non_reconciled_transactions_from_the_balance_sum(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)
	reconciledTxnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "100.00", PostedDate: &posted, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: reconciledTxnPK, Amount: "100.00"})
	unclearedTxnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "50.00", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: unclearedTxnPK, Amount: "50.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &posted, EndingBalance: "100.00"})
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
	assert.Empty(t, result.Validation.Balances.Mismatched)
}

// An investment account is counted but never appears in the
// never-reconciled list, even though it has no statement of its own.
func Test_import_counts_an_investment_account_without_listing_it_as_never_reconciled(t *testing.T) {
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
	assert.Equal(t, 1, result.Validation.Balances.InvestmentAccounts)
	require.Len(t, result.Validation.Balances.NeverReconciled, 1)
	assert.Equal(t, "Savings", result.Validation.Balances.NeverReconciled[0].Name)
}

// Both a closed and an open-inactive account are checked like any other
// reconciled account: their closed/active flags are preserved and neither
// is skipped from the balance gate.
func Test_import_checks_closed_and_inactive_accounts_like_any_other(t *testing.T) {
	b := v9fixture.NewBuilder()
	closedPK := b.Account(v9fixture.AccountRow{Name: "Closed Card", Type: "CREDITCARD", Currency: "CAD", Closed: true})
	inactivePK := b.Account(v9fixture.AccountRow{Name: "Old Wallet", Type: "SAVINGS", Currency: "CAD"})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)
	closedTxnPK := b.Transaction(v9fixture.TransactionRow{Account: closedPK, Amount: "50.00", PostedDate: &posted, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: closedTxnPK, Amount: "50.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: closedPK, EndDate: &posted, EndingBalance: "50.00"})
	inactiveTxnPK := b.Transaction(v9fixture.TransactionRow{Account: inactivePK, Amount: "20.00", PostedDate: &posted, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: inactiveTxnPK, Amount: "20.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: inactivePK, EndDate: &posted, EndingBalance: "20.00"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
	assert.Equal(t, 2, result.Validation.Balances.Checked)
	assert.Empty(t, result.Validation.Balances.Mismatched)
	require.Len(t, fake.Rows.Accounts, 2)
	for _, acct := range fake.Rows.Accounts {
		switch acct.Name {
		case "Closed Card":
			assert.True(t, acct.Closed)
		case "Old Wallet":
			assert.False(t, acct.Active)
		}
	}
}

// A transaction's splits one cent short of its amount is reported with the
// exact amount/splits_total cents involved, and its payee's name.
func Test_import_reports_a_transaction_whose_splits_do_not_sum_to_its_amount(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	payeePK := b.Payee(v9fixture.PayeeRow{Name: "Costco"})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "100.00", PostedDate: &posted, Payee: payeePK})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "99.99"})
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	require.ErrorIs(t, err, store.ErrValidationFailed)
	require.Len(t, result.Validation.Splits.Mismatched, 1)
	mismatch := result.Validation.Splits.Mismatched[0]
	assert.Equal(t, int64(10000), mismatch.Amount)
	assert.Equal(t, int64(9999), mismatch.SplitsTotal)
	assert.Equal(t, "Costco", mismatch.Payee)
}

// A transaction with zero splits fails the same way a mis-summed one does,
// even when its own amount happens to be nonzero.
func Test_import_reports_a_transaction_with_no_splits(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "100.00", PostedDate: &posted})
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	require.ErrorIs(t, err, store.ErrValidationFailed)
	require.Len(t, result.Validation.Splits.Mismatched, 1)
	assert.Equal(t, int64(0), result.Validation.Splits.Mismatched[0].SplitsTotal)
}
