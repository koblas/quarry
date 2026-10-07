package importer_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An exponent-form amount is quoted as SQLite renders the stored REAL.
func Test_import_refuses_a_transaction_amount_quarry_cannot_hold(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		amount string
		want   string
	}{
		{
			name: "more_than_2_decimal_places", amount: "12.345",
			want: `a transaction on 2024-03-02 in "Visa Infinite" has an amount of 12.345, which has more than 2 decimal places`,
		},
		{
			name: "too_large_for_quarry", amount: "10000000000000.5",
			want: `a transaction on 2024-03-02 in "Visa Infinite" has an amount of 10000000000000.5, which is too large for quarry's amounts`,
		},
		{
			name: "a_small_negative_exponent_has_too_many_decimals", amount: "0.00001",
			want: `a transaction on 2024-03-02 in "Visa Infinite" has an amount of 1.0e-05, which has more than 2 decimal places`,
		},
		{
			name: "a_positive_exponent_is_too_large", amount: "1e20",
			want: `a transaction on 2024-03-02 in "Visa Infinite" has an amount of 1.0e+20, which is too large for quarry's amounts`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			acctPK := newVisa(b)
			posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
			b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: c.amount, PostedDate: &posted})

			reason, _ := importRefused(t, b)

			assert.Equal(t, c.want, reason)
		})
	}
}

func Test_import_imports_a_transaction_and_split_carrying_float_residue_as_their_cent(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "HELOC", Type: "CREDITCARD", Currency: "CAD", Active: true})
	apr13 := time.Date(2020, 4, 13, 0, 0, 0, 0, time.UTC)
	apr14 := time.Date(2020, 4, 14, 0, 0, 0, 0, time.UTC)
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "-55396.139999999992", PostedDate: &apr13})
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "-15.67000000001", PostedDate: &apr14})

	fake, _ := importOK(t, b)

	assert.ElementsMatch(t, []int64{-5539614, -1567}, transactionAmounts(fake.Rows))
	assert.ElementsMatch(t, []int64{-5539614, -1567}, splitAmounts(fake.Rows))
}

func Test_import_imports_a_near_zero_residue_amount_as_zero_cents(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newVisa(b)
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "5.5511151231257827e-17", PostedDate: &posted})

	fake, _ := importOK(t, b)

	assert.Equal(t, []int64{0}, transactionAmounts(fake.Rows))
}

func Test_import_refuses_an_amount_beyond_the_snap_tolerance(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newVisa(b)
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.3400011", PostedDate: &posted})

	reason, fake := importRefused(t, b)

	assert.Equal(t,
		`a transaction on 2024-03-02 in "Visa Infinite" has an amount of 12.3400011, which has more than 2 decimal places`,
		reason)
	assert.Empty(t, fake.Rows.Transactions)
}

func Test_import_imports_an_amount_on_the_snap_tolerance_as_its_cent(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newVisa(b)
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "12.340001", PostedDate: &posted})

	fake, _ := importOK(t, b)

	assert.Equal(t, []int64{1234}, transactionAmounts(fake.Rows))
}

func Test_import_refuses_a_residue_that_rounds_up_to_the_bound_as_too_large(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newVisa(b)
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "999999999.99999988", PostedDate: &posted})

	reason, fake := importRefused(t, b)

	assert.Equal(t,
		`a transaction on 2024-03-02 in "Visa Infinite" has an amount of 999999999.99999988, which is too large for quarry's amounts`,
		reason)
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
	acctPK := newVisa(b)
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	status := int64(7)
	b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.34", PostedDate: &posted, Status: &status})

	reason, _ := importRefused(t, b)

	assert.Equal(t,
		`a transaction on 2024-03-02 in "Visa Infinite" has reconcile status 7, which quarry does not map yet`,
		reason)
}

func Test_import_skips_a_transaction_with_no_account_and_its_split(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	transactionWithEntry(b, v9fixture.TransactionRow{Amount: "12.34", PostedDate: &posted})

	fake, _ := importOK(t, b)

	assert.Empty(t, fake.Rows.Transactions)
	assert.Empty(t, fake.Rows.Splits)
}

func Test_import_refuses_a_transaction_with_no_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newVisa(b)
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	b.Transaction(v9fixture.TransactionRow{Account: acctPK, PostedDate: &posted})

	reason, _ := importRefused(t, b)

	assert.Equal(t, `a transaction on 2024-03-02 in "Visa Infinite" has no amount`, reason)
}

