package duckstore_test

import (
	"os"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
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

const (
	acctSecond = "acct-second"
	acctThird  = "acct-o'brien"
	acctFourth = "acct-fourth"
)

// accountRows is spendRows plus three more in-report accounts, so four accounts can each spend.
func accountRows() store.Rows {
	rows := spendRows()
	rows.Accounts = append(rows.Accounts,
		store.Account{ID: acctSecond, SourceID: 3, Name: "Savings", Type: "chequing", Currency: "CAD", Active: true},
		store.Account{ID: acctThird, SourceID: 4, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
		store.Account{ID: acctFourth, SourceID: 5, Name: "Cash", Type: "cash", Currency: "CAD", Active: true})
	return rows
}

func namedAccounts(params store.SpendingParams, ids ...string) store.SpendingParams {
	params.AccountIDs = ids
	return params
}

func Test_spending_counts_only_the_named_accounts(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "first", account: acctInReports, category: new(catExpense), amount: -100})
	addSplit(&rows, splitSpec{id: "second", account: acctSecond, category: new(catExpense), amount: -200})
	addSplit(&rows, splitSpec{id: "third", account: acctThird, category: new(catExpense), amount: -400})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), namedAccounts(spendingParams(), acctInReports, acctSecond))

	require.NoError(t, err)
	assert.Equal(t, store.Spending{
		Rows:   []store.SpendingRow{{Key: new("Groceries"), Currency: "CAD", Spent: 300}},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 300}},
	}, got)
}

func Test_spending_counts_three_named_accounts_binding_an_id_with_a_quote(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "first", account: acctInReports, category: new(catExpense), amount: -100})
	addSplit(&rows, splitSpec{id: "second", account: acctSecond, category: new(catExpense), amount: -200})
	addSplit(&rows, splitSpec{id: "third", account: acctThird, category: new(catExpense), amount: -400})
	addSplit(&rows, splitSpec{id: "fourth", account: acctFourth, category: new(catExpense), amount: -800})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), namedAccounts(spendingParams(), acctThird, acctInReports, acctSecond))

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 700}}, got.Totals)
}

func Test_spending_by_month_counts_only_the_named_accounts(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "first", account: acctInReports, category: new(catExpense), amount: -100})
	addSplit(&rows, splitSpec{id: "second", account: acctSecond, category: new(catExpense), amount: -200})
	addSplit(&rows, splitSpec{id: "third", account: acctThird, category: new(catExpense), amount: -400})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), namedAccounts(monthParams(), acctSecond, acctThird))

	require.NoError(t, err)
	assert.Equal(t, store.Spending{
		Rows:   []store.SpendingRow{{Key: new("2026-03"), Currency: "CAD", Spent: 600}},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 600}},
	}, got)
}

func Test_spending_by_tag_counts_only_the_named_accounts(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addTag(&rows, "t-trip", "Trip")
	addSplit(&rows, splitSpec{id: "first", account: acctInReports, category: new(catExpense), amount: -100, tags: []string{"t-trip"}})
	addSplit(&rows, splitSpec{id: "second", account: acctSecond, category: new(catExpense), amount: -200, tags: []string{"t-trip"}})
	addSplit(&rows, splitSpec{id: "third", account: acctThird, category: new(catExpense), amount: -400})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), namedAccounts(tagParams(), acctInReports, acctSecond))

	require.NoError(t, err)
	assert.Equal(t, store.Spending{
		Rows:   []store.SpendingRow{{Key: new("Trip"), Currency: "CAD", Spent: 300}},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 300}},
	}, got)
}

func Test_spending_by_tag_counts_multi_tag_splits_only_in_the_named_accounts(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addTag(&rows, "t-trip", "Trip")
	addTag(&rows, "t-work", "Work")
	both := []string{"t-trip", "t-work"}
	addSplit(&rows, splitSpec{id: "named", account: acctInReports, category: new(catExpense), amount: -100, tags: both})
	addSplit(&rows, splitSpec{id: "other-a", account: acctThird, category: new(catExpense), amount: -200, tags: both})
	addSplit(&rows, splitSpec{id: "other-b", account: acctThird, category: new(catExpense), amount: -400, tags: both})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), namedAccounts(tagParams(), acctInReports))

	require.NoError(t, err)
	assert.Equal(t, 1, got.MultiTagSplits)
}

