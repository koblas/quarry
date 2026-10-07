package duckstore_test

import (
	"math"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cashFlowParams() store.CashFlowParams {
	return store.CashFlowParams{Window: store.Window{Since: windowSince, Until: windowUntil}, By: store.CashFlowByMonth}
}

// emptyCashFlowParams is a window before any transaction of minimalRows.
func emptyCashFlowParams() store.CashFlowParams {
	return store.CashFlowParams{Window: emptyWindowParams().Window}
}

func namedCashFlowAccounts(ids ...string) store.CashFlowParams {
	params := cashFlowParams()
	params.AccountIDs = ids
	return params
}

// incomeAndSpending is a fixture whose only splits are one income and one expense in March 2026.
func incomeAndSpending(income, spent int64) store.Rows {
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "pay", category: new(catIncome), amount: income})
	addSplit(&rows, splitSpec{id: "buy", category: new(catExpense), amount: -spent})
	return rows
}

func Test_cash_flow_gives_income_spent_and_net_per_period_and_in_total(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "jan-pay", category: new(catIncome), amount: 900, date: day(2026, time.January, 5)})
	addSplit(&rows, splitSpec{id: "jan-buy", category: new(catExpense), amount: -200, date: day(2026, time.January, 6)})
	addSplit(&rows, splitSpec{id: "feb-pay", category: new(catIncome), amount: 600, date: day(2026, time.February, 5)})
	addSplit(&rows, splitSpec{id: "feb-buy", category: new(catExpense), amount: -500, date: day(2026, time.February, 6)})
	st := newStoreWith(t, rows)

	got, err := st.CashFlow(t.Context(), cashFlowParams())

	require.NoError(t, err)
	assert.Equal(t, store.CashFlow{
		Rows: []store.CashFlowRow{
			{Period: "2026-01", Currency: "CAD", Income: 900, Spent: 200, Net: 700, SavingsRatePct: new(77.8)},
			{Period: "2026-02", Currency: "CAD", Income: 600, Spent: 500, Net: 100, SavingsRatePct: new(16.7)},
		},
		Totals: []store.CashFlowTotal{{Currency: "CAD", Income: 1500, Spent: 700, Net: 800, SavingsRatePct: new(53.3)}},
	}, got)
}

func Test_cash_flow_gives_a_period_with_only_income_no_spending(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "pay", category: new(catIncome), amount: 900})
	st := newStoreWith(t, rows)

	got, err := st.CashFlow(t.Context(), cashFlowParams())

	require.NoError(t, err)
	assert.Equal(t, []store.CashFlowRow{{Period: "2026-03", Currency: "CAD", Income: 900, Spent: 0, Net: 900, SavingsRatePct: new(100.0)}}, got.Rows)
}

func Test_cash_flow_gives_a_period_with_only_spending_no_income_and_no_rate(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "buy", category: new(catExpense), amount: -300})
	st := newStoreWith(t, rows)

	got, err := st.CashFlow(t.Context(), cashFlowParams())

	require.NoError(t, err)
	assert.Equal(t, []store.CashFlowRow{{Period: "2026-03", Currency: "CAD", Income: 0, Spent: 300, Net: -300}}, got.Rows)
}

func Test_cash_flow_nets_an_expense_refund_against_spending(t *testing.T) {
	t.Parallel()
	rows := incomeAndSpending(1000, 400)
	addSplit(&rows, splitSpec{id: "refund", category: new(catExpense), amount: 150})
	st := newStoreWith(t, rows)

	got, err := st.CashFlow(t.Context(), cashFlowParams())

	require.NoError(t, err)
	assert.Equal(t, []store.CashFlowTotal{{Currency: "CAD", Income: 1000, Spent: 250, Net: 750, SavingsRatePct: new(75.0)}}, got.Totals)
}

func Test_cash_flow_counts_an_uncategorized_split_by_the_sign_of_its_amount(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "in", amount: 700})
	addSplit(&rows, splitSpec{id: "out", amount: -200})
	st := newStoreWith(t, rows)

	got, err := st.CashFlow(t.Context(), cashFlowParams())

	require.NoError(t, err)
	assert.Equal(t, []store.CashFlowTotal{{Currency: "CAD", Income: 700, Spent: 200, Net: 500, SavingsRatePct: new(71.4)}}, got.Totals)
}

