package duckstore_test

import (
	"os"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	windowSince = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	windowUntil = time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
)

// spendRows is reportRows without its `keep` split, so a test's splits are the only spending.
func spendRows(categories ...store.Category) store.Rows {
	rows := reportRows()
	rows.Transactions, rows.Splits = nil, nil
	rows.Categories = append(rows.Categories, categories...)
	return rows
}

func expenseCategory(id, path string) store.Category {
	return store.Category{ID: id, SourceID: int64(len(id)), Name: path, FullPath: path, Kind: "expense"}
}

func spendingParams() store.SpendingParams {
	return store.SpendingParams{Window: store.Window{Since: windowSince, Until: windowUntil}, By: store.SpendByCategory}
}

func Test_spending_counts_the_windows_first_and_last_day_only(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "before", category: new(catExpense), amount: -100, date: windowSince.AddDate(0, 0, -1)})
	addSplit(&rows, splitSpec{id: "first", category: new(catExpense), amount: -200, date: windowSince})
	addSplit(&rows, splitSpec{id: "last", category: new(catExpense), amount: -400, date: windowUntil})
	addSplit(&rows, splitSpec{id: "after", category: new(catExpense), amount: -800, date: windowUntil.AddDate(0, 0, 1)})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), spendingParams())

	require.NoError(t, err)
	assert.Equal(t, store.Spending{
		Rows:   []store.SpendingRow{{Key: new("Groceries"), Currency: "CAD", Spent: 600}},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 600}},
	}, got)
}

func Test_spending_sorts_categories_ignoring_case_with_uncategorized_first(t *testing.T) {
	t.Parallel()
	rows := spendRows(expenseCategory("cat-zeta", "Zeta"), expenseCategory("cat-alpha", "alpha"))
	addSplit(&rows, splitSpec{id: "zeta-usd", category: new("cat-zeta"), currency: "USD", amount: -10})
	addSplit(&rows, splitSpec{id: "zeta-cad", category: new("cat-zeta"), amount: -20})
	addSplit(&rows, splitSpec{id: "alpha-usd", category: new("cat-alpha"), currency: "USD", amount: -30})
	addSplit(&rows, splitSpec{id: "alpha-cad", category: new("cat-alpha"), amount: -40})
	addSplit(&rows, splitSpec{id: "none", amount: -50})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), spendingParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{
		{Key: nil, Currency: "CAD", Spent: 50},
		{Key: new("alpha"), Currency: "CAD", Spent: 40},
		{Key: new("alpha"), Currency: "USD", Spent: 30},
		{Key: new("Zeta"), Currency: "CAD", Spent: 20},
		{Key: new("Zeta"), Currency: "USD", Spent: 10},
	}, got.Rows)
}

func Test_spending_breaks_a_case_only_tie_by_byte_order_then_currency(t *testing.T) {
	t.Parallel()
	rows := spendRows(expenseCategory("cat-food-upper", "Food"), expenseCategory("cat-food-l", "food"))
	addSplit(&rows, splitSpec{id: "upper-cad", category: new("cat-food-upper"), amount: -10})
	addSplit(&rows, splitSpec{id: "upper-usd", category: new("cat-food-upper"), currency: "USD", amount: -20})
	addSplit(&rows, splitSpec{id: "lower-cad", category: new("cat-food-l"), amount: -30})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), spendingParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{
		{Key: new("Food"), Currency: "CAD", Spent: 10},
		{Key: new("Food"), Currency: "USD", Spent: 20},
		{Key: new("food"), Currency: "CAD", Spent: 30},
	}, got.Rows)
}

func Test_spending_keeps_a_category_that_nets_negative(t *testing.T) {
	t.Parallel()
	rows := spendRows(expenseCategory("cat-refunded", "Refunded"))
	addSplit(&rows, splitSpec{id: "refund", category: new("cat-refunded"), amount: 2500})
	addSplit(&rows, splitSpec{id: "bought", category: new(catExpense), amount: -1000})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), spendingParams())

	require.NoError(t, err)
	assert.Equal(t, store.Spending{
		Rows: []store.SpendingRow{
			{Key: new("Groceries"), Currency: "CAD", Spent: 1000},
			{Key: new("Refunded"), Currency: "CAD", Spent: -2500},
		},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: -1500}},
	}, got)
}