func Test_spending_with_no_named_accounts_counts_every_account(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addTag(&rows, "t-trip", "Trip")
	addTag(&rows, "t-work", "Work")
	addSplit(&rows, splitSpec{id: "first", account: acctInReports, category: new(catExpense), amount: -100, tags: []string{"t-trip", "t-work"}})
	addSplit(&rows, splitSpec{id: "second", account: acctSecond, category: new(catExpense), amount: -200, tags: []string{"t-trip", "t-work"}})
	st := newStoreWith(t, rows)

	byCategory, err := st.Spending(t.Context(), spendingParams())
	require.NoError(t, err)
	byTag, err := st.Spending(t.Context(), tagParams())
	require.NoError(t, err)

	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 300}}, byCategory.Totals)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 300}}, byTag.Totals)
	assert.Equal(t, 2, byTag.MultiTagSplits)
}

func Test_spending_counts_nothing_for_a_named_account_left_out_of_reports(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "counted", account: acctInReports, category: new(catExpense), amount: -100})
	addSplit(&rows, splitSpec{id: "left-out", account: acctNotReports, category: new(catExpense), amount: -200})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), namedAccounts(spendingParams(), acctNotReports))

	require.NoError(t, err)
	assert.Equal(t, store.Spending{}, got)
}

func Test_spending_counts_nothing_for_a_named_account_that_uses_linked_account_tracking(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "counted", account: acctInReports, category: new(catExpense), amount: -100})
	addSplit(&rows, splitSpec{id: "linked", account: acctLinked, category: new(catExpense), amount: -200})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), namedAccounts(spendingParams(), acctLinked))

	require.NoError(t, err)
	assert.Equal(t, store.Spending{}, got)
}

func Test_spending_counts_a_named_accounts_splits_inside_the_window_only(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "inside", account: acctInReports, category: new(catExpense), amount: -100})
	addSplit(&rows, splitSpec{id: "before", account: acctInReports, category: new(catExpense), amount: -200, date: windowSince.AddDate(0, 0, -1)})
	addSplit(&rows, splitSpec{id: "after", account: acctInReports, category: new(catExpense), amount: -400, date: windowUntil.Add(24 * time.Hour)})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), namedAccounts(spendingParams(), acctInReports, acctThird))

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 100}}, got.Totals)
}

// Rates: Friday 2026-03-13 at 1.25 and Monday 2026-03-16 at 1.30 CAD per USD; 2026-03-10
// is before the first rate.

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
	assert.Equal(t, store.Unconverted{FirstRate: march(13)}, got.Unconverted)
	got.Unconverted = native.Unconverted
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
	assert.Equal(t, spendingRowsOf(
		store.SpendingRow{Key: new("Groceries"), Currency: "CAD", Spent: 700},
		store.SpendingRow{Key: new("Groceries"), Currency: "USD", Spent: 500}), got.Rows)
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

func Test_spending_by_tag_in_usd_keeps_an_unrated_cad_split_native_ahead_of_the_usd_total(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	addTag(&rows, "t-trip", "Trip")
	addSplit(&rows, splitSpec{id: "cad-unrated", date: march(10), category: new(catExpense), amount: -700, tags: []string{"t-trip"}})
	addSplit(&rows, splitSpec{id: "usd", account: acctUSD, currency: "USD", date: march(16), category: new(catExpense), amount: -500, tags: []string{"t-trip"}})
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.Spending(t.Context(), spendingIn(money.USD, store.SpendByTag))

	require.NoError(t, err)
	assert.Equal(t, spendingRowsOf(
		store.SpendingRow{Key: new("Trip"), Currency: "CAD", Spent: 700},
		store.SpendingRow{Key: new("Trip"), Currency: "USD", Spent: 500}), got.Rows)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 700}, {Currency: "USD", Spent: 500}}, got.Totals)
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

