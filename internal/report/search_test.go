package report_test

import (
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_search_passes_the_window_the_accounts_ids_and_the_limit_to_the_store(t *testing.T) {
	var got store.SearchParams
	since, until := day(2026, time.January, 1), day(2026, time.March, 31)
	srv := report.NewServer(report.WithStore(fakeStore{accounts: accountsOf(chqAccount, visaAccount), gotSearch: &got}))

	_, err := srv.Search(t.Context(), report.SearchRequest{
		Window: store.SearchWindow{Since: &since, Until: &until}, Accounts: []string{"Visa", "acct-chq"}, Limit: 20,
	})

	require.NoError(t, err)
	assert.Equal(t, store.SearchParams{
		Window: store.SearchWindow{Since: &since, Until: &until}, AccountIDs: []string{"acct-visa", "acct-chq"}, Limit: 20,
	}, got)
}

func Test_search_names_every_account_when_none_is_given(t *testing.T) {
	var got store.SearchParams
	reads := 0
	srv := report.NewServer(report.WithStore(fakeStore{gotSearch: &got, accountsReads: &reads}))

	result, err := srv.Search(t.Context(), report.SearchRequest{})

	require.NoError(t, err)
	assert.Empty(t, got.AccountIDs)
	assert.Empty(t, result.Accounts)
	assert.Zero(t, reads)
}

func Test_search_echoes_the_accounts_the_request_named_in_the_order_given(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{accounts: accountsOf(chqAccount, visaAccount)}))

	result, err := srv.Search(t.Context(), report.SearchRequest{Accounts: []string{"Visa", "acct-chq", "visa"}})

	require.NoError(t, err)
	assert.Equal(t, []store.Account{visaAccount, chqAccount}, result.Accounts)
}

func Test_search_carries_what_the_store_matched_and_the_request_it_answered(t *testing.T) {
	since := day(2026, time.January, 1)
	found := store.Search{
		Rows:         []store.SearchRow{{TransactionID: "txn-1"}},
		Matched:      4,
		Transactions: store.TransactionRange{First: day(2003, time.January, 2), Last: day(2026, time.September, 30)},
	}
	srv := report.NewServer(report.WithStore(fakeStore{search: found}))

	result, err := srv.Search(t.Context(), report.SearchRequest{Window: store.SearchWindow{Since: &since}, Limit: 1})

	require.NoError(t, err)
	assert.Equal(t, report.Search{
		Search: found, Window: store.SearchWindow{Since: &since}, Limit: 1,
	}, result)
}

func Test_search_truncated_is_true_only_when_matches_exceed_the_rows(t *testing.T) {
	cases := []struct {
		name   string
		search report.Search
		want   bool
	}{
		{name: "more matches than rows", search: report.Search{Search: store.Search{Rows: make([]store.SearchRow, 2), Matched: 3}}, want: true},
		{name: "as many matches as rows", search: report.Search{Search: store.Search{Rows: make([]store.SearchRow, 2), Matched: 2}}},
		{name: "no matches", search: report.Search{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.search.Truncated())
		})
	}
}

func Test_search_refuses_an_account_it_cannot_pick_without_reading_matches(t *testing.T) {
	var got store.SearchParams
	srv := report.NewServer(report.WithStore(fakeStore{accounts: accountsOf(chqAccount), gotSearch: &got}))

	_, err := srv.Search(t.Context(), report.SearchRequest{Accounts: []string{"Nowhere"}})

	var refusal report.RefusalError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, report.RefusalUnknownAccount, refusal.Kind)
	assert.Zero(t, got)
}

func Test_search_refuses_a_store_fault_as_a_store_refusal(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))

	_, err := srv.Search(t.Context(), report.SearchRequest{})

	var refusal report.RefusalError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, report.RefusalStore, refusal.Kind)
	assert.EqualError(t, err, "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it")
}

func Test_search_reports_an_interrupt_during_the_read(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Search(ctx, report.SearchRequest{})

	assert.EqualError(t, err, "search interrupted")
}

func Test_search_reports_an_interrupt_during_the_accounts_read(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Search(ctx, report.SearchRequest{Accounts: []string{"Visa"}})

	assert.EqualError(t, err, "search interrupted")
}
