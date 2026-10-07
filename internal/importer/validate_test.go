package importer_test

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// replaceCalls stays 0: a failed check must never reach Replace.
func Test_import_does_not_replace_the_store_when_a_check_fails(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.01"})

	_, fake := importFailingValidation(t, b)

	assert.Equal(t, 0, fake.replaceCalls)
}

// Control for the guard above: an exact match still replaces the store.
func Test_import_replaces_the_store_when_the_balance_check_passes(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := chequingWithOneReconciledTxn(b, "100.00")
	feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: "100.00"})

	fake, _ := importOK(t, b)

	assert.Equal(t, 1, fake.replaceCalls)
}

func Test_import_reports_an_account_whose_reconciled_sum_differs_from_its_statement(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                     string
		reconciled, statement    string
		quarry, quicken, differs int64
	}{
		{name: "one_cent_short", reconciled: "100.00", statement: "100.01", quarry: 10000, quicken: 10001, differs: -1},
		{name: "one_cent_over", reconciled: "100.01", statement: "100.00", quarry: 10001, quicken: 10000, differs: 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			acctPK := chequingWithOneReconciledTxn(b, c.reconciled)
			feb := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
			b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &feb, EndingBalance: c.statement})

			result, _ := importFailingValidation(t, b)

			require.Len(t, result.Validation.Balances.Mismatched, 1)
			mismatch := result.Validation.Balances.Mismatched[0]
			assert.Equal(t, c.quarry, mismatch.Quarry)
			assert.Equal(t, c.quicken, mismatch.Quicken)
			assert.Equal(t, c.differs, mismatch.Difference)
		})
	}
}

// A non-reconciled transaction must not count toward the reconciled sum:
// with it excluded the account still matches its statement.
func Test_import_excludes_non_reconciled_transactions_from_the_balance_sum(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "100.00", PostedDate: &posted, Status: &reconciled})
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "50.00", PostedDate: &posted})
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &posted, EndingBalance: "100.00"})

	_, result := importOK(t, b)

	assert.Empty(t, result.Validation.Balances.Mismatched)
}

// An investment account is counted but never appears in the
// never-reconciled list, even though it has no statement of its own.
func Test_import_counts_an_investment_account_without_listing_it_as_never_reconciled(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	newBrokerage(b)
	b.Account(v9fixture.AccountRow{Name: "Savings", Type: "SAVINGS", Currency: "CAD", Active: true})

	_, result := importOK(t, b)

	assert.Equal(t, 1, result.Validation.Balances.InvestmentAccounts)
	require.Len(t, result.Validation.Balances.NeverReconciled, 1)
	assert.Equal(t, "Savings", result.Validation.Balances.NeverReconciled[0].Name)
}

// Two separate accounts, so closed and inactive are each proven on their own.
func Test_import_checks_closed_and_inactive_accounts_like_any_other(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	closedPK := b.Account(v9fixture.AccountRow{Name: "Closed Card", Type: "CREDITCARD", Currency: "CAD", Closed: true})
	inactivePK := b.Account(v9fixture.AccountRow{Name: "Old Wallet", Type: "SAVINGS", Currency: "CAD"})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)
	transactionWithEntry(b, v9fixture.TransactionRow{Account: closedPK, Amount: "50.00", PostedDate: &posted, Status: &reconciled})
	b.Reconcile(v9fixture.ReconcileRow{Account: closedPK, EndDate: &posted, EndingBalance: "50.00"})
	transactionWithEntry(b, v9fixture.TransactionRow{Account: inactivePK, Amount: "20.00", PostedDate: &posted, Status: &reconciled})
	b.Reconcile(v9fixture.ReconcileRow{Account: inactivePK, EndDate: &posted, EndingBalance: "20.00"})

	fake, result := importOK(t, b)

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