func Test_spending_by_month_keeps_an_unrated_split_native_beside_the_converted_ones(t *testing.T) {
	t.Parallel()
	rows := fxSpendRows()
	expense(&rows, "usd-rated", "USD", march(16), -1000)
	expense(&rows, "usd-unrated", "USD", march(10), -700)
	st := newStoreWithRates(t, rows, fridayAndMonday()...)

	got, err := st.Spending(t.Context(), spendingIn(money.CAD, store.SpendByMonth))

	require.NoError(t, err)
	assert.Equal(t, spendingRowsOf(
		store.SpendingRow{Key: new("2026-03"), Currency: "CAD", Spent: 1300},
		store.SpendingRow{Key: new("2026-03"), Currency: "USD", Spent: 700}), got.Rows)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 1300}, {Currency: "USD", Spent: 700}}, got.Totals)
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

func monthParams() store.SpendingParams {
	params := spendingParams()
	params.By = store.SpendByMonth
	return params
}

func Test_spending_by_month_groups_each_currency_by_calendar_month(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "mar-first", category: new(catExpense), amount: -100, date: day(2026, time.March, 1)})
	addSplit(&rows, splitSpec{id: "mar-last", category: new(catExpense), amount: -200, date: day(2026, time.March, 31)})
	addSplit(&rows, splitSpec{id: "mar-usd", category: new(catExpense), currency: "USD", amount: -400, date: day(2026, time.March, 15)})
	addSplit(&rows, splitSpec{id: "apr", category: new(catExpense), amount: -800, date: day(2026, time.April, 1)})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), monthParams())

	require.NoError(t, err)
	assert.Equal(t, store.Spending{
		Rows: []store.SpendingRow{
			{Key: new("2026-03"), Currency: "CAD", Spent: 300},
			{Key: new("2026-03"), Currency: "USD", Spent: 400},
			{Key: new("2026-04"), Currency: "CAD", Spent: 800},
		},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 1100}, {Currency: "USD", Spent: 400}},
	}, got)
}

func Test_spending_by_month_sorts_across_a_year_end_by_month_then_currency(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "jan-cad", category: new(catExpense), amount: -10, date: day(2026, time.January, 5)})
	addSplit(&rows, splitSpec{id: "dec-usd", category: new(catExpense), currency: "USD", amount: -20, date: day(2025, time.December, 5)})
	addSplit(&rows, splitSpec{id: "dec-cad", category: new(catExpense), amount: -30, date: day(2025, time.December, 6)})
	st := newStoreWith(t, rows)
	params := monthParams()
	params.Window.Since = day(2025, time.December, 1)

	got, err := st.Spending(t.Context(), params)

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{
		{Key: new("2025-12"), Currency: "CAD", Spent: 30},
		{Key: new("2025-12"), Currency: "USD", Spent: 20},
		{Key: new("2026-01"), Currency: "CAD", Spent: 10},
	}, got.Rows)
}

func Test_spending_by_month_lists_a_months_currencies_in_alphabetical_order(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "usd", category: new(catExpense), currency: "USD", amount: -100})
	addSplit(&rows, splitSpec{id: "gbp", category: new(catExpense), currency: "GBP", amount: -200})
	addSplit(&rows, splitSpec{id: "eur", category: new(catExpense), currency: "EUR", amount: -300})
	addSplit(&rows, splitSpec{id: "cad", category: new(catExpense), currency: "CAD", amount: -400})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), monthParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{
		{Key: new("2026-03"), Currency: "CAD", Spent: 400},
		{Key: new("2026-03"), Currency: "EUR", Spent: 300},
		{Key: new("2026-03"), Currency: "GBP", Spent: 200},
		{Key: new("2026-03"), Currency: "USD", Spent: 100},
	}, got.Rows)
	assert.Equal(t, []store.SpendingTotal{
		{Currency: "CAD", Spent: 400}, {Currency: "EUR", Spent: 300}, {Currency: "GBP", Spent: 200}, {Currency: "USD", Spent: 100},
	}, got.Totals)
}

func Test_spending_by_month_omits_a_month_that_nets_to_zero_and_keeps_it_in_the_total(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "feb-out", category: new(catExpense), amount: -500, date: day(2026, time.February, 3)})
	addSplit(&rows, splitSpec{id: "feb-back", category: new(catExpense), amount: 500, date: day(2026, time.February, 4)})
	addSplit(&rows, splitSpec{id: "mar", category: new(catExpense), amount: -300, date: day(2026, time.March, 4)})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), monthParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{{Key: new("2026-03"), Currency: "CAD", Spent: 300}}, got.Rows)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 300}}, got.Totals)
}

