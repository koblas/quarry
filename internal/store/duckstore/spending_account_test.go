package duckstore_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	acctSecond = "acct-second"
	acctThird  = "acct-third"
)

// accountRows is spendRows plus two more in-report accounts, so three accounts can each spend.
func accountRows() store.Rows {
	rows := spendRows()
	rows.Accounts = append(rows.Accounts,
		store.Account{ID: acctSecond, SourceID: 3, Name: "Savings", Type: "chequing", Currency: "CAD", Active: true},
		store.Account{ID: acctThird, SourceID: 4, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true})
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
