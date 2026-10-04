package duckstore_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// holdingsRead reads st's holdings on date in the named accounts.
func holdingsRead(t *testing.T, st *duckstore.Store, date time.Time, ids ...string) store.Holdings {
	t.Helper()
	got, err := st.Holdings(t.Context(), store.HoldingsParams{AsOf: date, AccountIDs: ids})
	require.NoError(t, err)
	return got
}

func Test_holdings_reads_the_first_and_last_investment_transaction_dates_of_every_account(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows(
		buy(acctOne, secAcme, 1, marchDay(3), oneShare),
		buy(acctTwo, secUSD, 2, marchDay(1), oneShare),
		buy(acctOne, secAcme, 3, marchDay(5), oneShare)))

	got := holdingsRead(t, st, marchDay(5))

	assert.Equal(t, []time.Time{marchDay(1), marchDay(5)}, []time.Time{got.FirstTransaction, got.LastTransaction})
}

func Test_holdings_reads_no_transaction_dates_for_a_store_without_investment_transactions(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows())

	got := holdingsRead(t, st, marchDay(5))

	assert.True(t, got.FirstTransaction.IsZero())
	assert.True(t, got.LastTransaction.IsZero())
}

func Test_holdings_reads_the_transaction_span_of_only_the_named_accounts(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows(
		buy(acctTwo, secUSD, 1, marchDay(1), oneShare),
		buy(acctOne, secAcme, 2, marchDay(3), oneShare),
		buy(acctOne, secAcme, 3, marchDay(4), oneShare),
		buy(acctTwo, secUSD, 4, marchDay(8), oneShare)))

	got := holdingsRead(t, st, marchDay(5), acctOne)

	assert.Equal(t, []time.Time{marchDay(3), marchDay(4)}, []time.Time{got.FirstTransaction, got.LastTransaction})
}

func Test_holdings_reads_no_transaction_dates_for_an_id_that_names_no_account(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows(buy(acctOne, secAcme, 1, marchDay(3), oneShare)))

	got := holdingsRead(t, st, marchDay(5), acctNone)

	assert.True(t, got.FirstTransaction.IsZero())
	assert.True(t, got.LastTransaction.IsZero())
}

func Test_holdings_reads_the_same_transaction_span_on_a_day_before_the_first_transaction(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, holdingRows(
		buy(acctOne, secAcme, 1, marchDay(3), oneShare),
		buy(acctOne, secAcme, 2, marchDay(5), oneShare)))

	got := holdingsRead(t, st, marchDay(1))

	assert.Empty(t, got.Holdings)
	assert.Equal(t, []time.Time{marchDay(3), marchDay(5)}, []time.Time{got.FirstTransaction, got.LastTransaction})
}

func Test_holdings_counts_a_cash_only_and_a_future_dated_transaction_in_the_span(t *testing.T) {
	t.Parallel()
	cash := buy(acctOne, secAcme, 1, marchDay(1), 0)
	cash.Action, cash.SecurityID, cash.Shares = "div", nil, nil
	future := localToday().AddDate(0, 0, 3)
	st := newStoreWith(t, holdingRows(cash, buy(acctOne, secAcme, 2, marchDay(2), oneShare), buy(acctOne, secAcme, 3, future, oneShare)))

	got := holdingsRead(t, st, marchDay(2))

	assert.Equal(t, []time.Time{marchDay(1), future}, []time.Time{got.FirstTransaction, got.LastTransaction})
}

func Test_holdings_returns_the_transaction_span_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault(`query rows "SELECT min"`)
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 2, queryFault: fault}))

	_, err := st.Holdings(t.Context(), store.HoldingsParams{AsOf: marchDay(2)})

	assertOtherFault(t, err, "disk read failed")
	assert.ErrorIs(t, err, fault)
}

func Test_holdings_returns_a_transaction_span_scan_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 2, scanFault: errScanFailed}))

	_, err := st.Holdings(t.Context(), store.HoldingsParams{AsOf: marchDay(2)})

	assertOtherFault(t, err, errScanFailed.Error())
	assert.ErrorIs(t, err, errScanFailed)
}
