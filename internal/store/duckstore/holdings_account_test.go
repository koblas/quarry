package duckstore_test

import (
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const acctNone = "acct-none"

// accountHoldingsStore holds Acme and Globex in acct-1 (Chequing) and Globex in acct-2 (Brokerage USD),
// on 2026-03-02.
func accountHoldingsStore(t *testing.T, mutate func(*store.Rows)) *duckstore.Store {
	t.Helper()
	rows := holdingRows(
		buy(acctOne, secAcme, 1, marchDay(1), oneShare),
		buy(acctOne, secUSD, 2, marchDay(1), oneShare),
		buy(acctTwo, secUSD, 3, marchDay(1), oneShare))
	if mutate != nil {
		mutate(&rows)
	}
	return newStoreWith(t, rows)
}

// heldIn is "account/security" ids of the holdings accountHoldingsStore's st has on 2026-03-02 in the named accounts.
func heldIn(t *testing.T, st *duckstore.Store, ids ...string) []string {
	t.Helper()
	got, err := st.Holdings(t.Context(), store.HoldingsParams{AsOf: marchDay(2), AccountIDs: ids})
	require.NoError(t, err)
	order := make([]string, len(got.Holdings))
	for i, h := range got.Holdings {
		order[i] = h.AccountID + "/" + h.SecurityID
	}
	return order
}

func Test_holdings_reads_only_the_named_accounts(t *testing.T) {
	t.Parallel()
	st := accountHoldingsStore(t, nil)

	assert.Equal(t, []string{acctOne + "/" + secAcme, acctOne + "/" + secUSD}, heldIn(t, st, acctOne))
	assert.Equal(t, []string{acctTwo + "/" + secUSD}, heldIn(t, st, acctTwo))
}

func Test_holdings_reads_every_account_when_none_is_named(t *testing.T) {
	t.Parallel()
	st := accountHoldingsStore(t, nil)

	got := heldIn(t, st)

	assert.Equal(t, []string{acctTwo + "/" + secUSD, acctOne + "/" + secAcme, acctOne + "/" + secUSD}, got)
}

func Test_holdings_reads_two_named_accounts_in_table_order_whatever_order_they_are_named(t *testing.T) {
	t.Parallel()
	st := accountHoldingsStore(t, nil)

	got := heldIn(t, st, acctOne, acctTwo)

	assert.Equal(t, []string{acctTwo + "/" + secUSD, acctOne + "/" + secAcme, acctOne + "/" + secUSD}, got)
}

func Test_holdings_reads_nothing_for_an_id_that_names_no_account(t *testing.T) {
	t.Parallel()
	st := accountHoldingsStore(t, nil)

	got := heldIn(t, st, acctNone)

	assert.Empty(t, got)
}

func Test_holdings_reads_a_named_closed_account(t *testing.T) {
	t.Parallel()
	st := accountHoldingsStore(t, func(rows *store.Rows) { rows.Accounts[1].Closed = true })

	got := heldIn(t, st, acctTwo)

	assert.Equal(t, []string{acctTwo + "/" + secUSD}, got)
}