func Test_spending_by_month_counts_the_windows_first_and_last_day_only(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "before", category: new(catExpense), amount: -100, date: windowSince.AddDate(0, 0, -1)})
	addSplit(&rows, splitSpec{id: "first", category: new(catExpense), amount: -200, date: windowSince})
	addSplit(&rows, splitSpec{id: "last", category: new(catExpense), amount: -400, date: windowUntil})
	addSplit(&rows, splitSpec{id: "after", category: new(catExpense), amount: -800, date: windowUntil.AddDate(0, 0, 1)})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), monthParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{
		{Key: new("2026-01"), Currency: "CAD", Spent: 200},
		{Key: new("2026-09"), Currency: "CAD", Spent: 400},
	}, got.Rows)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 600}}, got.Totals)
}

func payeeParams() store.SpendingParams {
	params := spendingParams()
	params.By = store.SpendByPayee
	return params
}

// addPayees registers payees named names, each with id "payee-"+name.
func addPayees(rows *store.Rows, names ...string) {
	for _, name := range names {
		rows.Payees = append(rows.Payees, store.Payee{ID: "payee-" + name, SourceID: int64(len(rows.Payees) + 1), Name: name})
	}
}

func payeeSplit(id, payee, currency string, amount int64) splitSpec {
	spec := splitSpec{id: id, category: new(catExpense), currency: currency, amount: amount}
	if payee != "" {
		spec.payee = new("payee-" + payee)
	}
	return spec
}

func Test_spending_by_payee_lists_each_currencys_biggest_payee_first_and_a_refunded_one_last(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addPayees(&rows, "Zed", "Ann", "Refunder")
	addSplit(&rows, payeeSplit("zed-cad", "Zed", "CAD", -500))
	addSplit(&rows, payeeSplit("ann-cad", "Ann", "CAD", -900))
	addSplit(&rows, payeeSplit("none-cad", "", "CAD", -700))
	addSplit(&rows, payeeSplit("refund-cad", "Refunder", "CAD", 200))
	addSplit(&rows, payeeSplit("zed-usd", "Zed", "USD", -100))
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), payeeParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{
		{Key: new("Ann"), Currency: "CAD", Spent: 900},
		{Key: nil, Currency: "CAD", Spent: 700},
		{Key: new("Zed"), Currency: "CAD", Spent: 500},
		{Key: new("Refunder"), Currency: "CAD", Spent: -200},
		{Key: new("Zed"), Currency: "USD", Spent: 100},
	}, got.Rows)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 1900}, {Currency: "USD", Spent: 100}}, got.Totals)
}

func Test_spending_by_payee_breaks_a_tie_by_name_ignoring_case(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addPayees(&rows, "banana", "Cherry", "apple")
	addSplit(&rows, payeeSplit("banana", "banana", "CAD", -100))
	addSplit(&rows, payeeSplit("cherry", "Cherry", "CAD", -100))
	addSplit(&rows, payeeSplit("apple", "apple", "CAD", -100))
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), payeeParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{
		{Key: new("apple"), Currency: "CAD", Spent: 100},
		{Key: new("banana"), Currency: "CAD", Spent: 100},
		{Key: new("Cherry"), Currency: "CAD", Spent: 100},
	}, got.Rows)
}

func Test_spending_by_payee_breaks_a_case_only_tie_by_byte_order(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addPayees(&rows, "alpha", "Alpha")
	addSplit(&rows, payeeSplit("lower", "alpha", "CAD", -100))
	addSplit(&rows, payeeSplit("upper", "Alpha", "CAD", -100))
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), payeeParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{
		{Key: new("Alpha"), Currency: "CAD", Spent: 100},
		{Key: new("alpha"), Currency: "CAD", Spent: 100},
	}, got.Rows)
}

func Test_spending_by_payee_sorts_no_payee_after_a_named_payee_it_ties_with(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addPayees(&rows, "Zed")
	addSplit(&rows, payeeSplit("none", "", "CAD", -100))
	addSplit(&rows, payeeSplit("zed", "Zed", "CAD", -100))
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), payeeParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{
		{Key: new("Zed"), Currency: "CAD", Spent: 100},
		{Key: nil, Currency: "CAD", Spent: 100},
	}, got.Rows)
}

