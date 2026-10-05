package report_test

import (
	"testing"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_findings_names_each_listed_id_that_is_no_account(t *testing.T) {
	c := report.Classification{Registered: []string{"acct-1", "acct-99"}, NonRegistered: []string{"acct-2", "acct-98"}}

	got := accountsFindings(t, report.FindingsRequest{Classification: c}, []store.Account{brokerage("acct-1", "A"), brokerage("acct-2", "B")})

	assert.Equal(t, report.UnmatchedAccounts{Registered: []string{"acct-99"}, NonRegistered: []string{"acct-98"}}, got.UnmatchedAccounts)
}

func Test_findings_keeps_a_repeated_unmatched_id_once_per_listing(t *testing.T) {
	c := report.Classification{Registered: []string{"acct-99", "acct-1", "acct-99"}}

	got := accountsFindings(t, report.FindingsRequest{Classification: c}, []store.Account{brokerage("acct-1", "A")})

	assert.Equal(t, []string{"acct-99", "acct-99"}, got.UnmatchedAccounts.Registered)
}

func Test_findings_does_not_name_a_listed_id_whose_account_is_not_an_investment_account(t *testing.T) {
	c := report.Classification{Registered: []string{"acct-1"}}

	got := accountsFindings(t, report.FindingsRequest{Classification: c}, []store.Account{{ID: "acct-1", Type: "checking"}})

	assert.Empty(t, got.UnmatchedAccounts.Registered)
}

func Test_findings_does_not_name_a_listed_id_whose_account_is_closed(t *testing.T) {
	c := report.Classification{NonRegistered: []string{"acct-1"}}

	got := accountsFindings(t, report.FindingsRequest{Classification: c}, []store.Account{{ID: "acct-1", Type: store.AccountTypeBrokerage, Closed: true}})

	assert.Empty(t, got.UnmatchedAccounts.NonRegistered)
}

func Test_findings_names_an_unmatched_id_whatever_type_it_selects(t *testing.T) {
	c := report.Classification{Registered: []string{"acct-99"}}

	got := accountsFindings(t, report.FindingsRequest{Classification: c, Type: finding.Duplicate}, nil)

	assert.Equal(t, []string{"acct-99"}, got.UnmatchedAccounts.Registered)
}

func Test_accounts_names_each_listed_id_that_is_no_account(t *testing.T) {
	c := report.Classification{Registered: []string{"acct-1", "acct-99"}, NonRegistered: []string{"acct-98"}}
	srv := report.NewServer(report.WithStore(fakeStore{
		accounts: store.AccountList{Accounts: []store.AccountBalance{{Account: brokerage("acct-1", "A")}}},
	}))

	got, err := srv.Accounts(t.Context(), false, money.Native, c)

	require.NoError(t, err)
	assert.Equal(t, report.UnmatchedAccounts{Registered: []string{"acct-99"}, NonRegistered: []string{"acct-98"}}, got.UnmatchedAccounts)
}

func Test_accounts_warns_nothing_for_a_listed_closed_account(t *testing.T) {
	c := report.Classification{Registered: []string{"acct-1"}}
	srv := report.NewServer(report.WithStore(fakeStore{
		accounts: store.AccountList{Accounts: []store.AccountBalance{{ID: "acct-1", Closed: true}}},
	}))

	got, err := srv.Accounts(t.Context(), false, money.Native, c)

	require.NoError(t, err)
	assert.Equal(t, 1, got.Hidden)
	assert.Empty(t, got.UnmatchedAccounts.Registered)
}

func Test_accounts_names_an_unmatched_id_with_closed_accounts_included(t *testing.T) {
	c := report.Classification{Registered: []string{"acct-99"}}
	srv := report.NewServer(report.WithStore(fakeStore{
		accounts: store.AccountList{Accounts: []store.AccountBalance{{ID: "acct-1", Closed: true}}},
	}))

	got, err := srv.Accounts(t.Context(), true, money.Native, c)

	require.NoError(t, err)
	assert.Equal(t, []string{"acct-99"}, got.UnmatchedAccounts.Registered)
}
