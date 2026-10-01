package duckstore_test

import (
	"testing"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Rates are fridayAndMonday's: the first is 2026-03-13; march(10) is before it.

// also adds a second split of cents to the transaction of the split named of.
func also(rows *store.Rows, of, id string, cents int64) {
	rows.Splits = append(rows.Splits, store.Split{
		ID: id, SourceID: int64(len(rows.Splits) + 1), TransactionID: "txn-" + of,
		CategoryID: new(catExpense), Amount: cents,
	})
}

func unconvertedOfSpending(t *testing.T, rows store.Rows, params store.SpendingParams) store.Unconverted {
	t.Helper()
	got, err := newStoreWithRates(t, rows, fridayAndMonday()...).Spending(t.Context(), params)
	require.NoError(t, err)
	return got.Unconverted
}

func Test_spending_counts_a_usd_split_before_the_first_rate_in_cad_mode_and_gives_that_rate_date(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "cad", "CAD", march(10), -300)
	expense(&rows, "usd-unrated", "USD", march(10), -700)
	expense(&rows, "usd-rated", "USD", march(16), -500)

	got := unconvertedOfSpending(t, rows, spendingIn(money.CAD, store.SpendByCategory))

	assert.Equal(t, store.Unconverted{Transactions: 1, FirstRate: march(13)}, got)
}

func Test_spending_counts_a_cad_split_before_the_first_rate_in_usd_mode(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "cad-unrated", "CAD", march(10), -700)
	expense(&rows, "usd", "USD", march(10), -300)

	got := unconvertedOfSpending(t, rows, spendingIn(money.USD, store.SpendByCategory))

	assert.Equal(t, store.Unconverted{Transactions: 1, FirstRate: march(13)}, got)
}

func Test_spending_counts_every_cad_and_usd_split_but_gives_no_rate_date_when_the_store_has_no_rates(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "cad", "CAD", march(16), -300)
	expense(&rows, "usd-one", "USD", march(16), -700)
	expense(&rows, "usd-two", "USD", march(10), -500)

	got, err := newStoreWith(t, rows).Spending(t.Context(), spendingIn(money.CAD, store.SpendByCategory))

	require.NoError(t, err)
	assert.Equal(t, store.Unconverted{Transactions: 2}, got.Unconverted)
}

func Test_spending_counts_two_unconverted_splits_of_one_transaction_once(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "usd", "USD", march(10), -700)
	also(&rows, "usd", "usd-second", -200)

	got := unconvertedOfSpending(t, rows, spendingIn(money.CAD, store.SpendByCategory))

	assert.Equal(t, 1, got.Transactions)
}

func Test_spending_counts_nothing_for_a_split_on_or_after_the_first_rate(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "first-day", "USD", march(13), -700)
	expense(&rows, "weekend", "USD", march(14), -500)
	expense(&rows, "after", "USD", march(20), -300)

	got := unconvertedOfSpending(t, rows, spendingIn(money.CAD, store.SpendByCategory))

	assert.Equal(t, store.Unconverted{FirstRate: march(13)}, got)
}

func Test_spending_does_not_count_a_third_currency_split_as_unconverted(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	addSplit(&rows, splitSpec{id: "eur", currency: "EUR", date: march(16), category: new(catExpense), amount: -900})

	got := unconvertedOfSpending(t, rows, spendingIn(money.CAD, store.SpendByCategory))

	assert.Equal(t, store.Unconverted{FirstRate: march(13)}, got)
}

func Test_spending_counts_only_the_named_accounts_unconverted_splits(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "cad", "CAD", march(10), -300)
	expense(&rows, "usd-unrated", "USD", march(10), -700)
	params := namedAccounts(spendingIn(money.CAD, store.SpendByCategory), acctInReports)

	cadOnly := unconvertedOfSpending(t, rows, params)
	both := unconvertedOfSpending(t, rows, spendingIn(money.CAD, store.SpendByCategory))

	assert.Equal(t, 0, cadOnly.Transactions)
	assert.Equal(t, 1, both.Transactions)
}

func Test_spending_counts_only_unconverted_splits_inside_the_window(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		window store.Window
	}{
		{name: "a window starting after the split", window: store.Window{Since: march(11), Until: march(31)}},
		{name: "a window ending before the split", window: store.Window{Since: march(1), Until: march(9)}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := fxSpendRows()
			expense(&rows, "usd-unrated", "USD", march(10), -700)
			params := spendingIn(money.CAD, store.SpendByCategory)
			params.Window = c.window

			got := unconvertedOfSpending(t, rows, params)

			assert.Equal(t, 0, got.Transactions)
		})
	}
}