func Test_spending_omits_a_category_that_nets_to_zero_and_keeps_it_in_the_total(t *testing.T) {
	t.Parallel()
	rows := spendRows(expenseCategory("cat-refunded", "Refunded"))
	addSplit(&rows, splitSpec{id: "bought", category: new("cat-refunded"), amount: -500})
	addSplit(&rows, splitSpec{id: "refund", category: new("cat-refunded"), amount: 500})
	addSplit(&rows, splitSpec{id: "usd", category: new(catExpense), currency: "USD", amount: -1000})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), spendingParams())

	require.NoError(t, err)
	assert.Equal(t, store.Spending{
		Rows:   []store.SpendingRow{{Key: new("Groceries"), Currency: "USD", Spent: 1000}},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 0}, {Currency: "USD", Spent: 1000}},
	}, got)
}

func Test_spending_totals_each_currency_cad_before_usd(t *testing.T) {
	t.Parallel()
	rows := spendRows(expenseCategory("cat-rent", "Rent"))
	addSplit(&rows, splitSpec{id: "usd", category: new(catExpense), currency: "USD", amount: -700})
	addSplit(&rows, splitSpec{id: "cad-rent", category: new("cat-rent"), amount: -3000})
	addSplit(&rows, splitSpec{id: "cad-food", category: new(catExpense), amount: -250})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), spendingParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 3250}, {Currency: "USD", Spent: 700}}, got.Totals)
}

func Test_spending_lists_a_categorys_currencies_in_alphabetical_order(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "usd", category: new(catExpense), currency: "USD", amount: -100})
	addSplit(&rows, splitSpec{id: "gbp", category: new(catExpense), currency: "GBP", amount: -200})
	addSplit(&rows, splitSpec{id: "eur", category: new(catExpense), currency: "EUR", amount: -300})
	addSplit(&rows, splitSpec{id: "cad", category: new(catExpense), currency: "CAD", amount: -400})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), spendingParams())

	require.NoError(t, err)
	assert.Equal(t, store.Spending{
		Rows: []store.SpendingRow{
			{Key: new("Groceries"), Currency: "CAD", Spent: 400},
			{Key: new("Groceries"), Currency: "EUR", Spent: 300},
			{Key: new("Groceries"), Currency: "GBP", Spent: 200},
			{Key: new("Groceries"), Currency: "USD", Spent: 100},
		},
		Totals: []store.SpendingTotal{
			{Currency: "CAD", Spent: 400}, {Currency: "EUR", Spent: 300}, {Currency: "GBP", Spent: 200}, {Currency: "USD", Spent: 100},
		},
	}, got)
}

func Test_spending_counts_a_split_dated_after_today_when_the_window_reaches_it(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "future", category: new(catExpense), amount: -700, date: day(2099, time.June, 1)})
	st := newStoreWith(t, rows)
	params := spendingParams()
	params.Window = store.Window{Since: day(2099, time.January, 1), Until: day(2099, time.December, 31)}

	got, err := st.Spending(t.Context(), params)

	require.NoError(t, err)
	assert.Equal(t, store.Spending{
		Rows:   []store.SpendingRow{{Key: new("Groceries"), Currency: "CAD", Spent: 700}},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 700}},
	}, got)
}

func Test_spending_refuses_a_grouping_it_does_not_know(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, spendRows())
	params := spendingParams()
	params.By = store.SpendingGroup(99)

	_, err := st.Spending(t.Context(), params)

	require.ErrorIs(t, err, duckstore.ErrUnsupportedGrouping)
	assert.EqualError(t, err, "spending grouping is not supported: 99")
}

func Test_spending_returns_the_open_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault("open store read-only")
	st := newBuiltStore(t, failingOpener(fault))

	_, err := st.Spending(t.Context(), spendingParams())

	require.ErrorIs(t, err, fault)
}

func Test_spending_returns_the_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault(`query rows "SELECT"`)
	st := newBuiltStore(t, spyOpener(&spyReadDB{queryFault: fault}))

	_, err := st.Spending(t.Context(), spendingParams())

	assertOtherFault(t, err, "disk read failed")
	assert.ErrorIs(t, err, fault)
}

func Test_spending_returns_a_scan_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, spyOpener(&spyReadDB{scanFault: errScanFailed}))

	_, err := st.Spending(t.Context(), spendingParams())

	assertOtherFault(t, err, errScanFailed.Error())
	assert.ErrorIs(t, err, errScanFailed)
}

func Test_spending_closes_the_connection_on_success_and_on_a_query_fault(t *testing.T) {
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

			_, _ = st.Spending(t.Context(), spendingParams())

			assert.Equal(t, 1, spy.closes)
		})
	}
}

func Test_spending_fails_on_a_missing_store_without_creating_it(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := duckstore.New(dir).Spending(t.Context(), spendingParams())

	require.Error(t, err)
	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}