func Test_cash_flow_counts_only_the_named_accounts(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "first", account: acctInReports, category: new(catIncome), amount: 100})
	addSplit(&rows, splitSpec{id: "second", account: acctSecond, category: new(catIncome), amount: 200})
	addSplit(&rows, splitSpec{id: "third", account: acctThird, category: new(catIncome), amount: 400})
	st := newStoreWith(t, rows)

	got, err := st.CashFlow(t.Context(), namedCashFlowAccounts(acctInReports, acctSecond))

	require.NoError(t, err)
	assert.Equal(t, []store.CashFlowTotal{{Currency: "CAD", Income: 300, Net: 300, SavingsRatePct: new(100.0)}}, got.Totals)
}

func Test_cash_flow_counts_the_windows_first_and_last_day_only(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "before", category: new(catIncome), amount: 100, date: windowSince.AddDate(0, 0, -1)})
	addSplit(&rows, splitSpec{id: "first", category: new(catIncome), amount: 200, date: windowSince})
	addSplit(&rows, splitSpec{id: "last", category: new(catIncome), amount: 400, date: windowUntil})
	addSplit(&rows, splitSpec{id: "after", category: new(catIncome), amount: 800, date: windowUntil.AddDate(0, 0, 1)})
	st := newStoreWith(t, rows)

	got, err := st.CashFlow(t.Context(), cashFlowParams())

	require.NoError(t, err)
	assert.Equal(t, int64(600), got.Totals[0].Income)
}

func Test_cash_flow_by_year_keys_each_row_by_calendar_year(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "y24-pay", category: new(catIncome), amount: 500, date: day(2024, time.June, 1)})
	addSplit(&rows, splitSpec{id: "y24-buy", category: new(catExpense), amount: -100, date: day(2024, time.December, 31)})
	addSplit(&rows, splitSpec{id: "y25-pay", category: new(catIncome), amount: 300, date: day(2025, time.January, 1)})
	st := newStoreWith(t, rows)
	params := cashFlowParams()
	params.By = store.CashFlowByYear
	params.Window.Since = day(2024, time.January, 1)

	got, err := st.CashFlow(t.Context(), params)

	require.NoError(t, err)
	assert.Equal(t, []store.CashFlowRow{
		{Period: "2024", Currency: "CAD", Income: 500, Spent: 100, Net: 400, SavingsRatePct: new(80.0)},
		{Period: "2025", Currency: "CAD", Income: 300, Net: 300, SavingsRatePct: new(100.0)},
	}, got.Rows)
}

func Test_cash_flow_lists_currencies_alphabetically_in_each_period_and_in_the_totals(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "usd-jan", category: new(catIncome), currency: "USD", amount: 10, date: day(2026, time.January, 5)})
	addSplit(&rows, splitSpec{id: "gbp-jan", category: new(catIncome), currency: "GBP", amount: 20, date: day(2026, time.January, 5)})
	addSplit(&rows, splitSpec{id: "eur-jan", category: new(catIncome), currency: "EUR", amount: 30, date: day(2026, time.January, 5)})
	addSplit(&rows, splitSpec{id: "cad-jan", category: new(catIncome), amount: 40, date: day(2026, time.January, 5)})
	addSplit(&rows, splitSpec{id: "cad-feb", category: new(catIncome), amount: 50, date: day(2026, time.February, 5)})
	st := newStoreWith(t, rows)

	got, err := st.CashFlow(t.Context(), cashFlowParams())

	require.NoError(t, err)
	assert.Equal(t, [][2]string{
		{"2026-01", "CAD"}, {"2026-01", "EUR"}, {"2026-01", "GBP"}, {"2026-01", "USD"}, {"2026-02", "CAD"},
	}, periodCurrencies(got.Rows))
	assert.Equal(t, []string{"CAD", "EUR", "GBP", "USD"}, totalCurrencies(got.Totals))
}

func periodCurrencies(rows []store.CashFlowRow) [][2]string {
	keys := make([][2]string, len(rows))
	for i, r := range rows {
		keys[i] = [2]string{r.Period, r.Currency}
	}
	return keys
}

func totalCurrencies(totals []store.CashFlowTotal) []string {
	currencies := make([]string, len(totals))
	for i, total := range totals {
		currencies[i] = total.Currency
	}
	return currencies
}

