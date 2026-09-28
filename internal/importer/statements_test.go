package importer_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/importer"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func chequingWithOneReconciledTxn(b *v9fixture.Builder, cents string) int64 {
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: cents, PostedDate: &posted, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: cents})
	return acctPK
}

// The correct (later) record is inserted first, so a "last row scanned
// wins" bug would pick the wrong (earlier) one instead.
func Test_import_uses_the_newest_statement_by_date(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	jan := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &jan, EndingBalance: "999.00"})
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
	assert.Empty(t, result.Validation.Balances.Mismatched)
}

// The undated record is inserted first, so a "last row scanned wins" bug
// would pick the dated one instead and the import would succeed.
func Test_import_ranks_an_undated_statement_as_newest_over_a_dated_one(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	pk := b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndingBalance: "100.00"})
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.00"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	assert.Equal(t, `a statement for "Chequing" (source id `+itoa(pk)+`) has no date`, importReason(t, err))
}

// Same ZENDDATE on two records: the higher Z_PK's balance wins.
func Test_import_breaks_a_statement_date_tie_with_the_higher_source_id(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "999.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.00"})
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
	assert.Empty(t, result.Validation.Balances.Mismatched)
}

// A deleted record dated after the true newest must not be selected: its
// balance would mismatch if it were.
func Test_import_ignores_a_deleted_newer_statement(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	mar := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &mar, EndingBalance: "1.00", Deleted: true})
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
	assert.Empty(t, result.Validation.Balances.Mismatched)
}

// The newest (good) record is inserted first, so a "last row scanned
// wins" bug would read the older, bad one and refuse the import instead.
func Test_import_ignores_an_older_statements_bad_balance(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	jan := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &jan, EndingBalance: "12.345"})
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
	assert.Empty(t, result.Validation.Balances.Mismatched)
}

// A malformed reconcile record on an investment account must not refuse
// the import: investment accounts are counted, never checked.
func Test_import_skips_a_reconcile_record_on_an_investment_account(t *testing.T) {
	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: brokeragePK, EndDate: &feb, EndingBalance: "not-a-number"})
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
	assert.Equal(t, 1, result.Validation.Balances.InvestmentAccounts)
}

// A reconcile record on a deleted account must not refuse the import: the
// account it names was never mapped.
func Test_import_skips_a_reconcile_record_on_a_deleted_account(t *testing.T) {
	b := v9fixture.NewBuilder()
	deletedPK := b.Account(v9fixture.AccountRow{Name: "Old", Type: "CHECKING", Currency: "CAD", Deleted: true})
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: deletedPK, EndDate: &feb, EndingBalance: "not-a-number"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
}

// A reconcile record whose ZACCOUNT points to no row at all must not
// refuse the import.
func Test_import_skips_a_reconcile_record_on_a_missing_account(t *testing.T) {
	b := v9fixture.NewBuilder()
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: 999, EndDate: &feb, EndingBalance: "not-a-number"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	require.NoError(t, err)
}

func Test_import_refuses_a_statement_with_no_balance(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	assert.Equal(t, `the 2026-02-28 statement for "Chequing" has no balance`, importReason(t, err))
}

func Test_import_refuses_an_undated_statement(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	pk := b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndingBalance: "100.00"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	assert.Equal(t, `a statement for "Chequing" (source id `+itoa(pk)+`) has no date`, importReason(t, err))
}

func Test_import_refuses_a_statement_with_more_than_2_decimal_places(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "12.345"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	assert.Equal(t,
		`the 2026-02-28 statement for "Chequing" has a balance of 12.345, which has more than 2 decimal places`,
		importReason(t, err))
}

func Test_import_refuses_a_statement_with_a_balance_too_large_for_quarry(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "10000000000000.5"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	assert.Equal(t,
		`the 2026-02-28 statement for "Chequing" has a balance of 10000000000000.5, which is too large for quarry's amounts`,
		importReason(t, err))
}

func Test_import_refuses_a_statement_with_a_text_balance(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "not-a-number"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	assert.Equal(t, `the 2026-02-28 statement for "Chequing" has a balance that is not a number`, importReason(t, err))
}

func Test_import_refuses_an_undated_statement_with_a_bad_balance_using_the_source_id_subject(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	pk := b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndingBalance: "12.345"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	assert.Equal(t,
		`a statement for "Chequing" (source id `+itoa(pk)+`) has a balance of 12.345, which has more than 2 decimal places`,
		importReason(t, err))
}

// Same as the precision case above, for the too-large fault.
func Test_import_refuses_an_undated_statement_with_a_too_large_balance_using_the_source_id_subject(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	pk := b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndingBalance: "10000000000000.5"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	assert.Equal(t,
		`a statement for "Chequing" (source id `+itoa(pk)+`) has a balance of 10000000000000.5, which is too large for quarry's amounts`,
		importReason(t, err))
}

// Same as the precision case above, for the not-a-number fault.
func Test_import_refuses_an_undated_statement_with_a_text_balance_using_the_source_id_subject(t *testing.T) {
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	pk := b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndingBalance: "not-a-number"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), bundle.DataPath)

	assert.Equal(t,
		`a statement for "Chequing" (source id `+itoa(pk)+`) has a balance that is not a number`,
		importReason(t, err))
}
