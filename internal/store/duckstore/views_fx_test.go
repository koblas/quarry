package duckstore_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	acctInvestment = "acct-brokerage"
	fridayRate     = 1_250_000
	mondayRate     = 1_300_000
)

// march is a date in March 2026: the 13th is a Friday, the 14th and 15th the weekend after it.
func march(day int) time.Time { return time.Date(2026, 3, day, 0, 0, 0, 0, time.UTC) }

// fxRows is reportRows with a USD account that Quicken's reports count.
func fxRows() store.Rows {
	rows := reportRows()
	rows.Accounts = append(rows.Accounts,
		store.Account{ID: acctUSD, SourceID: 3, Name: "US Chequing", Type: "chequing", Currency: "USD", Active: true},
		store.Account{ID: acctInvestment, SourceID: 4, Name: "Brokerage", Type: store.AccountTypeBrokerage, Currency: "USD", Active: true})
	return rows
}

// expense adds an expense split of cents in currency on date, on the account of that currency.
func expense(rows *store.Rows, id, currency string, date time.Time, cents int64) {
	account := acctInReports
	if currency == "USD" {
		account = acctUSD
	}
	addSplit(rows, splitSpec{id: id, account: account, currency: currency, date: date, category: new(catExpense), amount: cents})
}

func newStoreWithRates(t *testing.T, rows store.Rows, rates ...store.Rate) *duckstore.Store {
	t.Helper()
	dir := t.TempDir()
	src := &fakeRates{refresh: store.RatesRefresh{Rates: rates}}
	_, err := duckstore.New(dir, duckstore.WithRates(src)).Replace(t.Context(), rows)
	require.NoError(t, err)
	return duckstore.New(dir)
}

func fridayAndMonday() []store.Rate {
	return []store.Rate{ratesOn(13, fridayRate, "FXUSDCAD"), ratesOn(16, mondayRate, "FXUSDCAD")}
}

const convertedAmounts = "SELECT amount_cad, amount_usd FROM v_cash_flow WHERE split_id = 'x'"

func Test_cash_flow_converts_a_weekend_split_at_the_fridays_rate(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "USD", march(14), 1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got := queryTexts(t, st, convertedAmounts)

	assert.Equal(t, [][]string{{"12.50", "10.00"}}, got)
}

func Test_cash_flow_converts_a_split_on_a_rate_date_at_that_dates_rate(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "USD", march(16), 1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got := queryTexts(t, st, convertedAmounts)

	assert.Equal(t, [][]string{{"13.00", "10.00"}}, got)
}

func Test_cash_flow_converts_a_split_after_the_last_rate_at_the_last_rate(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "USD", march(20), 1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got := queryTexts(t, st, convertedAmounts)

	assert.Equal(t, [][]string{{"13.00", "10.00"}}, got)
}

func Test_cash_flow_rounds_half_a_cent_away_from_zero_and_below_half_down(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		currency string
		cents    int64
		rate     money.Rate
		want     []string
	}{
		{name: "USD to CAD at exactly half a cent", currency: "USD", cents: 10, rate: 1_250_000, want: []string{"0.13", "0.10"}},
		{name: "USD to CAD at exactly minus half a cent", currency: "USD", cents: -10, rate: 1_250_000, want: []string{"-0.13", "-0.10"}},
		{name: "USD to CAD just below half a cent", currency: "USD", cents: 10, rate: 1_249_999, want: []string{"0.12", "0.10"}},
		{name: "CAD to USD at exactly half a cent", currency: "CAD", cents: 20, rate: 1_600_000, want: []string{"0.20", "0.13"}},
		{name: "CAD to USD at exactly minus half a cent", currency: "CAD", cents: -20, rate: 1_600_000, want: []string{"-0.20", "-0.13"}},
		{name: "CAD to USD just below half a cent", currency: "CAD", cents: 20, rate: 1_600_001, want: []string{"0.20", "0.12"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := fxRows()
			expense(&rows, "x", c.currency, march(13), c.cents)
			st := newStoreWithRates(t, rows, ratesOn(13, c.rate, "FXUSDCAD"))

			got := queryTexts(t, st, convertedAmounts)

			assert.Equal(t, [][]string{c.want}, got)
		})
	}
}

func Test_cash_flow_keeps_an_amount_in_its_own_currency_when_the_store_has_no_rates(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "CAD", march(13), -250)
	expense(&rows, "y", "USD", march(13), -300)
	st := newStoreWithRates(t, rows)

	got := queryTexts(t, st, "SELECT split_id, amount_cad, amount_usd FROM v_cash_flow WHERE split_id IN ('x', 'y') ORDER BY split_id")

	assert.Equal(t, [][]string{{"x", "-2.50", "NULL"}, {"y", "NULL", "-3.00"}}, got)
}