func Test_cash_flow_has_no_savings_rate_when_income_is_zero_or_less(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		income, spent int64
	}{
		{name: "income is zero and money went out", income: 0, spent: 500},
		{name: "income is negative", income: -100, spent: 500},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := spendRows()
			addSplit(&rows, splitSpec{id: "pay", category: new(catIncome), amount: c.income})
			addSplit(&rows, splitSpec{id: "buy", category: new(catExpense), amount: -c.spent})
			st := newStoreWith(t, rows)

			got, err := st.CashFlow(t.Context(), cashFlowParams())

			require.NoError(t, err)
			assert.Nil(t, got.Rows[0].SavingsRatePct)
			assert.Nil(t, got.Totals[0].SavingsRatePct)
		})
	}
}

func Test_cash_flow_has_a_savings_rate_when_income_is_one_cent(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, incomeAndSpending(1, 0))

	got, err := st.CashFlow(t.Context(), cashFlowParams())

	require.NoError(t, err)
	assert.Equal(t, new(100.0), got.Rows[0].SavingsRatePct)
}

func Test_cash_flow_rounds_the_savings_rate_half_away_from_zero_to_one_decimal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		income, spent int64
		want          float64
	}{
		{name: "a positive tie rounds up", income: 800, spent: 550, want: 31.3},
		{name: "a negative tie rounds away from zero", income: 800, spent: 1050, want: -31.3},
		{name: "below the tie rounds down", income: 3000, spent: 2999, want: 0.0},
		{name: "a negative rate keeps its sign", income: 800, spent: 1000, want: -25.0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreWith(t, incomeAndSpending(c.income, c.spent))

			got, err := st.CashFlow(t.Context(), cashFlowParams())

			require.NoError(t, err)
			assert.Equal(t, new(c.want), got.Rows[0].SavingsRatePct)
		})
	}
}

func Test_cash_flow_rate_that_rounds_to_zero_is_not_a_negative_zero(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, incomeAndSpending(1000000, 1000300))

	got, err := st.CashFlow(t.Context(), cashFlowParams())

	require.NoError(t, err)
	assert.Equal(t, int64(-300), got.Rows[0].Net)
	assert.False(t, math.Signbit(*got.Rows[0].SavingsRatePct))
	assert.False(t, math.Signbit(*got.Totals[0].SavingsRatePct))
}

func Test_cash_flow_counts_a_split_dated_after_today_when_the_window_reaches_it(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "future", category: new(catExpense), amount: -700, date: day(2099, time.June, 1)})
	st := newStoreWith(t, rows)
	params := cashFlowParams()
	params.Window = store.Window{Since: day(2099, time.January, 1), Until: day(2099, time.December, 31)}

	got, err := st.CashFlow(t.Context(), params)

	require.NoError(t, err)
	assert.Equal(t, store.CashFlow{
		Rows:   []store.CashFlowRow{{Period: "2099-06", Currency: "CAD", Spent: 700, Net: -700}},
		Totals: []store.CashFlowTotal{{Currency: "CAD", Spent: 700, Net: -700}},
	}, got)
}

func Test_cash_flow_refuses_a_period_it_does_not_know(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, spendRows())
	params := cashFlowParams()
	params.By = store.CashFlowPeriod(99)

	_, err := st.CashFlow(t.Context(), params)

	require.ErrorIs(t, err, duckstore.ErrUnsupportedPeriod)
	assert.EqualError(t, err, "cash-flow period is not supported: 99")
}

func Test_cash_flow_gives_the_transaction_range_when_the_window_holds_no_income_or_spending(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "old", category: new(catIncome), amount: 500, date: day(2003, 1, 4)})
	addSplit(&rows, splitSpec{id: "late", account: acctNotReports, category: new(catExpense), amount: -100, date: day(2025, 12, 31)})
	st := newStoreWith(t, rows)

	got, err := st.CashFlow(t.Context(), cashFlowParams())

	require.NoError(t, err)
	assert.Equal(t, store.TransactionRange{First: day(2003, 1, 4), Last: day(2025, 12, 31)}, got.Transactions)
	assert.Empty(t, got.Totals)
}