func Test_import_reports_a_transaction_whose_splits_do_not_sum_to_its_amount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		payee       func(b *v9fixture.Builder) int64
		split       string
		splitsTotal int64
		wantPayee   string
	}{
		{
			name: "one_cent_short", payee: func(b *v9fixture.Builder) int64 { return b.Payee(v9fixture.PayeeRow{Name: "Costco"}) },
			split: "99.99", splitsTotal: 9999, wantPayee: "Costco",
		},
		{
			name: "one_cent_over_with_no_payee", payee: func(*v9fixture.Builder) int64 { return 0 },
			split: "100.01", splitsTotal: 10001, wantPayee: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			acctPK := newChequing(b)
			payeePK := c.payee(b)
			posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
			txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "100.00", PostedDate: &posted, Payee: payeePK})
			b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: c.split})

			result, _ := importFailingValidation(t, b)

			require.Len(t, result.Validation.Splits.Mismatched, 1)
			mismatch := result.Validation.Splits.Mismatched[0]
			assert.Equal(t, int64(10000), mismatch.Amount)
			assert.Equal(t, c.splitsTotal, mismatch.SplitsTotal)
			assert.Equal(t, c.wantPayee, mismatch.Payee)
		})
	}
}

// mismatchedBalanceAccount adds an account whose reconciled sum never
// matches its statement, for tests that only care about display order.
func mismatchedBalanceAccount(b *v9fixture.Builder, name, accType string, day time.Time) {
	acctPK := b.Account(v9fixture.AccountRow{Name: name, Type: accType, Currency: "CAD", Active: true})
	reconciled := int64(2)
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "10.00", PostedDate: &day, Status: &reconciled})
	b.Reconcile(v9fixture.ReconcileRow{Account: acctPK, EndDate: &day, EndingBalance: "20.00"})
}

// Zebra is inserted first but sorts last; two "Chequing" accounts at
// source ids 9 and 10 prove the tie-break is numeric, not string.
func Test_import_sorts_balance_mismatches_for_display(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	day := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	mismatchedBalanceAccount(b, "Zebra", "CHECKING", day) // source id 1
	mismatchedBalanceAccount(b, "Alpha", "CHECKING", day) // source id 2
	for i := range 6 {
		b.Account(v9fixture.AccountRow{Name: fmt.Sprintf("Filler %d", i), Type: "SAVINGS", Currency: "CAD", Active: true})
	}
	mismatchedBalanceAccount(b, "Chequing", "CHECKING", day) // source id 9
	mismatchedBalanceAccount(b, "Chequing", "CHECKING", day) // source id 10

	result, _ := importFailingValidation(t, b)

	require.Len(t, result.Validation.Balances.Mismatched, 4)
	names := make([]string, len(result.Validation.Balances.Mismatched))
	sourceIDs := make([]int64, len(result.Validation.Balances.Mismatched))
	for i, m := range result.Validation.Balances.Mismatched {
		names[i] = m.Name
		sourceIDs[i] = m.SourceID
	}
	assert.Equal(t, []string{"Alpha", "Chequing", "Chequing", "Zebra"}, names)
	assert.Equal(t, []int64{2, 9, 10, 1}, sourceIDs)
}

// mismatchedSplitTransaction adds a transaction on acctPK whose splits
// never sum to its amount, for tests that only care about display order.
func mismatchedSplitTransaction(b *v9fixture.Builder, acctPK int64, day time.Time) int64 {
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "10.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "9.00"})
	return txnPK
}

// matchingSplitTransaction adds a transaction whose splits sum correctly,
// so it never reaches Mismatched.
func matchingSplitTransaction(b *v9fixture.Builder, acctPK int64, day time.Time) {
	transactionWithEntry(b, v9fixture.TransactionRow{Account: acctPK, Amount: "10.00", PostedDate: &day})
}