func Test_import_refuses_a_transaction_with_no_date(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		amount string
	}{
		{name: "with_an_amount", amount: "12.34"},
		{name: "with_no_amount_either", amount: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			acctPK := newVisa(b)
			txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: c.amount})

			reason, _ := importRefused(t, b)

			assert.Equal(t, `a transaction in "Visa Infinite" (source id `+itoa(txnPK)+`) has no date`, reason)
		})
	}
}

// Both rows are excluded by the SQL row filter itself, before any
// required-field check runs.
func Test_import_does_not_refuse_a_deleted_or_investment_transaction_missing_fields(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	newChequing(b)
	b.Transaction(v9fixture.TransactionRow{Deleted: true})
	b.Transaction(v9fixture.TransactionRow{Entity: v9fixture.EntInvestmentTransaction})

	fake, _ := importOK(t, b)

	assert.Empty(t, fake.Rows.Transactions)
}

func Test_import_refuses_a_split_with_more_than_2_decimal_places(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newVisa(b)
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "12.345", PostedDate: &posted})

	reason, _ := importRefused(t, b)

	assert.Equal(t,
		`a transaction on 2024-03-02 in "Visa Infinite" has an amount of 12.345, which has more than 2 decimal places`,
		reason)
}

// A later-added but earlier-dated transaction sorts first while every
// existing transaction keeps its own id.
func Test_import_keeps_every_other_id_when_the_snapshot_gains_a_row(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	later := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	earlier := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	txn1PK := transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &later})
	txn2PK := transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "2.00", PostedDate: &later})
	newPK := transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "3.00", PostedDate: &earlier})

	fake, _ := importOK(t, b)

	require.Len(t, fake.Rows.Transactions, 3)
	assert.Equal(t, "txn-"+itoa(newPK), fake.Rows.Transactions[0].ID)
	assert.Contains(t, transactionIDs(fake), "txn-"+itoa(txn1PK))
	assert.Contains(t, transactionIDs(fake), "txn-"+itoa(txn2PK))
}

func Test_import_marks_a_transaction_excluded_from_reports(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted, ExcludeFromReports: new(int64(1))})
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "2.00", PostedDate: &posted, ExcludeFromReports: new(int64(0))})
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "3.00", PostedDate: &posted})
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "4.00", PostedDate: &posted, ExcludeFromReports: new(int64(2))})

	fake, _ := importOK(t, b)

	require.Len(t, fake.Rows.Transactions, 4)
	assert.True(t, fake.Rows.Transactions[0].ExcludedFromReports, "1 is excluded")
	assert.False(t, fake.Rows.Transactions[1].ExcludedFromReports, "0 is included")
	assert.False(t, fake.Rows.Transactions[2].ExcludedFromReports, "NULL is included")
	assert.True(t, fake.Rows.Transactions[3].ExcludedFromReports, "any non-zero is excluded")
}

func Test_import_dates_a_transaction_by_its_entered_date(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	entered := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	posted := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	postedOnly := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	enteredOnly := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", EnteredDate: &entered, PostedDate: &posted})
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "2.00", PostedDate: &postedOnly})
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "3.00", EnteredDate: &enteredOnly})

	fake, _ := importOK(t, b)

	require.Len(t, fake.Rows.Transactions, 3)
	assert.Equal(t, entered, fake.Rows.Transactions[0].Date)
	assert.Equal(t, &posted, fake.Rows.Transactions[0].PostedDate)
	assert.Equal(t, postedOnly, fake.Rows.Transactions[1].Date)
	assert.Equal(t, &postedOnly, fake.Rows.Transactions[1].PostedDate)
	assert.Equal(t, enteredOnly, fake.Rows.Transactions[2].Date)
	assert.Nil(t, fake.Rows.Transactions[2].PostedDate)
}

func Test_import_orders_transactions_by_register_date(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	lateEntered := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	earlyPosted := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	earlyEntered := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	latePosted := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", EnteredDate: &lateEntered, PostedDate: &earlyPosted})
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "2.00", EnteredDate: &earlyEntered, PostedDate: &latePosted})

	fake, _ := importOK(t, b)

	require.Len(t, fake.Rows.Transactions, 2)
	assert.Equal(t, int64(200), fake.Rows.Transactions[0].Amount)
	assert.Equal(t, int64(100), fake.Rows.Transactions[1].Amount)
}

func Test_import_refuses_a_transaction_amount_that_is_not_a_number(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		amount string
	}{
		{name: "text_amount", amount: "not-a-number"},
		{name: "whitespace_amount", amount: "   "},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			acctPK := newVisa(b)
			posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
			b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: c.amount, PostedDate: &posted})

			reason, _ := importRefused(t, b)

			assert.Equal(t, `a transaction on 2024-03-02 in "Visa Infinite" has an amount that is not a number`, reason)
			assert.NotContains(t, reason, "too large")
		})
	}
}

