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

func Test_import_refuses_a_transaction_with_more_than_2_decimal_places(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.345", PostedDate: &posted})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t,
		`a transaction on 2024-03-02 in "Visa Infinite" has an amount of 12.345, which has more than 2 decimal places`,
		importReason(t, err))
}

func Test_import_refuses_a_transaction_with_an_amount_too_large_for_quarry(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "10000000000000.5", PostedDate: &posted})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t,
		`a transaction on 2024-03-02 in "Visa Infinite" has an amount of 10000000000000.5, which is too large for quarry's amounts`,
		importReason(t, err))
}

// The amount is quoted as SQLite renders the stored REAL.
func Test_import_refuses_an_exponent_form_amount_by_the_exponents_sign(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		amount string
		want   string
	}{
		{
			name: "a small negative exponent has too many decimals", amount: "0.00001",
			want: `a transaction on 2024-03-02 in "Visa Infinite" has an amount of 1.0e-05, which has more than 2 decimal places`,
		},
		{
			name: "a positive exponent is too large", amount: "1e20",
			want: `a transaction on 2024-03-02 in "Visa Infinite" has an amount of 1.0e+20, which is too large for quarry's amounts`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
			posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
			b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: c.amount, PostedDate: &posted})
			bundle := b.WriteBundle(t, t.TempDir())

			_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

			assert.Equal(t, c.want, importReason(t, err))
		})
	}
}

func Test_import_imports_a_transaction_and_split_carrying_float_residue_as_their_cent(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "HELOC", Type: "CREDITCARD", Currency: "CAD", Active: true})
	apr13 := time.Date(2020, 4, 13, 0, 0, 0, 0, time.UTC)
	apr14 := time.Date(2020, 4, 14, 0, 0, 0, 0, time.UTC)
	first := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "-55396.139999999992", PostedDate: &apr13})
	b.Entry(v9fixture.EntryRow{Parent: first, Amount: "-55396.139999999992"})
	second := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "-15.67000000001", PostedDate: &apr14})
	b.Entry(v9fixture.EntryRow{Parent: second, Amount: "-15.67000000001"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{-5539614, -1567}, transactionAmounts(fake.Rows))
	assert.ElementsMatch(t, []int64{-5539614, -1567}, splitAmounts(fake.Rows))
}

func Test_import_imports_a_near_zero_residue_amount_as_zero_cents(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	txn := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "5.5511151231257827e-17", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "5.5511151231257827e-17"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Equal(t, []int64{0}, transactionAmounts(fake.Rows))
}

func Test_import_refuses_an_amount_beyond_the_snap_tolerance(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.3400011", PostedDate: &posted})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t,
		`a transaction on 2024-03-02 in "Visa Infinite" has an amount of 12.3400011, which has more than 2 decimal places`,
		importReason(t, err))
	assert.Empty(t, fake.Rows.Transactions)
}

func Test_import_imports_an_amount_on_the_snap_tolerance_as_its_cent(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	txn := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.340001", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txn, Amount: "12.340001"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Equal(t, []int64{1234}, transactionAmounts(fake.Rows))
}

func Test_import_refuses_a_residue_that_rounds_up_to_the_bound_as_too_large(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "999999999.99999988", PostedDate: &posted})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t,
		`a transaction on 2024-03-02 in "Visa Infinite" has an amount of 999999999.99999988, which is too large for quarry's amounts`,
		importReason(t, err))
	assert.Empty(t, fake.Rows.Transactions)
}

func transactionAmounts(rows store.Rows) []int64 {
	amounts := make([]int64, len(rows.Transactions))
	for i, txn := range rows.Transactions {
		amounts[i] = txn.Amount
	}
	return amounts
}

func splitAmounts(rows store.Rows) []int64 {
	amounts := make([]int64, len(rows.Splits))
	for i, split := range rows.Splits {
		amounts[i] = split.Amount
	}
	return amounts
}

