package duckstore_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func civil(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// emptyWindowParams is a window before any transaction of minimalRows.
func emptyWindowParams() store.SpendingParams {
	return store.SpendingParams{Window: store.Window{Since: civil(1990, 1, 1), Until: civil(1990, 1, 31)}}
}

func Test_spending_gives_the_store_transaction_range_when_the_window_holds_no_spending(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "income", category: new(catIncome), amount: 500, date: civil(2003, 1, 4)})
	addSplit(&rows, splitSpec{id: "late", account: acctNotReports, category: new(catExpense), amount: -100, date: civil(2025, 12, 31)})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), spendingParams())

	require.NoError(t, err)
	assert.Equal(t, store.TransactionRange{First: civil(2003, 1, 4), Last: civil(2025, 12, 31)}, got.Transactions)
	assert.Empty(t, got.Totals)
}

func Test_spending_gives_a_zero_range_when_the_store_has_no_transactions(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, spendRows())

	got, err := st.Spending(t.Context(), spendingParams())

	require.NoError(t, err)
	assert.Zero(t, got.Transactions)
}

func Test_spending_leaves_the_transaction_range_unset_when_the_window_holds_spending(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "old", category: new(catExpense), amount: -100, date: civil(2003, 1, 4)})
	addSplit(&rows, splitSpec{id: "now", category: new(catExpense), amount: -200})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), spendingParams())

	require.NoError(t, err)
	assert.Zero(t, got.Transactions)
	assert.NotEmpty(t, got.Totals)
}

func Test_spending_ranges_over_the_in_report_accounts_it_is_named_for(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "left-out", account: acctNotReports, category: new(catExpense), amount: -100, date: civil(2001, 5, 5)})
	addSplit(&rows, splitSpec{id: "first", account: acctInReports, category: new(catExpense), amount: -100, date: civil(2019, 3, 2)})
	addSplit(&rows, splitSpec{id: "last", account: acctInReports, category: new(catExpense), amount: -100, date: civil(2024, 11, 30)})
	addSplit(&rows, splitSpec{id: "unnamed", account: acctSecond, category: new(catExpense), amount: -100, date: civil(2010, 2, 2)})
	addSplit(&rows, splitSpec{id: "unnamed-late", account: acctSecond, category: new(catExpense), amount: -100, date: civil(2025, 8, 8)})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), namedAccounts(spendingParams(), acctNotReports, acctInReports))

	require.NoError(t, err)
	assert.Equal(t, store.TransactionRange{First: civil(2019, 3, 2), Last: civil(2024, 11, 30)}, got.Transactions)
}

func Test_spending_gives_a_zero_range_when_every_named_account_is_left_out_of_reports(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "left-out", account: acctNotReports, category: new(catExpense), amount: -100, date: civil(2001, 5, 5)})
	addSplit(&rows, splitSpec{id: "unnamed", account: acctSecond, category: new(catExpense), amount: -100, date: civil(2010, 2, 2)})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), namedAccounts(spendingParams(), acctNotReports))

	require.NoError(t, err)
	assert.Zero(t, got.Transactions)
}

func Test_spending_returns_the_transaction_range_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault(`query rows "SELECT min"`)
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 1, queryFault: fault}))

	_, err := st.Spending(t.Context(), emptyWindowParams())

	assertOtherFault(t, err, "disk read failed")
	assert.ErrorIs(t, err, fault)
}

func Test_spending_returns_a_transaction_range_scan_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 1, scanFault: errScanFailed}))

	_, err := st.Spending(t.Context(), emptyWindowParams())

	assertOtherFault(t, err, errScanFailed.Error())
	assert.ErrorIs(t, err, errScanFailed)
}
