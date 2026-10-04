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

func Test_accounts_echoes_the_currency_it_was_asked_for(t *testing.T) {
	cases := []struct {
		name     string
		currency money.Currency
		closed   bool
	}{
		{name: "CAD with closed accounts left out", currency: money.CAD},
		{name: "USD with closed accounts left out", currency: money.USD},
		{name: "native with closed accounts left out", currency: money.Native},
		{name: "USD with closed accounts listed", currency: money.USD, closed: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := report.NewServer(report.WithStore(fakeStore{accounts: store.AccountList{}}))

			got, err := srv.Accounts(t.Context(), c.closed, c.currency)

			require.NoError(t, err)
			assert.Equal(t, c.currency, got.Currency)
		})
	}
}

func Test_accounts_reads_the_store_once(t *testing.T) {
	var reads int
	srv := report.NewServer(report.WithStore(fakeStore{accountsReads: &reads}))

	_, err := srv.Accounts(t.Context(), false, money.CAD)

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
}

func Test_accounts_converted_balance_picks_the_cell_of_the_listings_currency(t *testing.T) {
	a := store.AccountBalance{Balance: 800, BalanceCAD: new(int64(1000)), BalanceUSD: new(int64(790))}
	cases := []struct {
		name     string
		currency money.Currency
		want     *int64
	}{
		{name: "CAD listing reads the CAD cell", currency: money.CAD, want: new(int64(1000))},
		{name: "USD listing reads the USD cell", currency: money.USD, want: new(int64(790))},
		{name: "native listing has none", currency: money.Native, want: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			listing := report.AccountListing{Currency: c.currency}

			assert.Equal(t, c.want, listing.ConvertedBalance(a))
		})
	}
}

func Test_accounts_needs_a_rate_only_for_a_balance_with_no_cell_in_a_converted_listing(t *testing.T) {
	imported := store.AccountBalance{Balance: 800}
	investment := store.AccountBalance{Type: store.AccountTypeBrokerage, Currency: "USD", Cash: 800, HoldingsValue: new(int64(0)), Balance: 800}
	converted := store.AccountBalance{Balance: 800, BalanceCAD: new(int64(1000))}
	cases := []struct {
		name     string
		currency money.Currency
		account  store.AccountBalance
		want     bool
	}{
		{name: "imported, no cell, CAD", currency: money.CAD, account: imported, want: true},
		{name: "imported, no cell, USD", currency: money.USD, account: imported, want: true},
		{name: "imported with its cell", currency: money.CAD, account: converted, want: false},
		{name: "a USD investment account, no cell, CAD", currency: money.CAD, account: investment, want: true},
		{name: "native listing", currency: money.Native, account: imported, want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			listing := report.AccountListing{Currency: c.currency}

			assert.Equal(t, c.want, listing.NeedsRate(c.account))
		})
	}
}
