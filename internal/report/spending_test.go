package report_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_default_window_runs_from_january_first_to_today_in_the_instants_own_zone(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
		want store.Window
	}{
		{
			name: "mid-year",
			now:  time.Date(2026, 9, 29, 22, 0, 0, 0, time.FixedZone("UTC-5", -5*60*60)),
			want: store.Window{Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)},
		},
		{
			name: "an instant already in the next year in UTC",
			now:  time.Date(2025, 12, 31, 22, 0, 0, 0, time.FixedZone("UTC-5", -5*60*60)),
			want: store.Window{Since: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Until: time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, report.DefaultWindow(c.now))
		})
	}
}

func Test_spend_reads_the_requested_window_by_category(t *testing.T) {
	window := store.Window{Since: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Until: time.Date(2024, 5, 31, 0, 0, 0, 0, time.UTC)}
	groceries := "Groceries"
	spending := store.Spending{
		Rows:   []store.SpendingRow{{Key: &groceries, Currency: "CAD", Spent: 1250}},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 1250}},
	}
	var got store.SpendingParams
	srv := report.NewServer(report.WithStore(fakeStore{spending: spending, gotSpending: &got}))

	result, err := srv.Spend(t.Context(), report.SpendRequest{Window: window})

	require.NoError(t, err)
	assert.Equal(t, store.SpendingParams{Window: window, By: store.SpendByCategory}, got)
	assert.Equal(t, report.Spending{
		Rows:   []report.SpendingRow{{SpendingRow: spending.Rows[0]}},
		Totals: spending.Totals,
		Window: window,
	}, result)
}

func Test_spend_reads_the_requested_grouping_and_returns_it_with_the_result(t *testing.T) {
	var got store.SpendingParams
	srv := report.NewServer(report.WithStore(fakeStore{gotSpending: &got}))

	result, err := srv.Spend(t.Context(), report.SpendRequest{By: store.SpendByPayee})

	require.NoError(t, err)
	assert.Equal(t, store.SpendByPayee, got.By)
	assert.Equal(t, store.SpendByPayee, result.By)
}

func Test_spend_reads_the_requested_currency_and_returns_it_with_the_result(t *testing.T) {
	var got store.SpendingParams
	srv := report.NewServer(report.WithStore(fakeStore{gotSpending: &got}))

	result, err := srv.Spend(t.Context(), report.SpendRequest{Currency: money.USD})

	require.NoError(t, err)
	assert.Equal(t, money.USD, got.Currency)
	assert.Equal(t, money.USD, result.Currency)
}

func Test_spend_returns_the_store_fault(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Spend(t.Context(), report.SpendRequest{})

	assert.Equal(t, errDiskRead, err)
}

func spendAccounts(t *testing.T, list store.AccountList, names ...string) (report.Spending, store.SpendingParams, error) {
	t.Helper()
	var got store.SpendingParams
	srv := report.NewServer(report.WithStore(fakeStore{accounts: list, gotSpending: &got}))

	result, err := srv.Spend(t.Context(), report.SpendRequest{Accounts: names})

	return result, got, err
}

func Test_spend_passes_every_named_account_to_the_store_in_the_order_given(t *testing.T) {
	list := accountsOf(chequing, savings, oldCard)

	result, got, err := spendAccounts(t, list, "acct-300", "Chequing")

	require.NoError(t, err)
	assert.Equal(t, []string{"acct-300", "acct-100"}, got.AccountIDs)
	assert.Equal(t, []store.Account{oldCard, chequing}, result.Accounts)
}

func Test_spend_passes_the_id_of_the_account_the_argument_points_at(t *testing.T) {
	cases := []struct {
		name string
		list store.AccountList
		arg  string
		want []string
	}{
		{name: "a_name_ignoring_case", list: accountsOf(chequing, savings), arg: "cHEQUING", want: []string{"acct-100"}},
		{name: "a_closed_account", list: accountsOf(chequing, oldCard), arg: "old card", want: []string{"acct-300"}},
		{
			name: "an_id_before_a_name_equal_to_it",
			list: accountsOf(
				store.Account{ID: "acct-1", Name: "acct-2"},
				store.Account{ID: "acct-2", Name: "Other"},
			),
			arg:  "acct-2",
			want: []string{"acct-2"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, got, err := spendAccounts(t, c.list, c.arg)

			require.NoError(t, err)
			assert.Equal(t, c.want, got.AccountIDs)
		})
	}
}

func Test_spend_names_an_account_given_by_id_and_by_name_once(t *testing.T) {
	list := accountsOf(savings, chequing)

	result, got, err := spendAccounts(t, list, "chequing", "acct-100", "Chequing", "Savings")

	require.NoError(t, err)
	assert.Equal(t, []string{"acct-100", "acct-200"}, got.AccountIDs)
	assert.Equal(t, []store.Account{chequing, savings}, result.Accounts)
}

