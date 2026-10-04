package report_test

import (
	"errors"
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func holdingsAccounts(t *testing.T, list store.AccountList, names ...string) (report.Holdings, store.HoldingsParams, int, error) {
	t.Helper()
	var got store.HoldingsParams
	var reads int
	srv := report.NewServer(report.WithStore(fakeStore{accounts: list, gotHoldings: &got, holdingsReads: &reads}))

	result, err := srv.Holdings(t.Context(), report.HoldingsRequest{Accounts: names})

	return result, got, reads, err
}

func Test_holdings_names_the_accounts_given_by_name_id_and_name_ignoring_case_in_the_order_given(t *testing.T) {
	list := accountsOf(chequing, savings, oldCard)

	result, got, reads, err := holdingsAccounts(t, list, "Old Card", "acct-100", "SAVINGS")

	require.NoError(t, err)
	assert.Equal(t, []store.Account{oldCard, chequing, savings}, result.Accounts)
	assert.Equal(t, []string{"acct-300", "acct-100", "acct-200"}, got.AccountIDs)
	assert.Equal(t, 1, reads)
}

func Test_holdings_names_an_account_given_by_name_and_by_id_once(t *testing.T) {
	list := accountsOf(chequing, savings)

	result, got, _, err := holdingsAccounts(t, list, "chequing", "acct-100", "Chequing")

	require.NoError(t, err)
	assert.Equal(t, []store.Account{chequing}, result.Accounts)
	assert.Equal(t, []string{"acct-100"}, got.AccountIDs)
}

func Test_holdings_refuses_an_unknown_or_empty_account_without_reading_holdings(t *testing.T) {
	cases := []struct {
		name string
		arg  string
	}{
		{name: "an unknown name", arg: "Chequeing"},
		{name: "an empty argument", arg: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, reads, err := holdingsAccounts(t, accountsOf(chequing), "Chequing", c.arg)

			refusal, ok := errors.AsType[report.RefusalError](err)
			require.True(t, ok)
			assert.Equal(t, report.RefusalUnknownAccount, refusal.Kind)
			assert.Equal(t, c.arg, refusal.Arg)
			assert.Zero(t, reads)
		})
	}
}

func Test_holdings_refuses_an_ambiguous_account_name(t *testing.T) {
	list := accountsOf(store.Account{ID: "acct-977", Name: "Visa"}, store.Account{ID: "acct-812", Name: "visa"})

	_, _, reads, err := holdingsAccounts(t, list, "VISA")

	refusal, ok := errors.AsType[report.RefusalError](err)
	require.True(t, ok)
	assert.Equal(t, report.RefusalAmbiguousAccount, refusal.Kind)
	assert.Equal(t, []string{"acct-812", "acct-977"}, refusal.IDs)
	assert.Zero(t, reads)
}

func Test_holdings_refuses_when_the_accounts_read_fails_to_open_the_store(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath}
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))

	_, err := srv.Holdings(t.Context(), report.HoldingsRequest{Accounts: []string{"Chequing"}})

	assert.EqualError(t, err, "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it")
}

func Test_holdings_does_not_read_accounts_when_none_is_named(t *testing.T) {
	var accountsReads int
	var got store.HoldingsParams
	srv := report.NewServer(report.WithStore(fakeStore{accountsReads: &accountsReads, gotHoldings: &got}))

	result, err := srv.Holdings(t.Context(), report.HoldingsRequest{})

	require.NoError(t, err)
	assert.Zero(t, accountsReads)
	assert.Nil(t, got.AccountIDs)
	assert.Nil(t, result.Accounts)
}
