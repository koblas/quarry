package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_accounts_leaves_closed_accounts_out_unless_asked(t *testing.T) {
	asOf := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	chequing := store.AccountBalance{ID: "acct-1", Name: "Chequing", Active: true}
	visa := store.AccountBalance{ID: "acct-2", Name: "Visa", Closed: true, Active: true}
	oldCard := store.AccountBalance{ID: "acct-3", Name: "Old Card", Closed: true, Active: false}
	savings := store.AccountBalance{ID: "acct-4", Name: "Savings", Active: false}
	all := store.AccountList{AsOf: asOf, Accounts: []store.AccountBalance{chequing, visa, oldCard, savings}}
	cases := []struct {
		name          string
		includeClosed bool
		want          report.AccountListing
	}{
		{
			name: "open accounts only by default", includeClosed: false,
			want: report.AccountListing{
				AccountList: store.AccountList{AsOf: asOf, Accounts: []store.AccountBalance{chequing, savings}},
				Hidden:      2,
			},
		},
		{name: "every account when closed ones are asked for", includeClosed: true, want: report.AccountListing{AccountList: all}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := report.NewServer(report.WithStore(fakeStore{accounts: all}))

			got, err := srv.Accounts(t.Context(), c.includeClosed, money.Native)

			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func Test_accounts_counts_the_closed_accounts_it_left_out(t *testing.T) {
	closed := func(id string) store.AccountBalance {
		return store.AccountBalance{ID: id, Closed: true}
	}
	open := store.AccountBalance{ID: "acct-9"}
	srv := report.NewServer(report.WithStore(fakeStore{
		accounts: store.AccountList{Accounts: []store.AccountBalance{closed("acct-1"), open, closed("acct-2"), closed("acct-3")}},
	}))

	got, err := srv.Accounts(t.Context(), false, money.Native)

	require.NoError(t, err)
	assert.Equal(t, 3, got.Hidden)
}

func Test_accounts_reports_every_account_hidden_only_when_none_are_left(t *testing.T) {
	closed := store.AccountBalance{ID: "acct-1", Closed: true}
	open := store.AccountBalance{ID: "acct-2"}
	cases := []struct {
		name          string
		accounts      []store.AccountBalance
		includeClosed bool
		want          bool
	}{
		{name: "every account closed", accounts: []store.AccountBalance{closed, closed}, want: true},
		{name: "one open account among closed ones", accounts: []store.AccountBalance{closed, open}, want: false},
		{name: "no accounts at all", accounts: nil, want: false},
		{name: "closed accounts asked for", accounts: []store.AccountBalance{closed, closed}, includeClosed: true, want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := report.NewServer(report.WithStore(fakeStore{accounts: store.AccountList{Accounts: c.accounts}}))

			got, err := srv.Accounts(t.Context(), c.includeClosed, money.Native)

			require.NoError(t, err)
			assert.Equal(t, c.want, got.AllHidden())
		})
	}
}

func Test_accounts_returns_the_store_fault(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Accounts(t.Context(), true, money.Native)

	require.ErrorIs(t, err, errDiskRead)
}