func Test_spending_in_native_gives_the_zero_unconverted_even_with_a_split_before_the_first_rate(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "usd-unrated", "USD", march(10), -700)

	got := unconvertedOfSpending(t, rows, spendingIn(money.Native, store.SpendByCategory))

	assert.Zero(t, got)
}

func Test_spending_by_tag_counts_a_split_with_two_tags_once(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	addTag(&rows, "t-trip", "Trip")
	addTag(&rows, "t-work", "Work")
	addSplit(&rows, splitSpec{
		id: "both", account: acctUSD, currency: "USD", date: march(10),
		category: new(catExpense), amount: -700, tags: []string{"t-trip", "t-work"},
	})

	got := unconvertedOfSpending(t, rows, spendingIn(money.CAD, store.SpendByTag))

	assert.Equal(t, 1, got.Transactions)
}

func Test_cash_flow_counts_a_pre_rate_income_split_that_spending_leaves_out(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	earn(&rows, "usd-pay", "USD", march(10), 5000)
	expense(&rows, "usd-buy", "USD", march(10), -700)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	flow, err := st.CashFlow(t.Context(), cashFlowIn(money.CAD, store.CashFlowByMonth))
	require.NoError(t, err)
	spend, err := st.Spending(t.Context(), spendingIn(money.CAD, store.SpendByCategory))
	require.NoError(t, err)

	assert.Equal(t, store.Unconverted{Transactions: 2, FirstRate: march(13)}, flow.Unconverted)
	assert.Equal(t, store.Unconverted{Transactions: 1, FirstRate: march(13)}, spend.Unconverted)
}

func Test_cash_flow_counts_a_cad_split_before_the_first_rate_in_usd_mode(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	earn(&rows, "cad-pay", "CAD", march(10), 5000)
	earn(&rows, "usd-pay", "USD", march(10), 5000)

	got, err := newStoreWithRates(t, rows, fridayAndMonday()...).CashFlow(t.Context(), cashFlowIn(money.USD, store.CashFlowByYear))

	require.NoError(t, err)
	assert.Equal(t, store.Unconverted{Transactions: 1, FirstRate: march(13)}, got.Unconverted)
}

func Test_cash_flow_counts_two_unconverted_splits_of_one_transaction_once_and_ignores_other_currencies(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "usd", "USD", march(10), -700)
	also(&rows, "usd", "usd-second", -200)
	addSplit(&rows, splitSpec{id: "eur", currency: "EUR", date: march(16), category: new(catIncome), amount: 900})

	got, err := newStoreWithRates(t, rows, fridayAndMonday()...).CashFlow(t.Context(), cashFlowIn(money.CAD, store.CashFlowByMonth))

	require.NoError(t, err)
	assert.Equal(t, 1, got.Unconverted.Transactions)
}

func Test_cash_flow_counts_only_the_named_accounts_and_the_window_and_nothing_in_native(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "cad", "CAD", march(10), -300)
	expense(&rows, "usd-unrated", "USD", march(10), -700)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)
	named := namedCashFlowAccounts(acctInReports)
	named.Currency = money.CAD
	late := cashFlowIn(money.CAD, store.CashFlowByMonth)
	late.Window = store.Window{Since: march(11), Until: march(31)}
	native := cashFlowIn(money.Native, store.CashFlowByMonth)

	cases := []struct {
		name   string
		params store.CashFlowParams
	}{
		{name: "a CAD-only account", params: named},
		{name: "a window after the split", params: late},
		{name: "native", params: native},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := st.CashFlow(t.Context(), c.params)

			require.NoError(t, err)
			assert.Equal(t, 0, got.Unconverted.Transactions)
		})
	}
}

func Test_cash_flow_ignores_a_pre_rate_split_in_a_category_that_is_neither_income_nor_expense(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	rows.Categories = append(rows.Categories, store.Category{ID: "cat-other", SourceID: 9, Name: "Other", FullPath: "Other", Kind: "other"})
	addSplit(&rows, splitSpec{id: "other", account: acctUSD, currency: "USD", date: march(10), category: new("cat-other"), amount: -700})

	got, err := newStoreWithRates(t, rows, fridayAndMonday()...).CashFlow(t.Context(), cashFlowIn(money.CAD, store.CashFlowByMonth))

	require.NoError(t, err)
	assert.Equal(t, 0, got.Unconverted.Transactions)
}