// Sorted by date, then account name, then account source id, then
// transaction source id — each tie-break numeric, not string.
func Test_import_sorts_split_mismatches_for_display(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b) // source id 1
	filler := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	for range 7 {
		matchingSplitTransaction(b, acctPK, filler) // txn source ids 1-7
	}
	late := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	latePK := mismatchedSplitTransaction(b, acctPK, late) // txn source id 8
	early := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	earlyFirstPK := mismatchedSplitTransaction(b, acctPK, early)  // txn source id 9
	earlySecondPK := mismatchedSplitTransaction(b, acctPK, early) // txn source id 10

	alphaPK := b.Account(v9fixture.AccountRow{Name: "Alpha Bank", Type: "CHECKING", Currency: "CAD", Active: true}) // source id 2
	alphaTxnPK := mismatchedSplitTransaction(b, alphaPK, early)

	zuluFirstPK := b.Account(v9fixture.AccountRow{Name: "Zulu", Type: "CHECKING", Currency: "CAD", Active: true}) // source id 3
	zuluFirstTxnPK := mismatchedSplitTransaction(b, zuluFirstPK, early)
	zuluSecondPK := b.Account(v9fixture.AccountRow{Name: "Zulu", Type: "CHECKING", Currency: "CAD", Active: true}) // source id 4
	zuluSecondTxnPK := mismatchedSplitTransaction(b, zuluSecondPK, early)

	result, _ := importFailingValidation(t, b)

	require.Len(t, result.Validation.Splits.Mismatched, 6)
	ids := make([]string, len(result.Validation.Splits.Mismatched))
	sourceIDs := make([]int64, len(result.Validation.Splits.Mismatched))
	for i, m := range result.Validation.Splits.Mismatched {
		ids[i] = m.ID
		sourceIDs[i] = m.SourceID
	}
	assert.Equal(t, []string{
		fmt.Sprintf("txn-%d", alphaTxnPK), fmt.Sprintf("txn-%d", earlyFirstPK), fmt.Sprintf("txn-%d", earlySecondPK),
		fmt.Sprintf("txn-%d", zuluFirstTxnPK), fmt.Sprintf("txn-%d", zuluSecondTxnPK), fmt.Sprintf("txn-%d", latePK),
	}, ids)
	assert.Equal(t, []int64{11, 9, 10, 12, 13, 8}, sourceIDs)
}

// Amount "0.00" isolates the has-at-least-one-split guard: a nonzero
// amount would already fail the sum comparison alone.
func Test_import_reports_a_transaction_with_no_splits(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "0.00", PostedDate: &posted})

	result, _ := importFailingValidation(t, b)

	require.Len(t, result.Validation.Splits.Mismatched, 1)
	mismatch := result.Validation.Splits.Mismatched[0]
	assert.Equal(t, int64(0), mismatch.Amount)
	assert.Equal(t, int64(0), mismatch.SplitsTotal)
}

func Test_import_carries_each_split_mismatchs_account_closed_and_active_flags(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	closedPK := b.Account(v9fixture.AccountRow{Name: "Closed Visa", Type: "CREDITCARD", Currency: "CAD", Closed: true, Active: true})
	inactivePK := b.Account(v9fixture.AccountRow{Name: "Dormant Savings", Type: "SAVINGS", Currency: "CAD"})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	b.Transaction(v9fixture.TransactionRow{Account: closedPK, Amount: "1.00", PostedDate: &posted})
	b.Transaction(v9fixture.TransactionRow{Account: inactivePK, Amount: "2.00", PostedDate: &posted})

	result, _ := importFailingValidation(t, b)

	got := make([][2]bool, 0, len(result.Validation.Splits.Mismatched))
	for _, m := range result.Validation.Splits.Mismatched {
		got = append(got, [2]bool{m.Closed, m.Active})
	}
	assert.Equal(t, [][2]bool{{true, true}, {false, false}}, got)
}

func Test_import_fails_validation_when_a_transaction_lost_its_only_split(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := newChequing(b)
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.34", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: 999, Amount: "12.34"})

	result, _ := importFailingValidation(t, b)

	require.Len(t, result.Validation.Splits.Mismatched, 1)
	mismatch := result.Validation.Splits.Mismatched[0]
	assert.Equal(t, int64(1234), mismatch.Amount)
	assert.Equal(t, int64(0), mismatch.SplitsTotal)
}

// holdingOf is the share check's raw answer for one holding: ids only, as the store reports it.
func holdingOf(acctPK, secPK int64) store.ShareMismatch {
	return store.ShareMismatch{AccountID: fmt.Sprintf("acct-%d", acctPK), SecurityID: fmt.Sprintf("sec-%d", secPK), Quarry: 1, Quicken: 0}
}

func importShareMismatches(tb testing.TB, b *v9fixture.Builder, mismatched ...store.ShareMismatch) []store.ShareMismatch {
	tb.Helper()
	fake := &fakeStore{shareCheck: store.ShareCheck{Checked: len(mismatched), Mismatched: mismatched}}

	result, err := importBuilt(tb, fake, b)

	require.ErrorIs(tb, err, store.ErrValidationFailed)
	return result.Validation.Shares.Mismatched
}