func Test_spending_by_payee_drops_a_payee_that_nets_to_zero(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addPayees(&rows, "Ann", "Even")
	addSplit(&rows, payeeSplit("ann", "Ann", "CAD", -300))
	addSplit(&rows, payeeSplit("even-out", "Even", "CAD", -50))
	addSplit(&rows, payeeSplit("even-back", "Even", "CAD", 50))
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), payeeParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{{Key: new("Ann"), Currency: "CAD", Spent: 300}}, got.Rows)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 300}}, got.Totals)
}

// emptyWindowParams is a window before any transaction of minimalRows.
func emptyWindowParams() store.SpendingParams {
	return store.SpendingParams{Window: store.Window{Since: day(1990, 1, 1), Until: day(1990, 1, 31)}}
}

func Test_spending_gives_the_store_transaction_range_when_the_window_holds_no_spending(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "income", category: new(catIncome), amount: 500, date: day(2003, 1, 4)})
	addSplit(&rows, splitSpec{id: "late", account: acctNotReports, category: new(catExpense), amount: -100, date: day(2025, 12, 31)})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), spendingParams())

	require.NoError(t, err)
	assert.Equal(t, store.TransactionRange{First: day(2003, 1, 4), Last: day(2025, 12, 31)}, got.Transactions)
	assert.Empty(t, got.Totals)
}

func Test_spending_gives_a_zero_range_when_the_store_has_no_transactions(t *testing.T) {
	t.Parallel()
	st := newStoreWith(t, spendRows())

	got, err := st.Spending(t.Context(), spendingParams())

	require.NoError(t, err)
	assert.Zero(t, got.Transactions)
}

func Test_spending_leaves_the_transaction_range_unset_when_the_window_holds_spending(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "old", category: new(catExpense), amount: -100, date: day(2003, 1, 4)})
	addSplit(&rows, splitSpec{id: "now", category: new(catExpense), amount: -200})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), spendingParams())

	require.NoError(t, err)
	assert.Zero(t, got.Transactions)
	assert.NotEmpty(t, got.Totals)
}

func Test_spending_ranges_over_the_reported_accounts_it_is_named_for(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "left-out", account: acctNotReports, category: new(catExpense), amount: -100, date: day(2001, 5, 5)})
	addSplit(&rows, splitSpec{id: "linked", account: acctLinked, category: new(catExpense), amount: -100, date: day(1999, 1, 1)})
	addSplit(&rows, splitSpec{id: "first", account: acctInReports, category: new(catExpense), amount: -100, date: day(2019, 3, 2)})
	addSplit(&rows, splitSpec{id: "last", account: acctInReports, category: new(catExpense), amount: -100, date: day(2024, 11, 30)})
	addSplit(&rows, splitSpec{id: "unnamed", account: acctSecond, category: new(catExpense), amount: -100, date: day(2010, 2, 2)})
	addSplit(&rows, splitSpec{id: "unnamed-late", account: acctSecond, category: new(catExpense), amount: -100, date: day(2025, 8, 8)})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), namedAccounts(spendingParams(), acctNotReports, acctLinked, acctInReports))

	require.NoError(t, err)
	assert.Equal(t, store.TransactionRange{First: day(2019, 3, 2), Last: day(2024, 11, 30)}, got.Transactions)
}

func Test_spending_gives_a_zero_range_when_every_named_account_is_left_out_of_reports(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "left-out", account: acctNotReports, category: new(catExpense), amount: -100, date: day(2001, 5, 5)})
	addSplit(&rows, splitSpec{id: "unnamed", account: acctSecond, category: new(catExpense), amount: -100, date: day(2010, 2, 2)})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), namedAccounts(spendingParams(), acctNotReports))

	require.NoError(t, err)
	assert.Zero(t, got.Transactions)
}

func Test_spending_gives_a_zero_range_when_every_named_account_uses_linked_account_tracking(t *testing.T) {
	t.Parallel()
	rows := accountRows()
	addSplit(&rows, splitSpec{id: "linked", account: acctLinked, category: new(catExpense), amount: -100, date: day(2001, 5, 5)})
	addSplit(&rows, splitSpec{id: "unnamed", account: acctSecond, category: new(catExpense), amount: -100, date: day(2010, 2, 2)})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), namedAccounts(spendingParams(), acctLinked))

	require.NoError(t, err)
	assert.Zero(t, got.Transactions)
}

