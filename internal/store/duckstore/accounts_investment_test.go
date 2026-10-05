package duckstore_test

import (
	"math/big"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// balanceOf is the account with id in list.
func balanceOf(t *testing.T, list store.AccountList, id string) store.AccountBalance {
	t.Helper()
	for _, a := range list.Accounts {
		if a.ID == id {
			return a
		}
	}
	require.Failf(t, "account not listed", "id %s", id)
	return store.AccountBalance{}
}

func Test_accounts_reads_an_investment_accounts_cash_and_valued_holdings(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, marchDay(1), 3*oneShare))
	rows.Prices = []store.Price{quote(secAcme, 1, marchDay(1), tenUnits)}
	rows.Transactions = []store.Transaction{transaction("t1", acctOne, marchDay(1), 10_000)}
	st := newStoreWith(t, rows)

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	a := balanceOf(t, got, acctOne)
	require.NotNil(t, a.HoldingsValue)
	assert.Equal(t, []*big.Int{big.NewInt(10_000), big.NewInt(3_000), big.NewInt(13_000)}, []*big.Int{a.Cash, a.HoldingsValue, a.Balance})
}

func Test_accounts_reads_a_negative_investment_cash_balance(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, marchDay(1), 3*oneShare))
	rows.Prices = []store.Price{quote(secAcme, 1, marchDay(1), tenUnits)}
	rows.Transactions = []store.Transaction{transaction("t1", acctOne, marchDay(1), -10_000)}
	st := newStoreWith(t, rows)

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	a := balanceOf(t, got, acctOne)
	require.NotNil(t, a.HoldingsValue)
	assert.Equal(t, []*big.Int{big.NewInt(-10_000), big.NewInt(3_000), big.NewInt(-7_000)}, []*big.Int{a.Cash, a.HoldingsValue, a.Balance})
}

func Test_accounts_leaves_an_unpriced_holding_out_of_an_investment_balance(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, marchDay(1), oneShare), buy(acctOne, secControl, 2, marchDay(1), oneShare))
	rows.Prices = []store.Price{quote(secAcme, 1, marchDay(1), tenUnits)}
	st := newStoreWith(t, rows)

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	a := balanceOf(t, got, acctOne)
	require.NotNil(t, a.HoldingsValue)
	assert.Equal(t, []*big.Int{big.NewInt(0), big.NewInt(1_000), big.NewInt(1_000)}, []*big.Int{a.Cash, a.HoldingsValue, a.Balance})
}

func Test_accounts_converts_a_usd_holding_into_a_cad_investment_balance(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secUSD, 1, marchDay(1), oneShare))
	rows.Prices = []store.Price{quote(secUSD, 1, marchDay(1), tenUnits)}
	st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	assert.Equal(t, big.NewInt(1_250), balanceOf(t, got, acctOne).Balance)
}

func Test_accounts_gives_a_non_investment_account_no_holdings_value(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, balanceRows())

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	assert.Nil(t, balanceOf(t, got, acctChequing).HoldingsValue)
}

func Test_account_balances_lists_an_account_with_no_balance_row_as_zero(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		rows    store.Rows
		account string
		want    []string
	}{
		{
			name:    "an account whose only transaction is future-dated",
			rows:    cashRows(transaction("t1", acctOne, localToday().AddDate(0, 0, 3), 100)),
			account: acctOne, want: []string{"0.00", "NULL", "0.00"},
		},
		{
			name: "an account with no transaction", rows: cashRows(),
			account: acctOne, want: []string{"0.00", "NULL", "0.00"},
		},
		{
			name: "an investment account with no transaction and no holding", rows: balanceRows(),
			account: acctTwo, want: []string{"0.00", "0.00", "0.00"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreWith(t, c.rows)

			got := queryTexts(t, st, "SELECT cash, holdings_value, balance FROM v_account_balances WHERE id = '"+c.account+"'")

			assert.Equal(t, [][]string{c.want}, got)
		})
	}
}

func Test_accounts_reads_a_balance_past_64_bits_in_exact_cents(t *testing.T) {
	t.Parallel()
	rows := balanceRows(buy(acctOne, secAcme, 1, marchDay(1), maxDecimal18x6))
	rows.Prices = []store.Price{quote(secAcme, 1, marchDay(1), maxDecimal18x6)}
	st := newStoreWithRates(t, rows, ratesOn(1, 1_250_000, "FXUSDCAD"))

	got, err := st.Accounts(t.Context())

	require.NoError(t, err)
	a := balanceOf(t, got, acctOne)
	assert.Equal(t, []*big.Int{
		big.NewInt(0), cents("99999999999999999800000000"), cents("99999999999999999800000000"),
		cents("99999999999999999800000000"), cents("79999999999999999840000000"),
	},
		[]*big.Int{a.Cash, a.HoldingsValue, a.Balance, a.BalanceCAD, a.BalanceUSD})
}