func Test_cash_flow_leaves_the_other_currency_empty_for_a_date_before_the_first_rate(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "CAD", march(12), -250)
	expense(&rows, "y", "USD", march(12), -300)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got := queryTexts(t, st, "SELECT split_id, amount_cad, amount_usd FROM v_cash_flow WHERE split_id IN ('x', 'y') ORDER BY split_id")

	assert.Equal(t, [][]string{{"x", "-2.50", "NULL"}, {"y", "NULL", "-3.00"}}, got)
}

func Test_cash_flow_carries_the_rate_in_force_on_each_rows_date(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "CAD", march(12), -250)
	expense(&rows, "y", "CAD", march(14), -250)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got := queryTexts(t, st, "SELECT split_id, usd_cad FROM v_cash_flow WHERE split_id IN ('x', 'y') ORDER BY split_id")

	assert.Equal(t, [][]string{{"x", "NULL"}, {"y", "1.250000"}}, got)
}

func Test_cash_flow_leaves_a_third_currency_unconverted(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "EUR", march(13), -250)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got := queryTexts(t, st, convertedAmounts)

	assert.Equal(t, [][]string{{"NULL", "NULL"}}, got)
}

func Test_spending_leaves_a_third_currency_unconverted(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "EUR", march(13), -250)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got := queryTexts(t, st, "SELECT spent_cad, spent_usd FROM v_spending WHERE split_id = 'x'")

	assert.Equal(t, [][]string{{"NULL", "NULL"}}, got)
}

func Test_spending_keeps_an_amount_in_its_own_currency_when_the_store_has_no_rates(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "CAD", march(13), -250)
	expense(&rows, "y", "USD", march(13), -300)
	st := newStoreWithRates(t, rows)

	got := queryTexts(t, st, "SELECT split_id, spent_cad, spent_usd, usd_cad FROM v_spending WHERE split_id IN ('x', 'y') ORDER BY split_id")

	assert.Equal(t, [][]string{{"x", "2.50", "NULL", "NULL"}, {"y", "NULL", "3.00", "NULL"}}, got)
}

func Test_spending_carries_each_cash_flow_rows_converted_amounts_negated(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "x", "USD", march(14), -1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got := queryTexts(t, st, "SELECT spent, spent_cad, spent_usd, usd_cad FROM v_spending WHERE split_id = 'x'")

	assert.Equal(t, [][]string{{"10.00", "12.50", "10.00", "1.250000"}}, got)
}

func Test_money_convert_matches_every_spending_rows_converted_columns(t *testing.T) {
	t.Parallel()
	currencies := map[string]money.Currency{"CAD": money.CAD, "USD": money.USD}
	rateDates := []int{13, 16, 17}
	rows := fxRows()
	var id int
	for currency := range currencies {
		for _, cents := range []int64{-1, 1, 10, -10, 20, -20, 99, 12_345, -99_999_999} {
			for _, rateDate := range rateDates {
				id++
				expense(&rows, "s"+strconv.Itoa(id), currency, march(rateDate), cents)
			}
		}
	}
	st := newStoreWithRates(t, rows,
		ratesOn(13, 1_250_000, "FXUSDCAD"), ratesOn(16, 1_600_001, "FXUSDCAD"), ratesOn(17, 1_249_999, "FXUSDCAD"))
	got, err := st.Query(t.Context(), `SELECT currency, CAST(spent * 100 AS BIGINT), CAST(usd_cad * 1000000 AS BIGINT),
		CAST(spent_cad * 100 AS BIGINT), CAST(spent_usd * 100 AS BIGINT) FROM v_spending WHERE split_id <> 'keep'`, 0)
	require.NoError(t, err)

	require.Len(t, got.Rows, id)
	for _, row := range got.Rows {
		currency := currencies[row[0].Text]
		cents, rate := nativeInt(t, row[1]), money.Rate(nativeInt(t, row[2]))
		wantCAD, okCAD := money.Convert(cents, currency, money.CAD, rate)
		wantUSD, okUSD := money.Convert(cents, currency, money.USD, rate)
		require.True(t, okCAD && okUSD)
		assert.Equal(t, []int64{wantCAD, wantUSD}, []int64{nativeInt(t, row[3]), nativeInt(t, row[4])}, "%s %d at %d", row[0].Text, cents, rate)
	}
}

