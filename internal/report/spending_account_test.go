package report_test

import (
	"context"
	"errors"
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	chequing = store.Account{ID: "acct-100", Name: "Chequing"}
	savings  = store.Account{ID: "acct-200", Name: "Savings"}
	oldCard  = store.Account{ID: "acct-300", Name: "Old Card", Closed: true, NotInReports: true}
)

func accountsOf(accounts ...store.Account) store.AccountList {
	list := store.AccountList{}
	for _, a := range accounts {
		list.Accounts = append(list.Accounts, store.AccountBalance{Account: a})
	}
	return list
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

func Test_spend_matches_an_account_name_ignoring_case(t *testing.T) {
	list := accountsOf(chequing, savings)

	_, got, err := spendAccounts(t, list, "cHEQUING")

	require.NoError(t, err)
	assert.Equal(t, []string{"acct-100"}, got.AccountIDs)
}

func Test_spend_matches_a_closed_account(t *testing.T) {
	list := accountsOf(chequing, oldCard)

	_, got, err := spendAccounts(t, list, "old card")

	require.NoError(t, err)
	assert.Equal(t, []string{"acct-300"}, got.AccountIDs)
}

func Test_spend_resolves_an_id_before_a_name_equal_to_it(t *testing.T) {
	list := accountsOf(
		store.Account{ID: "acct-1", Name: "acct-2"},
		store.Account{ID: "acct-2", Name: "Other"},
	)

	_, got, err := spendAccounts(t, list, "acct-2")

	require.NoError(t, err)
	assert.Equal(t, []string{"acct-2"}, got.AccountIDs)
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

func Test_spend_refuses_the_first_account_it_cannot_pick(t *testing.T) {
	list := accountsOf(chequing)

	_, _, err := spendAccounts(t, list, "Chequing", "Missing", "Absent")

	assert.EqualError(t, err, `no account named "Missing"; run quarry accounts --all to list them`)
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

	assert.EqualError(t, err, "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it")
}

func Test_spend_reports_an_interrupt_during_the_accounts_read(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.Spend(ctx, report.SpendRequest{Accounts: []string{"Chequing"}})

	assert.EqualError(t, err, "spend interrupted")
}