func Test_import_refuses_a_transaction_with_reconcile_status_quarry_does_not_map(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	status := int64(7)
	b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.34", PostedDate: &posted, Status: &status})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t,
		`a transaction on 2024-03-02 in "Visa Infinite" has reconcile status 7, which quarry does not map yet`,
		importReason(t, err))
}

func Test_import_refuses_a_transaction_with_no_account(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Amount: "12.34", PostedDate: &posted})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `a transaction (source id `+itoa(txnPK)+`) has no account`, importReason(t, err))
}

func Test_import_refuses_a_transaction_with_no_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	b.Transaction(v9fixture.TransactionRow{Account: acctPK, PostedDate: &posted})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `a transaction on 2024-03-02 in "Visa Infinite" has no amount`, importReason(t, err))
}

func Test_import_refuses_a_transaction_with_no_date(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.34"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `a transaction in "Visa Infinite" (source id `+itoa(txnPK)+`) has no date`, importReason(t, err))
}

// No date wins over no amount when both are missing.
func Test_import_refuses_a_transaction_with_no_date_and_no_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `a transaction in "Visa Infinite" (source id `+itoa(txnPK)+`) has no date`, importReason(t, err))
}

// Both rows are excluded by the SQL row filter itself, before any
// required-field check runs.
func Test_import_does_not_refuse_a_deleted_or_investment_transaction_missing_fields(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	b.Transaction(v9fixture.TransactionRow{Deleted: true})
	b.Transaction(v9fixture.TransactionRow{Entity: v9fixture.EntInvestmentTransaction})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	assert.Empty(t, fake.Rows.Transactions)
}

func Test_import_refuses_a_split_with_more_than_2_decimal_places(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.345", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "12.345"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t,
		`a transaction on 2024-03-02 in "Visa Infinite" has an amount of 12.345, which has more than 2 decimal places`,
		importReason(t, err))
}

func Test_import_refuses_a_split_with_more_than_2_decimal_places_when_its_transaction_is_valid(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.34", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "12.345"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t,
		`a split of a transaction on 2024-03-02 in "Visa Infinite" has an amount of 12.345, which has more than 2 decimal places`,
		importReason(t, err))
}

func Test_import_refuses_a_split_with_an_amount_too_large_for_quarry(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.34", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "10000000000000.5"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t,
		`a split of a transaction on 2024-03-02 in "Visa Infinite" has an amount of 10000000000000.5, which is too large for quarry's amounts`,
		importReason(t, err))
}

func Test_import_refuses_a_split_with_no_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.34", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `a split of a transaction on 2024-03-02 in "Visa Infinite" has no amount`, importReason(t, err))
}

func Test_import_refuses_a_split_with_no_transaction(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	entryPK := b.Entry(v9fixture.EntryRow{Amount: "12.34"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `a split (source id `+itoa(entryPK)+`) has no transaction`, importReason(t, err))
}

// A later-added but earlier-dated transaction sorts first while every
// existing transaction keeps its own id.
func Test_import_keeps_every_other_id_when_the_snapshot_gains_a_row(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	later := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	earlier := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	txn1PK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &later})
	b.Entry(v9fixture.EntryRow{Parent: txn1PK, Amount: "1.00"})
	txn2PK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "2.00", PostedDate: &later})
	b.Entry(v9fixture.EntryRow{Parent: txn2PK, Amount: "2.00"})
	newPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "3.00", PostedDate: &earlier})
	b.Entry(v9fixture.EntryRow{Parent: newPK, Amount: "3.00"})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	_, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.NoError(t, err)
	require.Len(t, fake.Rows.Transactions, 3)
	assert.Equal(t, "txn-"+itoa(newPK), fake.Rows.Transactions[0].ID)
	assert.Contains(t, transactionIDs(fake), "txn-"+itoa(txn1PK))
	assert.Contains(t, transactionIDs(fake), "txn-"+itoa(txn2PK))
}
