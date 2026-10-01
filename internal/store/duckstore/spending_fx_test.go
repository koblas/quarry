package duckstore_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Rates in these tests: Friday 2026-03-13 at 1.25 and Monday 2026-03-16 at 1.30 CAD per USD
// (fridayAndMonday). 2026-03-10 is before the first rate; 2026-03-14 is a Saturday.

// fxSpendRows is fxRows with no splits, so a test's splits are the only spending.
func fxSpendRows() store.Rows {
	rows := fxRows()
	rows.Transactions, rows.Splits = nil, nil
	return rows
}

func spendingIn(currency money.Currency, by store.SpendingGroup) store.SpendingParams {
	params := spendingParams()
	params.By = by
	params.Currency = currency
	return params
}

func spendingRowsOf(rows ...store.SpendingRow) []store.SpendingRow { return rows }

func Test_spending_in_native_lists_each_currency_on_rows_of_its_own_despite_rates(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "cad", "CAD", march(16), -500)
	expense(&rows, "usd", "USD", march(16), -1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.Spending(t.Context(), spendingIn(money.Native, store.SpendByCategory))

	require.NoError(t, err)
	assert.Equal(t, spendingRowsOf(
		store.SpendingRow{Key: new("Groceries"), Currency: "CAD", Spent: 500},
		store.SpendingRow{Key: new("Groceries"), Currency: "USD", Spent: 1000}), got.Rows)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 500}, {Currency: "USD", Spent: 1000}}, got.Totals)
}

func Test_spending_in_cad_gives_a_store_of_only_cad_the_same_rows_as_native(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "a", "CAD", march(10), -500)
	expense(&rows, "b", "CAD", march(16), -700)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	native, err := st.Spending(t.Context(), spendingIn(money.Native, store.SpendByCategory))
	require.NoError(t, err)
	got, err := st.Spending(t.Context(), spendingIn(money.CAD, store.SpendByCategory))

	require.NoError(t, err)
	assert.Equal(t, native, got)
}

func Test_spending_in_cad_converts_a_usd_split_at_its_dates_rate_and_adds_it_to_the_cad_row(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "cad", "CAD", march(16), -500)
	expense(&rows, "usd", "USD", march(16), -1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.Spending(t.Context(), spendingIn(money.CAD, store.SpendByCategory))

	require.NoError(t, err)
	assert.Equal(t, spendingRowsOf(store.SpendingRow{Key: new("Groceries"), Currency: "CAD", Spent: 1800}), got.Rows)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 1800}}, got.Totals)
}

func Test_spending_in_usd_converts_a_cad_split_at_its_dates_rate_and_adds_it_to_the_usd_row(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "cad", "CAD", march(16), -1300)
	expense(&rows, "usd", "USD", march(16), -500)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.Spending(t.Context(), spendingIn(money.USD, store.SpendByCategory))

	require.NoError(t, err)
	assert.Equal(t, spendingRowsOf(store.SpendingRow{Key: new("Groceries"), Currency: "USD", Spent: 1500}), got.Rows)
	assert.Equal(t, []store.SpendingTotal{{Currency: "USD", Spent: 1500}}, got.Totals)
}

func Test_spending_keeps_a_split_with_no_rate_on_a_row_of_its_own_currency(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "cad", "CAD", march(16), -500)
	expense(&rows, "usd-rated", "USD", march(16), -1000)
	expense(&rows, "usd-unrated", "USD", march(10), -700)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.Spending(t.Context(), spendingIn(money.CAD, store.SpendByCategory))

	require.NoError(t, err)
	assert.Equal(t, spendingRowsOf(
		store.SpendingRow{Key: new("Groceries"), Currency: "CAD", Spent: 1800},
		store.SpendingRow{Key: new("Groceries"), Currency: "USD", Spent: 700}), got.Rows)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 1800}, {Currency: "USD", Spent: 700}}, got.Totals)
}

func Test_spending_in_usd_keeps_a_cad_split_before_the_first_rate_on_a_cad_row_and_total(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "cad-unrated", "CAD", march(10), -700)
	expense(&rows, "usd", "USD", march(16), -500)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.Spending(t.Context(), spendingIn(money.USD, store.SpendByCategory))

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 700}, {Currency: "USD", Spent: 500}}, got.Totals)
}

func Test_spending_in_cad_keeps_a_split_in_another_currency_on_a_row_of_its_own(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "cad", "CAD", march(16), -500)
	addSplit(&rows, splitSpec{id: "eur", currency: "EUR", date: march(16), category: new(catExpense), amount: -900})
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.Spending(t.Context(), spendingIn(money.CAD, store.SpendByCategory))

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 500}, {Currency: "EUR", Spent: 900}}, got.Totals)
}