func Test_import_refuses_a_transaction_amount_stored_as_neither_integer_nor_real(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		overwrite func(tb testing.TB, dataPath string, txnPK int64)
	}{
		{name: "empty_text_amount", overwrite: func(tb testing.TB, dataPath string, txnPK int64) {
			tb.Helper()
			setColumnEmptyText(tb, dataPath, "ZTRANSACTION", "ZAMOUNT", txnPK)
		}},
		{name: "blob_amount", overwrite: func(tb testing.TB, dataPath string, txnPK int64) {
			tb.Helper()
			setColumnBlob(tb, dataPath, "ZTRANSACTION", "ZAMOUNT", txnPK, []byte{0xde, 0xad, 0xbe, 0xef})
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			acctPK := newVisa(b)
			posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
			txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "0.00", PostedDate: &posted})
			dataPath := snapshotPath(t, b)
			c.overwrite(t, dataPath, txnPK)

			reason, _ := importRefusedFrom(t, dataPath)

			assert.Equal(t, `a transaction on 2024-03-02 in "Visa Infinite" has an amount that is not a number`, reason)
		})
	}
}

// A text/blob amount with no date falls back to the source-id subject, the
// same shape reason 10's "no date" form uses.
func Test_import_refuses_a_dateless_transaction_with_a_text_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newVisa(b)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "not-a-number"})

	reason, _ := importRefused(t, b)

	assert.Equal(t, `a transaction in "Visa Infinite" (source id `+itoa(txnPK)+`) has an amount that is not a number`, reason)
}

// A transaction in a deleted account, and its splits, are dropped
// silently — never validated, never counted, never in Rows.
func Test_import_skips_a_transaction_and_its_splits_in_a_deleted_account(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	deletedAcctPK := b.Account(v9fixture.AccountRow{Name: "Old", Type: "CHECKING", Currency: "CAD", Deleted: true})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	transactionWithEntry(b, v9fixture.TransactionRow{Account: deletedAcctPK, Amount: "1.00", PostedDate: &posted})

	fake, _ := importOK(t, b)

	assert.Empty(t, fake.Rows.Transactions)
	assert.Empty(t, fake.Rows.Splits)
}

func Test_import_skips_a_transaction_whose_account_does_not_exist(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	keptPK := transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "2.00", PostedDate: &posted})
	b.Transaction(v9fixture.TransactionRow{Account: 999, Amount: "1.00", PostedDate: &posted})

	fake, _ := importOK(t, b)

	require.Len(t, fake.Rows.Transactions, 1)
	assert.Equal(t, keptPK, fake.Rows.Transactions[0].SourceID)
}

func Test_import_nulls_a_transactions_payee_it_cannot_resolve(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		payee func(b *v9fixture.Builder) int64
	}{
		{name: "when_the_payee_is_deleted", payee: func(b *v9fixture.Builder) int64 {
			return b.Payee(v9fixture.PayeeRow{Name: "Old Shop", Deleted: true})
		}},
		{name: "when_the_payee_does_not_exist", payee: func(*v9fixture.Builder) int64 { return 999 }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			acctPK := newChequing(b)
			posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
			transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted, Payee: c.payee(b)})

			fake, _ := importOK(t, b)

			require.Len(t, fake.Rows.Transactions, 1)
			assert.Nil(t, fake.Rows.Transactions[0].PayeeID)
		})
	}
}

func Test_import_maps_cleared_and_reconciled_transaction_status(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	cleared := int64(1)
	reconciled := int64(2)
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted, Status: &cleared})
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "2.00", PostedDate: &posted, Status: &reconciled})

	fake, _ := importOK(t, b)

	statuses := make([]string, len(fake.Rows.Transactions))
	for i, txn := range fake.Rows.Transactions {
		statuses[i] = txn.Status
	}
	assert.ElementsMatch(t, []string{"cleared", "reconciled"}, statuses)
}

func Test_import_sets_transaction_memo_and_cheque_number_when_present(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "1.00", PostedDate: &posted, Note: "Groceries", CheckNumber: "101"})

	fake, _ := importOK(t, b)

	require.Len(t, fake.Rows.Transactions, 1)
	require.NotNil(t, fake.Rows.Transactions[0].Memo)
	require.NotNil(t, fake.Rows.Transactions[0].ChequeNumber)
	assert.Equal(t, "Groceries", *fake.Rows.Transactions[0].Memo)
	assert.Equal(t, "101", *fake.Rows.Transactions[0].ChequeNumber)
}