func holdingIDs(mismatched []store.ShareMismatch) []string {
	ids := make([]string, len(mismatched))
	for i, m := range mismatched {
		ids[i] = m.AccountID + "/" + m.SecurityID
	}
	return ids
}

// fillerAccounts adds n accounts so the next account's source id is n+1.
func fillerAccounts(b *v9fixture.Builder, n int) {
	for i := range n {
		b.Account(v9fixture.AccountRow{Name: fmt.Sprintf("Filler %d", i), Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	}
}

// fillerSecurities adds n securities so the next security's source id is n+1.
func fillerSecurities(b *v9fixture.Builder, n int) {
	for i := range n {
		b.Security(v9fixture.SecurityRow{Name: fmt.Sprintf("Filler %d", i)})
	}
}

func Test_import_labels_a_mismatched_holding_with_its_closed_inactive_account_and_security(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	fillerAccounts(b, 2)
	acctPK := b.Account(v9fixture.AccountRow{Name: "RRSP", Type: "BROKERAGENORMAL", Currency: "CAD", Closed: true})
	fillerSecurities(b, 3)
	secPK := b.Security(v9fixture.SecurityRow{Name: "iShares Core Equity ETF", Ticker: "XEQT"})
	holding := holdingOf(acctPK, secPK)
	holding.Quarry, holding.Quicken = 120500000, 110500000

	got := importShareMismatches(t, b, holding)

	assert.Equal(t, []store.ShareMismatch{{
		AccountID: "acct-3", SecurityID: "sec-4", Quarry: 120500000, Quicken: 110500000, Difference: 10000000,
		Account: "RRSP", Currency: "CAD", Closed: true, Active: false, AccountSourceID: 3,
		Security: "iShares Core Equity ETF", Ticker: new("XEQT"), SecuritySourceID: 4,
	}}, got)
}

func Test_import_labels_a_mismatched_holding_in_an_open_active_account_with_a_security_that_has_no_ticker(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "USD", Active: true})
	secPK := b.Security(v9fixture.SecurityRow{Name: "Bare Fund"})

	got := importShareMismatches(t, b, holdingOf(acctPK, secPK))

	require.Len(t, got, 1)
	assert.False(t, got[0].Closed)
	assert.True(t, got[0].Active)
	assert.Equal(t, "USD", got[0].Currency)
	assert.Nil(t, got[0].Ticker)
}

func Test_import_leaves_labels_empty_when_a_mismatched_holding_resolves_to_no_row(t *testing.T) {
	t.Parallel()

	got := importShareMismatches(t, v9fixture.NewBuilder(), holdingOf(7, 8))

	assert.Equal(t, []store.ShareMismatch{{AccountID: "acct-7", SecurityID: "sec-8", Quarry: 1, Difference: 1}}, got)
}