func nativeInt(t *testing.T, v store.QueryValue) int64 {
	t.Helper()
	n, ok := v.Native.(int64)
	require.True(t, ok, "cell %q is not a BIGINT", v.Text)
	return n
}

func balancesOf(t *testing.T, st *duckstore.Store, account string) [][]string {
	t.Helper()
	return queryTexts(t, st, "SELECT balance, balance_cad, balance_usd FROM v_account_balances WHERE id = '"+account+"'")
}

func Test_account_balances_convert_the_summed_balance_not_each_transaction(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "a", "USD", march(13), 10)
	expense(&rows, "b", "USD", march(14), 10)
	st := newStoreWithRates(t, rows, ratesOn(13, fridayRate, "FXUSDCAD"))

	got := balancesOf(t, st, acctUSD)

	assert.Equal(t, [][]string{{"0.20", "0.25", "0.20"}}, got)
}

func Test_account_balances_convert_at_the_latest_rate_dated_today_or_earlier(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "a", "USD", march(13), 1000)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got := balancesOf(t, st, acctUSD)

	assert.Equal(t, [][]string{{"10.00", "13.00", "10.00"}}, got)
}

func Test_account_balances_ignore_a_rate_dated_after_today(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "a", "USD", march(13), 1000)
	future := store.Rate{Date: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), USDCAD: 2_000_000, Series: "FXUSDCAD"}
	st := newStoreWithRates(t, rows, ratesOn(13, fridayRate, "FXUSDCAD"), future)

	got := balancesOf(t, st, acctUSD)

	assert.Equal(t, [][]string{{"10.00", "12.50", "10.00"}}, got)
}

func Test_account_balances_convert_a_cad_balance_to_usd(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	st := newStoreWithRates(t, rows, ratesOn(13, fridayRate, "FXUSDCAD"))

	got := balancesOf(t, st, acctInReports)

	assert.Equal(t, [][]string{{"-10.00", "-10.00", "-8.00"}}, got)
}

func Test_account_balances_stay_in_their_own_currency_when_the_store_has_no_rates(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	expense(&rows, "a", "USD", march(13), 1000)
	st := newStoreWithRates(t, rows)

	got := queryTexts(t, st, "SELECT id, balance_cad, balance_usd FROM v_account_balances WHERE id IN ('"+acctInReports+"', '"+acctUSD+"') ORDER BY id")

	assert.Equal(t, [][]string{{acctInReports, "-10.00", "NULL"}, {acctUSD, "NULL", "10.00"}}, got)
}

func Test_account_balances_leave_an_investment_accounts_balance_empty_in_both_currencies(t *testing.T) {
	t.Parallel()
	st := newStoreWithRates(t, fxRows(), ratesOn(13, fridayRate, "FXUSDCAD"))

	got := balancesOf(t, st, acctInvestment)

	assert.Equal(t, [][]string{{"NULL", "NULL", "NULL"}}, got)
}

func Test_account_balances_leave_a_third_currency_unconverted(t *testing.T) {
	t.Parallel()
	rows := fxRows()
	rows.Accounts = append(rows.Accounts, store.Account{ID: "acct-eur", SourceID: 5, Name: "Euro Chequing", Type: "chequing", Currency: "EUR", Active: true})
	st := newStoreWithRates(t, rows, ratesOn(13, fridayRate, "FXUSDCAD"))

	got := balancesOf(t, st, "acct-eur")

	assert.Equal(t, [][]string{{"0.00", "NULL", "NULL"}}, got)
}

func Test_account_balances_lists_its_columns_in_order(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, reportRows())

	got, err := st.Query(t.Context(), "SELECT * FROM v_account_balances", 0) //nolint:unqueryvet // every column is the point

	require.NoError(t, err)
	assert.Equal(t, []store.QueryColumn{
		{Name: "id", Type: "VARCHAR"},
		{Name: "source_id", Type: "BIGINT"},
		{Name: "name", Type: "VARCHAR"},
		{Name: "type", Type: "VARCHAR"},
		{Name: "currency", Type: "VARCHAR"},
		{Name: "institution", Type: "VARCHAR"},
		{Name: "closed", Type: "BOOLEAN"},
		{Name: "active", Type: "BOOLEAN"},
		{Name: "balance", Type: "DECIMAL(18,2)"},
		{Name: "balance_cad", Type: "DECIMAL(18,2)"},
		{Name: "balance_usd", Type: "DECIMAL(18,2)"},
	}, got.Columns)
}