func Test_cash_flow_ranges_over_the_reported_accounts_it_is_named_for(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "left-out", account: acctNotReports, category: new(catExpense), amount: -100, date: day(2001, 5, 5)})
	addSplit(&rows, splitSpec{id: "linked", account: acctLinked, category: new(catExpense), amount: -100, date: day(1999, 1, 1)})
	addSplit(&rows, splitSpec{id: "first", account: acctInReports, category: new(catExpense), amount: -100, date: day(2019, 3, 2)})
	addSplit(&rows, splitSpec{id: "unnamed", account: acctSecond, category: new(catExpense), amount: -100, date: day(2025, 8, 8)})
	st := newStoreWith(t, rows)

	got, err := st.CashFlow(t.Context(), namedCashFlowAccounts(acctNotReports, acctLinked, acctInReports))

	require.NoError(t, err)
	assert.Equal(t, store.TransactionRange{First: day(2019, 3, 2), Last: day(2019, 3, 2)}, got.Transactions)
}

func Test_cash_flow_gives_a_zero_range_when_every_named_account_uses_linked_account_tracking(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "linked", account: acctLinked, category: new(catExpense), amount: -100, date: day(2001, 5, 5)})
	addSplit(&rows, splitSpec{id: "unnamed", account: acctSecond, category: new(catExpense), amount: -100, date: day(2010, 2, 2)})
	st := newStoreWith(t, rows)

	got, err := st.CashFlow(t.Context(), namedCashFlowAccounts(acctLinked))

	require.NoError(t, err)
	assert.Zero(t, got.Transactions)
}

func Test_cash_flow_gives_a_zero_range_when_the_store_has_no_transactions(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, spendRows())

	got, err := st.CashFlow(t.Context(), cashFlowParams())

	require.NoError(t, err)
	assert.Zero(t, got.Transactions)
}

func Test_cash_flow_leaves_the_transaction_range_unset_when_the_window_holds_income(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "old", category: new(catIncome), amount: 100, date: day(2003, 1, 4)})
	addSplit(&rows, splitSpec{id: "now", category: new(catIncome), amount: 200})
	st := newStoreWith(t, rows)

	got, err := st.CashFlow(t.Context(), cashFlowParams())

	require.NoError(t, err)
	assert.Zero(t, got.Transactions)
}

func Test_cash_flow_returns_the_open_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault("open store read-only")
	st := newBuiltStore(t, failingOpener(fault))

	_, err := st.CashFlow(t.Context(), cashFlowParams())

	require.ErrorIs(t, err, fault)
}

func Test_cash_flow_returns_the_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault(`query rows "SELECT"`)
	st := newBuiltStore(t, spyOpener(&spyReadDB{queryFault: fault}))

	_, err := st.CashFlow(t.Context(), cashFlowParams())

	assertOtherFault(t, err, "disk read failed")
	assert.ErrorIs(t, err, fault)
}

func Test_cash_flow_returns_a_scan_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, spyOpener(&spyReadDB{scanFault: errScanFailed}))

	_, err := st.CashFlow(t.Context(), cashFlowParams())

	assertOtherFault(t, err, errScanFailed.Error())
	assert.ErrorIs(t, err, errScanFailed)
}

func Test_cash_flow_returns_the_transaction_range_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault(`query rows "SELECT min"`)
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 1, queryFault: fault}))

	_, err := st.CashFlow(t.Context(), emptyCashFlowParams())

	assertOtherFault(t, err, "disk read failed")
	assert.ErrorIs(t, err, fault)
}

func Test_cash_flow_returns_a_transaction_range_scan_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 1, scanFault: errScanFailed}))

	_, err := st.CashFlow(t.Context(), emptyCashFlowParams())

	assertOtherFault(t, err, errScanFailed.Error())
	assert.ErrorIs(t, err, errScanFailed)
}

func Test_cash_flow_closes_the_connection_on_success_and_on_a_query_fault(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		fault error
	}{
		{name: "after a successful read", fault: nil},
		{name: "after a query fault", fault: errQueryFailed},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			spy := &spyReadDB{queryFault: c.fault}
			st := newBuiltStore(t, spyOpener(spy))

			_, _ = st.CashFlow(t.Context(), cashFlowParams())

			assert.Equal(t, 1, spy.closes)
		})
	}
}

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
	assert.Equal(t, store.Unconverted{FirstRate: march(13)}, got.Unconverted)
	got.Unconverted = native.Unconverted
	assert.Equal(t, native, got)
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
