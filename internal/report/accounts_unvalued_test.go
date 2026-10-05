package report_test

import (
	"testing"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_accounts_warns_only_about_listed_accounts_holdings(t *testing.T) {
	open := store.AccountBalance{ID: "acct-1", Name: "Brokerage"}
	closed := store.AccountBalance{ID: "acct-2", Name: "Old Brokerage", Closed: true}
	openHeld := store.UnvaluedHolding{AccountID: "acct-1", SecurityID: "sec-1"}
	closedHeld := store.UnvaluedHolding{AccountID: "acct-2", SecurityID: "sec-2"}
	list := store.AccountList{Accounts: []store.AccountBalance{open, closed}, Unvalued: []store.UnvaluedHolding{closedHeld, openHeld}}
	cases := []struct {
		name          string
		includeClosed bool
		want          []store.UnvaluedHolding
	}{
		{name: "a closed account's holding is dropped with the account", includeClosed: false, want: []store.UnvaluedHolding{openHeld}},
		{name: "a closed account's holding stays when closed accounts are asked for", includeClosed: true, want: []store.UnvaluedHolding{closedHeld, openHeld}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := report.NewServer(report.WithStore(fakeStore{accounts: list}))

			got, err := srv.Accounts(t.Context(), c.includeClosed, money.Native, report.Classification{})

			require.NoError(t, err)
			assert.Equal(t, c.want, got.Unvalued)
		})
	}
}

func Test_accounts_filtering_unvalued_holdings_does_not_touch_the_store_s_rows(t *testing.T) {
	closedHeld := store.UnvaluedHolding{AccountID: "acct-2", SecurityID: "sec-2"}
	openHeld := store.UnvaluedHolding{AccountID: "acct-1", SecurityID: "sec-1"}
	held := []store.UnvaluedHolding{closedHeld, openHeld}
	srv := report.NewServer(report.WithStore(fakeStore{accounts: store.AccountList{
		Accounts: []store.AccountBalance{{ID: "acct-1"}, {ID: "acct-2", Closed: true}}, Unvalued: held,
	}}))

	_, err := srv.Accounts(t.Context(), false, money.Native, report.Classification{})

	require.NoError(t, err)
	assert.Equal(t, []store.UnvaluedHolding{closedHeld, openHeld}, held)
}