func Test_spending_rounds_each_split_to_the_cent_before_adding_them(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "one", "USD", march(14), -10)
	expense(&rows, "two", "USD", march(14), -10)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.Spending(t.Context(), spendingIn(money.CAD, store.SpendByCategory))

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 26}}, got.Totals)
}

func Test_spending_converts_a_weekend_split_at_the_fridays_rate(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "saturday", "USD", march(14), -1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.Spending(t.Context(), spendingIn(money.CAD, store.SpendByCategory))

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 1250}}, got.Totals)
}

func Test_spending_converts_a_split_after_the_last_rate_at_the_latest_earlier_rate(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "later", "USD", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), -1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.Spending(t.Context(), spendingIn(money.CAD, store.SpendByCategory))

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 1300}}, got.Totals)
}

func Test_spending_converts_a_closed_accounts_split(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	rows.Accounts = append(rows.Accounts,
		store.Account{ID: "acct-closed", SourceID: 9, Name: "Old US", Type: "chequing", Currency: "USD", Closed: true})
	addSplit(&rows, splitSpec{id: "closed", account: "acct-closed", currency: "USD", date: march(16), category: new(catExpense), amount: -1000})
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.Spending(t.Context(), spendingIn(money.CAD, store.SpendByCategory))

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 1300}}, got.Totals)
}

func Test_spending_by_tag_converts_both_each_tags_row_and_the_total(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	addTag(&rows, "t-trip", "Trip")
	addSplit(&rows, splitSpec{id: "usd", account: acctUSD, currency: "USD", date: march(16), category: new(catExpense), amount: -1000, tags: []string{"t-trip"}})
	addSplit(&rows, splitSpec{id: "cad", date: march(16), category: new(catExpense), amount: -500})
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.Spending(t.Context(), spendingIn(money.CAD, store.SpendByTag))

	require.NoError(t, err)
	assert.Equal(t, spendingRowsOf(
		store.SpendingRow{Key: nil, Currency: "CAD", Spent: 500},
		store.SpendingRow{Key: new("Trip"), Currency: "CAD", Spent: 1300}), got.Rows)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 1800}}, got.Totals)
}

func Test_spending_by_tag_keeps_an_unrated_split_native_in_its_row_and_the_total(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	addTag(&rows, "t-trip", "Trip")
	addSplit(&rows, splitSpec{id: "usd", account: acctUSD, currency: "USD", date: march(10), category: new(catExpense), amount: -700, tags: []string{"t-trip"}})
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.Spending(t.Context(), spendingIn(money.CAD, store.SpendByTag))

	require.NoError(t, err)
	assert.Equal(t, spendingRowsOf(store.SpendingRow{Key: new("Trip"), Currency: "USD", Spent: 700}), got.Rows)
	assert.Equal(t, []store.SpendingTotal{{Currency: "USD", Spent: 700}}, got.Totals)
}

func Test_spending_by_month_converts_a_usd_split_into_its_months_cad_row(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "usd", "USD", march(16), -1000)
	expense(&rows, "cad", "CAD", march(17), -500)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.Spending(t.Context(), spendingIn(money.CAD, store.SpendByMonth))

	require.NoError(t, err)
	assert.Equal(t, spendingRowsOf(store.SpendingRow{Key: new("2026-03"), Currency: "CAD", Spent: 1800}), got.Rows)
}

func Test_spending_by_payee_ranks_a_usd_payee_first_only_once_converted(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	addPayees(&rows, "Ann", "Bea")
	addSplit(&rows, payeeSplit("ann", "Ann", "CAD", -900))
	addSplit(&rows, splitSpec{id: "bea", account: acctUSD, currency: "USD", date: march(16), category: new(catExpense), amount: -800, payee: new("payee-Bea")})
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.Spending(t.Context(), spendingIn(money.CAD, store.SpendByPayee))

	require.NoError(t, err)
	assert.Equal(t, spendingRowsOf(
		store.SpendingRow{Key: new("Bea"), Currency: "CAD", Spent: 1040},
		store.SpendingRow{Key: new("Ann"), Currency: "CAD", Spent: 900}), got.Rows)
}

func Test_spending_drops_a_category_netting_to_zero_only_once_converted_but_keeps_the_total(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "refund", "CAD", march(16), 1300)
	expense(&rows, "spent", "USD", march(16), -1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.Spending(t.Context(), spendingIn(money.CAD, store.SpendByCategory))

	require.NoError(t, err)
	assert.Empty(t, got.Rows)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 0}}, got.Totals)
}

func Test_spending_of_one_usd_account_in_cad_gives_a_cad_total(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "cad", "CAD", march(16), -500)
	expense(&rows, "usd", "USD", march(16), -1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)
	params := spendingIn(money.CAD, store.SpendByCategory)
	params.AccountIDs = []string{acctUSD}

	got, err := st.Spending(t.Context(), params)

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 1300}}, got.Totals)
}
