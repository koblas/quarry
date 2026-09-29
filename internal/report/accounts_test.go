package report_test

import (
	"testing"
	"time"

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
		want          store.AccountList
	}{
		{
			name: "open accounts only by default", includeClosed: false,
			want: store.AccountList{AsOf: asOf, Accounts: []store.AccountBalance{chequing, savings}},
		},
		{name: "every account when closed ones are asked for", includeClosed: true, want: all},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := report.NewServer(report.WithStore(fakeStore{accounts: all}))

			got, err := srv.Accounts(t.Context(), c.includeClosed)

			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func Test_accounts_returns_the_store_fault(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Accounts(t.Context(), true)

	require.ErrorIs(t, err, errDiskRead)
}
