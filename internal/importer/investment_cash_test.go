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

func Test_import_gives_an_entry_less_investment_transaction_one_uncategorized_split_of_its_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: newBrokerage(b), Type: dividendCode, Amount: "12.00", PostedDate: &investDay})

	fake, result := importInvestments(t, b)

	assert.Equal(t, []store.Split{{
		ID: fmt.Sprintf("split-itxn-%d", pk), SourceID: -pk, TransactionID: fmt.Sprintf("txn-%d", pk), Amount: 1200,
	}}, fake.Rows.Splits)
	assert.Zero(t, result.Validation.Splits.Mismatched)
}

func Test_import_gives_an_entry_less_investment_transaction_a_split_that_collides_with_no_entry_split(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	investmentPK := b.InvestmentTransaction(v9fixture.TransactionRow{Account: newBrokerage(b), Type: dividendCode, Amount: "12.00", PostedDate: &investDay})
	registerPK := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-5.00", PostedDate: &investDay})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: registerPK, Amount: "-5.00"})
	require.Equal(t, investmentPK, entryPK)

	fake, _ := importInvestments(t, b)

	ids := make(map[string]bool)
	sourceIDs := make(map[int64]bool)
	for _, split := range fake.Rows.Splits {
		ids[split.ID] = true
		sourceIDs[split.SourceID] = true
	}
	assert.Len(t, fake.Rows.Splits, 2)
	assert.Len(t, ids, 2)
	assert.Len(t, sourceIDs, 2)
}

func Test_import_still_fails_validation_for_a_register_transaction_with_no_entry(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	pk := b.Transaction(v9fixture.TransactionRow{
		Account: b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true}),
		Amount:  "12.00", PostedDate: &investDay,
	})
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

func Test_import_gives_an_entry_less_investment_transaction_with_amount_zero_no_row_and_no_split(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	b.InvestmentTransaction(v9fixture.TransactionRow{Account: newBrokerage(b), Type: dividendCode, Amount: "0", PostedDate: &investDay})

	fake, _ := importInvestments(t, b)

	assert.Empty(t, fake.Rows.Transactions)
	assert.Empty(t, fake.Rows.Splits)
}

func Test_import_gives_an_investment_cash_row_the_currency_of_its_account(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	usdPK := b.Account(v9fixture.AccountRow{Name: "US Brokerage", Type: "BROKERAGENORMAL", Currency: "USD", Active: true})
	pk := investmentWithEntry(b, v9fixture.TransactionRow{Account: usdPK, Type: dividendCode, Amount: "12.00", PostedDate: &investDay})

	fake, _ := importInvestments(t, b)

	assert.Equal(t, "USD", cashRowOf(t, fake, pk).Currency)
}

// investmentTransferLeg adds an investment transaction in account whose one entry carries quickenID and the
// ZTRANSFER text link, returning the entry's Z_PK.
func investmentTransferLeg(b *v9fixture.Builder, account int64, amount string, quickenID int64, link string) int64 {
	pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: account, Type: dividendCode, Amount: amount, PostedDate: &investDay})
	return b.Entry(v9fixture.EntryRow{Parent: pk, Amount: amount, QuickenID: quickenID, Transfer: link})
}

func Test_import_pairs_an_investment_transfer_entry_with_its_counterpart_leg(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		counterpart  v9fixture.AccountRow
		counterLeg   func(b *v9fixture.Builder, account int64, amount string, quickenID int64, link string) int64
		wantTransfer store.TransferCheck
	}{
		{
			name:         "a chequing register entry",
			counterpart:  v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true},
			counterLeg:   transferLeg,
			wantTransfer: store.TransferCheck{Paired: 1},
		},
		{
			name:         "an entry of another brokerage",
			counterpart:  v9fixture.AccountRow{Name: "Second Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true},
			counterLeg:   investmentTransferLeg,
			wantTransfer: store.TransferCheck{Paired: 1},
		},
		{
			name:         "a USD chequing register entry",
			counterpart:  v9fixture.AccountRow{Name: "US Chequing", Type: "CHECKING", Currency: "USD", Active: true},
			counterLeg:   transferLeg,
			wantTransfer: store.TransferCheck{Paired: 1, CrossCurrency: 1},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			brokeragePK := newBrokerage(b)
			counterpartPK := b.Account(c.counterpart)
			investmentLeg := investmentTransferLeg(b, brokeragePK, "50.00", 101, "102")
			counterLeg := c.counterLeg(b, counterpartPK, "-50.00", 102, "101")

			fake, result := importInvestments(t, b)

			assert.Equal(t, c.wantTransfer, result.Validation.Transfers)
			assert.Empty(t, result.Validation.Splits.Mismatched)
			assert.Equal(t, new(accountIDFor(counterpartPK)), splitByID(fake, splitIDFor(investmentLeg)).TransferAccountID)
			assert.Equal(t, new(accountIDFor(brokeragePK)), splitByID(fake, splitIDFor(counterLeg)).TransferAccountID)
		})
	}
}

func Test_import_keeps_an_investment_transfer_entry_named_for_an_account_as_a_one_sided_transfer(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	leg := investmentTransferLeg(b, newBrokerage(b), "50.00", 101, "Chequing")

	_, result := importInvestments(t, b)

	require.Len(t, result.Validation.Transfers.OneSided, 1)
	oneSided := result.Validation.Transfers.OneSided[0]
	assert.Equal(t, leg, oneSided.SourceID)
	assert.Equal(t, new(accountIDFor(chequingPK)), oneSided.OtherAccountID)
}

func Test_import_reports_an_investment_transfer_entry_that_differs_from_its_transaction_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	accountPK := newBrokerage(b)
	pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: accountPK, Type: dividendCode, Amount: "12.00", PostedDate: &investDay})
	b.Entry(v9fixture.EntryRow{Parent: pk, Amount: "10.00", QuickenID: 101, Transfer: "Chequing"})
	bundle := b.WriteBundle(t, t.TempDir())

	result, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	require.ErrorIs(t, err, store.ErrValidationFailed)
	require.Len(t, result.Validation.Splits.Mismatched, 1)
	assert.Equal(t, fmt.Sprintf("txn-%d", pk), result.Validation.Splits.Mismatched[0].ID)
	assert.Equal(t, int64(1000), result.Validation.Splits.Mismatched[0].SplitsTotal)
}

func Test_import_splits_an_investment_transaction_across_its_two_entries_with_no_synthetic_split(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: newBrokerage(b), Type: dividendCode, Amount: "12.00", PostedDate: &investDay})
	first := b.Entry(v9fixture.EntryRow{Parent: pk, Amount: "7.00"})
	second := b.Entry(v9fixture.EntryRow{Parent: pk, Amount: "5.00"})

	fake, result := importInvestments(t, b)

	assert.Equal(t, []string{splitIDFor(first), splitIDFor(second)}, []string{fake.Rows.Splits[0].ID, fake.Rows.Splits[1].ID})
	assert.Len(t, fake.Rows.Splits, 2)
	assert.Empty(t, result.Validation.Splits.Mismatched)
}