// Each case hands the share check its holdings in the reverse of the order Import must report them in.
func Test_import_sorts_mismatched_holdings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		holdings func(b *v9fixture.Builder) []store.ShareMismatch
		want     []string
	}{
		{name: "by_account_name_before_account_source_id", holdings: func(b *v9fixture.Builder) []store.ShareMismatch {
			zeta := b.Account(v9fixture.AccountRow{Name: "Zeta", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
			alpha := b.Account(v9fixture.AccountRow{Name: "Alpha", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
			sec := b.Security(v9fixture.SecurityRow{Name: "Fund"})
			return []store.ShareMismatch{holdingOf(zeta, sec), holdingOf(alpha, sec)}
		}, want: []string{"acct-2/sec-1", "acct-1/sec-1"}},
		{name: "of_equally_named_accounts_by_numeric_account_source_id", holdings: func(b *v9fixture.Builder) []store.ShareMismatch {
			fillerAccounts(b, 8)
			nine := b.Account(v9fixture.AccountRow{Name: "Same", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
			ten := b.Account(v9fixture.AccountRow{Name: "Same", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
			sec := b.Security(v9fixture.SecurityRow{Name: "Fund"})
			return []store.ShareMismatch{holdingOf(ten, sec), holdingOf(nine, sec)}
		}, want: []string{"acct-9/sec-1", "acct-10/sec-1"}},
		{name: "of_one_account_by_security_name_before_security_source_id", holdings: func(b *v9fixture.Builder) []store.ShareMismatch {
			acct := newBrokerage(b)
			zeta := b.Security(v9fixture.SecurityRow{Name: "Zeta Fund"})
			alpha := b.Security(v9fixture.SecurityRow{Name: "Alpha Fund"})
			return []store.ShareMismatch{holdingOf(acct, zeta), holdingOf(acct, alpha)}
		}, want: []string{"acct-1/sec-2", "acct-1/sec-1"}},
		{name: "of_equally_named_securities_by_numeric_security_source_id", holdings: func(b *v9fixture.Builder) []store.ShareMismatch {
			acct := newBrokerage(b)
			fillerSecurities(b, 8)
			nine := b.Security(v9fixture.SecurityRow{Name: "Same Fund"})
			ten := b.Security(v9fixture.SecurityRow{Name: "Same Fund"})
			return []store.ShareMismatch{holdingOf(acct, ten), holdingOf(acct, nine)}
		}, want: []string{"acct-1/sec-9", "acct-1/sec-10"}},
		{name: "by_account_name_even_when_security_names_order_the_other_way", holdings: func(b *v9fixture.Builder) []store.ShareMismatch {
			zetaAcct := b.Account(v9fixture.AccountRow{Name: "Zeta", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
			alphaAcct := b.Account(v9fixture.AccountRow{Name: "Alpha", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
			zetaSec := b.Security(v9fixture.SecurityRow{Name: "Zeta Fund"})
			alphaSec := b.Security(v9fixture.SecurityRow{Name: "Alpha Fund"})
			return []store.ShareMismatch{holdingOf(zetaAcct, alphaSec), holdingOf(alphaAcct, zetaSec)}
		}, want: []string{"acct-2/sec-1", "acct-1/sec-2"}},
		{name: "by_account_source_id_even_when_security_names_order_the_other_way", holdings: func(b *v9fixture.Builder) []store.ShareMismatch {
			fillerAccounts(b, 8)
			nine := b.Account(v9fixture.AccountRow{Name: "Same", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
			ten := b.Account(v9fixture.AccountRow{Name: "Same", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
			zetaSec := b.Security(v9fixture.SecurityRow{Name: "Zeta Fund"})
			alphaSec := b.Security(v9fixture.SecurityRow{Name: "Alpha Fund"})
			return []store.ShareMismatch{holdingOf(ten, alphaSec), holdingOf(nine, zetaSec)}
		}, want: []string{"acct-9/sec-1", "acct-10/sec-2"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			b := v9fixture.NewBuilder()
			holdings := c.holdings(b)

			got := importShareMismatches(t, b, holdings...)

			assert.Equal(t, c.want, holdingIDs(got))
		})
	}
}

func Test_import_reports_a_mismatched_holdings_difference_as_quarry_minus_quicken_clamped_to_int64(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name            string
		quarry, quicken int64
		want            int64
	}{
		{name: "quarry above quicken is positive", quarry: 5, quicken: 3, want: 2},
		{name: "quarry below quicken is negative", quarry: 3, quicken: 5, want: -2},
		{name: "largest count less a negative one clamps instead of wrapping", quarry: math.MaxInt64, quicken: -1, want: math.MaxInt64},
		{name: "smallest count less a positive one clamps instead of wrapping", quarry: math.MinInt64, quicken: 1, want: math.MinInt64},
		{name: "a difference that lands exactly on the smallest count is kept", quarry: math.MinInt64 + 1, quicken: 1, want: math.MinInt64},
		{name: "a difference that lands exactly on the largest count is kept", quarry: math.MaxInt64 - 1, quicken: -1, want: math.MaxInt64},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			holding := holdingOf(7, 8)
			holding.Quarry, holding.Quicken = c.quarry, c.quicken

			got := importShareMismatches(t, v9fixture.NewBuilder(), holding)

			require.Len(t, got, 1)
			assert.Equal(t, c.want, got[0].Difference)
		})
	}
}