func Test_spend_refuses_an_unknown_account(t *testing.T) {
	list := accountsOf(chequing)

	_, _, err := spendAccounts(t, list, "Chequeing")

	refusal, ok := errors.AsType[report.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, `no account named "Chequeing"; run quarry accounts --all to list them`, refusal.Error())
	assert.NoError(t, errors.Unwrap(refusal))
}

func Test_spend_refuses_an_ambiguous_name_listing_its_ids_sorted(t *testing.T) {
	list := accountsOf(
		store.Account{ID: "acct-977", Name: "Visa"},
		store.Account{ID: "acct-812", Name: "visa"},
		chequing,
	)

	_, _, err := spendAccounts(t, list, "VISA")

	refusal, ok := errors.AsType[report.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, `2 accounts are named "VISA"; pass one of their ids instead: acct-812, acct-977`, refusal.Error())
}

func Test_spend_refuses_an_unknown_account_with_the_callers_text_as_its_part(t *testing.T) {
	list := accountsOf(chequing)

	_, _, err := spendAccounts(t, list, "Chequeing")

	refusal, ok := errors.AsType[report.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, report.RefusalUnknownAccount, refusal.Kind)
	assert.Equal(t, "Chequeing", refusal.Arg)
	assert.Empty(t, refusal.IDs)
}

func Test_spend_refuses_an_ambiguous_name_with_its_text_and_sorted_ids_as_parts(t *testing.T) {
	list := accountsOf(
		store.Account{ID: "acct-977", Name: "Visa"},
		store.Account{ID: "acct-812", Name: "visa"},
		chequing,
	)

	_, _, err := spendAccounts(t, list, "VISA")

	refusal, ok := errors.AsType[report.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, report.RefusalAmbiguousAccount, refusal.Kind)
	assert.Equal(t, "VISA", refusal.Arg)
	assert.Equal(t, []string{"acct-812", "acct-977"}, refusal.IDs)
}

func Test_spend_refuses_the_first_argument_that_points_at_no_account(t *testing.T) {
	cases := []struct {
		name string
		list store.AccountList
		args []string
		want string
	}{
		{
			name: "an_empty_argument_even_when_an_account_has_an_empty_name",
			list: accountsOf(chequing, store.Account{ID: "acct-400", Name: ""}),
			args: []string{""},
			want: `no account named ""; run quarry accounts --all to list them`,
		},
		{
			name: "the_first_account_it_cannot_pick",
			list: accountsOf(chequing),
			args: []string{"Chequing", "Missing", "Absent"},
			want: `no account named "Missing"; run quarry accounts --all to list them`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := spendAccounts(t, c.list, c.args...)

			assert.EqualError(t, err, c.want)
		})
	}
}

func Test_spend_does_not_ask_the_store_for_spending_when_it_refuses_an_account(t *testing.T) {
	list := accountsOf(chequing)
	var got store.SpendingParams
	srv := report.NewServer(report.WithStore(fakeStore{accounts: list, gotSpending: &got}))

	_, err := srv.Spend(t.Context(), report.SpendRequest{Accounts: []string{"Missing"}})

	require.Error(t, err)
	assert.Equal(t, store.SpendingParams{}, got)
}

func Test_spend_does_not_read_accounts_when_none_is_named(t *testing.T) {
	var reads int
	srv := report.NewServer(report.WithStore(fakeStore{accountsReads: &reads}))

	result, err := srv.Spend(t.Context(), report.SpendRequest{})

	require.NoError(t, err)
	assert.Zero(t, reads)
	assert.Empty(t, result.Accounts)
}

func Test_spend_refuses_when_the_accounts_read_fails_to_open_the_store(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))

	_, err := srv.Spend(t.Context(), report.SpendRequest{Accounts: []string{"Chequing"}})

	assert.EqualError(t, err, missingStoreRefusal)
}

func Test_spend_reports_an_interrupt_during_the_accounts_read(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Spend(ctx, report.SpendRequest{Accounts: []string{"Chequing"}})

	assert.EqualError(t, err, "spend interrupted")
}

func Test_spend_reports_the_transaction_range_the_store_found(t *testing.T) {
	span := store.TransactionRange{
		First: time.Date(2003, 1, 4, 0, 0, 0, 0, time.UTC),
		Last:  time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
	}
	srv := report.NewServer(report.WithStore(fakeStore{spending: store.Spending{Transactions: span}}))

	got, err := srv.Spend(t.Context(), report.SpendRequest{})

	require.NoError(t, err)
	assert.Equal(t, span, got.Transactions)
}

