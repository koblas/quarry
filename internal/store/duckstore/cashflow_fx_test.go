package duckstore_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Rates are fridayAndMonday's: 2026-03-13 at 1.25 and 2026-03-16 at 1.30; march(10) is before the first.

func cashFlowIn(currency money.Currency, by store.CashFlowPeriod) store.CashFlowParams {
	params := cashFlowParams()
	params.By = by
	params.Currency = currency
	return params
}

// earn adds an income split of cents in currency on date, on the account of that currency.
func earn(rows *store.Rows, id, currency string, date time.Time, cents int64) {
	account := acctInReports
	if currency == "USD" {
		account = acctUSD
	}
	addSplit(rows, splitSpec{id: id, account: account, currency: currency, date: date, category: new(catIncome), amount: cents})
}

func Test_cash_flow_read_in_native_lists_each_currency_on_rows_of_its_own_despite_rates(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	earn(&rows, "cad-pay", "CAD", march(16), 5000)
	expense(&rows, "usd-buy", "USD", march(16), -1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.CashFlow(t.Context(), cashFlowIn(money.Native, store.CashFlowByMonth))

	require.NoError(t, err)
	assert.Equal(t, []store.CashFlowRow{
		{Period: "2026-03", Currency: "CAD", Income: 5000, Net: 5000, SavingsRatePct: new(100.0)},
		{Period: "2026-03", Currency: "USD", Spent: 1000, Net: -1000},
	}, got.Rows)
	assert.Equal(t, []store.CashFlowTotal{
		{Currency: "CAD", Income: 5000, Net: 5000, SavingsRatePct: new(100.0)},
		{Currency: "USD", Spent: 1000, Net: -1000},
	}, got.Totals)
}

func Test_cash_flow_read_in_cad_gives_a_store_of_only_cad_the_same_rows_as_native(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	earn(&rows, "pay", "CAD", march(10), 9000)
	expense(&rows, "buy", "CAD", march(16), -700)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	native, err := st.CashFlow(t.Context(), cashFlowIn(money.Native, store.CashFlowByMonth))
	require.NoError(t, err)
	got, err := st.CashFlow(t.Context(), cashFlowIn(money.CAD, store.CashFlowByMonth))

	require.NoError(t, err)
	assert.Equal(t, native.Rows, got.Rows)
	assert.Equal(t, native.Totals, got.Totals)
	assert.Zero(t, got.Unconverted.Transactions)
}

func Test_cash_flow_read_in_cad_converts_usd_income_and_spending_at_their_dates_rates_into_the_cad_row(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	earn(&rows, "cad-pay", "CAD", march(16), 5000)
	earn(&rows, "usd-pay", "USD", march(16), 1000)
	expense(&rows, "cad-buy", "CAD", march(16), -500)
	expense(&rows, "usd-buy", "USD", march(13), -1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.CashFlow(t.Context(), cashFlowIn(money.CAD, store.CashFlowByMonth))

	require.NoError(t, err)
	want := store.CashFlowRow{Period: "2026-03", Currency: "CAD", Income: 6300, Spent: 1750, Net: 4550, SavingsRatePct: new(72.2)}
	assert.Equal(t, []store.CashFlowRow{want}, got.Rows)
	assert.Equal(t, []store.CashFlowTotal{{Currency: "CAD", Income: 6300, Spent: 1750, Net: 4550, SavingsRatePct: new(72.2)}}, got.Totals)
}

func Test_cash_flow_read_in_usd_converts_cad_income_and_spending_into_the_usd_row(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	earn(&rows, "cad-pay", "CAD", march(16), 1300)
	expense(&rows, "usd-buy", "USD", march(16), -500)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.CashFlow(t.Context(), cashFlowIn(money.USD, store.CashFlowByMonth))

	require.NoError(t, err)
	assert.Equal(t, []store.CashFlowTotal{{Currency: "USD", Income: 1000, Spent: 500, Net: 500, SavingsRatePct: new(50.0)}}, got.Totals)
}

func Test_cash_flow_read_keeps_an_unrated_split_native_beside_the_converted_ones(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	earn(&rows, "cad-pay", "CAD", march(16), 5000)
	earn(&rows, "usd-rated", "USD", march(16), 1000)
	earn(&rows, "usd-unrated", "USD", march(10), 700)
	expense(&rows, "usd-unrated-buy", "USD", march(10), -200)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.CashFlow(t.Context(), cashFlowIn(money.CAD, store.CashFlowByMonth))

	require.NoError(t, err)
	assert.Equal(t, []store.CashFlowRow{
		{Period: "2026-03", Currency: "CAD", Income: 6300, Net: 6300, SavingsRatePct: new(100.0)},
		{Period: "2026-03", Currency: "USD", Income: 700, Spent: 200, Net: 500, SavingsRatePct: new(71.4)},
	}, got.Rows)
	assert.Equal(t, []store.CashFlowTotal{
		{Currency: "CAD", Income: 6300, Net: 6300, SavingsRatePct: new(100.0)},
		{Currency: "USD", Income: 700, Spent: 200, Net: 500, SavingsRatePct: new(71.4)},
	}, got.Totals)
}

func Test_cash_flow_read_by_year_keeps_an_unrated_split_native_beside_the_converted_ones(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	earn(&rows, "usd-rated", "USD", march(16), 1000)
	expense(&rows, "usd-unrated", "USD", march(10), -200)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.CashFlow(t.Context(), cashFlowIn(money.CAD, store.CashFlowByYear))

	require.NoError(t, err)
	assert.Equal(t, []store.CashFlowRow{
		{Period: "2026", Currency: "CAD", Income: 1300, Net: 1300, SavingsRatePct: new(100.0)},
		{Period: "2026", Currency: "USD", Spent: 200, Net: -200},
	}, got.Rows)
	assert.Equal(t, []store.CashFlowTotal{
		{Currency: "CAD", Income: 1300, Net: 1300, SavingsRatePct: new(100.0)},
		{Currency: "USD", Spent: 200, Net: -200},
	}, got.Totals)
}

func Test_cash_flow_read_in_usd_keeps_a_cad_split_before_the_first_rate_on_a_cad_row_ahead_of_the_usd_total(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	earn(&rows, "cad-unrated", "CAD", march(10), 700)
	earn(&rows, "usd", "USD", march(16), 500)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.CashFlow(t.Context(), cashFlowIn(money.USD, store.CashFlowByMonth))

	require.NoError(t, err)
	assert.Equal(t, []store.CashFlowTotal{
		{Currency: "CAD", Income: 700, Net: 700, SavingsRatePct: new(100.0)},
		{Currency: "USD", Income: 500, Net: 500, SavingsRatePct: new(100.0)},
	}, got.Totals)
}

func Test_cash_flow_read_in_cad_keeps_a_split_in_another_currency_on_a_row_of_its_own(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	earn(&rows, "cad", "CAD", march(16), 500)
	addSplit(&rows, splitSpec{id: "eur", currency: "EUR", date: march(16), category: new(catExpense), amount: -900})
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.CashFlow(t.Context(), cashFlowIn(money.CAD, store.CashFlowByMonth))

	require.NoError(t, err)
	assert.Equal(t, []store.CashFlowTotal{
		{Currency: "CAD", Income: 500, Net: 500, SavingsRatePct: new(100.0)},
		{Currency: "EUR", Spent: 900, Net: -900},
	}, got.Totals)
}

func Test_cash_flow_read_rounds_each_split_to_the_cent_before_adding_them(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	earn(&rows, "pay-1", "USD", march(13), 30)
	earn(&rows, "pay-2", "USD", march(13), 30)
	expense(&rows, "buy", "USD", march(13), -20)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.CashFlow(t.Context(), cashFlowIn(money.CAD, store.CashFlowByMonth))

	require.NoError(t, err)
	assert.Equal(t, []store.CashFlowTotal{{Currency: "CAD", Income: 76, Spent: 25, Net: 51, SavingsRatePct: new(67.1)}}, got.Totals)
	assert.Equal(t, []store.CashFlowRow{{Period: "2026-03", Currency: "CAD", Income: 76, Spent: 25, Net: 51, SavingsRatePct: new(67.1)}}, got.Rows)
}

func Test_cash_flow_read_rounds_spending_split_by_split_as_it_does_income(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "buy-1", "USD", march(13), -10)
	expense(&rows, "buy-2", "USD", march(13), -10)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.CashFlow(t.Context(), cashFlowIn(money.CAD, store.CashFlowByMonth))

	require.NoError(t, err)
	assert.Equal(t, []store.CashFlowTotal{{Currency: "CAD", Spent: 26, Net: -26}}, got.Totals)
}

func Test_cash_flow_read_takes_the_savings_rate_from_the_converted_sums_not_the_native_ones(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	earn(&rows, "pay-1", "USD", march(13), 30)
	earn(&rows, "pay-2", "USD", march(13), 30)
	expense(&rows, "buy", "USD", march(13), -20)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	native, err := st.CashFlow(t.Context(), cashFlowIn(money.Native, store.CashFlowByMonth))
	require.NoError(t, err)
	got, err := st.CashFlow(t.Context(), cashFlowIn(money.CAD, store.CashFlowByMonth))

	require.NoError(t, err)
	assert.Equal(t, new(66.7), native.Totals[0].SavingsRatePct)
	assert.Equal(t, new(67.1), got.Totals[0].SavingsRatePct)
}

func Test_cash_flow_read_converts_a_weekend_split_at_the_fridays_rate(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	earn(&rows, "pay", "USD", march(14), 1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.CashFlow(t.Context(), cashFlowIn(money.CAD, store.CashFlowByMonth))

	require.NoError(t, err)
	assert.Equal(t, int64(1250), got.Totals[0].Income)
}

func Test_cash_flow_read_converts_a_split_after_the_last_rate_at_the_latest_earlier_rate(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	earn(&rows, "pay", "USD", march(20), 1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.CashFlow(t.Context(), cashFlowIn(money.CAD, store.CashFlowByMonth))

	require.NoError(t, err)
	assert.Equal(t, int64(1300), got.Totals[0].Income)
}

func Test_cash_flow_read_converts_a_closed_accounts_split(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	rows.Accounts = append(rows.Accounts,
		store.Account{ID: "acct-closed", SourceID: 9, Name: "Old US", Type: "chequing", Currency: "USD", Closed: true})
	addSplit(&rows, splitSpec{id: "closed", account: "acct-closed", currency: "USD", date: march(16), category: new(catIncome), amount: 1000})
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.CashFlow(t.Context(), cashFlowIn(money.CAD, store.CashFlowByMonth))

	require.NoError(t, err)
	assert.Equal(t, int64(1300), got.Totals[0].Income)
}

func Test_cash_flow_read_of_one_usd_account_in_cad_gives_a_cad_total(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	earn(&rows, "cad", "CAD", march(16), 500)
	earn(&rows, "usd", "USD", march(16), 1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)
	params := cashFlowIn(money.CAD, store.CashFlowByMonth)
	params.AccountIDs = []string{acctUSD}

	got, err := st.CashFlow(t.Context(), params)

	require.NoError(t, err)
	assert.Equal(t, []store.CashFlowTotal{{Currency: "CAD", Income: 1300, Net: 1300, SavingsRatePct: new(100.0)}}, got.Totals)
}