// listsOtherCurrency reports whether rows hold a CAD or USD currency other than target.
func listsOtherCurrency(target string, currencies ...string) bool {
	for _, c := range currencies {
		if c != target && (c == "CAD" || c == "USD") {
			return true
		}
	}
	return false
}

func Test_unconverted_transactions_are_positive_exactly_when_the_rows_list_the_other_currency(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		currency money.Currency
		target   string
		build    func(rows *store.Rows)
		rates    []store.Rate
	}{
		{
			name: "pre-rate USD in CAD", currency: money.CAD, target: "CAD", rates: fridayAndMonday(),
			build: func(rows *store.Rows) {
				expense(rows, "u", "USD", march(10), -700)
				earn(rows, "p", "USD", march(10), 900)
			},
		},
		{
			name: "pre-rate CAD in USD", currency: money.USD, target: "USD", rates: fridayAndMonday(),
			build: func(rows *store.Rows) {
				expense(rows, "c", "CAD", march(10), -700)
				earn(rows, "p", "CAD", march(10), 900)
			},
		},
		{
			name: "all rated in CAD", currency: money.CAD, target: "CAD", rates: fridayAndMonday(),
			build: func(rows *store.Rows) {
				expense(rows, "u", "USD", march(16), -700)
				earn(rows, "p", "USD", march(16), 900)
			},
		},
		{
			name: "no rates, USD data in CAD", currency: money.CAD, target: "CAD",
			build: func(rows *store.Rows) {
				expense(rows, "u", "USD", march(16), -700)
				earn(rows, "p", "USD", march(16), 900)
			},
		},
		{
			name: "all CAD data in CAD", currency: money.CAD, target: "CAD", rates: fridayAndMonday(),
			build: func(rows *store.Rows) {
				expense(rows, "c", "CAD", march(10), -700)
				earn(rows, "p", "CAD", march(10), 900)
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := fxSpendRows()
			c.build(&rows)
			st := newStoreWithRates(t, rows, c.rates...)

			spend, err := st.Spending(t.Context(), spendingIn(c.currency, store.SpendByCategory))
			require.NoError(t, err)
			flow, err := st.CashFlow(t.Context(), cashFlowIn(c.currency, store.CashFlowByMonth))
			require.NoError(t, err)

			var spendCurrencies, flowCurrencies []string
			for _, r := range spend.Rows {
				spendCurrencies = append(spendCurrencies, r.Currency)
			}
			for _, r := range flow.Rows {
				flowCurrencies = append(flowCurrencies, r.Currency)
			}
			assert.Equal(t, listsOtherCurrency(c.target, spendCurrencies...), spend.Unconverted.Transactions > 0, "spending")
			assert.Equal(t, listsOtherCurrency(c.target, flowCurrencies...), flow.Unconverted.Transactions > 0, "cash flow")
		})
	}
}

func Test_spending_returns_the_unconverted_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault(`query rows "SELECT count"`)
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 1, queryFault: fault}))

	_, err := st.Spending(t.Context(), spendingIn(money.CAD, store.SpendByCategory))

	assertOtherFault(t, err, "disk read failed")
	assert.ErrorIs(t, err, fault)
}

func Test_spending_returns_an_unconverted_scan_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 1, scanFault: errScanFailed}))

	_, err := st.Spending(t.Context(), spendingIn(money.CAD, store.SpendByCategory))

	assertOtherFault(t, err, errScanFailed.Error())
	assert.ErrorIs(t, err, errScanFailed)
}

func Test_cash_flow_returns_the_unconverted_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault(`query rows "SELECT count"`)
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 1, queryFault: fault}))

	_, err := st.CashFlow(t.Context(), cashFlowIn(money.CAD, store.CashFlowByMonth))

	assertOtherFault(t, err, "disk read failed")
	assert.ErrorIs(t, err, fault)
}

func Test_cash_flow_returns_an_unconverted_scan_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 1, scanFault: errScanFailed}))

	_, err := st.CashFlow(t.Context(), cashFlowIn(money.CAD, store.CashFlowByMonth))

	assertOtherFault(t, err, errScanFailed.Error())
	assert.ErrorIs(t, err, errScanFailed)
}