func spendByMonth(t *testing.T, spending store.Spending, since, until time.Time) report.Spending {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{spending: spending}))

	result, err := srv.Spend(t.Context(), report.SpendRequest{
		Window: store.Window{Since: since, Until: until},
		By:     store.SpendByMonth,
	})

	require.NoError(t, err)
	return result
}

// cadOnly is a spending with a CAD total, so a month series has a currency to fill.
func cadOnly() store.Spending {
	return store.Spending{Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 1}}}
}

func partialFlags(result report.Spending) []bool {
	flags := make([]bool, len(result.Rows))
	for i, r := range result.Rows {
		flags[i] = r.Partial
	}
	return flags
}

func monthLabels(result report.Spending) []string {
	labels := make([]string, len(result.Rows))
	for i, r := range result.Rows {
		labels[i] = *r.Key
	}
	return labels
}

func Test_spend_by_month_marks_a_month_partial_only_when_the_window_cuts_it(t *testing.T) {
	cases := []struct {
		name         string
		since, until time.Time
		want         []bool
	}{
		{name: "since_on_the_first_day", since: day(2026, time.January, 1), until: day(2026, time.February, 28), want: []bool{false, false}},
		{name: "since_on_the_second_day", since: day(2026, time.January, 2), until: day(2026, time.February, 28), want: []bool{true, false}},
		{name: "until_the_day_before_the_last", since: day(2026, time.January, 1), until: day(2026, time.February, 27), want: []bool{false, true}},
		{name: "leap_year_until_the_29th", since: day(2024, time.February, 1), until: day(2024, time.February, 29), want: []bool{false}},
		{name: "leap_year_until_the_28th", since: day(2024, time.February, 1), until: day(2024, time.February, 28), want: []bool{true}},
		{name: "common_year_until_the_28th", since: day(2025, time.February, 1), until: day(2025, time.February, 28), want: []bool{false}},
		{name: "a_one_month_window_cut_at_both_ends", since: day(2026, time.March, 5), until: day(2026, time.March, 20), want: []bool{true}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := spendByMonth(t, cadOnly(), c.since, c.until)

			assert.Equal(t, c.want, partialFlags(result))
		})
	}
}

func Test_spend_by_month_runs_the_series_across_a_year_end(t *testing.T) {
	result := spendByMonth(t, cadOnly(), day(2025, time.November, 10), day(2026, time.February, 10))

	assert.Equal(t, []string{"2025-11", "2025-12", "2026-01", "2026-02"}, monthLabels(result))
}

func Test_spend_by_month_fills_a_month_with_no_spending_as_zero(t *testing.T) {
	spending := store.Spending{
		Rows: []store.SpendingRow{
			{Key: new("2026-01"), Currency: "CAD", Spent: 1250},
			{Key: new("2026-03"), Currency: "CAD", Spent: 400},
		},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 1650}},
	}

	result := spendByMonth(t, spending, day(2026, time.January, 1), day(2026, time.March, 31))

	assert.Equal(t, []report.SpendingRow{
		{Key: new("2026-01"), Currency: "CAD", Spent: 1250},
		{Key: new("2026-02"), Currency: "CAD", Spent: 0},
		{Key: new("2026-03"), Currency: "CAD", Spent: 400},
	}, result.Rows)
}

func Test_spend_by_month_fills_every_month_for_each_currency_in_the_totals(t *testing.T) {
	spending := store.Spending{
		Rows:   []store.SpendingRow{{Key: new("2026-01"), Currency: "CAD", Spent: 900}},
		Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 900}, {Currency: "USD", Spent: 0}},
	}

	result := spendByMonth(t, spending, day(2026, time.January, 1), day(2026, time.February, 28))

	assert.Equal(t, []report.SpendingRow{
		{Key: new("2026-01"), Currency: "CAD", Spent: 900},
		{Key: new("2026-01"), Currency: "USD", Spent: 0},
		{Key: new("2026-02"), Currency: "CAD", Spent: 0},
		{Key: new("2026-02"), Currency: "USD", Spent: 0},
	}, result.Rows)
}

func Test_spend_by_month_lists_no_rows_when_the_window_holds_no_spending(t *testing.T) {
	result := spendByMonth(t, store.Spending{}, day(2026, time.January, 1), day(2026, time.March, 31))

	assert.Empty(t, result.Rows)
}

func Test_spend_by_month_marks_only_the_first_and_last_month_partial(t *testing.T) {
	result := spendByMonth(t, cadOnly(), day(2026, time.January, 15), day(2026, time.March, 10))

	assert.Equal(t, []bool{true, false, true}, partialFlags(result))
}
