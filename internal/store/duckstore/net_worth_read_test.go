package duckstore_test

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// netWorthOn reads st's net worth rows on dates.
func netWorthOn(t *testing.T, st *duckstore.Store, dates ...time.Time) []store.NetWorthRow {
	t.Helper()
	got, err := st.NetWorth(t.Context(), store.NetWorthParams{Dates: dates})
	require.NoError(t, err)
	return got.Rows
}

func cents(text string) *big.Int {
	n, _ := new(big.Int).SetString(text, 10)
	return n
}

func Test_net_worth_reads_every_column_of_a_converted_row(t *testing.T) {
	t.Parallel()
	st := newStoreWithRates(t, netWorthRows(
		[]store.Account{account(acctTwo, 2, "Second", "chequing", "CAD")},
		transaction("t1", acctOne, marchDay(1), 10_000), transaction("t2", acctTwo, marchDay(1), 5_000)),
		ratesOn(1, 1_250_000, "FXUSDCAD"))

	got := netWorthOn(t, st, marchDay(2))

	assert.Equal(t, []store.NetWorthRow{{
		Date: marchDay(2), Type: "chequing", Currency: "CAD", Accounts: 2,
		Balance: big.NewInt(15_000), BalanceCAD: big.NewInt(15_000), BalanceUSD: big.NewInt(12_000),
	}}, got)
}

func Test_net_worth_leaves_nil_a_balance_the_store_holds_as_null(t *testing.T) {
	t.Parallel()
	st := newStoreWithRates(t, cashRows(transaction("t1", acctOne, marchDay(1), 10_000)), ratesOn(10, 1_250_000, "FXUSDCAD"))

	got := netWorthOn(t, st, marchDay(5))

	assert.Equal(t, []store.NetWorthRow{{
		Date: marchDay(5), Type: "chequing", Currency: "CAD", Accounts: 1,
		Balance: big.NewInt(10_000), BalanceCAD: big.NewInt(10_000), BalanceUSD: nil,
	}}, got)
}

func Test_net_worth_orders_rows_by_type_then_currency_whatever_order_the_accounts_were_stored_in(t *testing.T) {
	t.Parallel()
	cad, usd := account(acctOne, 1, "Savings CAD", "savings", "CAD"), account(acctTwo, 2, "Savings USD", "savings", "USD")
	card := account("acct-3", 3, "Card", "credit_card", "CAD")
	cases := []struct {
		name     string
		accounts []store.Account
	}{
		{name: "CAD, USD, then the earlier type", accounts: []store.Account{cad, usd, card}},
		{name: "the earlier type, USD, then CAD", accounts: []store.Account{card, usd, cad}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := noTransactionRows()
			rows.Accounts = c.accounts
			rows.Transactions = []store.Transaction{
				transaction("t1", acctOne, marchDay(1), 100), transaction("t2", acctTwo, marchDay(1), 100),
				transaction("t3", "acct-3", marchDay(1), 100),
			}
			st := newStoreWith(t, rows)

			got := netWorthOn(t, st, marchDay(1))

			require.Len(t, got, 3)
			assert.Equal(t, [][2]string{{"credit_card", "CAD"}, {"savings", "CAD"}, {"savings", "USD"}},
				[][2]string{{got[0].Type, got[0].Currency}, {got[1].Type, got[1].Currency}, {got[2].Type, got[2].Currency}})
		})
	}
}

func Test_net_worth_reads_two_dates_in_one_call_in_date_order(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, cashRows(transaction("t1", acctOne, marchDay(1), 10_000), transaction("t2", acctOne, marchDay(3), 500)))

	got := netWorthOn(t, st, marchDay(3), marchDay(1))

	require.Len(t, got, 2)
	assert.Equal(t, []time.Time{marchDay(1), marchDay(3)}, []time.Time{got[0].Date, got[1].Date})
	assert.Equal(t, []*big.Int{big.NewInt(10_000), big.NewInt(10_500)}, []*big.Int{got[0].Balance, got[1].Balance})
}

func Test_net_worth_has_no_rows_for_a_date_before_the_first_transaction(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, cashRows(transaction("t1", acctOne, marchDay(5), 10_000)))

	before := netWorthOn(t, st, marchDay(4))
	onFirst := netWorthOn(t, st, marchDay(5))

	assert.Empty(t, before)
	assert.Len(t, onFirst, 1)
}

func Test_net_worth_leaves_out_a_transaction_dated_after_the_day_asked(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, cashRows(transaction("t1", acctOne, marchDay(1), 10_000), transaction("t2", acctOne, marchDay(3), 500)))

	got := netWorthOn(t, st, marchDay(2))

	require.Len(t, got, 1)
	assert.Equal(t, big.NewInt(10_000), got[0].Balance)
}

func Test_net_worth_reads_no_rows_for_no_dates(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, cashRows(transaction("t1", acctOne, marchDay(1), 10_000)))

	assert.Len(t, netWorthOn(t, st, marchDay(1)), 1)
	assert.Empty(t, netWorthOn(t, st))
}

func Test_net_worth_for_no_dates_still_refuses_a_missing_store(t *testing.T) {
	t.Parallel()

	_, err := duckstore.New(t.TempDir()).NetWorth(t.Context(), store.NetWorthParams{})

	openErr, ok := errors.AsType[*store.OpenError](err)
	require.True(t, ok, "want *store.OpenError, got %v", err)
	assert.Equal(t, store.OpenFaultMissing, openErr.Fault)
}

func Test_net_worth_reads_a_balance_past_64_bits_in_exact_cents(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, marchDay(1), maxDecimal18x6))
	rows.Prices = []store.Price{quote(secAcme, 1, marchDay(1), maxDecimal18x6)}
	st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

	got := netWorthOn(t, st, marchDay(2))

	require.Len(t, got, 1)
	assert.Equal(t, []*big.Int{cents("99999999999999999800000000"), cents("99999999999999999800000000"), cents("79999999999999999840000000")},
		[]*big.Int{got[0].Balance, got[0].BalanceCAD, got[0].BalanceUSD})
}
