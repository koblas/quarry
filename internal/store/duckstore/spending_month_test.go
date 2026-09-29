package duckstore_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func monthParams() store.SpendingParams {
	params := spendingParams()
	params.By = store.SpendByMonth
	return params
}

func day(year int, month time.Month, dayOfMonth int) time.Time {
	return time.Date(year, month, dayOfMonth, 0, 0, 0, 0, time.UTC)
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