func Test_spending_returns_the_transaction_range_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault(`query rows "SELECT min"`)
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 1, queryFault: fault}))

	_, err := st.Spending(t.Context(), emptyWindowParams())

	assertOtherFault(t, err, "disk read failed")
	assert.ErrorIs(t, err, fault)
}

func Test_spending_returns_a_transaction_range_scan_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 1, scanFault: errScanFailed}))

	_, err := st.Spending(t.Context(), emptyWindowParams())

	assertOtherFault(t, err, errScanFailed.Error())
	assert.ErrorIs(t, err, errScanFailed)
}

func tagParams() store.SpendingParams {
	params := spendingParams()
	params.By = store.SpendByTag
	return params
}

func addTag(rows *store.Rows, id, name string) {
	rows.Tags = append(rows.Tags, store.Tag{ID: id, SourceID: int64(len(rows.Tags) + 1), Name: name})
}

func Test_spending_by_tag_counts_a_two_tag_split_under_both_tags_and_once_in_the_total(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addTag(&rows, "t-trip", "Trip")
	addTag(&rows, "t-work", "Work")
	addSplit(&rows, splitSpec{id: "both", category: new(catExpense), amount: -1000, tags: []string{"t-trip", "t-work"}})
	addSplit(&rows, splitSpec{id: "trip-only", category: new(catExpense), amount: -500, tags: []string{"t-trip"}})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), tagParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{
		{Key: new("Trip"), Currency: "CAD", Spent: 1500},
		{Key: new("Work"), Currency: "CAD", Spent: 1000},
	}, got.Rows)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 1500}}, got.Totals)
}

func Test_spending_by_tag_groups_untagged_splits_under_a_nil_key_first(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addTag(&rows, "t-trip", "Trip")
	addSplit(&rows, splitSpec{id: "tagged", category: new(catExpense), amount: -100, tags: []string{"t-trip"}})
	addSplit(&rows, splitSpec{id: "untagged", category: new(catExpense), amount: -300})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), tagParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{
		{Key: nil, Currency: "CAD", Spent: 300},
		{Key: new("Trip"), Currency: "CAD", Spent: 100},
	}, got.Rows)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 400}}, got.Totals)
}

func Test_spending_by_tag_treats_a_link_to_a_missing_tag_as_untagged(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addSplit(&rows, splitSpec{id: "dangling", category: new(catExpense), amount: -200, tags: []string{"t-gone"}})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), tagParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{{Key: nil, Currency: "CAD", Spent: 200}}, got.Rows)
}

func Test_spending_by_tag_sorts_ignoring_case_then_byte_order_then_currency(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addTag(&rows, "t-zed", "Zed")
	addTag(&rows, "t-lower", "alpha")
	addTag(&rows, "t-upper", "Alpha")
	addSplit(&rows, splitSpec{id: "zed", category: new(catExpense), amount: -10, tags: []string{"t-zed"}})
	addSplit(&rows, splitSpec{id: "lower-usd", category: new(catExpense), currency: "USD", amount: -20, tags: []string{"t-lower"}})
	addSplit(&rows, splitSpec{id: "lower-cad", category: new(catExpense), amount: -30, tags: []string{"t-lower"}})
	addSplit(&rows, splitSpec{id: "upper", category: new(catExpense), amount: -40, tags: []string{"t-upper"}})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), tagParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{
		{Key: new("Alpha"), Currency: "CAD", Spent: 40},
		{Key: new("alpha"), Currency: "CAD", Spent: 30},
		{Key: new("alpha"), Currency: "USD", Spent: 20},
		{Key: new("Zed"), Currency: "CAD", Spent: 10},
	}, got.Rows)
}

func Test_spending_by_tag_totals_each_currency_cad_before_usd(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addTag(&rows, "t-trip", "Trip")
	addSplit(&rows, splitSpec{id: "usd", category: new(catExpense), currency: "USD", amount: -700, tags: []string{"t-trip"}})
	addSplit(&rows, splitSpec{id: "cad", category: new(catExpense), amount: -250, tags: []string{"t-trip"}})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), tagParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingTotal{{Currency: "CAD", Spent: 250}, {Currency: "USD", Spent: 700}}, got.Totals)
}

