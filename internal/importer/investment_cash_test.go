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

// cashRowOf returns the transactions row of the investment transaction at source pk, by its id.
func cashRowOf(t *testing.T, fake *fakeStore, pk int64) store.Transaction {
	t.Helper()
	id := fmt.Sprintf("txn-%d", pk)
	for _, txn := range fake.Rows.Transactions {
		if txn.ID == id {
			return txn
		}
	}
	require.Failf(t, "no cash row", "transactions has no %s, only %v", id, transactionIDs(fake))
	return store.Transaction{}
}

func Test_import_gives_an_investment_transaction_with_an_amount_a_cash_row_carrying_its_fields(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	pk := investmentWithEntry(b, v9fixture.TransactionRow{
		Account: accountPK, Type: dividendCode, Amount: "-12.00", PostedDate: &investDay, EnteredDate: &investLater,
		Note: "quarterly", Status: new(int64(1)), ExcludeFromReports: new(int64(1)),
	})

	fake, _ := importInvestments(t, b)

	assert.Equal(t, store.Transaction{
		ID: fmt.Sprintf("txn-%d", pk), SourceID: pk, AccountID: fmt.Sprintf("acct-%d", accountPK),
		Date: investDay, PostedDate: &investDay, Amount: -1200, Currency: "CAD", Status: "cleared",
		ExcludedFromReports: true, Memo: new("quarterly"), InvestmentTransactionID: new(fmt.Sprintf("itxn-%d", pk)),
	}, cashRowOf(t, fake, pk))
}

func Test_import_dates_an_investment_cash_row_by_the_investments_posted_day_else_its_entered_day(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		posted     *time.Time
		wantDate   time.Time
		wantPosted *time.Time
	}{
		{name: "posted and entered", posted: &investDay, wantDate: investDay, wantPosted: &investDay},
		{name: "entered only", posted: nil, wantDate: investLater, wantPosted: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			pk := investmentWithEntry(b, v9fixture.TransactionRow{
				Account: newBrokerage(b), Type: dividendCode, Amount: "12.00", PostedDate: c.posted, EnteredDate: &investLater,
			})

			fake, _ := importInvestments(t, b)

			cash := cashRowOf(t, fake, pk)
			assert.Equal(t, c.wantDate, cash.Date)
			assert.Equal(t, c.wantPosted, cash.PostedDate)
		})
	}
}

func Test_import_reads_an_investment_cash_rows_status_from_the_reconcile_status(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status *int64
		want   string
	}{
		{name: "NULL is uncleared", status: nil, want: "uncleared"},
		{name: "0 is uncleared", status: new(int64(0)), want: "uncleared"},
		{name: "1 is cleared", status: new(int64(1)), want: "cleared"},
		{name: "2 is reconciled", status: new(int64(2)), want: "reconciled"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			pk := investmentWithEntry(b, v9fixture.TransactionRow{
				Account: newBrokerage(b), Type: dividendCode, Amount: "12.00", PostedDate: &investDay, Status: c.status,
			})

			fake, _ := importInvestments(t, b)

			assert.Equal(t, c.want, cashRowOf(t, fake, pk).Status)
		})
	}
}

func Test_import_refuses_an_investment_transaction_with_an_amount_and_an_unmapped_reconcile_status(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	investmentWithEntry(b, v9fixture.TransactionRow{
		Account: newBrokerage(b), Type: dividendCode, Amount: "12.00", PostedDate: &investDay, Status: new(int64(3)),
	})

	reason, fake := importInvestmentsRefused(t, b)

	assert.Equal(t, `a transaction on 2026-03-01 in "Brokerage" has reconcile status 3, which quarry does not map yet`, reason)
	assert.Zero(t, fake.replaceCalls)
}

func Test_import_ignores_the_reconcile_status_of_an_investment_transaction_with_amount_zero(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	investmentWithEntry(b, v9fixture.TransactionRow{
		Account: newBrokerage(b), Type: dividendCode, Amount: "0", PostedDate: &investDay, Status: new(int64(3)),
	})

	fake, _ := importInvestments(t, b)

	assert.Len(t, fake.Rows.InvestmentTransactions, 1)
}

