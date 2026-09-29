package duckstore_test

import (
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