func Test_spending_by_tag_lists_a_tags_currencies_in_alphabetical_order(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addTag(&rows, "t-trip", "Trip")
	addSplit(&rows, splitSpec{id: "usd", category: new(catExpense), currency: "USD", amount: -100, tags: []string{"t-trip"}})
	addSplit(&rows, splitSpec{id: "gbp", category: new(catExpense), currency: "GBP", amount: -200, tags: []string{"t-trip"}})
	addSplit(&rows, splitSpec{id: "eur", category: new(catExpense), currency: "EUR", amount: -300, tags: []string{"t-trip"}})
	addSplit(&rows, splitSpec{id: "cad", category: new(catExpense), currency: "CAD", amount: -400, tags: []string{"t-trip"}})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), tagParams())

	require.NoError(t, err)
	assert.Equal(t, []store.SpendingRow{
		{Key: new("Trip"), Currency: "CAD", Spent: 400},
		{Key: new("Trip"), Currency: "EUR", Spent: 300},
		{Key: new("Trip"), Currency: "GBP", Spent: 200},
		{Key: new("Trip"), Currency: "USD", Spent: 100},
	}, got.Rows)
	assert.Equal(t, []store.SpendingTotal{
		{Currency: "CAD", Spent: 400}, {Currency: "EUR", Spent: 300}, {Currency: "GBP", Spent: 200}, {Currency: "USD", Spent: 100},
	}, got.Totals)
}

func Test_spending_by_tag_omits_a_tag_that_nets_to_zero_and_keeps_it_in_the_total(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addTag(&rows, "t-refund", "Refund")
	addSplit(&rows, splitSpec{id: "bought", category: new(catExpense), currency: "USD", amount: -500, tags: []string{"t-refund"}})
	addSplit(&rows, splitSpec{id: "refund", category: new(catExpense), currency: "USD", amount: 500, tags: []string{"t-refund"}})
	addSplit(&rows, splitSpec{id: "cad", category: new(catExpense), amount: -100})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), tagParams())

	require.NoError(t, err)
	assert.Equal(t, store.Spending{
		Rows:   []store.SpendingRow{{Key: nil, Currency: "CAD", Spent: 100}},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 100}, {Currency: "USD", Spent: 0}},
	}, got)
}

func Test_spending_by_tag_counts_a_split_once_under_two_tags_sharing_a_name(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addTag(&rows, "t-first", "Trip")
	addTag(&rows, "t-second", "Trip")
	addSplit(&rows, splitSpec{id: "both", category: new(catExpense), amount: -1000, tags: []string{"t-first", "t-second"}})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), tagParams())

	require.NoError(t, err)
	assert.Equal(t, store.Spending{
		Rows:   []store.SpendingRow{{Key: new("Trip"), Currency: "CAD", Spent: 1000}},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 1000}},
	}, got)
}

func Test_spending_by_tag_counts_splits_with_several_tags_not_their_tags(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addTag(&rows, "t-a", "A")
	addTag(&rows, "t-b", "B")
	addTag(&rows, "t-c", "C")
	addSplit(&rows, splitSpec{id: "three", category: new(catExpense), amount: -10, tags: []string{"t-a", "t-b", "t-c"}})
	addSplit(&rows, splitSpec{id: "two", category: new(catExpense), amount: -10, tags: []string{"t-a", "t-b"}})
	addSplit(&rows, splitSpec{id: "one", category: new(catExpense), amount: -10, tags: []string{"t-a"}})
	addSplit(&rows, splitSpec{id: "outside", category: new(catExpense), amount: -10, date: windowSince.AddDate(0, 0, -1), tags: []string{"t-a", "t-b"}})
	addSplit(&rows, splitSpec{id: "income", category: new(catIncome), amount: 10, tags: []string{"t-a", "t-b"}})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), tagParams())

	require.NoError(t, err)
	assert.Equal(t, 2, got.MultiTagSplits)
}