func Test_import_flags_an_investment_cash_row_excluded_from_reports_only_when_the_source_is(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		exclude *int64
		want    bool
	}{
		{name: "NULL", exclude: nil, want: false},
		{name: "0", exclude: new(int64(0)), want: false},
		{name: "1", exclude: new(int64(1)), want: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			pk := investmentWithEntry(b, v9fixture.TransactionRow{
				Account: newBrokerage(b), Type: dividendCode, Amount: "12.00", PostedDate: &investDay, ExcludeFromReports: c.exclude,
			})

			fake, _ := importInvestments(t, b)

			assert.Equal(t, c.want, cashRowOf(t, fake, pk).ExcludedFromReports)
		})
	}
}

func Test_import_gives_an_investment_cash_row_the_investments_note_as_memo_and_none_for_an_empty_note(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	notedPK := investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: dividendCode, Amount: "12.00", PostedDate: &investDay, Note: "quarterly"})
	barePK := investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: dividendCode, Amount: "13.00", PostedDate: &investDay})

	fake, _ := importInvestments(t, b)

	assert.Equal(t, new("quarterly"), cashRowOf(t, fake, notedPK).Memo)
	assert.Nil(t, cashRowOf(t, fake, barePK).Memo)
}

func Test_import_gives_no_cash_row_to_an_investment_transaction_with_amount_zero(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		code int64
	}{
		{name: "add shares", code: 2},
		{name: "remove shares", code: 17},
		{name: "split", code: 23},
		{name: "reinvested dividend", code: 15},
		{name: "buy of 0.00", code: 3},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			accountPK := newBrokerage(b)
			controlPK := investmentWithEntry(b, v9fixture.TransactionRow{Account: accountPK, Type: dividendCode, Amount: "12.00", PostedDate: &investDay})
			investmentWithEntry(b, v9fixture.TransactionRow{
				Account: accountPK, Type: &c.code, Amount: "0", PostedDate: &investDay, Units: "0", Numerator: "1", Denominator: "1",
			})

			fake, _ := importInvestments(t, b)

			assert.Len(t, fake.Rows.InvestmentTransactions, 2)
			assert.Equal(t, []string{fmt.Sprintf("txn-%d", controlPK)}, transactionIDs(fake))
		})
	}
}

func Test_import_gives_an_investment_transactions_entry_a_split_with_its_category_amount_and_memo(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	incomePK := b.Category(v9fixture.TagRow{Name: "Dividends", Type: new(int64(0))})
	pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: accountPK, Type: dividendCode, Amount: "12.00", PostedDate: &investDay})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: pk, Amount: "12.00", CategoryTag: incomePK, Note: "ACME Q1"})

	fake, _ := importInvestments(t, b)

	assert.Equal(t, []store.Split{{
		ID: fmt.Sprintf("split-%d", entryPK), SourceID: entryPK, TransactionID: fmt.Sprintf("txn-%d", pk),
		Amount: 1200, CategoryID: new(fmt.Sprintf("cat-%d", incomePK)), Memo: new("ACME Q1"),
	}}, fake.Rows.Splits)
}

func Test_import_fails_validation_for_an_investment_transaction_with_an_amount_and_no_entry(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: newBrokerage(b), Type: dividendCode, Amount: "12.00", PostedDate: &investDay})
	bundle := b.WriteBundle(t, t.TempDir())
	fake := &fakeStore{}

	result, err := importer.NewServer(importer.WithStore(fake)).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.ErrorIs(t, err, store.ErrValidationFailed)
	require.Len(t, result.Validation.Splits.Mismatched, 1)
	mismatch := result.Validation.Splits.Mismatched[0]
	assert.Equal(t, fmt.Sprintf("txn-%d", pk), mismatch.ID)
	assert.Equal(t, int64(1200), mismatch.Amount)
	assert.Zero(t, mismatch.SplitsTotal)
	assert.Zero(t, fake.replaceCalls)
}