func Test_spending_by_tag_counts_the_windows_first_and_last_day_only(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addTag(&rows, "t-a", "A")
	addTag(&rows, "t-b", "B")
	both := []string{"t-a", "t-b"}
	addSplit(&rows, splitSpec{id: "before", category: new(catExpense), amount: -100, date: windowSince.AddDate(0, 0, -1), tags: both})
	addSplit(&rows, splitSpec{id: "first", category: new(catExpense), amount: -200, date: windowSince, tags: both})
	addSplit(&rows, splitSpec{id: "last", category: new(catExpense), amount: -400, date: windowUntil, tags: both})
	addSplit(&rows, splitSpec{id: "after", category: new(catExpense), amount: -800, date: windowUntil.AddDate(0, 0, 1), tags: both})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), tagParams())

	require.NoError(t, err)
	assert.Equal(t, store.Spending{
		Rows: []store.SpendingRow{
			{Key: new("A"), Currency: "CAD", Spent: 600},
			{Key: new("B"), Currency: "CAD", Spent: 600},
		},
		Totals:         []store.SpendingTotal{{Currency: "CAD", Spent: 600}},
		MultiTagSplits: 2,
	}, got)
}

func Test_spending_by_tag_counts_one_multi_tag_split_as_one(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addTag(&rows, "t-a", "A")
	addTag(&rows, "t-b", "B")
	addSplit(&rows, splitSpec{id: "two", category: new(catExpense), amount: -10, tags: []string{"t-a", "t-b"}})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), tagParams())

	require.NoError(t, err)
	assert.Equal(t, 1, got.MultiTagSplits)
}

func Test_spending_by_tag_counts_no_multi_tag_split_when_every_split_has_at_most_one_tag(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addTag(&rows, "t-a", "A")
	addSplit(&rows, splitSpec{id: "one", category: new(catExpense), amount: -10, tags: []string{"t-a"}})
	addSplit(&rows, splitSpec{id: "none", category: new(catExpense), amount: -10})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), tagParams())

	require.NoError(t, err)
	assert.Zero(t, got.MultiTagSplits)
}

func Test_spending_by_tag_does_not_count_a_split_with_one_real_tag_and_a_link_to_a_missing_tag(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addTag(&rows, "t-a", "A")
	addSplit(&rows, splitSpec{id: "dangling", category: new(catExpense), amount: -10, tags: []string{"t-a", "t-gone"}})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), tagParams())

	require.NoError(t, err)
	assert.Zero(t, got.MultiTagSplits)
}

func Test_spending_by_tag_does_not_count_a_split_whose_two_tags_share_a_name(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addTag(&rows, "t-first", "Trip")
	addTag(&rows, "t-second", "Trip")
	addSplit(&rows, splitSpec{id: "both", category: new(catExpense), amount: -10, tags: []string{"t-first", "t-second"}})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), tagParams())

	require.NoError(t, err)
	assert.Zero(t, got.MultiTagSplits)
}

func Test_spending_by_category_reports_no_multi_tag_splits(t *testing.T) {
	t.Parallel()
	rows := spendRows()
	addTag(&rows, "t-a", "A")
	addTag(&rows, "t-b", "B")
	addSplit(&rows, splitSpec{id: "two", category: new(catExpense), amount: -10, tags: []string{"t-a", "t-b"}})
	st := newStoreWith(t, rows)

	got, err := st.Spending(t.Context(), spendingParams())

	require.NoError(t, err)
	assert.Zero(t, got.MultiTagSplits)
}

func Test_spending_by_tag_returns_the_multi_tag_count_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault(`query rows "SELECT count"`)
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 1, queryFault: fault}))

	_, err := st.Spending(t.Context(), tagParams())

	assertOtherFault(t, err, "disk read failed")
	assert.ErrorIs(t, err, fault)
}

func Test_spending_by_tag_returns_a_multi_tag_count_scan_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 1, scanFault: errScanFailed}))

	_, err := st.Spending(t.Context(), tagParams())

	assertOtherFault(t, err, errScanFailed.Error())
	assert.ErrorIs(t, err, errScanFailed)
}

func Test_spending_by_tag_closes_the_connection_after_a_multi_tag_count_fault(t *testing.T) {
	t.Parallel()
	spy := &spyReadDB{passQueries: 1, queryFault: errQueryFailed}
	st := newBuiltStore(t, spyOpener(spy))

	_, _ = st.Spending(t.Context(), tagParams())

	assert.Equal(t, 1, spy.closes)
}
